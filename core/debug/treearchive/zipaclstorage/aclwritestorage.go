package zipaclstorage

import (
	"archive/zip"
	"context"

	"github.com/anyproto/any-sync/commonspace/object/acl/liststorage"
	"github.com/anyproto/any-sync/consensus/consensusproto"

	"strings"
)

type zipAclWriteStorage struct {
	id   string
	head string
	zw   *zip.Writer
}

func NewAclWriteStorage(root *consensusproto.RawRecordWithId, zw *zip.Writer) (ls liststorage.ListStorage, err error) {
	ls = &zipAclWriteStorage{
		id:   root.Id,
		head: root.Id,
		zw:   zw,
	}
	err = ls.AddRawRecord(context.Background(), root)
	return
}

func (z *zipAclWriteStorage) Id() string {
	return z.id
}

func (z *zipAclWriteStorage) Root() (*consensusproto.RawRecordWithId, error) {
	panic("should not be called")
}

func (z *zipAclWriteStorage) Head() (string, error) {
	return z.id, nil
}

func (z *zipAclWriteStorage) SetHead(headId string) error {
	// TODO: As soon as our acls are writeable, this should be implemented
	panic("should not be called")
}

func (z *zipAclWriteStorage) GetRawRecord(ctx context.Context, id string) (*consensusproto.RawRecordWithId, error) {
	panic("should not be called")
}

func (z *zipAclWriteStorage) AddRawRecord(ctx context.Context, rec *consensusproto.RawRecordWithId) (err error) {
	wr, err := z.zw.Create(strings.Join([]string{z.id, rec.Id}, "/"))
	if err != nil {
		return
	}
	_, err = wr.Write(rec.Payload)
	return
}

func (z *zipAclWriteStorage) FlushStorage() error {
	return nil
}
