package ftsearch

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewFTSearch(t *testing.T) {
	tmpDir, _ := ioutil.TempDir("", "")
	defer os.RemoveAll(tmpDir)
	ft, err := NewFTSearch(tmpDir)
	require.NoError(t, err)
	require.NoError(t, ft.Index(SearchDoc{
		Id:    "test",
		Title: "one",
		Text:  "two",
	}))
	res, err := ft.Search("one")
	require.NoError(t, err)
	assert.Len(t, res, 1)
	ft.Close()
	ft, err = NewFTSearch(tmpDir)
	require.NoError(t, err)
	res, err = ft.Search("one")
	require.NoError(t, err)
	assert.Len(t, res, 1)
	ft.Close()
}
