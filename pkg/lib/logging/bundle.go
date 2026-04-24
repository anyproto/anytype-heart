package logging

import (
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// bundleChunkSize controls how often we flush the output gzip stream to
// measure the on-disk size against the cap. Each Flush forces a gzip sync
// boundary which fragments the deflate stream and hurts compression ratio,
// so the value is a deliberate trade-off — smaller = tighter cap, larger =
// better compression. 1MB keeps the deflate stream healthy while still
// producing predictable cap enforcement (worst-case overshoot ~1MB on a
// 10MB cap, ~10%).
const bundleChunkSize = 1024 * 1024

// WriteLogBundle writes a single gzipped log file to destPath containing at
// most maxBytes of compressed output. It streams the active log file (the
// newest by modification time) first, then falls back to older rotated files
// (transparently decompressing .gz rotations) until the cap is hit or no more
// input is available. The result is always a valid single-stream gzip file
// regardless of where the input was truncated.
//
// logsDir is expected to contain lumberjack-style entries (anytype.log plus
// rotated anytype-*.log / anytype-*.log.gz). Files that aren't prefixed with
// "anytype" are ignored. If logsDir doesn't exist or has no matching files,
// the destination is created but empty (valid empty gzip stream).
func WriteLogBundle(logsDir, destPath string, maxBytes int64) error {
	if maxBytes <= 0 {
		return errors.New("WriteLogBundle: maxBytes must be positive")
	}
	sources, err := sortedLogFiles(logsDir)
	if err != nil {
		return fmt.Errorf("list logs: %w", err)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create bundle file: %w", err)
	}
	defer out.Close()

	gz := gzip.NewWriter(out)
	defer gz.Close()

	buf := make([]byte, bundleChunkSize)
	capped := false
	for _, path := range sources {
		if capped {
			break
		}
		capped, err = appendToBundle(gz, out, maxBytes, buf, path)
		if err != nil {
			// Bundle-best-effort: skip this input, keep going so partial
			// content still reaches disk. Surface via stderr so the caller
			// can tell a file was skipped.
			fmt.Fprintf(os.Stderr, "WriteLogBundle: skipping %s: %v\n", path, err)
		}
	}
	if err := gz.Close(); err != nil {
		return fmt.Errorf("close bundle gzip: %w", err)
	}
	return nil
}

// appendToBundle copies the plaintext contents of path into gz, flushing
// periodically so the on-disk size of out can be measured against maxBytes.
// Returns (capped, err): capped == true means the cap was reached (caller
// should stop iterating over subsequent inputs).
func appendToBundle(gz *gzip.Writer, out *os.File, maxBytes int64, buf []byte, path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()

	var src io.Reader = f
	if strings.HasSuffix(path, ".gz") {
		gr, err := gzip.NewReader(f)
		if err != nil {
			return false, fmt.Errorf("open gzip reader: %w", err)
		}
		defer gr.Close()
		src = gr
	}

	for {
		n, readErr := src.Read(buf)
		if n > 0 {
			if _, err := gz.Write(buf[:n]); err != nil {
				return false, fmt.Errorf("gzip write: %w", err)
			}
			// Flush so the output file reflects the compressed bytes we just
			// wrote, allowing an accurate size check against the cap.
			if err := gz.Flush(); err != nil {
				return false, fmt.Errorf("gzip flush: %w", err)
			}
			info, err := out.Stat()
			if err != nil {
				return false, fmt.Errorf("stat bundle: %w", err)
			}
			if info.Size() >= maxBytes {
				return true, nil
			}
		}
		if readErr == io.EOF {
			return false, nil
		}
		if readErr != nil {
			return false, fmt.Errorf("read %s: %w", path, readErr)
		}
	}
}

// sortedLogFiles returns the anytype-prefixed log files in logsDir ordered
// by modification time, newest first.
func sortedLogFiles(logsDir string) ([]string, error) {
	entries, err := os.ReadDir(logsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	type entryWithTime struct {
		path    string
		modTime int64
	}
	var files []entryWithTime
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, "anytype") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, entryWithTime{
			path:    filepath.Join(logsDir, name),
			modTime: info.ModTime().UnixNano(),
		})
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime > files[j].modTime
	})
	paths := make([]string, len(files))
	for i, f := range files {
		paths[i] = f.path
	}
	return paths, nil
}
