package ziptreestorage

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"github.com/anytypeio/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/anytypeio/any-sync/commonspace/object/tree/treestorage"
	"io"
	"strings"
)

type zipTreeReadStorage struct {
	id    string
	heads []string
	files map[string]*zip.File
	zr    *zip.ReadCloser
}

func NewZipTreeReadStorage(id string, zr *zip.ReadCloser) (st treestorage.TreeStorage, err error) {
	zrs := &zipTreeReadStorage{
		id:    id,
		heads: nil,
		files: map[string]*zip.File{},
		zr:    zr,
	}
	for _, f := range zr.Reader.File {
		if len(f.Name) > len(id) && strings.Contains(f.Name, id) {
			split := strings.Split(f.Name, "/")
			last := split[len(split)-1]
			zrs.files[last] = f
		}
	}
	data, ok := zrs.files["data.json"]
	if !ok {
		err = fmt.Errorf("no data.json in archive")
		return
	}
	dataOpened, err := data.Open()
	if err != nil {
		return
	}
	defer dataOpened.Close()
	headsEntry := &HeadsJsonEntry{}
	if err = json.NewDecoder(dataOpened).Decode(headsEntry); err != nil {
		return
	}
	zrs.heads = headsEntry.Heads
	st = zrs
	return
}

func (z *zipTreeReadStorage) Id() string {
	return z.id
}

func (z *zipTreeReadStorage) Root() (root *treechangeproto.RawTreeChangeWithId, err error) {
	return z.readChange(z.id)
}

func (z *zipTreeReadStorage) Heads() ([]string, error) {
	return z.heads, nil
}

func (z *zipTreeReadStorage) SetHeads(heads []string) (err error) {
	panic("should not be called")
}

func (z *zipTreeReadStorage) AddRawChange(change *treechangeproto.RawTreeChangeWithId) (err error) {
	panic("should not be called")
}

func (z *zipTreeReadStorage) AddRawChangesSetHeads(changes []*treechangeproto.RawTreeChangeWithId, heads []string) (err error) {
	panic("should not be called")
}

func (z *zipTreeReadStorage) GetRawChange(ctx context.Context, id string) (*treechangeproto.RawTreeChangeWithId, error) {
	return z.readChange(id)
}

func (z *zipTreeReadStorage) HasChange(ctx context.Context, id string) (ok bool, err error) {
	_, ok = z.files[id]
	return
}

func (z *zipTreeReadStorage) Delete() error {
	panic("should not be called")
}

func (z *zipTreeReadStorage) readChange(id string) (change *treechangeproto.RawTreeChangeWithId, err error) {
	file, ok := z.files[id]
	if !ok {
		err = fmt.Errorf("object not found in storage")
		return
	}
	opened, err := file.Open()
	if err != nil {
		return
	}
	defer opened.Close()

	buf, err := io.ReadAll(opened)
	if err != nil {
		return
	}
	change = &treechangeproto.RawTreeChangeWithId{RawChange: buf, Id: id}
	return
}
