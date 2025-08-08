package export

import (
	"archive/zip"
	"io/ioutil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDirWriter_WriteFile(t *testing.T) {
	path, err := ioutil.TempDir("", "")
	require.NoError(t, err)
	defer os.RemoveAll(path)

	lastModifiedDate := int64(1692203040)
	wr, err := newDirWriter(path, false)
	require.NoError(t, err)
	require.NoError(t, wr.WriteFile("some.test", strings.NewReader("some string"), lastModifiedDate))
	require.NoError(t, wr.Close())

	assert.True(t, strings.HasPrefix(wr.Path(), path))
	data, err := ioutil.ReadFile(filepath.Join(wr.Path(), "some.test"))
	require.NoError(t, err)
	assert.Equal(t, "some string", string(data))

	file, err := os.Open(filepath.Join(wr.Path(), "some.test"))
	require.NoError(t, err)
	defer file.Close()

	stat, err := file.Stat()
	require.NoError(t, err)

	assert.Equal(t, lastModifiedDate, stat.ModTime().Unix())
}

func TestZipWriter_WriteFile(t *testing.T) {
	path, err := ioutil.TempDir("", "")
	require.NoError(t, err)
	defer os.RemoveAll(path)

	lastModifiedDate := int64(1692203040)

	wr, err := newZipWriter(path, uniqName()+".zip")
	require.NoError(t, err)
	require.NoError(t, wr.WriteFile("some.test", strings.NewReader("some string"), lastModifiedDate))
	require.NoError(t, wr.Close())

	assert.True(t, strings.HasPrefix(wr.Path(), path))
	assert.True(t, strings.HasSuffix(wr.Path(), ".zip"))

	zr, err := zip.OpenReader(wr.Path())
	require.NoError(t, err)
	defer zr.Close()

	var found bool

	for _, zf := range zr.Reader.File {
		if zf.Name == "some.test" {
			found = true
			f, e := zf.Open()
			require.NoError(t, e)
			data, err := ioutil.ReadAll(f)
			require.NoError(t, err)
			f.Close()
			assert.Equal(t, "some string", string(data))

			assert.Equal(t, lastModifiedDate, zf.Modified.Unix())
		}

	}
	assert.True(t, found)
}

func TestZipWriter_Get(t *testing.T) {
	t.Run("file without name", func(t *testing.T) {
		// given
		path, err := ioutil.TempDir("", "")
		require.NoError(t, err)
		defer os.RemoveAll(path)

		wr, err := newZipWriter(path, uniqName()+".zip")
		require.NoError(t, err)

		// when
		name := wr.Namer().Get(Files, "hash", "", "")

		// then
		require.NoError(t, wr.Close())
		assert.Equal(t, filepath.Join(Files, defaultFileName), name)
	})
}

func TestDeepLinkNamer(t *testing.T) {
	t.Run("markdown links include spaceId", func(t *testing.T) {
		u, _ := url.Parse("http://gateway.example.com")
		namer := &deepLinkNamer{
			gatewayUrl: *u,
			spaceId:    "bafyreieorkzqmzegus55jn5ptho4bp6jtrzc7anfbd54fo3g5wrbp3j3fu.scoxzd7vu6rz",
		}

		// Test markdown link
		result := namer.Get("", "bafyreiblv65o2t64yownuweyf6rvgd7cyosxkhd7hg4zzhpqjhaerlqxbi", "Meeting Notes", ".md")
		expected := "anytype://object?objectId=bafyreiblv65o2t64yownuweyf6rvgd7cyosxkhd7hg4zzhpqjhaerlqxbi&spaceId=bafyreieorkzqmzegus55jn5ptho4bp6jtrzc7anfbd54fo3g5wrbp3j3fu.scoxzd7vu6rz"
		assert.Equal(t, expected, result)
	})

	t.Run("file links via gateway", func(t *testing.T) {
		u, _ := url.Parse("http://gateway.example.com")
		namer := &deepLinkNamer{
			gatewayUrl: *u,
			spaceId:    "bafyreieorkzqmzegus55jn5ptho4bp6jtrzc7anfbd54fo3g5wrbp3j3fu.scoxzd7vu6rz",
		}

		// Test image link
		result := namer.Get("files", "imageHash123", "photo", ".jpg")
		expected := "http://gateway.example.com/image/imageHash123"
		assert.Equal(t, expected, result)

		// Test regular file link
		result = namer.Get("files", "fileHash456", "document", ".pdf")
		expected = "http://gateway.example.com/file/fileHash456"
		assert.Equal(t, expected, result)
	})

	t.Run("fallback to anytype:// when no gateway", func(t *testing.T) {
		namer := &deepLinkNamer{
			gatewayUrl: url.URL{}, // empty URL
			spaceId:    "bafyreieorkzqmzegus55jn5ptho4bp6jtrzc7anfbd54fo3g5wrbp3j3fu.scoxzd7vu6rz",
		}

		// Should fallback to anytype:// scheme with spaceId
		result := namer.Get("files", "fileHash789", "backup", ".txt")
		expected := "anytype://object?objectId=fileHash789&spaceId=bafyreieorkzqmzegus55jn5ptho4bp6jtrzc7anfbd54fo3g5wrbp3j3fu.scoxzd7vu6rz"
		assert.Equal(t, expected, result)
	})
}
