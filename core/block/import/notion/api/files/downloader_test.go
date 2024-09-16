package files

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/process"
)

func TestFileDownloader_Init(t *testing.T) {
	t.Run("create dir success", func(t *testing.T) {
		// given
		fileDownloader := NewFileDownloader(process.NewNoOp())

		// when
		err := fileDownloader.Init(context.Background())
		defer fileDownloader.StopDownload()

		// then
		assert.Nil(t, err)
	})
	t.Run("create dir - already exist error", func(t *testing.T) {
		// given
		fileDownloader := NewFileDownloader(process.NewNoOp()).(*fileDownloader)

		// when
		_, err := fileDownloader.createTempDir()
		assert.Nil(t, err)
		_, err = fileDownloader.createTempDir()

		// then
		assert.Nil(t, err)
	})
}

func TestFileDownloader_AddToQueue(t *testing.T) {
	t.Run("add to queue success, file were processed", func(t *testing.T) {
		// given
		fileDownloader := NewFileDownloader(process.NewNoOp()).(*fileDownloader)

		// when
		fileDownloader.urlToFile.Store("test", NewFile("test"))
		_, stop := fileDownloader.QueueFileForDownload("test")

		// then
		assert.False(t, stop)
		_, ok := fileDownloader.filesInProgress.Load("test")
		assert.False(t, ok)
	})

	t.Run("add to queue success, file in progress", func(t *testing.T) {
		// given
		fileDownloader := NewFileDownloader(process.NewNoOp()).(*fileDownloader)

		// when
		fileDownloader.filesInProgress.Store("test", NewFile("test"))
		_, stop := fileDownloader.QueueFileForDownload("test")

		// then
		assert.False(t, stop)
		_, ok := fileDownloader.filesInProgress.Load("test")
		assert.True(t, ok)
		_, ok = fileDownloader.urlToFile.Load("test")
		assert.False(t, ok)
	})

	t.Run("add to queue success, new file", func(t *testing.T) {
		// given
		fileDownloader := NewFileDownloader(process.NewNoOp()).(*fileDownloader)

		// when
		err := fileDownloader.Init(context.Background())
		assert.Nil(t, err)
		defer fileDownloader.StopDownload()
		_, stop := fileDownloader.QueueFileForDownload("test")

		// then
		assert.False(t, stop)
		_, ok := fileDownloader.filesInProgress.Load("test")
		assert.True(t, ok)
		_, ok = fileDownloader.urlToFile.Load("test")
		assert.False(t, ok)
		assert.Nil(t, os.RemoveAll("tmp"))
	})
}

func TestFileDownloader_process(t *testing.T) {
	t.Run("process file success", func(t *testing.T) {
		// given
		fileDownloader := NewFileDownloader(process.NewNoOp()).(*fileDownloader)

		// when
		f := NewFile("test")
		downloadFile := f.(*file)
		downloadFile.localPath = "localPath"
		fileDownloader.process(downloadFile)

		// then
		_, ok := fileDownloader.filesInProgress.Load("test")
		assert.False(t, ok)
		newFile, ok := fileDownloader.urlToFile.Load("test")
		assert.True(t, ok)
		assert.Equal(t, "localPath", newFile.(LocalFileProvider).GetLocalPath())
	})
}
