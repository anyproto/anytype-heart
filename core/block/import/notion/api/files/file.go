package files

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/miolini/datacounter"
	"github.com/zeebo/blake3"

	"github.com/anyproto/anytype-heart/core/block/import/common/workerpool"
)

var hashersPool = &sync.Pool{
	New: func() any {
		return blake3.New()
	},
}

type DataObject struct {
	dirPath string
	ctx     context.Context
}

type LocalFileProvider interface {
	workerpool.ITask

	GetUrl() string
	GetLocalPath() string
	WaitForLocalPath() (string, error)
	LoadDone(string)
}

type file struct {
	url       string
	localPath string

	loadDone chan struct{}
	errCh    chan error
}

func NewFile(url string) LocalFileProvider {
	return &file{
		url:      url,
		loadDone: make(chan struct{}),
		errCh:    make(chan error),
	}
}

func (f *file) LoadDone(localPath string) {
	f.localPath = localPath
	close(f.loadDone)
}

func (f *file) GetUrl() string {
	return f.url
}

func (f *file) GetLocalPath() string {
	return f.localPath
}

func (f *file) WaitForLocalPath() (string, error) {
	for {
		select {
		case <-f.loadDone:
			return f.localPath, nil
		case err := <-f.errCh:
			return "", err
		}
	}
}

func (f *file) Execute(data interface{}) interface{} {
	do, ok := data.(*DataObject)
	if !ok {
		return fmt.Errorf("wrong format of data for file downloading")
	}

	fullPath, tmpFile, err := f.generateFileName(do)

	defer tmpFile.Close()
	if err != nil {
		if os.IsExist(err) {
			f.LoadDone(fullPath)
			return f
		}
		f.loadFinishWithError(err)
		return f
	}

	if err = f.downloadFile(do.ctx, tmpFile, fullPath); err != nil {
		f.loadFinishWithError(err)
		return f
	}

	f.LoadDone(fullPath)
	return f
}

func (f *file) downloadFile(ctx context.Context, tmpFile *os.File, fullPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.url, nil)
	if err != nil {
		return fmt.Errorf("failed to make request with context: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make http request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status code: %d", resp.StatusCode)
	}
	counter := datacounter.NewReaderCounter(resp.Body)
	progressCh := make(chan struct{}, 1)
	done := make(chan struct{})
	defer close(done)
	go f.monitorFileDownload(ctx, counter, done, progressCh)

	_, err = io.Copy(tmpFile, counter)
	if err != nil {
		os.Remove(fullPath)
		return fmt.Errorf("failed to download file: %w", err)
	}
	select {
	case <-ctx.Done():
		return fmt.Errorf("file download were canceled")
	case <-progressCh:
		return fmt.Errorf("failed to download file, no progress")
	default:
		return nil
	}
}

func (f *file) loadFinishWithError(err error) {
	f.errCh <- err
	close(f.errCh)
}

func (f *file) generateFileName(do *DataObject) (string, *os.File, error) {
	hasher := hashersPool.Get().(*blake3.Hasher)
	defer hashersPool.Put(hasher)

	hasher.Reset()
	// nolint: errcheck
	hasher.Write([]byte(f.url))
	parsesUrl, err := url.Parse(f.url)
	if err != nil {
		return "", nil, err
	}
	fileExt := filepath.Ext(parsesUrl.Path)
	fileName := hex.EncodeToString(hasher.Sum(nil)) + fileExt
	fullPath := filepath.Join(do.dirPath, fileName)
	file, err := os.Open(fullPath)
	if err != nil && !os.IsNotExist(err) {
		return "", nil, err
	}
	if file != nil {
		return fullPath, file, os.ErrExist
	}
	tmpFile, err := os.Create(fullPath)
	if err != nil {
		return "", nil, err
	}
	return fullPath, tmpFile, nil
}

func (f *file) monitorFileDownload(
	ctx context.Context,
	counter *datacounter.ReaderCounter,
	done, progressCh chan struct{},
) {
	timeout := time.Second * 30
	func() {
		lastCount := counter.Count()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
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
}
