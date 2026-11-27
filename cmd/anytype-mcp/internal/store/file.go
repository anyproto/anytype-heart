package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofrs/flock"
)

// File is the item store: <home>/.anytype/ai-connect/<items folder>, one
// <id>.item per connection, with <id>.lock beside it. Every file the
// forwarder opens is inside that one folder, and the folder and its
// ancestors are checked before every use (the folder rule); an item is
// checked again through its open handle (the item rule). A check that
// fails reads, writes and deletes nothing.
type File struct {
	home    string
	homeErr error
	folder  string

	// uid replaces the effective user id in the owner checks; tests use
	// it because a non-root process can't chown.
	uid func() int //nolint:unused // read by the Unix rules only
	// hook fails a write at a named stage; tests only.
	hook func(stage string) error
}

// Open returns the store of a namespace under the user's home directory,
// which comes from os.UserHomeDir and nothing else.
func Open(namespace string) *File {
	home, err := os.UserHomeDir()
	f := OpenAt(home, namespace)
	if err != nil || home == "" {
		f.homeErr = errors.New("the home directory is unknown")
	}
	return f
}

// OpenAt returns the store of a namespace under an explicit home.
func OpenAt(home, namespace string) *File {
	return &File{home: home, folder: FolderName(namespace), uid: os.Geteuid}
}

func (f *File) Kind() string { return "file" }

// Dir is the items folder.
func (f *File) Dir() string {
	return filepath.Join(f.home, ".anytype", "ai-connect", f.folder)
}

func itemName(id string) string { return id + ".item" }

// folder applies the folder rule and returns the items folder. With
// create unset, a missing folder is ErrNotFound and nothing is made.
func (f *File) checked(create bool) (string, error) {
	if f.homeErr != nil {
		return "", unreachable(f.homeErr)
	}
	items := f.Dir()
	chain := []string{filepath.Dir(filepath.Dir(items)), filepath.Dir(items), items}
	for i, dir := range chain {
		last := i == len(chain)-1
		fi, err := os.Lstat(dir)
		if errors.Is(err, os.ErrNotExist) {
			if !create {
				return "", ErrNotFound
			}
			if err := f.mkdir(dir, last); err != nil && !errors.Is(err, os.ErrExist) {
				return "", unreachable(err)
			}
			fi, err = os.Lstat(dir)
		}
		if err != nil {
			return "", unreachable(err)
		}
		if err := f.checkDir(dir, fi, last); err != nil {
			return "", err
		}
	}
	return items, nil
}

func (f *File) Probe(context.Context) error {
	_, err := f.checked(false)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

func (f *File) Read(_ context.Context, id string) ([]byte, error) {
	dir, err := f.checked(false)
	if err != nil {
		return nil, err
	}
	return f.readItem(dir, itemName(id))
}

// readChecked reads an opened, checked item: at most MaxItemBytes.
func readChecked(fh *os.File) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(fh, MaxItemBytes+1))
	if err != nil {
		return nil, unreachable(err)
	}
	if len(data) > MaxItemBytes {
		return nil, ErrInvalidItem
	}
	return data, nil
}

func (f *File) stage(name string) error {
	if f.hook == nil {
		return nil
	}
	return f.hook(name)
}

// Write replaces the item: a new private temp file, synced, renamed over
// the item, the folder synced. Any failure before the rename removes the
// temp file and is temporary: nothing was written.
func (f *File) Write(_ context.Context, id string, value []byte) error {
	dir, err := f.checked(true)
	if err != nil {
		return err
	}
	f.sweep(dir, id)
	suffix := make([]byte, 8)
	if _, err := rand.Read(suffix); err != nil {
		return unreachable(err)
	}
	tmp := id + "." + hex.EncodeToString(suffix) + ".tmp"
	fh, err := f.createPrivate(dir, tmp)
	if err != nil {
		return err
	}
	fail := func(err error) error {
		// best effort: the temp file holds the key and must not stay
		ignore(fh.Close())
		ignore(removeIn(dir, tmp))
		return unreachable(err)
	}
	if _, err := fh.Write(value); err != nil {
		return fail(err)
	}
	if err := fh.Sync(); err != nil {
		return fail(err)
	}
	if err := fh.Close(); err != nil {
		return fail(err)
	}
	if err := f.stage("before-rename"); err != nil {
		return fail(err)
	}
	if err := renameIn(dir, tmp, itemName(id)); err != nil {
		return fail(err)
	}
	// from here the new item is in place
	if err := f.stage("after-rename"); err != nil {
		return &UnconfirmedError{Err: err}
	}
	if err := syncDir(dir); err != nil {
		return &UnconfirmedError{Err: err}
	}
	return nil
}

// sweep removes this connection's temp files left by a crash: each holds
// a whole item, key included, and none is ever read.
func (f *File) sweep(dir, id string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, id+".") && strings.HasSuffix(name, ".tmp") {
			ignore(removeIn(dir, name))
		}
	}
}

// ignore drops the error of a best-effort cleanup step.
func ignore(error) {}

func (f *File) Delete(_ context.Context, id string) error {
	dir, err := f.checked(false)
	if err != nil {
		return err
	}
	err = removeIn(dir, itemName(id))
	switch {
	case err == nil:
		return nil
	case errors.Is(err, os.ErrNotExist):
		return ErrNotFound
	}
	return unreachable(err)
}

// Lock takes <id>.lock in the items folder.
func (f *File) Lock(ctx context.Context, id string) (func(), error) {
	dir, err := f.checked(true)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, id+".lock")
	if fi, err := os.Lstat(path); err == nil && !fi.Mode().IsRegular() {
		return nil, denied("the lock is not a regular file")
	}
	lock := flock.New(path, flock.SetPermissions(0o600))
	ok, err := lock.TryLockContext(ctx, 25*time.Millisecond)
	if err != nil || !ok {
		ignore(lock.Close())
		return nil, unreachable(errors.New("another process holds this connection's lock"))
	}
	return func() { ignore(lock.Unlock()) }, nil
}
