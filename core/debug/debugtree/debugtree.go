package debugtree

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/anytypeio/any-sync/commonspace/object/tree/exporter"
	"github.com/anytypeio/any-sync/commonspace/object/tree/objecttree"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/core/debug/ziparchive"
	"github.com/anytypeio/go-anytype-middleware/core/debug/ziparchive/zipaclstorage"
	"github.com/anytypeio/go-anytype-middleware/core/debug/ziparchive/ziptreestorage"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/gogo/protobuf/jsonpb"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

var ErrNotImplemented = errors.New("not implemented for debug tree")

type DebugTreeStats struct {
	SnapshotCount int
	ChangeCount   int
}

func (dts DebugTreeStats) String() string {
	return fmt.Sprintf("snapshots: %d; changes: %d",
		dts.SnapshotCount,
		dts.ChangeCount,
	)
}

func (dts DebugTreeStats) MlString() string {
	return fmt.Sprintf("Snapshots:\t%d\nChanges:\t%d",
		dts.SnapshotCount,
		dts.ChangeCount,
	)
}

type DebugTree interface {
	objecttree.ReadableObjectTree
	Stats() DebugTreeStats
	LocalStore() (*model.ObjectInfo, error)
	BuildState() (*state.State, error)
	Close() error
}

// Open expects debug tree zip file
// return DebugTree that implements objecttree.ReadableObjectTree
func Open(filename string) (tr DebugTree, err error) {
	zr, err := zip.OpenReader(filename)
	if err != nil {
		return nil, err
	}
	exported, err := zr.Open("exported.json")
	if err != nil {
		return
	}
	defer exported.Close()
	expJson := &ziparchive.ExportedObjectsJson{}
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

	tree, err := exporter.ViewObjectTree(listStorage, treeStorage)
	if err != nil {
		return
	}

	return &debugTree{
		ReadableObjectTree: tree,
		zr:                 zr,
	}, nil
}

type debugTree struct {
	objecttree.ReadableObjectTree
	zr *zip.ReadCloser
}

func (r *debugTree) Type() (sbt smartblock.SmartBlockType) {
	sbt = smartblock.SmartBlockTypePage
	changeType := string(r.UnmarshalledHeader().Data)
	if v, exists := model.SmartBlockType_value[changeType]; exists {
		sbt = smartblock.SmartBlockType(v)
	}
	return
}

func (r *debugTree) Stats() (s DebugTreeStats) {
	// TODO: [MR] Implement debug stats
	return
}

func (r *debugTree) BuildState() (*state.State, error) {
	st, _, err := source.BuildState(nil, r.ReadableObjectTree, "")
	if err != nil {
		return nil, err
	}

	if _, _, err = state.ApplyStateFast(st); err != nil {
		return nil, err
	}
	return st, nil
}

func (r *debugTree) LocalStore() (*model.ObjectInfo, error) {
	for _, f := range r.zr.File {
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

func (r *debugTree) Close() (err error) {
	return r.zr.Close()
}
