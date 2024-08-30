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

const workersNumber = 4

type fileDownloader struct {
	pool            *workerpool.WorkerPool
	tempDirProvider core.TempDirProvider

	fileToHash map[string]string
	sync.Mutex
}

func newFileDownloader(tempDirProvider core.TempDirProvider) *fileDownloader {
	return &fileDownloader{
		pool:            workerpool.NewPool(workersNumber),
		tempDirProvider: tempDirProvider,
		fileToHash:      make(map[string]string, 0),
	}
}

func (d *fileDownloader) Init(ctx context.Context, token string) error {
	tokenHash := string(md5.New().Sum([]byte(token)))
	dirPath := filepath.Join(d.tempDirProvider.TempDir(), tokenHash)
	err := os.MkdirAll(dirPath, 0700)
	if err != nil {
		if os.IsExist(err) {
			return nil
		}
		return err
	}
	d.pool.Start(&DataObject{
		dirPath: dirPath,
		ctx:     ctx,
	})
	return nil
}

func (d *fileDownloader) AddToQueue(url string) {
	d.pool.AddWork(NewFile(url))
}

func (d *fileDownloader) ReadResult(url string) {
	for result := range d.pool.Results() {
		res := result.(*Result)
		if res != nil {
			d.Lock()
			d.fileToHash[res.Url] = res.FilePath
			d.Unlock()
		}
	}
}

type DataObject struct {
	dirPath string
	ctx     context.Context
}

type Result struct {
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
		if os.IsExist(err) {
			return &Result{FilePath: fullPath, Url: f.url}
		}
		return &Result{Err: err}
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
		return &Result{Err: fmt.Errorf("failed to make request with context: %w", err)}
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return &Result{Err: fmt.Errorf("failed to make http request: %w", err)}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return &Result{Err: fmt.Errorf("bad status code: %d", resp.StatusCode)}
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
		return &Result{Err: fmt.Errorf("failed to download file: %w", err)}
	}

	select {
	case <-progressCh:
		return &Result{Err: fmt.Errorf("failed to download file, no progress")}
	default:
		return &Result{FilePath: fullPath, Url: f.url}
	}
}
