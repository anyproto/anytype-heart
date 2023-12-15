package export

import (
	"archive/zip"
	"io/ioutil"
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
