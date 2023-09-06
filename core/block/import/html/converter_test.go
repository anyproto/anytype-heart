package html

import (
	"archive/zip"
	"errors"
	"github.com/anyproto/anytype-heart/core/block/import/source"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"

	cv "github.com/anyproto/anytype-heart/core/block/import/converter"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type MockTempDirProvider struct{}

func (p *MockTempDirProvider) TempDir() string {
	return os.TempDir()
}

func TestHTML_GetSnapshots(t *testing.T) {
	h := &HTML{}
	p := process.NewProgress(pb.ModelProcess_Import)
	sn, err := h.GetSnapshots(&pb.RpcObjectImportRequest{
		Params: &pb.RpcObjectImportRequestParamsOfHtmlParams{
			HtmlParams: &pb.RpcObjectImportRequestHtmlParams{Path: []string{"testdata/test.html", "testdata/test"}},
		},
		Type: pb.RpcObjectImportRequest_Txt,
		Mode: pb.RpcObjectImportRequest_IGNORE_ERRORS,
	}, p)

	assert.NotNil(t, sn)
	assert.Len(t, sn.Snapshots, 2)
	assert.Contains(t, sn.Snapshots[0].FileName, "test.html")
	assert.NotEmpty(t, sn.Snapshots[0].Snapshot.Data.Details.Fields["name"])
	assert.Equal(t, sn.Snapshots[0].Snapshot.Data.Details.Fields["name"], pbtypes.String("test"))

	assert.Contains(t, sn.Snapshots[1].FileName, rootCollectionName)
	assert.NotEmpty(t, sn.Snapshots[1].Snapshot.Data.ObjectTypes)
	assert.Equal(t, sn.Snapshots[1].Snapshot.Data.ObjectTypes[0], bundle.TypeKeyCollection.URL())

	assert.NotEmpty(t, err)
	assert.True(t, errors.Is(err.GetResultError(pb.RpcObjectImportRequest_Html), cv.ErrNoObjectsToImport))
}

func TestHTML_provideFileName(t *testing.T) {
	t.Run("web link in file block - return web link", func(t *testing.T) {
		// given
		h := &HTML{}
		currentDir, err := os.Getwd()
		assert.Nil(t, err)
		source := source.GetSource(currentDir)

		// when
		newFileName, _, err := h.provideFileName("http://example.com", source, currentDir)

		// then
		assert.Nil(t, err)
		assert.Equal(t, "http://example.com", newFileName)
	})
	t.Run("absolute file name exist on local machine - return not changed file name", func(t *testing.T) {
		// given
		h := &HTML{}
		currentDir, err := os.Getwd()
		assert.Nil(t, err)
		source := source.GetSource(currentDir)

		// when
		absPath, err := filepath.Abs("testdata/test")
		assert.Nil(t, err)
		newFileName, _, err := h.provideFileName(absPath, source, currentDir)

		// then
		assert.Nil(t, err)
		assert.Equal(t, absPath, newFileName)
	})
	t.Run("given relative file name from imported directory - return absolute path", func(t *testing.T) {
		// given
		h := &HTML{}
		currentDir, err := os.Getwd()
		assert.Nil(t, err)
		source := source.GetSource(currentDir)

		// when
		newFileName, _, err := h.provideFileName("testdata/test", source, currentDir)

		// then
		assert.Nil(t, err)
		absPath, err := filepath.Abs("testdata/test")
		assert.Nil(t, err)
		assert.Equal(t, absPath, newFileName)
	})
	t.Run("archive with files is imported - return path to temp directory", func(t *testing.T) {
		// given
		h := HTML{}
		h.tempDirProvider = &MockTempDirProvider{}
		testFileName, archiveName := prepareArchivedFiles(t)
		defer os.Remove(archiveName)
		source := source.GetSource(archiveName)
		err := source.Initialize(archiveName)
		assert.Nil(t, err)

		// when
		newFileName, _, err := h.provideFileName(testFileName, source, archiveName)
		defer os.Remove(newFileName)

		// then
		assert.Nil(t, err)
		absoluteFileName := filepath.Join(os.TempDir(), testFileName)
		assert.Equal(t, absoluteFileName, newFileName)
	})
	t.Run("file doesn't exist - not change original path", func(t *testing.T) {
		// given
		h := HTML{}
		source := source.GetSource("test")

		// when
		newFileName, _, err := h.provideFileName("test", source, "imported path")

		// then
		assert.Nil(t, err)
		assert.Equal(t, "test", newFileName)
	})
}

func prepareArchivedFiles(t *testing.T) (string, string) {
	// create test archive
	archiveName := filepath.Join(".", strconv.FormatInt(rand.Int63(), 10)+".zip")
	file, err := os.Create(archiveName)
	assert.Nil(t, err)

	// write test file to archive
	writer := zip.NewWriter(file)
	testFileName := "testfile"
	_, err = writer.Create(testFileName)
	assert.Nil(t, err)
	writer.Close()
	file.Close()

	// open zip archive for reading
	reader, err := zip.OpenReader(archiveName)
	assert.Nil(t, err)

	assert.Len(t, reader.File, 1)
	return testFileName, archiveName
}
