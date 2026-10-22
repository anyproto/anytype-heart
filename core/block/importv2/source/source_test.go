package source

import (
	"archive/zip"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeZip(t *testing.T, entries map[string]string) string {
	t.Helper()
	zipPath := filepath.Join(t.TempDir(), "test.zip")
	f, err := os.Create(zipPath)
	require.NoError(t, err)
	w := zip.NewWriter(f)
	for name, content := range entries {
		entry, err := w.Create(name)
		require.NoError(t, err)
		_, err = entry.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())
	require.NoError(t, f.Close())
	return zipPath
}

func readEntry(t *testing.T, s Source, name string) string {
	t.Helper()
	r, err := s.Open(context.Background(), name)
	require.NoError(t, err)
	defer r.Close()
	data, err := io.ReadAll(r)
	require.NoError(t, err)
	return string(data)
}

func TestZipSource(t *testing.T) {
	t.Run("zip-slip entries are rejected, not resolved", func(t *testing.T) {
		// given
		zipPath := writeZip(t, map[string]string{
			"../../evil.md":   "escape",
			"/abs/path.md":    "absolute",
			"nested/../ok.md": "cleans inside root",
			"nested/../../e":  "escapes via clean",
			"docs/page.md":    "# hi",
			"__MACOSX/j.md":   "junk",
			"docs/.DS_Store":  "junk",
		})
		want := []string{"docs/page.md", "ok.md"}

		// when
		s, err := newZipSource(zipPath)
		require.NoError(t, err)
		defer s.Close()

		// then
		var names []string
		require.NoError(t, s.Walk(context.Background(), func(e Entry) error {
			names = append(names, e.Name)
			return nil
		}))
		assert.Equal(t, want, names)
		assert.ElementsMatch(t, []string{"../../evil.md", "/abs/path.md", "nested/../../e"}, s.Rejected())
		_, err = s.Open(context.Background(), "../../evil.md")
		assert.ErrorIs(t, err, ErrNotFound)
	})

	t.Run("walk is deterministic and re-readable", func(t *testing.T) {
		// given
		zipPath := writeZip(t, map[string]string{"b.md": "b", "a.md": "a", "c/d.md": "d"})
		s, err := newZipSource(zipPath)
		require.NoError(t, err)
		defer s.Close()
		want := []string{"a.md", "b.md", "c/d.md"}

		// when / then — two passes yield identical listings
		for i := 0; i < 2; i++ {
			var names []string
			require.NoError(t, s.Walk(context.Background(), func(e Entry) error {
				names = append(names, e.Name)
				return nil
			}))
			assert.Equal(t, want, names)
		}
		assert.Equal(t, "d", readEntry(t, s, "c/d.md"))
	})

	t.Run("open streams without whole-entry buffering", func(t *testing.T) {
		// given a 4MB entry
		big := strings.Repeat("x", 4<<20)
		zipPath := writeZip(t, map[string]string{"big.bin": big})
		s, err := newZipSource(zipPath)
		require.NoError(t, err)
		defer s.Close()

		// when — read only the first 16 bytes, then close
		r, err := s.Open(context.Background(), "big.bin")
		require.NoError(t, err)
		buf := make([]byte, 16)
		_, err = io.ReadFull(r, buf)
		require.NoError(t, err)
		require.NoError(t, r.Close())

		// then
		assert.Equal(t, strings.Repeat("x", 16), string(buf))
		e, ok := s.Stat("big.bin")
		require.True(t, ok)
		assert.Equal(t, int64(4<<20), e.Size)
	})

	t.Run("cancelled context stops walk and open", func(t *testing.T) {
		// given
		zipPath := writeZip(t, map[string]string{"a.md": "a"})
		s, err := newZipSource(zipPath)
		require.NoError(t, err)
		defer s.Close()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		// when / then
		err = s.Walk(ctx, func(e Entry) error { return nil })
		assert.ErrorIs(t, err, context.Canceled)
		_, err = s.Open(ctx, "a.md")
		assert.ErrorIs(t, err, context.Canceled)
	})
}

func TestDirSource(t *testing.T) {
	t.Run("lists, normalizes and reads", func(t *testing.T) {
		// given
		root := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(root, "sub"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(root, "sub", "page.md"), []byte("# p"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(root, ".DS_Store"), []byte("junk"), 0o644))
		want := []string{"sub/page.md"}

		// when
		s, err := newDirSource(root)
		require.NoError(t, err)
		defer s.Close()

		// then
		var names []string
		require.NoError(t, s.Walk(context.Background(), func(e Entry) error {
			names = append(names, e.Name)
			return nil
		}))
		assert.Equal(t, want, names)
		assert.Equal(t, "# p", readEntry(t, s, "sub/page.md"))
		e, ok := s.Stat("sub/page.md")
		require.True(t, ok)
		assert.NotEmpty(t, e.FSPath)
	})
}

func TestFileSource(t *testing.T) {
	t.Run("single file by base name", func(t *testing.T) {
		// given
		root := t.TempDir()
		p := filepath.Join(root, "note.md")
		require.NoError(t, os.WriteFile(p, []byte("hello"), 0o644))

		// when
		s, err := newFileSource(p)
		require.NoError(t, err)
		defer s.Close()

		// then
		assert.Equal(t, "hello", readEntry(t, s, "note.md"))
		_, err = s.Open(context.Background(), "other.md")
		assert.ErrorIs(t, err, ErrNotFound)
	})
}

func TestOpenDispatch(t *testing.T) {
	t.Run("dispatches dir, zip and file", func(t *testing.T) {
		// given
		root := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(root, "a.md"), []byte("a"), 0o644))
		zipPath := writeZip(t, map[string]string{"z.md": "z"})

		// when / then
		dirSrc, err := Open(root)
		require.NoError(t, err)
		defer dirSrc.Close()
		_, ok := dirSrc.(*dirSource)
		assert.True(t, ok)

		zipSrc, err := Open(zipPath)
		require.NoError(t, err)
		defer zipSrc.Close()
		_, ok = zipSrc.(*zipSource)
		assert.True(t, ok)

		fileSrc, err := Open(filepath.Join(root, "a.md"))
		require.NoError(t, err)
		defer fileSrc.Close()
		_, ok = fileSrc.(*fileSource)
		assert.True(t, ok)
	})
}

