package anyblock

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DirWriter writes bundle files under one fixed directory — the
// deterministic Writer for tests and cmd tooling: no timestamped wrapper
// (the legacy dir writer bakes the clock into its root name), no state
// beyond the root. File mtimes follow each document's own
// lastModifiedDate, like the legacy writers, so a bundle browses by real
// dates; content, not mtimes, is what determinism is measured on.
type DirWriter struct {
	root string
}

// NewDirWriter creates root (and parents) and writes everything below it.
func NewDirWriter(root string) (*DirWriter, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("create export directory: %w", err)
	}
	return &DirWriter{root: root}, nil
}

// Path is the directory the bundle lands in.
func (w *DirWriter) Path() string {
	return w.root
}

// WriteFile writes one file at its bundle-relative, slash-separated path.
// The path must stay inside the root — the plan guarantees it for every
// path it mints, and this guards the invariant against any other caller.
func (w *DirWriter) WriteFile(filename string, r io.Reader, lastModifiedDate int64) error {
	full := filepath.Join(w.root, filepath.FromSlash(filename))
	if rel, err := filepath.Rel(w.root, full); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path %q escapes the bundle root", filename)
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return fmt.Errorf("create subdirectory: %w", err)
	}
	f, err := os.Create(full)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer f.Close()
	if _, err := io.Copy(f, r); err != nil {
		return fmt.Errorf("copy content to file: %w", err)
	}
	if lastModifiedDate > 0 {
		if err := os.Chtimes(full, time.Now(), time.Unix(lastModifiedDate, 0)); err != nil {
			return fmt.Errorf("set modified date: %w", err)
		}
	}
	return nil
}
