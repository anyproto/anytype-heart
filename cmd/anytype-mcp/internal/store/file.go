package store

import (
	"bytes"
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

// FailAt installs a function that can fail a write or a delete at a named
// stage. It exists for tests of what the verbs report; no flag, file or
// environment variable reaches it.
func (f *File) FailAt(hook func(stage string) error) { f.hook = hook }

// Dir is the items folder.
func (f *File) Dir() string {
	return filepath.Join(f.home, ".anytype", "ai-connect", f.folder)
}

func itemName(id string) string { return id + ".item" }

// folder kinds on the item path: the folder rule differs for each.
const (
	kindHome    = iota // the user's home: exists already, never created
	kindAnytype        // .anytype and ai-connect
	kindItems          // the items folder
)

// checked applies the folder rule to the whole chain, home first, and
// returns the items folder. No other OS user may be able to rename or
// replace a folder on the item path. With create unset, a missing folder
// is ErrNotFound and nothing is made; home itself is never created.
func (f *File) checked(create bool) (string, error) {
	if f.homeErr != nil {
		return "", unreachable(f.homeErr)
	}
	items := f.Dir()
	chain := []struct {
		dir  string
		kind int
	}{
		{f.home, kindHome},
		{filepath.Dir(filepath.Dir(items)), kindAnytype},
		{filepath.Dir(items), kindAnytype},
		{items, kindItems},
	}
	for _, c := range chain {
		fi, err := os.Lstat(c.dir)
		if errors.Is(err, os.ErrNotExist) && c.kind != kindHome {
			if !create {
				return "", ErrNotFound
			}
			if err := f.mkdir(c.dir, c.kind); err != nil && !errors.Is(err, os.ErrExist) {
				return "", unreachable(err)
			}
			// a new folder is durable before anything is written into it
			if err := f.stage("sync-parent"); err != nil {
				return "", unreachable(err)
			}
			if err := syncParent(filepath.Dir(c.dir)); err != nil {
				return "", unreachable(err)
			}
			fi, err = os.Lstat(c.dir)
		}
		if err != nil {
			return "", unreachable(err)
		}
		if err := f.checkDir(c.dir, fi, c.kind); err != nil {
			return "", err
		}
	}
	return items, nil
}

// deniedAt is a failed rule on a path: the message names both.
func deniedAt(path, rule string) error { return denied(rule + ": " + path) }

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
// the item, then made durable. Any failure before the rename removes the
// temp file and is temporary: nothing was written. After a successful
// write the item is the only file that holds this connection's key.
func (f *File) Write(ctx context.Context, id string, value []byte) error {
	dir, err := f.checked(true)
	if err != nil {
		return err
	}
	if err := f.sweep(dir, id); err != nil {
		return err
	}
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
		// the temp file holds the key and must not stay; a leftover is
		// swept by the next write or delete
		ignore(fh.Close())
		ignore(removeIn(dir, tmp))
		if IsTemporary(err) {
			return err
		}
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
	// the commit point: nothing is replaced once the caller's deadline has
	// passed
	if !commitAllowed(ctx) {
		return fail(errors.New("the deadline passed before the item was replaced"))
	}
	err = renameIn(dir, tmp, itemName(id))
	if err == nil {
		err = f.stage("rename-result")
	}
	if err != nil {
		// an error from the rename does not prove the rename did not
		// happen (a network home): the item itself says which it was
		switch f.holds(dir, id, value) {
		case holdsNew:
		case holdsOther:
			return fail(err)
		default:
			ignore(removeIn(dir, tmp))
			return &UnconfirmedError{Err: err}
		}
	}
	// from here the new item is in place
	if err := f.stage("after-rename"); err != nil {
		return &UnconfirmedError{Err: err}
	}
	if err := settle(dir, itemName(id)); err != nil {
		return &UnconfirmedError{Err: err}
	}
	return nil
}

// sweep removes this connection's temp files left by a crash: each holds
// a whole item, key included, and none is ever read. A leftover that
// can't be removed fails the verb before it changes anything.
func (f *File) sweep(dir, id string) error {
	if err := f.stage("sweep"); err != nil {
		return unreachable(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return unreachable(err)
	}
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, id+".") || !strings.HasSuffix(name, ".tmp") {
			continue
		}
		if err := removeIn(dir, name); err != nil && !errors.Is(err, os.ErrNotExist) {
			return unreachable(err)
		}
	}
	return nil
}

