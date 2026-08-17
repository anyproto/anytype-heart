package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCopyBundleFiles_NoFilesDir(t *testing.T) {
	in := t.TempDir()
	out := t.TempDir()

	n, err := copyBundleFiles(in, out)
	require.NoError(t, err)
	assert.Equal(t, 0, n)
	_, statErr := os.Stat(filepath.Join(out, "files"))
	assert.True(t, os.IsNotExist(statErr), "no files/ dir should be created when the bundle has none")
}

func TestCopyBundleFiles_CopiesAndSkipsDSStore(t *testing.T) {
	in := t.TempDir()
	out := t.TempDir()

	filesDir := filepath.Join(in, "files")
	require.NoError(t, os.MkdirAll(filesDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(filesDir, "icon.png"), []byte("fake-png-bytes"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(filesDir, ".DS_Store"), []byte("junk"), 0o644))

	n, err := copyBundleFiles(in, out)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	got, err := os.ReadFile(filepath.Join(out, "files", "icon.png"))
	require.NoError(t, err)
	assert.Equal(t, "fake-png-bytes", string(got))

	_, statErr := os.Stat(filepath.Join(out, "files", ".DS_Store"))
	assert.True(t, os.IsNotExist(statErr), ".DS_Store must not ride along as a bogus archive entry")
}

func TestCopyBundleFiles_PreservesSubdirectories(t *testing.T) {
	in := t.TempDir()
	out := t.TempDir()

	nested := filepath.Join(in, "files", "sub")
	require.NoError(t, os.MkdirAll(nested, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(nested, "cover.jpg"), []byte("jpg"), 0o644))

	n, err := copyBundleFiles(in, out)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	got, err := os.ReadFile(filepath.Join(out, "files", "sub", "cover.jpg"))
	require.NoError(t, err)
	assert.Equal(t, "jpg", string(got))
}

func writeJSON(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	return path
}

func TestCheckFileSources_FlagsMissingFile(t *testing.T) {
	in := t.TempDir()
	f := writeJSON(t, in, "icon.json", `{
	  "version": 1,
	  "kind": "file_object",
	  "id": "icon-1",
	  "properties": {"name": "icon-1", "source": "files/icon.png"}
	}`)

	dangling, err := checkFileSources(in, []string{f})
	require.NoError(t, err)
	require.Len(t, dangling, 1)
	assert.Equal(t, "files/icon.png", dangling[0].source)
}

func TestCheckFileSources_PassesWhenFileExists(t *testing.T) {
	in := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(in, "files"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(in, "files", "icon.png"), []byte("x"), 0o644))
	f := writeJSON(t, in, "icon.json", `{
	  "version": 1,
	  "kind": "file_object",
	  "id": "icon-1",
	  "properties": {"name": "icon-1", "source": "files/icon.png"}
	}`)

	dangling, err := checkFileSources(in, []string{f})
	require.NoError(t, err)
	assert.Empty(t, dangling)
}

func TestCheckFileSources_IgnoresNonFilesSource(t *testing.T) {
	in := t.TempDir()
	// a "source" property that isn't a files/ path is someone else's use of
	// the key (e.g. a bookmark's URL) and is none of this check's business
	f := writeJSON(t, in, "bookmark.json", `{
	  "version": 1,
	  "id": "bm-1",
	  "properties": {"name": "Anytype", "source": "https://anytype.io"}
	}`)

	dangling, err := checkFileSources(in, []string{f})
	require.NoError(t, err)
	assert.Empty(t, dangling)
}
