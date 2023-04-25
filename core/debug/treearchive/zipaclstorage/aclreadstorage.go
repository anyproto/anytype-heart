package zipaclstorage

import (
	"archive/zip"
	"context"
	"fmt"
	"github.com/anytypeio/any-sync/commonspace/object/acl/aclrecordproto"
	"github.com/anytypeio/any-sync/commonspace/object/acl/liststorage"
	"io"
	"strings"
)

type zipAclReadStorage struct {
	id    string
	head  string
	files map[string]*zip.File
	zr    *zip.ReadCloser
}

func NewZipAclReadStorage(id string, zr *zip.ReadCloser) (ls liststorage.ListStorage, err error) {
	aclStorage := &zipAclReadStorage{
		id:    id,
		head:  id,
		files: map[string]*zip.File{},
		zr:    zr,
	}
	for _, f := range zr.Reader.File {
		if len(f.Name) > len(id) && strings.Contains(f.Name, id) {
			split := strings.SplitAfter(id, "/")
			last := split[len(split)-1]
			aclStorage.files[last] = f
		}
	}
	ls = aclStorage
	return
}

func (z *zipAclReadStorage) Id() string {
	return z.id
}

func (z *zipAclReadStorage) Root() (*aclrecordproto.RawAclRecordWithId, error) {
	return z.readRecord(z.id)
}

func (z *zipAclReadStorage) Head() (string, error) {
	return z.id, nil
}

func (z *zipAclReadStorage) SetHead(headId string) error {
	panic("should not be called")
}

func (z *zipAclReadStorage) GetRawRecord(ctx context.Context, id string) (*aclrecordproto.RawAclRecordWithId, error) {
	return z.readRecord(id)
}

func (z *zipAclReadStorage) AddRawRecord(ctx context.Context, rec *aclrecordproto.RawAclRecordWithId) (err error) {
	panic("should not be called")
}

func (z *zipAclReadStorage) readRecord(id string) (rec *aclrecordproto.RawAclRecordWithId, err error) {
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
	rec = &aclrecordproto.RawAclRecordWithId{Payload: buf, Id: id}
	return
}
