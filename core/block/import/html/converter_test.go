package html

import (
	"archive/zip"
	"errors"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"

	cv "github.com/anyproto/anytype-heart/core/block/import/converter"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	sb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type MockTempDirProvider struct{}

func (p *MockTempDirProvider) TempDir() string {
	return os.TempDir()
}

func TestHTML_GetSnapshots(t *testing.T) {
	t.Run("successful import", func(t *testing.T) {
		// given
		h := &HTML{}
		p := process.NewProgress(pb.ModelProcess_Import)

		// when
		sn, err := h.GetSnapshots(&pb.RpcObjectImportRequest{
			Params: &pb.RpcObjectImportRequestParamsOfHtmlParams{
				HtmlParams: &pb.RpcObjectImportRequestHtmlParams{Path: []string{"testdata/test.html", "testdata/test"}},
			},
			Type: pb.RpcObjectImportRequest_Txt,
			Mode: pb.RpcObjectImportRequest_IGNORE_ERRORS,
		}, p, 0)

		// then
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
	})
	t.Run("snapshots have relation importDate", func(t *testing.T) {
		// given
		h := &HTML{}
		p := process.NewProgress(pb.ModelProcess_Import)

		// when
		sn, err := h.GetSnapshots(&pb.RpcObjectImportRequest{
			Params: &pb.RpcObjectImportRequestParamsOfHtmlParams{
				HtmlParams: &pb.RpcObjectImportRequestHtmlParams{Path: []string{"testdata/test.html", "testdata/test"}},
			},
			Type: pb.RpcObjectImportRequest_Txt,
			Mode: pb.RpcObjectImportRequest_IGNORE_ERRORS,
		}, p, 1)

		// then
		assert.NotNil(t, sn)
		assert.NotEmpty(t, err)

		for _, snapshot := range sn.Snapshots {
			if snapshot.SbType == sb.SmartBlockTypeSubObject ||
				lo.Contains(snapshot.Snapshot.Data.ObjectTypes, bundle.TypeKeyCollection.URL()) {
				continue
			}
			assert.Contains(t, snapshot.Snapshot.Data.Details.Fields, bundle.RelationKeyImportDate.String())
			assert.Equal(t, int64(1), pbtypes.GetInt64(snapshot.Snapshot.Data.Details, bundle.RelationKeyImportDate.String()))
		}
	})
}

func TestHTML_provideFileName(t *testing.T) {
	t.Run("web link in file block - return web link", func(t *testing.T) {
		// given
		h := &HTML{}
		currentDir, err := os.Getwd()
		assert.Nil(t, err)

		// when
		newFileName, _, err := cv.ProvideFileName("http://example.com", nil, currentDir, h.tempDirProvider)

		// then
		assert.Nil(t, err)
		assert.Equal(t, "http://example.com", newFileName)
	})
	t.Run("absolute file name exist on local machine - return not changed file name", func(t *testing.T) {
		// given
		h := &HTML{}
		currentDir, err := os.Getwd()
		assert.Nil(t, err)

		// when
		absPath, err := filepath.Abs("testdata/test")
		assert.Nil(t, err)
		newFileName, _, err := cv.ProvideFileName(absPath, nil, currentDir, h.tempDirProvider)

		// then
		assert.Nil(t, err)
		assert.Equal(t, absPath, newFileName)
	})
	t.Run("given relative file name from imported directory - return absolute path", func(t *testing.T) {
		// given
		h := &HTML{}
		currentDir, err := os.Getwd()
		assert.Nil(t, err)

		// when
		newFileName, _, err := cv.ProvideFileName("testdata/test", nil, currentDir, h.tempDirProvider)

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
		filesFromArchive, testFileName, archiveName := prepareArchivedFiles(t)
		defer os.Remove(archiveName)
		defer filesFromArchive[testFileName].Close()

		// when
		newFileName, _, err := cv.ProvideFileName(testFileName, filesFromArchive, archiveName, h.tempDirProvider)
		defer os.Remove(newFileName)

		// then
		assert.Nil(t, err)
		absoluteFileName := filepath.Join(os.TempDir(), testFileName)
		assert.Equal(t, absoluteFileName, newFileName)
	})
	t.Run("file doesn't exist - not change original path", func(t *testing.T) {
		// given
		h := HTML{}

		// when
		newFileName, _, err := cv.ProvideFileName("test", nil, "imported path", h.tempDirProvider)

		// then
		assert.Nil(t, err)
		assert.Equal(t, "test", newFileName)
	})
}

func prepareArchivedFiles(t *testing.T) (map[string]io.ReadCloser, string, string) {
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

	// get test file reader from archive
	assert.Len(t, reader.File, 1)
	testFile := reader.File[0]
	rc, err := testFile.Open()
	assert.Nil(t, err)

	// fill map with files from archive
	filesFromArchive := map[string]io.ReadCloser{testFileName: rc}
	return filesFromArchive, testFileName, archiveName
}
