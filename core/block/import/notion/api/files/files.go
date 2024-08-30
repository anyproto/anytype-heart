package files

import (
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/miolini/datacounter"

	"github.com/anyproto/anytype-heart/core/block/import/common/workerpool"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
)

const workersNumber = 5

type FileDownloader struct {
	pool            *workerpool.WorkerPool
	tempDirProvider core.TempDirProvider

	urlToLocalPath map[string]string
	urlsInProgress map[string]struct{}
	sync.Mutex
}

func NewFileDownloader(tempDirProvider core.TempDirProvider) *FileDownloader {
	return &FileDownloader{
		pool:            workerpool.NewPool(workersNumber),
		tempDirProvider: tempDirProvider,
		urlToLocalPath:  make(map[string]string, 0),
		urlsInProgress:  make(map[string]struct{}, 0),
	}
}

func (d *FileDownloader) Init(ctx context.Context, token string) error {
	tokenHash := string(md5.New().Sum([]byte(token)))
	dirPath := filepath.Join(d.tempDirProvider.TempDir(), tokenHash)
	err := os.MkdirAll(dirPath, 0700)
	if err != nil && !os.IsExist(err) {
		return err
	}
	d.pool.Start(&DataObject{
		dirPath: dirPath,
		ctx:     ctx,
	})
	return nil
}

func (d *FileDownloader) AddToQueue(url string) {
	d.Lock()
	defer d.Unlock()
	if _, ok := d.urlToLocalPath[url]; ok {
		return
	}
	if _, ok := d.urlsInProgress[url]; ok {
		return
	}
	d.urlsInProgress[url] = struct{}{}
	d.pool.AddWork(NewFile(url))
}

func (d *FileDownloader) MapUrlToLocalPath() {
	for result := range d.pool.Results() {
		downloadResult := result.(*DownloadResult)
		if downloadResult != nil && downloadResult.Err == nil {
			d.Lock()
			d.urlToLocalPath[downloadResult.Url] = downloadResult.FilePath
			delete(d.urlsInProgress, downloadResult.Url)
			d.Unlock()
		}
	}
}

func (d *FileDownloader) WaitForLocalPath(url string) string {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			d.Lock()
			if localPath, ok := d.urlToLocalPath[url]; ok {
				d.Unlock()
				return localPath
			}
			d.Unlock()
		}
	}
}

type DataObject struct {
	dirPath string
	ctx     context.Context
}

type DownloadResult struct {
	FilePath string
	Url      string
	Err      error
}

type File struct {
	url string
}

func NewFile(url string) workerpool.ITask {
	return &File{url: url}
}

func (f *File) Execute(data interface{}) interface{} {
	do, ok := data.(*DataObject)
	if !ok {
		return fmt.Errorf("wrong format of data for file downloading")
	}
	fileName := string(md5.New().Sum([]byte(f.url)))
	fullPath := filepath.Join(do.dirPath, fileName)
	tmpFile, err := os.Create(fullPath)
	if err != nil {
		return &DownloadResult{Err: err}
	}
	defer func() {
		if err != nil {
			err := os.Remove(fullPath)
			if err != nil {
				// TODO log
			}
		}
	}()

	req, err := http.NewRequestWithContext(do.ctx, http.MethodGet, f.url, nil)
	if err != nil {
		return &DownloadResult{Err: fmt.Errorf("failed to make request with context: %w", err)}
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return &DownloadResult{Err: fmt.Errorf("failed to make http request: %w", err)}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return &DownloadResult{Err: fmt.Errorf("bad status code: %d", resp.StatusCode)}
	}
	counter := datacounter.NewReaderCounter(resp.Body)
	progressCh := make(chan struct{}, 1)
	timeout := time.Second * 30
	go func() {
		lastCount := counter.Count()
		for {
			select {
			case <-do.ctx.Done():
				return
			case <-time.After(timeout):
				currentCount := counter.Count()
				if currentCount == lastCount {
					progressCh <- struct{}{}
					return
				}
				lastCount = currentCount
			}
		}
	}()

	_, err = io.Copy(tmpFile, counter)
	if err != nil {
		return &DownloadResult{Err: fmt.Errorf("failed to download file: %w", err)}
	}
	select {
	case <-progressCh:
		return &DownloadResult{Err: fmt.Errorf("failed to download file, no progress")}
	default:
		return &DownloadResult{FilePath: fullPath, Url: f.url}
	}
}
