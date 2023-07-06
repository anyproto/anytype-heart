package zipaclstorage

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/anyproto/any-sync/commonspace/object/acl/liststorage"
	"github.com/anyproto/any-sync/consensus/consensusproto"
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

func (z *zipAclReadStorage) Root() (*consensusproto.RawRecordWithId, error) {
	return z.readRecord(z.id)
}

func (z *zipAclReadStorage) Head() (string, error) {
	return z.id, nil
}

func (z *zipAclReadStorage) SetHead(headId string) error {
	panic("should not be called")
}

func (z *zipAclReadStorage) GetRawRecord(_ context.Context, id string) (*consensusproto.RawRecordWithId, error) {
	return z.readRecord(id)
}

func (z *zipAclReadStorage) AddRawRecord(_ context.Context, _ *consensusproto.RawRecordWithId) (err error) {
	panic("should not be called")
}

func (z *zipAclReadStorage) readRecord(id string) (rec *consensusproto.RawRecordWithId, err error) {
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
	rec = &consensusproto.RawRecordWithId{Payload: buf, Id: id}
	return
}
