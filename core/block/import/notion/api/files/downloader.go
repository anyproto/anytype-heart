package files

import (
	"context"
	"os"
	"path/filepath"
	"sync"

	"github.com/anyproto/anytype-heart/core/block/import/common/workerpool"
	"github.com/anyproto/anytype-heart/core/block/process"
)

const (
	workersNumber          = 5
	anytypeNotionImportDir = "anytype_notion_import"
)

type Downloader interface {
	Init(ctx context.Context) error
	QueueFileForDownload(url string) (LocalFileProvider, bool)
	ProcessDownloadedFiles()
	StopDownload()
}

type fileDownloader struct {
	pool     *workerpool.WorkerPool
	progress process.Progress

	urlToFile       sync.Map
	filesInProgress sync.Map
}

func NewFileDownloader(progress process.Progress) Downloader {
	return &fileDownloader{
		pool:     workerpool.NewPool(workersNumber),
		progress: progress,
	}
}

func (d *fileDownloader) Init(ctx context.Context) error {
	dirPath, err := d.createTempDir()
	if err != nil {
		return err
	}
	go d.pool.Start(&DataObject{
		dirPath: dirPath,
		ctx:     ctx,
	})
	return nil
}

func (d *fileDownloader) createTempDir() (string, error) {
	dirPath := filepath.Join(os.TempDir(), anytypeNotionImportDir)
	err := os.MkdirAll(dirPath, 0700)
	if err != nil && !os.IsExist(err) {
		return "", err
	}
	return dirPath, nil
}

func (d *fileDownloader) QueueFileForDownload(url string) (LocalFileProvider, bool) {
	select {
	case <-d.progress.Canceled():
		return nil, true
	default:
	}

	if cachedFile, ok := d.getFileFromCache(url); ok {
		return cachedFile, false
	}

	if fileInProgress, ok := d.isFileInProgress(url); ok {
		return fileInProgress, false
	}

	newFile := NewFile(url)
	d.markFileInProgress(newFile)
	return newFile, d.pool.AddWork(newFile)
}

func (d *fileDownloader) ProcessDownloadedFiles() {
	for result := range d.pool.Results() {
		downloadedFile := result.(LocalFileProvider)
		d.process(downloadedFile)
	}
}

func (d *fileDownloader) process(downloadedFile LocalFileProvider) {
	url := downloadedFile.GetUrl()

	d.saveFileInfo(downloadedFile)
	d.markFileCompleted(url)
}

func (d *fileDownloader) getFileFromCache(url string) (LocalFileProvider, bool) {
	if file, ok := d.urlToFile.Load(url); ok {
		return file.(LocalFileProvider), true
	}
	return nil, false
}

func (d *fileDownloader) isFileInProgress(url string) (LocalFileProvider, bool) {
	file, inProgress := d.filesInProgress.Load(url)
	if inProgress {
		return file.(LocalFileProvider), inProgress
	}
	return nil, false
}

func (d *fileDownloader) markFileInProgress(newFile LocalFileProvider) {
	d.filesInProgress.Store(newFile.GetUrl(), newFile)
}

func (d *fileDownloader) saveFileInfo(downloadedFile LocalFileProvider) {
	if downloadedFile.GetLocalPath() != "" {
		d.urlToFile.Store(downloadedFile.GetUrl(), downloadedFile)
	}
}

func (d *fileDownloader) markFileCompleted(url string) {
	d.filesInProgress.Delete(url)
}

func (d *fileDownloader) StopDownload() {
	d.pool.Stop()
	d.pool.CloseTask()
}
