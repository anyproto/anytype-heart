package export

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sync"
	"time"

	"github.com/anyproto/anytype-heart/util/anyerror"
)

type writer interface {
	Path() string
	Namer() *namer
	WriteFile(filename string, r io.Reader, lastModifiedDate int64) (err error)
	Close() (err error)
}

func uniqName() string {
	return time.Now().Format("Anytype.20060102.150405.99")
}

func newDirWriter(path string, includeFiles bool) (writer, error) {
	path = filepath.Join(path, uniqName())
	fullPath := path
	if includeFiles {
		fullPath = filepath.Join(path, "files")
	}
	if err := os.MkdirAll(fullPath, 0777); err != nil {
		return nil, err
	}
	return &dirWriter{
		path: path,
	}, nil
}

type dirWriter struct {
	path string
	fn   *namer
	m    sync.Mutex
}

func (d *dirWriter) Namer() *namer {
	d.m.Lock()
	defer d.m.Unlock()
	if d.fn == nil {
		d.fn = newNamer()
	}
	return d.fn
}

func (d *dirWriter) Path() string {
	return d.path
}

func (d *dirWriter) WriteFile(filename string, r io.Reader, lastModifiedDate int64) (err error) {
	dir := filepath.Dir(filename)
	err = os.MkdirAll(filepath.Join(d.path, dir), 0700)
	if err != nil {
		return err
	}
	filename = path.Join(d.path, filename)
	f, err := os.Create(filename)
	if err != nil {
		return
	}
	defer f.Close()
	if _, err = io.Copy(f, r); err != nil {
		return
	}
	if lastModifiedDate == 0 {
		lastModifiedDate = time.Now().Unix()
	}
	lastModifiedDateUnix := time.Unix(lastModifiedDate, 0)
	err = os.Chtimes(filename, time.Now(), lastModifiedDateUnix)
	if err != nil {
		return fmt.Errorf("failed to set date modified of export file: %w", anyerror.CleanupError(err))
	}
	return
}

func (d *dirWriter) Close() (err error) {
	return nil
}

func newZipWriter(path, name string) (writer, error) {
	fileName := filepath.Join(path, name)
	f, err := os.Create(fileName)
	if err != nil {
		return nil, err
	}
	return &zipWriter{
		path: fileName,
		zw:   zip.NewWriter(f),
		f:    f,
	}, nil
}

type zipWriter struct {
	path string
	zw   *zip.Writer
	f    io.Closer
	m    sync.Mutex
	fn   *namer
}

func (d *zipWriter) Namer() *namer {
	d.m.Lock()
	defer d.m.Unlock()
	if d.fn == nil {
		d.fn = newNamer()
	}
	return d.fn
}

func (d *zipWriter) Path() string {
	return d.path
}

func (d *zipWriter) WriteFile(filename string, r io.Reader, lastModifiedDate int64) (err error) {
	d.m.Lock()
	defer d.m.Unlock()
	if lastModifiedDate == 0 {
		lastModifiedDate = time.Now().Unix()
	}
	zf, err := d.zw.CreateHeader(&zip.FileHeader{
		Name:     filename,
		Method:   zip.Deflate,
		Modified: time.Unix(lastModifiedDate, 0),
	})
	if err != nil {
		return
	}
	_, err = io.Copy(zf, r)
	return
}

func (d *zipWriter) Close() (err error) {
	if err = d.zw.Close(); err != nil {
		return
	}
	return d.f.Close()
}

func getZipName(path string) string {
	return filepath.Join(path, uniqName()+".zip")
}

type FakeWriter struct {
	data map[string][]byte
	fn   *namer
	m    sync.Mutex
}

func (d *FakeWriter) Namer() *namer {
	d.m.Lock()
	defer d.m.Unlock()
	if d.fn == nil {
		d.fn = newNamer()
	}
	return d.fn
}

func (d *FakeWriter) Path() string {
	return ""
}

func (d *FakeWriter) WriteFile(filename string, r io.Reader, lastModifiedDate int64) (err error) {
	if d.data == nil {
		d.data = make(map[string][]byte)
	}
	b, err := io.ReadAll(r)
	if err != nil {
		return
	}
	d.data[filename] = b
	return
}

func (d *FakeWriter) Close() (err error) {
	return nil
}

func (d *FakeWriter) GetData(id string) []byte {
	return d.data[id]
}
