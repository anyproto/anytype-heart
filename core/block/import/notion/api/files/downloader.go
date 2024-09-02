package files

import (
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"sync"

	"github.com/zeebo/blake3"

	"github.com/anyproto/anytype-heart/core/block/import/common/workerpool"
	"github.com/anyproto/anytype-heart/core/block/process"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
)

const workersNumber = 5

type Downloader interface {
	Init(ctx context.Context, token string) error
	QueueFileForDownload(file LocalFileProvider) bool
	ProcessDownloadedFiles()
	StopDownload()
}

type fileDownloader struct {
	pool            *workerpool.WorkerPool
	tempDirProvider core.TempDirProvider
	progress        process.Progress

	urlToLocalPath    sync.Map
	filesInProgress   sync.Map
	filesSubscription sync.Map
}

func NewFileDownloader(tempDirProvider core.TempDirProvider, progress process.Progress) Downloader {
	return &fileDownloader{
		pool:            workerpool.NewPool(workersNumber),
		tempDirProvider: tempDirProvider,
		progress:        progress,
	}
}

func (d *fileDownloader) Init(ctx context.Context, token string) error {
	dirPath, err := d.createTempDir(token)
	if err != nil {
		return err
	}
	go d.pool.Start(&DataObject{
		dirPath: dirPath,
		ctx:     ctx,
	})
	return nil
}

func (d *fileDownloader) createTempDir(token string) (string, error) {
	hasher := hashersPool.Get().(*blake3.Hasher)
	defer hashersPool.Put(hasher)

	hasher.Reset()
	// nolint: errcheck
	hasher.Write([]byte(token))
	tokenHash := hex.EncodeToString(hasher.Sum(nil))

	dirPath := filepath.Join(d.tempDirProvider.TempDir(), tokenHash)
	err := os.MkdirAll(dirPath, 0700)
	if err != nil && !os.IsExist(err) {
		return "", err
	}
	return dirPath, nil
}

func (d *fileDownloader) QueueFileForDownload(file LocalFileProvider) bool {
	select {
	case <-d.progress.Canceled():
		return true
	default:
	}

	url := file.GetUrl()
	if localPath, ok := d.getFileFromCache(url); ok {
		file.LoadDone(localPath)
		return false
	}

	if d.isFileInProgress(url) {
		d.subscribeToFile(url, file)
		return false
	}

	d.markFileInProgress(url)
	return d.pool.AddWork(file)
}

func (d *fileDownloader) ProcessDownloadedFiles() {
	for result := range d.pool.Results() {
		downloadedFile := result.(FileInfoProvider)
		d.process(downloadedFile)
	}
}

func (d *fileDownloader) process(downloadedFile FileInfoProvider) {
	url := downloadedFile.GetUrl()

	localPath := downloadedFile.GetLocalPath()
	d.saveFileInfo(url, localPath)
	d.notifySubscribers(url, localPath)
	d.markFileCompleted(url)
}

func (d *fileDownloader) getFileFromCache(url string) (string, bool) {
	if path, ok := d.urlToLocalPath.Load(url); ok {
		return path.(string), true
	}
	return "", false
}

func (d *fileDownloader) isFileInProgress(url string) bool {
	_, inProgress := d.filesInProgress.Load(url)
	return inProgress
}

func (d *fileDownloader) markFileInProgress(url string) {
	d.filesInProgress.Store(url, struct{}{})
}

func (d *fileDownloader) subscribeToFile(url string, file LocalFileProvider) {
	subscriptionCh := make(chan string)
	subscribers, _ := d.filesSubscription.LoadOrStore(url, []chan string{})
	d.filesSubscription.Store(url, append(subscribers.([]chan string), subscriptionCh))
	file.SubscribeToExistingDownload(subscriptionCh)
}

func (d *fileDownloader) saveFileInfo(url, localPath string) {
	if localPath != "" {
		d.urlToLocalPath.Store(url, localPath)
	}
}

func (d *fileDownloader) notifySubscribers(url, localPath string) {
	if subscribers, ok := d.filesSubscription.Load(url); ok {
		for _, sub := range subscribers.([]chan string) {
			sub <- localPath
			close(sub)
		}
		d.filesSubscription.Delete(url)
	}
}

func (d *fileDownloader) markFileCompleted(url string) {
	d.filesInProgress.Delete(url)
}

func (d *fileDownloader) StopDownload() {
	d.pool.Stop()
	d.pool.CloseTask()
}
