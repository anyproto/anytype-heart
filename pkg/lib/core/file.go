package core

import (
	"context"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/files"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/storage"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/gogo/protobuf/types"
)

type File interface {
	Meta() *FileMeta
	Hash() string
	Reader() (io.ReadSeeker, error)
	Details() (*types.Struct, error)
	Info() *storage.FileInfo
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

func (i *file) Details() (*types.Struct, error) {
	meta := i.Meta()
	return &types.Struct{
		Fields: map[string]*types.Value{
			"id":           pbtypes.String(i.hash),
			"type":         pbtypes.StringList([]string{bundle.TypeKeyFile.URL()}),
			"fileMimeType": pbtypes.String(meta.Media),
			"name":         pbtypes.String(strings.TrimSuffix(meta.Name, filepath.Ext(meta.Name))),
			"sizeInBytes":  pbtypes.Float64(float64(meta.Size)),
			"addedDate":    pbtypes.Float64(float64(meta.Added.Unix())),
		},
	}, nil
}

func (i *file) Info() *storage.FileInfo {
	return i.info
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
