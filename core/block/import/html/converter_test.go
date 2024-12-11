package html

import (
	"archive/zip"
	"context"
	"errors"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/block/import/common/source"
	"github.com/anyproto/anytype-heart/core/block/import/common/test"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type MockTempDirProvider struct{}

func (p *MockTempDirProvider) TempDir() string {
	return os.TempDir()
}

func TestHTML_GetSnapshots(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		h := &HTML{}
		p := process.NewNoOp()
		sn, err := h.GetSnapshots(
			context.Background(),
			&pb.RpcObjectImportRequest{
				Params: &pb.RpcObjectImportRequestParamsOfHtmlParams{
					HtmlParams: &pb.RpcObjectImportRequestHtmlParams{Path: []string{filepath.Join("testdata", "test.html"), filepath.Join("testdata", "test")}},
				},
				Type: model.Import_Html,
				Mode: pb.RpcObjectImportRequest_IGNORE_ERRORS,
			},
			p,
		)

		assert.NotNil(t, sn)
		assert.Len(t, sn.Snapshots, 2)
		assert.Contains(t, sn.Snapshots[0].FileName, "test.html")
		assert.NotEmpty(t, sn.Snapshots[0].Snapshot.Data.Details.GetString("name"))
		assert.Equal(t, sn.Snapshots[0].Snapshot.Data.Details.GetString("name"), "test")

		assert.Contains(t, sn.Snapshots[1].FileName, rootCollectionName)
		assert.NotEmpty(t, sn.Snapshots[1].Snapshot.Data.ObjectTypes)
		assert.Equal(t, sn.Snapshots[1].Snapshot.Data.ObjectTypes[0], bundle.TypeKeyCollection.String())

		assert.NotEmpty(t, err)
		assert.True(t, errors.Is(err.GetResultError(model.Import_Html), common.ErrFileImportNoObjectsInDirectory))
	})
	t.Run("no object in archive", func(t *testing.T) {
		// given
		dir := t.TempDir()
		zipPath := filepath.Join(dir, "empty.zip")
		test.CreateEmptyZip(t, zipPath)
		html := HTML{}
		p := process.NewProgress(&pb.ModelProcessMessageOfImport{Import: &pb.ModelProcessImport{}})

		// when
		_, ce := html.GetSnapshots(context.Background(), &pb.RpcObjectImportRequest{
			Params: &pb.RpcObjectImportRequestParamsOfHtmlParams{
				HtmlParams: &pb.RpcObjectImportRequestHtmlParams{
					Path: []string{zipPath},
				},
			},
			Type: model.Import_Html,
			Mode: pb.RpcObjectImportRequest_IGNORE_ERRORS,
		}, p)

		// then
		assert.NotNil(t, ce)
		assert.True(t, errors.Is(ce.GetResultError(model.Import_Html), common.ErrFileImportNoObjectsInZipArchive))
	})
	t.Run("no object in dir", func(t *testing.T) {
		// given
		dir := t.TempDir()
		html := HTML{}
		p := process.NewProgress(&pb.ModelProcessMessageOfImport{Import: &pb.ModelProcessImport{}})

		// when
		_, ce := html.GetSnapshots(context.Background(), &pb.RpcObjectImportRequest{
			Params: &pb.RpcObjectImportRequestParamsOfHtmlParams{
				HtmlParams: &pb.RpcObjectImportRequestHtmlParams{
					Path: []string{dir},
				},
			},
			Type: model.Import_Html,
			Mode: pb.RpcObjectImportRequest_IGNORE_ERRORS,
		}, p)

		// then
		assert.NotNil(t, ce)
		assert.True(t, errors.Is(ce.GetResultError(model.Import_Html), common.ErrFileImportNoObjectsInDirectory))
	})
}

func TestHTML_provideFileName(t *testing.T) {
	t.Run("web link in file block - return web link", func(t *testing.T) {
		// given
		h := &HTML{}
		currentDir, err := os.Getwd()
		assert.Nil(t, err)
		source := source.GetSource(currentDir)

		// when
		newFileName, _, err := common.ProvideFileName("http://example.com", source, currentDir, h.tempDirProvider)

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
		filePath := filepath.Join("testdata", "test")

		// when
		absPath, err := filepath.Abs(filePath)
		assert.Nil(t, err)
		newFileName, _, err := common.ProvideFileName(absPath, source, currentDir, h.tempDirProvider)

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
		filePath := filepath.Join("testdata", "test")

		// when
		newFileName, _, err := common.ProvideFileName(filePath, source, currentDir, h.tempDirProvider)

		// then
		assert.Nil(t, err)
		absPath, err := filepath.Abs(filePath)
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
		newFileName, _, err := common.ProvideFileName(testFileName, source, archiveName, h.tempDirProvider)
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
		newFileName, _, err := common.ProvideFileName("test", source, "imported path", h.tempDirProvider)

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