func TestSpill(t *testing.T) {
	t.Run("archive entry spills under sanitized flat name", func(t *testing.T) {
		// given
		zipPath := writeZip(t, map[string]string{"docs/img.png": "bytes", "other/img.png": "other"})
		s, err := newZipSource(zipPath)
		require.NoError(t, err)
		defer s.Close()
		dir := t.TempDir()

		// when
		p1, err := Spill(context.Background(), s, "docs/img.png", dir)
		require.NoError(t, err)
		p2, err := Spill(context.Background(), s, "other/img.png", dir)
		require.NoError(t, err)
		again, err := Spill(context.Background(), s, "docs/img.png", dir)
		require.NoError(t, err)

		// then — same-basename entries do not collide; repeat is idempotent
		assert.NotEqual(t, p1, p2)
		assert.Equal(t, p1, again)
		assert.True(t, strings.HasPrefix(p1, dir))
		data, err := os.ReadFile(p1)
		require.NoError(t, err)
		assert.Equal(t, "bytes", string(data))
	})

	t.Run("disk-backed entry returns its own path without copy", func(t *testing.T) {
		// given
		root := t.TempDir()
		p := filepath.Join(root, "a.md")
		require.NoError(t, os.WriteFile(p, []byte("a"), 0o644))
		s, err := newDirSource(root)
		require.NoError(t, err)
		defer s.Close()

		// when
		got, err := Spill(context.Background(), s, "a.md", t.TempDir())

		// then
		require.NoError(t, err)
		assert.Equal(t, p, got)
	})
}
