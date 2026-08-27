package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pb"
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

// The manifest `files` map has a reader, and this is it (§2c, v0.47): each
// binding is copied into the archive at its own relative path — the map,
// not the files/ convention, is the binding, so an authored bundle's
// assets/ layout travels — and the converted snapshot's `source` detail is
// written from the map, because that detail is the pb importer's own
// contract for locating a file's bytes (normalizeFilePath). Without this,
// a native bundle's blobs attach to nothing: the documents carry no path
// on purpose, and the archive would install with blank icons and dead
// file blocks.
//
// How this can fail: copy only files/ (the authored assets/ blob never
// reaches the archive); skip the source injection (the blob travels but no
// importer ever looks at it); or key the injection by the minted output id
// instead of the document's envelope id (the binding misses every
// re-minted document).
func TestRun_ManifestBindsBlobsIntoTheArchive(t *testing.T) {
	inDir := t.TempDir()
	outDir := t.TempDir()
	write := func(rel, body string) {
		path := filepath.Join(inDir, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	}
	write("files/img.anyblock.json", `{"version":1,"kind":"file_object","id":"img-1","properties":{"name":"Logo"}}`)
	write("assets/logo.png", "png bytes")
	write("index.json", `{"version":1,"manifest":{"files":{"img-1":"assets/logo.png"}}}`)

	// when
	require.NoError(t, run(inDir, outDir, false, false, formatPb))

	// then: the blob travelled at its authored path…
	copied, err := os.ReadFile(filepath.Join(outDir, "assets", "logo.png"))
	require.NoError(t, err)
	assert.Equal(t, "png bytes", string(copied))

	// …and the snapshot carries the binding as its source detail
	var found bool
	filepath.Walk(outDir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || filepath.Ext(p) != ".pb" {
			return nil
		}
		data, readErr := os.ReadFile(p)
		require.NoError(t, readErr)
		var sw pb.SnapshotWithType
		require.NoError(t, proto.Unmarshal(data, &sw))
		det := sw.Snapshot.GetData().GetDetails().GetFields()
		if det["id"].GetStringValue() != "img-1" {
			return nil
		}
		found = true
		assert.Equal(t, "assets/logo.png", det["source"].GetStringValue(),
			"the manifest binding reaches the archive as the source detail")
		return nil
	})
	require.True(t, found, "the file document must convert")

	t.Run("a binding the bundle cannot honour refuses the whole conversion", func(t *testing.T) {
		badIn, badOut := t.TempDir(), t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(badIn, "files"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(badIn, "files", "img.anyblock.json"),
			[]byte(`{"version":1,"kind":"file_object","id":"img-1","properties":{"name":"Logo"}}`), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(badIn, "index.json"),
			[]byte(`{"version":1,"manifest":{"files":{"img-1":"assets/missing.png"}}}`), 0o644))
		err := run(badIn, badOut, false, false, formatPb)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "manifest file binding")
	})
}
