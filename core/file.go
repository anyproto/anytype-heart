package core

import (
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/anytypeio/go-anytype-library/pb/lsmodel"
)

var filesKeysCache = make(map[string]map[string]string)
var filesKeysCacheMutex = sync.RWMutex{}

type File interface {
	Meta() *FileMeta
	Hash() string
	Reader() (io.ReadSeeker, error)
}

type file struct {
	hash  string
	index *lsmodel.FileIndex
	node  *Anytype
}

type FileMeta struct {
	Media string
	Name  string
	Size  int64
	Added time.Time
}

func (file *file) Meta() *FileMeta {
	return &FileMeta{
		Media: file.index.Media,
		Name:  file.index.Name,
		Size:  file.index.Size_,
		Added: time.Unix(file.index.Added, 0),
	}
}

func (file *file) Hash() string {
	return file.hash
}

func (file *file) Reader() (io.ReadSeeker, error) {
	return nil, fmt.Errorf("not implemented")
}
