package core

import (
	"context"
	"io"
	"time"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/files"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/storage"
)

type File interface {
	Meta() *FileMeta
	Hash() string
	Reader() (io.ReadSeeker, error)
}

type file struct {
	hash string
	info *storage.FileInfo
	node *files.Service
}

type FileMeta struct {
	Media string
	Name  string
	Size  int64
	Added time.Time
}

type FileKeys struct {
	Hash string
	Keys map[string]string
}

func (file *file) Meta() *FileMeta {
	return &FileMeta{
		Media: file.info.Media,
		Name:  file.info.Name,
		Size:  file.info.Size_,
		Added: time.Unix(file.info.Added, 0),
	}
}

func (file *file) Hash() string {
	return file.hash
}

func (file *file) Reader() (io.ReadSeeker, error) {
	return file.node.FileContentReader(context.Background(), file.info)
}
