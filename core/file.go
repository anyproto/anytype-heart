package core

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"time"

	"github.com/anytypeio/go-anytype-library/util"
	"github.com/gogo/protobuf/types"
	"github.com/textileio/go-textile/core"
	"github.com/textileio/go-textile/mill"
	"github.com/textileio/go-textile/pb"
)

type File struct {
	index *pb.FileIndex
	node  *Anytype
}

type FileMeta struct {
	Media string
	Name  string
	Size  int64
	Added time.Time
}

func (file *File) Meta() *FileMeta {
	added, _ := types.TimestampFromProto(util.CastTimestampToGogo(file.index.Added))
	// ignore error

	return &FileMeta{
		Media: file.index.Media,
		Name:  file.index.Checksum,
		Size:  file.index.Size,
		Added: added,
	}
}

func (file *File) URL() string {
	return fmt.Sprintf("http://%s/ipfs/%s?key=%s&type=%s", file.node.Textile.Node().Config().Addresses.Gateway, file.index.Hash, url.QueryEscape(file.index.Key), url.QueryEscape(file.index.Media))
}

func (file *File) Reader() (io.ReadSeeker, error) {
	return file.node.Textile.Node().FileIndexContent(file.index)
}

func (a *Anytype) FileByHash(hash string) (*File, error) {
	fileIndex, err := a.Textile.Node().FileMeta(hash)
	if err != nil {
		return nil, err
	}

	return &File{
		index: fileIndex,
		node:  a,
	}, nil
}

func (a *Anytype) FileAddWithBytes(content []byte, media string, name string) (*File, error) {
	fileIndex, err := a.Textile.Node().AddFileIndex(&mill.Blob{}, core.AddFileConfig{
		Input: content,
		Media: media,
		Name:  name,
	})
	if err != nil {
		return nil, err
	}

	return &File{
		index: fileIndex,
		node:  a,
	}, nil
}

func (a *Anytype) FileAddWithReader(content io.ReadCloser, media string, name string) (*File, error) {
	// todo: PR textile to be able to use reader instead of bytes
	defer content.Close()
	contentBytes, err := ioutil.ReadAll(content)
	if err != nil {
		return nil, err
	}

	return a.FileAddWithBytes(contentBytes, media, name)
}
