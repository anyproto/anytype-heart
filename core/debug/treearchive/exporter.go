package treearchive

import (
	"archive/zip"
	"encoding/json"
	"github.com/anytypeio/any-sync/commonspace/object/acl/aclrecordproto"
	"github.com/anytypeio/any-sync/commonspace/object/acl/liststorage"
	"github.com/anytypeio/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/anytypeio/any-sync/commonspace/object/tree/treestorage"
	"github.com/anytypeio/go-anytype-middleware/core/debug/treearchive/zipaclstorage"
	"github.com/anytypeio/go-anytype-middleware/core/debug/treearchive/ziptreestorage"
	"github.com/hashicorp/go-multierror"
	"io/fs"
	"os"
)

type ExportedObjectsJson struct {
	AclId  string `json:"aclId"`
	TreeId string `json:"treeId"`
}

type Exporter struct {
	zw     *zip.Writer
	zf     fs.File
	treeId string
	aclId  string
}

func NewExporter(path string) (*Exporter, error) {
	zf, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	zw := zip.NewWriter(zf)
	return &Exporter{
		zw: zw,
		zf: zf,
	}, nil
}

func (e *Exporter) Writer() *zip.Writer {
	return e.zw
}

func (e *Exporter) TreeStorage(root *treechangeproto.RawTreeChangeWithId) (treestorage.TreeStorage, error) {
	e.treeId = root.Id
	return ziptreestorage.NewZipTreeWriteStorage(root, e.zw)
}

func (e *Exporter) ListStorage(root *aclrecordproto.RawAclRecordWithId) (liststorage.ListStorage, error) {
	e.aclId = root.Id
	return zipaclstorage.NewAclWriteStorage(root, e.zw)
}

func (e *Exporter) Close() error {
	exportedHeader, err := e.zw.CreateHeader(&zip.FileHeader{
		Name:   "exported.json",
		Method: zip.Deflate,
	})
	enc := json.NewEncoder(exportedHeader)
	enc.SetIndent("", "\t")
	err = enc.Encode(ExportedObjectsJson{
		TreeId: e.treeId,
		AclId:  e.aclId,
	})

	var mErr multierror.Error
	if err != nil {
		mErr.Errors = append(mErr.Errors, err)
	}
	err = e.zw.Close()
	if err != nil {
		mErr.Errors = append(mErr.Errors, err)
	}
	err = e.zf.Close()
	if err != nil {
		mErr.Errors = append(mErr.Errors, err)
	}
	return &mErr
}
