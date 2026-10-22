package source

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"path"
	"sort"
	"unicode/utf8"
)

type zipSource struct {
	reader   *zip.ReadCloser
	entries  map[string]*zip.File // normalized Name → archive entry
	sorted   []Entry
	rejected []string
}

func newZipSource(zipPath string) (*zipSource, error) {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, fmt.Errorf("open zip: %w", err)
	}
	s := &zipSource{
		reader:  reader,
		entries: map[string]*zip.File{},
	}
	invalidNameCount := 0
	for _, f := range reader.File {
		if f.FileInfo().IsDir() {
			continue
		}
		raw := f.Name
		if !utf8.ValidString(raw) {
			invalidNameCount++
			raw = fmt.Sprintf("import file %d%s", invalidNameCount, path.Ext(sanitizeToUTF8(raw)))
		}
		name := NormalizeName(raw)
		if !safeRelName(name) {
			s.rejected = append(s.rejected, f.Name)
			continue
		}
		if skippedName(name) {
			continue
		}
		s.entries[name] = f
	}
	for name, f := range s.entries {
		s.sorted = append(s.sorted, Entry{
			Name:    name,
			Size:    int64(f.UncompressedSize64),
			ModTime: f.Modified,
		})
	}
	sort.Slice(s.sorted, func(i, j int) bool { return s.sorted[i].Name < s.sorted[j].Name })
	return s, nil
}

func sanitizeToUTF8(raw string) string {
	valid := make([]rune, 0, len(raw))
	for _, r := range raw {
		if r != utf8.RuneError {
			valid = append(valid, r)
		}
	}
	return string(valid)
}

func (s *zipSource) Walk(ctx context.Context, fn func(e Entry) error) error {
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

func (s *zipSource) Open(ctx context.Context, name string) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f, ok := s.entries[name]
	if !ok {
		return nil, fmt.Errorf("open zip entry %q: %w", name, ErrNotFound)
	}
	r, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("open zip entry %q: %w", name, err)
	}
	return r, nil
}

func (s *zipSource) Stat(name string) (Entry, bool) {
	f, ok := s.entries[name]
	if !ok {
		return Entry{}, false
	}
	return Entry{Name: name, Size: int64(f.UncompressedSize64), ModTime: f.Modified}, true
}

func (s *zipSource) Rejected() []string {
	return s.rejected
}

func (s *zipSource) Close() error {
	return s.reader.Close()
}