// ignore drops the error of a best-effort cleanup step.
func ignore(error) {}

// Delete removes the item and every temp file of the connection, and
// makes that durable: after it no file holds this connection's key. A
// failure after the item is gone is unconfirmed, never "try again".
func (f *File) Delete(ctx context.Context, id string) error {
	dir, err := f.checked(false)
	if err != nil {
		return err
	}
	if err := f.sweep(dir, id); err != nil {
		return err
	}
	if !commitAllowed(ctx) {
		return unreachable(errors.New("the deadline passed before the item was removed"))
	}
	err = removeIn(dir, itemName(id))
	if errors.Is(err, os.ErrNotExist) {
		return ErrNotFound
	}
	if err == nil {
		err = f.stage("unlink-result")
	}
	if err != nil {
		// as with the rename: the item says whether the unlink happened
		switch f.holds(dir, id, nil) {
		case holdsNothing:
		case holdsOther:
			return unreachable(err)
		default:
			return &UnconfirmedError{Err: err}
		}
	}
	if err := f.stage("after-unlink"); err != nil {
		return &UnconfirmedError{Err: err}
	}
	if err := syncDir(dir); err != nil {
		return &UnconfirmedError{Err: err}
	}
	return nil
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

// renameIn replaces the item with os.Root.Rename, which on Windows is a
// POSIX-semantics replace: it works while a reader holds the old item
// open. busy says whether a failure is worth retrying (Windows: a handle
// that denies delete sharing); the retry is bounded.
func renameIn(dir, from, to string) error {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()
	deadline := time.Now().Add(busyRetryFor)
	for {
		err = root.Rename(from, to)
		if err == nil || !busy(err) || time.Now().After(deadline) {
			return err
		}
		time.Sleep(25 * time.Millisecond)
	}
}

// removeIn removes a name in the folder, with the same bounded retry.
func removeIn(dir, name string) error {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()
	deadline := time.Now().Add(busyRetryFor)
	for {
		err = root.Remove(name)
		if err == nil || !busy(err) || time.Now().After(deadline) {
			return err
		}
		time.Sleep(25 * time.Millisecond)
	}
}

// busyRetryFor bounds the wait for a file something else holds open.
const busyRetryFor = 2 * time.Second

// what the item path holds after an operation that reported an error
const (
	holdsUnknown = iota // it can't be read: nothing is proven
	holdsNew            // exactly the value that was being written
	holdsOther          // some other item: the operation did not happen
	holdsNothing        // no item
)

// holds re-reads the item to learn whether an operation that reported an
// error took effect anyway.
func (f *File) holds(dir, id string, value []byte) int {
	if err := f.stage("holds"); err != nil {
		return holdsUnknown
	}
	data, err := f.readItem(dir, itemName(id))
	switch {
	case errors.Is(err, ErrNotFound):
		if value == nil {
			return holdsNothing
		}
		return holdsOther
	case err == nil && value != nil && bytes.Equal(data, value):
		return holdsNew
	case err == nil || errors.Is(err, ErrInvalidItem):
		return holdsOther
	}
	return holdsUnknown
}

type commitGateKey struct{}

// WithCommitGate attaches a gate that a write or a delete asks at its
// commit point, just before the item is replaced or removed. A gate that
// says no stops the operation with nothing changed: the key verbs use it
// so that nothing is written after their deadline.
func WithCommitGate(ctx context.Context, allow func() bool) context.Context {
	return context.WithValue(ctx, commitGateKey{}, allow)
}

func commitAllowed(ctx context.Context) bool {
	if ctx.Err() != nil {
		return false
	}
	if allow, ok := ctx.Value(commitGateKey{}).(func() bool); ok {
		return allow()
	}
	return true
}
