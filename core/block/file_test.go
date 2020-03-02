package block

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDropFilesProcess_Init(t *testing.T) {
	fp := &dropFilesProcess{}
	path1, _ := filepath.Abs("./testdata/testdir")
	path2, _ := filepath.Abs("./testdata/testdir2")
	err := fp.Init([]string{path1, path2})
	require.NoError(t, err)

	assert.Equal(t, "testdir", fp.root.child[0].name)
	assert.True(t, fp.root.child[0].isDir)
	assert.Equal(t, "testdir2", fp.root.child[1].name)
}
