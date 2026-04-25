package logging

import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteLogBundle(t *testing.T) {
	t.Run("active file first, bundle is valid gzip", func(t *testing.T) {
		dir := t.TempDir()
		writeLogFile(t, dir, "anytype-2026-01-01.log.gz", "OLD\n", time.Now().Add(-2*time.Hour), true)
		writeLogFile(t, dir, "anytype.log", "ACTIVE\n", time.Now(), false)

		dest := filepath.Join(dir, "bundle.log.gz")
		require.NoError(t, WriteLogBundle(dir, dest, 10*1024*1024))

		got := readGzip(t, dest)
		// Active file streams first; the older rotated .gz follows.
		assert.Equal(t, "ACTIVE\nOLD\n", got)
	})

	t.Run("cap truncates without corrupting gzip stream", func(t *testing.T) {
		dir := t.TempDir()
		// ~1MB of highly compressible repeated content.
		big := strings.Repeat("a", 1024*1024)
		writeLogFile(t, dir, "anytype.log", big, time.Now(), false)

		dest := filepath.Join(dir, "bundle.log.gz")
		require.NoError(t, WriteLogBundle(dir, dest, 4096)) // 4KB cap

		info, err := os.Stat(dest)
		require.NoError(t, err)
		// Cap enforcement is at chunk boundaries, so we allow a modest overshoot.
		assert.LessOrEqual(t, info.Size(), int64(4096+bundleChunkSize))

		// The truncated bundle must still be a well-formed gzip stream.
		got := readGzip(t, dest)
		assert.NotEmpty(t, got)
		assert.True(t, strings.HasPrefix(got, "a"))
	})

	t.Run("skips non-anytype files", func(t *testing.T) {
		dir := t.TempDir()
		writeLogFile(t, dir, "unrelated.log", "IGNORE_ME\n", time.Now(), false)
		writeLogFile(t, dir, "anytype.log", "KEEP\n", time.Now(), false)

		dest := filepath.Join(dir, "bundle.log.gz")
		require.NoError(t, WriteLogBundle(dir, dest, 10*1024*1024))

		assert.Equal(t, "KEEP\n", readGzip(t, dest))
	})

	t.Run("missing directory produces an empty but valid bundle", func(t *testing.T) {
		dir := t.TempDir()
		dest := filepath.Join(dir, "bundle.log.gz")

		require.NoError(t, WriteLogBundle(filepath.Join(dir, "nope"), dest, 10*1024*1024))

		assert.Equal(t, "", readGzip(t, dest))
	})

	t.Run("rejects non-positive cap", func(t *testing.T) {
		err := WriteLogBundle(t.TempDir(), filepath.Join(t.TempDir(), "x.gz"), 0)
		assert.Error(t, err)
	})
}

func writeLogFile(t *testing.T, dir, name, content string, modTime time.Time, gzipped bool) {
	t.Helper()
	path := filepath.Join(dir, name)
	var body []byte
	if gzipped {
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		_, err := gz.Write([]byte(content))
		require.NoError(t, err)
		require.NoError(t, gz.Close())
		body = buf.Bytes()
	} else {
		body = []byte(content)
	}
	require.NoError(t, os.WriteFile(path, body, 0644))
	require.NoError(t, os.Chtimes(path, modTime, modTime))
}

func readGzip(t *testing.T, path string) string {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()
	gr, err := gzip.NewReader(f)
	if err == io.EOF {
		return ""
	}
	require.NoError(t, err)
	defer gr.Close()
	b, err := io.ReadAll(gr)
	require.NoError(t, err)
	return string(b)
}
