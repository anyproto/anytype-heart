package core

import (
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/anytypeio/go-anytype-library/util"
	"github.com/gogo/protobuf/types"
	tpb "github.com/textileio/go-textile/pb"
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
	index *tpb.FileIndex
	node  *Anytype
}

type FileMeta struct {
	Media string
	Name  string
	Size  int64
	Added time.Time
}

func (file *file) Meta() *FileMeta {
	added, _ := types.TimestampFromProto(util.CastTimestampToGogo(file.index.Added))
	// ignore error

	return &FileMeta{
		Media: file.index.Media,
		Name:  file.index.Name,
		Size:  file.index.Size,
		Added: added,
	}
}

func (file *file) Hash() string {
	return file.hash
}

func (file *file) Reader() (io.ReadSeeker, error) {
	return nil, fmt.Errorf("not implemented")
}
