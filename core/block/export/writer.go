package export

import (
	"archive/zip"
	"io"
	"os"
	"path"
	"path/filepath"
	"sync"
	"time"
)

type writer interface {
	Path() string
	Namer() *fileNamer
	WriteFile(filename string, r io.Reader) (err error)
	Close() (err error)
}

func uniqName() string {
	return time.Now().Format("Anytype.20060102.150405.99")
}

func newDirWriter(path string) (writer, error) {
	path = filepath.Join(path, uniqName())
	if err := os.MkdirAll(filepath.Join(path, "files"), 0777); err != nil {
		return nil, err
	}
	return &dirWriter{
		path: path,
	}, nil
}

type dirWriter struct {
	path string
	fn   *fileNamer
	m    sync.Mutex
}

func (d *dirWriter) Namer() *fileNamer {
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

func (d *dirWriter) WriteFile(filename string, r io.Reader) (err error) {
	filename = path.Join(d.path, filename)
	f, err := os.Create(filename)
	if err != nil {
		return
	}
	defer f.Close()
	if _, err = io.Copy(f, r); err != nil {
		return
	}
	return
}

func (d *dirWriter) Close() (err error) {
	return nil
}

func newZipWriter(path string) (writer, error) {
	filename := filepath.Join(path, uniqName()+".zip")
	f, err := os.Create(filename)
	if err != nil {
		return nil, err
	}
	return &zipWriter{
		path: filename,
		zw:   zip.NewWriter(f),
		f:    f,
	}, nil
}

type zipWriter struct {
	path string
	zw   *zip.Writer
	f    io.Closer
	m    sync.Mutex
	fn   *fileNamer
}

func (d *zipWriter) Namer() *fileNamer {
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

func (d *zipWriter) WriteFile(filename string, r io.Reader) (err error) {
	d.m.Lock()
	defer d.m.Unlock()
	zf, err := d.zw.Create(filename)
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
