package source

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type fileSource struct {
	entry Entry
}

func newFileSource(filePath string) (*fileSource, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("stat file: %w", err)
	}
	return &fileSource{
		entry: Entry{
			Name:    NormalizeName(filepath.Base(filePath)),
			Size:    info.Size(),
			ModTime: info.ModTime(),
			FSPath:  filePath,
		},
	}, nil
}

func (s *fileSource) Walk(ctx context.Context, fn func(e Entry) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return fn(s.entry)
}

func (s *fileSource) Open(ctx context.Context, name string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if name != s.entry.Name {
		return nil, fmt.Errorf("open file entry %q: %w", name, ErrNotFound)
	}
	f, err := os.Open(s.entry.FSPath)
	if err != nil {
		return nil, fmt.Errorf("open file entry %q: %w", name, err)
	}
	return f, nil
}

func (s *fileSource) Stat(name string) (Entry, bool) {
	if name != s.entry.Name {
		return Entry{}, false
	}
	return s.entry, true
}

func (s *fileSource) Rejected() []string {
	return nil
}

func (s *fileSource) Close() error {
	return nil
}
