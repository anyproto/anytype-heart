package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// File is the test build's store: one file per connection in a directory
// the test names. A file named ".state" in that directory makes every
// call fail the way a real store would: "locked", "denied", "nostore".
// It is never linked into a shipped binary's Open.
type File struct{ Dir string }

func (f *File) Kind() string { return "file" }

func (f *File) fail() error {
	raw, err := os.ReadFile(filepath.Join(f.Dir, ".state"))
	if err != nil {
		return nil
	}
	switch strings.TrimSpace(string(raw)) {
	case "locked":
		return Temporary(ReasonLocked, nil)
	case "denied":
		return Temporary(ReasonDenied, nil)
	case "nostore":
		return ErrNoStore
	}
	return nil
}

func (f *File) path(id string) string { return filepath.Join(f.Dir, id+".item") }

func (f *File) Probe(context.Context) error { return f.fail() }

func (f *File) Read(_ context.Context, id string, _ bool) ([]byte, error) {
	if err := f.fail(); err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(f.path(id))
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	return raw, err
}

func (f *File) Write(_ context.Context, id string, value []byte, _ bool) error {
	if err := f.fail(); err != nil {
		return err
	}
	if err := os.MkdirAll(f.Dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(f.Dir, id+".*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(value); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), f.path(id))
}

func (f *File) Delete(_ context.Context, id string) error {
	if err := f.fail(); err != nil {
		return err
	}
	err := os.Remove(f.path(id))
	if errors.Is(err, os.ErrNotExist) {
		return ErrNotFound
	}
	return err
}
