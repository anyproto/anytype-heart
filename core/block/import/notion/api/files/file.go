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
	LoadDone()
}

type file struct {
	*os.File

	url       string
	localPath string

	loadDone chan struct{}
	errCh    chan error
}

func NewFile(url string) LocalFileProvider {
	return &file{
		url:      url,
		loadDone: make(chan struct{}),
		errCh:    make(chan error, 1),
	}
}

func (f *file) LoadDone() {
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
		f.loadFinishWithError(fmt.Errorf("wrong format of data for file downloading"))
		return f
	}

	err := f.generateFile(do)

	defer func() {
		f.Close()
		if err != nil && !os.IsExist(err) && f.File != nil {
			os.Remove(f.Name())
		}
	}()

	if err != nil {
		if os.IsExist(err) {
			f.LoadDone()
			return f
		}
		f.loadFinishWithError(err)
		return f
	}

	if err = f.downloadFile(do.ctx); err != nil {
		f.loadFinishWithError(err)
		return f
	}

	f.LoadDone()
	return f
}

func (f *file) downloadFile(ctx context.Context) error {
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
	go f.monitorFileDownload(counter, done, progressCh)

	_, err = io.Copy(f.File, counter)
	if err != nil {
		return fmt.Errorf("failed to download file: %w", err)
	}
	select {
	case <-ctx.Done():
		return fmt.Errorf("file download were canceled")
	case <-progressCh:
		return fmt.Errorf("failed to download file, no progress")
	default:
		f.Close()
		return os.Rename(f.File.Name(), f.localPath)
	}
}

func (f *file) loadFinishWithError(err error) {
	f.errCh <- err
	close(f.errCh)
}

func (f *file) generateFile(do *DataObject) error {
	fileName, err := f.makeFileName()
	if err != nil {
		return err
	}
	fullPath := filepath.Join(do.dirPath, fileName)
	file, err := os.OpenFile(fullPath, os.O_RDWR, 0600)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if file != nil {
		f.File = file
		f.localPath = fullPath
		return os.ErrExist
	}
	tempFilePath := filepath.Join(do.dirPath, "_"+fileName)
	tmpFile, err := os.Create(tempFilePath)
	if err != nil {
		return err
	}
	f.File = tmpFile
	f.localPath = fullPath
	return nil
}

func (f *file) makeFileName() (string, error) {
	hasher := hashersPool.Get().(*blake3.Hasher)
	defer hashersPool.Put(hasher)
	hasher.Reset()
	parsesUrl, err := url.Parse(f.url)
	if err != nil {
		return "", err
	}
	// nolint: errcheck
	hasher.Write([]byte(parsesUrl.Path))
	fileExt := filepath.Ext(parsesUrl.Path)
	fileName := hex.EncodeToString(hasher.Sum(nil)) + fileExt
	return fileName, nil
}

func (f *file) monitorFileDownload(counter *datacounter.ReaderCounter, done, progressCh chan struct{}) {
	timeout := time.Second * 30
	func() {
		lastCount := counter.Count()
		for {
			select {
			case <-done:
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
