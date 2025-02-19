package ziptreestorage

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
)

type HeadsJsonEntry struct {
	Heads  []string `json:"heads"`
	RootId string   `json:"rootId"`
}

type zipTreeWriteStorage struct {
	id    string
	heads []string
	zw    *zip.Writer
}

func NewZipTreeWriteStorage(root *treechangeproto.RawTreeChangeWithId, zw *zip.Writer) (st treestorage.TreeStorage, err error) {
	z := &zipTreeWriteStorage{
		id: root.Id,
		zw: zw,
	}
	err = z.SetHeads([]string{root.Id})
	if err != nil {
		return
	}
	err = z.AddRawChange(root)
	if err != nil {
		return
	}
	st = z
	return
}

func (z *zipTreeWriteStorage) Id() string {
	return z.id
}

func (t *zipTreeWriteStorage) GetAllChangeIds() (chs []string, err error) {
	return nil, fmt.Errorf("get all change ids should not be called")
}

func (z *zipTreeWriteStorage) Root() (*treechangeproto.RawTreeChangeWithId, error) {
	panic("should not be implemented")
}

func (z *zipTreeWriteStorage) Heads() ([]string, error) {
	return z.heads, nil
}

func (z *zipTreeWriteStorage) SetHeads(heads []string) (err error) {
	z.heads = heads
	return
}

func (z *zipTreeWriteStorage) AddRawChange(change *treechangeproto.RawTreeChangeWithId) (err error) {
	wr, err := z.zw.Create(strings.Join([]string{z.id, change.Id}, "/"))
	if err != nil {
		return
	}
	_, err = wr.Write(change.RawChange)
	return
}

func (z *zipTreeWriteStorage) AddRawChangesSetHeads(changes []*treechangeproto.RawTreeChangeWithId, heads []string) (err error) {
	for _, ch := range changes {
		err = z.AddRawChange(ch)
		if err != nil {
			return
		}
	}
	return z.SetHeads(heads)
}

func (z *zipTreeWriteStorage) GetRawChange(ctx context.Context, id string) (*treechangeproto.RawTreeChangeWithId, error) {
	panic("should not be called")
}

func (z *zipTreeWriteStorage) GetAppendRawChange(ctx context.Context, buf []byte, id string) (*treechangeproto.RawTreeChangeWithId, error) {
	panic("should not be called")
}

func (z *zipTreeWriteStorage) HasChange(ctx context.Context, id string) (ok bool, err error) {
	panic("should not be called")
}

func (z *zipTreeWriteStorage) Delete() error {
	panic("should not be called")
}

func (z *zipTreeWriteStorage) FlushStorage() (err error) {
	chw, err := z.zw.CreateHeader(&zip.FileHeader{
		Name:   strings.Join([]string{z.id, "data.json"}, "/"),
		Method: zip.Deflate,
	})
	enc := json.NewEncoder(chw)
	enc.SetIndent("", "\t")
	err = enc.Encode(HeadsJsonEntry{
		Heads:  z.heads,
		RootId: z.id,
	})
	return
}
