package zipaclstorage

import (
	"archive/zip"
	"context"
	"strings"

	"github.com/anyproto/any-sync/commonspace/object/acl/liststorage"
	"github.com/anyproto/any-sync/consensus/consensusproto"
)

type zipACLWriteStorage struct {
	id string
	zw *zip.Writer
}

func NewACLWriteStorage(root *consensusproto.RawRecordWithId, zw *zip.Writer) (ls liststorage.ListStorage, err error) {
	ls = &zipACLWriteStorage{
		id: root.Id,
		zw: zw,
	}
	err = ls.AddRawRecord(context.Background(), root)
	return
}

// nolint:revive
func (z *zipACLWriteStorage) Id() string {
	return z.id
}

func (z *zipACLWriteStorage) Root() (*consensusproto.RawRecordWithId, error) {
	panic("should not be called")
}

func (z *zipACLWriteStorage) Head() (string, error) {
	return z.id, nil
}

func (z *zipACLWriteStorage) SetHead(_ string) error {
	// TODO: As soon as our acls are writeable, this should be implemented
	panic("should not be called")
}

func (z *zipACLWriteStorage) GetRawRecord(_ context.Context, _ string) (*consensusproto.RawRecordWithId, error) {
	panic("should not be called")
}

func (z *zipACLWriteStorage) AddRawRecord(_ context.Context, rec *consensusproto.RawRecordWithId) (err error) {
	wr, err := z.zw.Create(strings.Join([]string{z.id, rec.Id}, "/"))
	if err != nil {
		return
	}
	_, err = wr.Write(rec.Payload)
	return
}

func (z *zipACLWriteStorage) FlushStorage() error {
	return nil
}
