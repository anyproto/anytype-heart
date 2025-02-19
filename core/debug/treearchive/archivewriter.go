package treearchive

import (
	"archive/zip"
	"encoding/json"
	"io/fs"
	"os"

	"github.com/anyproto/any-sync/commonspace/object/acl/liststorage"
	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/anyproto/any-sync/consensus/consensusproto"

	"github.com/anyproto/anytype-heart/core/debug/treearchive/zipaclstorage"
	"github.com/anyproto/anytype-heart/core/debug/treearchive/ziptreestorage"
)

type ExportedObjectsJson struct {
	AclId  string `json:"aclId"`
	TreeId string `json:"treeId"`
}

type ArchiveWriter struct {
	zw       *zip.Writer
	zf       fs.File
	treeId   string
	aclId    string
	storages []flushableStorage
}

type flushableStorage interface {
	FlushStorage() error
}

func NewArchiveWriter(path string) (*ArchiveWriter, error) {
	zf, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	zw := zip.NewWriter(zf)
	return &ArchiveWriter{
		zw: zw,
		zf: zf,
	}, nil
}

func (e *ArchiveWriter) ZipWriter() *zip.Writer {
	return e.zw
}

func (e *ArchiveWriter) TreeStorage(root *treechangeproto.RawTreeChangeWithId) (treestorage.TreeStorage, error) {
	e.treeId = root.Id
	st, err := ziptreestorage.NewZipTreeWriteStorage(root, e.zw)
	if err != nil {
		return nil, err
	}
	e.storages = append(e.storages, st.(flushableStorage))
	return st, nil
}

func (e *ArchiveWriter) ListStorage(root *consensusproto.RawRecordWithId) (liststorage.ListStorage, error) {
	e.aclId = root.Id
	st, err := zipaclstorage.NewACLWriteStorage(root, e.zw)
	if err != nil {
		return nil, err
	}
	e.storages = append(e.storages, st.(flushableStorage))
	return st, nil
}

func (e *ArchiveWriter) Close() (err error) {
	for _, st := range e.storages {
		err = st.FlushStorage()
		if err != nil {
			return
		}
	}
	exportedHeader, err := e.zw.CreateHeader(&zip.FileHeader{
		Name:   "exported.json",
		Method: zip.Deflate,
	})
	if err != nil {
		return
	}
	enc := json.NewEncoder(exportedHeader)
	enc.SetIndent("", "\t")
	err = enc.Encode(ExportedObjectsJson{
		TreeId: e.treeId,
		AclId:  e.aclId,
	})
	if err != nil {
		return
	}
	err = e.zw.Close()
	if err != nil {
		return
	}
	return e.zf.Close()
}
