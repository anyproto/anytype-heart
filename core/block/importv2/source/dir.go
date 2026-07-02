package source

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// ErrNotFound is returned by Open/Stat lookups for unknown entry names.
var ErrNotFound = errors.New("entry not found")

type dirSource struct {
	root    string
	entries map[string]Entry
	sorted  []Entry
}

func newDirSource(root string) (*dirSource, error) {
	s := &dirSource{
		root:    root,
		entries: map[string]Entry{},
	}
	err := filepath.Walk(root, func(fsPath string, info fs.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("walk %q: %w", fsPath, err)
		}
		if info.IsDir() || !info.Mode().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, fsPath)
		if err != nil {
			return fmt.Errorf("relativize %q: %w", fsPath, err)
		}
		name := NormalizeName(rel)
		if !safeRelName(name) || skippedName(name) {
			return nil
		}
		s.entries[name] = Entry{
			Name:    name,
			Size:    info.Size(),
			ModTime: info.ModTime(),
			FSPath:  fsPath,
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan directory: %w", err)
	}
	for _, e := range s.entries {
		s.sorted = append(s.sorted, e)
	}
	sort.Slice(s.sorted, func(i, j int) bool { return s.sorted[i].Name < s.sorted[j].Name })
	return s, nil
}

func (s *dirSource) Walk(ctx context.Context, fn func(e Entry) error) error {
	for _, e := range s.sorted {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := fn(e); err != nil {
			return err
		}
	}
	return nil
}

func (s *dirSource) Open(ctx context.Context, name string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	e, ok := s.entries[name]
	if !ok {
		return nil, fmt.Errorf("open directory entry %q: %w", name, ErrNotFound)
	}
	f, err := os.Open(e.FSPath)
	if err != nil {
		return nil, fmt.Errorf("open directory entry %q: %w", name, err)
	}
	return f, nil
}

func (s *dirSource) Stat(name string) (Entry, bool) {
	e, ok := s.entries[name]
	return e, ok
}

func (s *dirSource) Rejected() []string {
	return nil
}

func (s *dirSource) Close() error {
	return nil
}
