package treearchive

import (
	"archive/zip"
	"encoding/json"
	"fmt"

	"github.com/anyproto/any-sync/commonspace/object/acl/liststorage"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/gogo/protobuf/jsonpb"

	"github.com/anyproto/anytype-heart/core/debug/treearchive/zipaclstorage"
	"github.com/anyproto/anytype-heart/core/debug/treearchive/ziptreestorage"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type TreeArchive interface {
	ListStorage() liststorage.ListStorage
	TreeStorage() treestorage.TreeStorage
	LocalStore() (*model.ObjectInfo, error)
	Close() error
}

type treeArchive struct {
	listStorage liststorage.ListStorage
	treeStorage treestorage.TreeStorage
	zr          *zip.ReadCloser
}

// Open expects debug tree zip file
// returns TreeArchive that has ListStorage and TreeStorage
func Open(filename string) (tr TreeArchive, err error) {
	zr, err := zip.OpenReader(filename)
	if err != nil {
		return nil, err
	}
	exported, err := zr.Open("exported.json")
	if err != nil {
		return
	}
	defer exported.Close()
	expJson := &ExportedObjectsJson{}
	if err = json.NewDecoder(exported).Decode(expJson); err != nil {
		return
	}

	listStorage, err := zipaclstorage.NewZipAclReadStorage(expJson.AclId, zr)
	if err != nil {
		return
	}
	treeStorage, err := ziptreestorage.NewZipTreeReadStorage(expJson.TreeId, zr)
	if err != nil {
		return
	}

	return &treeArchive{
		listStorage: listStorage,
		treeStorage: treeStorage,
		zr:          zr,
	}, nil
}

func (a *treeArchive) ListStorage() liststorage.ListStorage {
	return a.listStorage
}

func (a *treeArchive) TreeStorage() treestorage.TreeStorage {
	return a.treeStorage
}

func (a *treeArchive) LocalStore() (*model.ObjectInfo, error) {
	for _, f := range a.zr.File {
		if f.Name == "localstore.json" {
			rd, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rd.Close()
			var oi = &model.ObjectInfo{}
			if err = jsonpb.Unmarshal(rd, oi); err != nil {
				return nil, err
			}
			return oi, nil
		}
	}
	return nil, fmt.Errorf("block logs file not found")
}

func (a *treeArchive) Close() (err error) {
	return a.zr.Close()
}
