// Package source provides re-readable, streaming access to import inputs
// (directory, zip archive, single file). Listings are cheap and deterministic
// (pass 1); entry content flows through short-lived readers (pass 2). Archive
// entry names are sanitized: path-traversal entries are rejected, never
// written or resolved outside their root.
package source

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/text/unicode/norm"
)

// Entry describes one regular file in a source listing.
type Entry struct {
	// Name is the entry's normalized identity: slash-separated, relative,
	// NFC-normalized. Stable across Walk calls — this is the sourceKey
	// material for path-addressed objects.
	Name    string
	Size    int64
	ModTime time.Time
	// FSPath is the real filesystem path when the entry lives on disk
	// (directory and single-file sources); empty for archive entries.
	FSPath string
}

// Source is a re-readable, streaming view of one import input. Walk and Open
// may be called multiple times and concurrently (persist workers stream file
// entries while the converter reads documents).
type Source interface {
	// Walk yields every entry in lexicographic Name order.
	Walk(ctx context.Context, fn func(e Entry) error) error
	// Open opens one entry by normalized Name.
	Open(ctx context.Context, name string) (io.ReadCloser, error)
	// Stat looks an entry up by normalized Name.
	Stat(name string) (Entry, bool)
	// Rejected lists raw entry names refused for safety (path traversal,
	// absolute paths). Callers surface them as issues.
	Rejected() []string
	Close() error
}

// Open dispatches by path shape: directory, .zip archive, or single file.
func Open(importPath string) (Source, error) {
	info, err := os.Stat(importPath)
	if err != nil {
		return nil, fmt.Errorf("stat import path: %w", err)
	}
	if info.IsDir() {
		return newDirSource(importPath)
	}
	if strings.EqualFold(filepath.Ext(importPath), ".zip") {
		return newZipSource(importPath)
	}
	return newFileSource(importPath)
}

// NormalizeName converts a raw entry name into the canonical Name form:
// slash-separated, cleaned, NFC-normalized.
func NormalizeName(raw string) string {
	return norm.NFC.String(path.Clean(filepath.ToSlash(raw)))
}

// safeRelName reports whether a cleaned slash-path stays inside its root.
func safeRelName(cleaned string) bool {
	if cleaned == "" || cleaned == "." {
		return false
	}
	if path.IsAbs(cleaned) || filepath.VolumeName(cleaned) != "" {
		return false
	}
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return false
	}
	return true
}

// skippedName reports OS/archive junk entries excluded from listings.
func skippedName(name string) bool {
	base := path.Base(name)
	if base == ".DS_Store" {
		return true
	}
	for part := range strings.SplitSeq(name, "/") {
		if part == "__MACOSX" {
			return true
		}
	}
	return false
}
