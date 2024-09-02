package files

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/pkg/lib/core/mock_core"
)

func TestFileDownloader_Init(t *testing.T) {
	t.Run("create dir success", func(t *testing.T) {
		// given
		tempDirProvider := mock_core.NewMockTempDirProvider(t)
		fileDownloader := NewFileDownloader(tempDirProvider, process.NewNoOp())
		tempDirProvider.EXPECT().TempDir().Return("tmp")

		// when
		err := fileDownloader.Init(context.Background(), "test")
		defer fileDownloader.StopDownload()

		// then
		assert.Nil(t, err)
		assert.Nil(t, os.Remove("tmp/4878ca0425c739fa427f7eda20fe845f6b2e46ba5fe2a14df5b1e32f50603215"))
		assert.Nil(t, os.Remove("tmp"))
	})
	t.Run("create dir - already exist error", func(t *testing.T) {
		// given
		tempDirProvider := mock_core.NewMockTempDirProvider(t)
		fileDownloader := NewFileDownloader(tempDirProvider, process.NewNoOp()).(*fileDownloader)
		tempDirProvider.EXPECT().TempDir().Return("tmp")

		// when
		_, err := fileDownloader.createTempDir("test")
		assert.Nil(t, err)
		_, err = fileDownloader.createTempDir("test")
		assert.Nil(t, err)

		// then
		assert.Nil(t, err)
		assert.Nil(t, os.Remove("tmp/4878ca0425c739fa427f7eda20fe845f6b2e46ba5fe2a14df5b1e32f50603215"))
		assert.Nil(t, os.Remove("tmp"))
	})
}

func TestFileDownloader_AddToQueue(t *testing.T) {
	t.Run("add to queue success, file were processed", func(t *testing.T) {
		// given
		tempDirProvider := mock_core.NewMockTempDirProvider(t)
		fileDownloader := NewFileDownloader(tempDirProvider, process.NewNoOp()).(*fileDownloader)

		// when
		fileDownloader.urlToLocalPath.Store("test", "test")
		stop := fileDownloader.QueueFileForDownload(NewFile("test"))

		// then
		assert.False(t, stop)
		_, ok := fileDownloader.filesInProgress.Load("test")
		assert.False(t, ok)
	})

	t.Run("add to queue success, file in progress", func(t *testing.T) {
		// given
		tempDirProvider := mock_core.NewMockTempDirProvider(t)
		fileDownloader := NewFileDownloader(tempDirProvider, process.NewNoOp()).(*fileDownloader)

		// when
		fileDownloader.filesInProgress.Store("test", struct{}{})
		stop := fileDownloader.QueueFileForDownload(NewFile("test"))

		// then
		assert.False(t, stop)
		_, ok := fileDownloader.filesInProgress.Load("test")
		assert.True(t, ok)
		_, ok = fileDownloader.urlToLocalPath.Load("test")
		assert.False(t, ok)
		_, ok = fileDownloader.filesSubscription.Load("test")
		assert.True(t, ok)
	})

	t.Run("add to queue success, new file", func(t *testing.T) {
		// given
		tempDirProvider := mock_core.NewMockTempDirProvider(t)
		fileDownloader := NewFileDownloader(tempDirProvider, process.NewNoOp()).(*fileDownloader)

		// when
		tempDirProvider.EXPECT().TempDir().Return("tmp")
		err := fileDownloader.Init(context.Background(), "test")
		assert.Nil(t, err)
		defer fileDownloader.StopDownload()
		stop := fileDownloader.QueueFileForDownload(NewFile("test"))

		// then
		assert.False(t, stop)
		_, ok := fileDownloader.filesInProgress.Load("test")
		assert.True(t, ok)
		_, ok = fileDownloader.urlToLocalPath.Load("test")
		assert.False(t, ok)
		_, ok = fileDownloader.filesSubscription.Load("test")
		assert.False(t, ok)
		assert.Nil(t, os.RemoveAll("tmp"))
	})
}

func TestFileDownloader_process(t *testing.T) {
	t.Run("process file success", func(t *testing.T) {
		// given
		tempDirProvider := mock_core.NewMockTempDirProvider(t)
		fileDownloader := NewFileDownloader(tempDirProvider, process.NewNoOp()).(*fileDownloader)

		// when
		file := NewFile("test")
		file.LoadDone("localPath")
		fileDownloader.process(file)

		// then
		_, ok := fileDownloader.filesInProgress.Load("test")
		assert.False(t, ok)
		localPath, ok := fileDownloader.urlToLocalPath.Load("test")
		assert.True(t, ok)
		assert.Equal(t, "localPath", localPath)
		_, ok = fileDownloader.filesSubscription.Load("test")
		assert.False(t, ok)
	})
	t.Run("process file success: notify subscribers", func(t *testing.T) {
		// given
		tempDirProvider := mock_core.NewMockTempDirProvider(t)
		fileDownloader := NewFileDownloader(tempDirProvider, process.NewNoOp()).(*fileDownloader)

		// when
		tempDirProvider.EXPECT().TempDir().Return("tmp")
		err := fileDownloader.Init(context.Background(), "test")
		assert.Nil(t, err)
		file := NewFile("test")
		StopDownload := fileDownloader.QueueFileForDownload(file)
		assert.False(t, StopDownload)
		fileSub := NewFile("test")
		StopDownload = fileDownloader.QueueFileForDownload(fileSub)
		assert.False(t, StopDownload)

		file.LoadDone("localPath")
		doneCh := make(chan struct{})
		go func() {
			defer close(doneCh)
			path, err := fileSub.WaitForLocalPath()
			assert.Nil(t, err)
			assert.Equal(t, "localPath", path)
		}()
		fileDownloader.process(file)

		// then
		_, ok := fileDownloader.filesInProgress.Load("test")
		assert.False(t, ok)
		localPath, ok := fileDownloader.urlToLocalPath.Load("test")
		assert.True(t, ok)
		assert.Equal(t, "localPath", localPath)
		_, ok = fileDownloader.filesSubscription.Load("test")
		assert.False(t, ok)
		assert.Nil(t, os.RemoveAll("tmp"))
		<-doneCh
	})
}
