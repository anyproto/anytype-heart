package source

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
)

// Spill materializes one entry as a real file under dir (for consumers that
// require a filesystem path, e.g. the file uploader) and returns its path.
// Entries that already live on disk are returned as-is without copying.
//
// The on-disk name is flat and sanitized: a short hash of the full entry name
// plus its base name, so hostile entry names can never escape dir and equal
// base names from different directories cannot collide. The caller owns dir
// (typically a per-run temp dir removed at the end of the run).
func Spill(ctx context.Context, s Source, name, dir string) (string, error) {
	entry, ok := s.Stat(name)
	if !ok {
		return "", fmt.Errorf("spill %q: %w", name, ErrNotFound)
	}
	if entry.FSPath != "" {
		return entry.FSPath, nil
	}

	sum := sha256.Sum256([]byte(entry.Name))
	base := filepath.Base(filepath.FromSlash(path.Base(entry.Name)))
	dst := filepath.Join(dir, hex.EncodeToString(sum[:4])+"_"+base)
	if _, err := os.Stat(dst); err == nil {
		return dst, nil // already spilled this run
	}

	r, err := s.Open(ctx, name)
	if err != nil {
		return "", fmt.Errorf("spill %q: %w", name, err)
	}
	defer r.Close()

	tmp, err := os.CreateTemp(dir, "spill-*")
	if err != nil {
		return "", fmt.Errorf("spill %q: create temp: %w", name, err)
	}
	_, err = io.Copy(tmp, contextReader{ctx: ctx, r: r})
	closeErr := tmp.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		os.Remove(tmp.Name())
		return "", fmt.Errorf("spill %q: write: %w", name, err)
	}
	if err := os.Rename(tmp.Name(), dst); err != nil {
		os.Remove(tmp.Name())
		return "", fmt.Errorf("spill %q: finalize: %w", name, err)
	}
	return dst, nil
}

// contextReader aborts a copy promptly when ctx is cancelled.
type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (c contextReader) Read(p []byte) (int, error) {
	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.r.Read(p)
}
