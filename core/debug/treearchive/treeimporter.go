package treearchive

import (
	"github.com/anytypeio/any-sync/commonspace/object/acl/liststorage"
	"github.com/anytypeio/any-sync/commonspace/object/tree/exporter"
	"github.com/anytypeio/any-sync/commonspace/object/tree/objecttree"
	"github.com/anytypeio/any-sync/commonspace/object/tree/treestorage"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/gogo/protobuf/proto"
)

type IdChange struct {
	Model *pb.Change
	Id    string
}

type TreeImporter interface {
	ObjectTree() objecttree.ReadableObjectTree
	State() (*state.State, error)
	Import(beforeId string) error
	ChangeAt(idx int) (IdChange, error)
}

type treeImporter struct {
	listStorage liststorage.ListStorage
	treeStorage treestorage.TreeStorage
	objectTree  objecttree.ReadableObjectTree
}

func NewTreeImporter(listStorage liststorage.ListStorage, treeStorage treestorage.TreeStorage) TreeImporter {
	return &treeImporter{
		listStorage: listStorage,
		treeStorage: treeStorage,
	}
}

func (t *treeImporter) ObjectTree() objecttree.ReadableObjectTree {
	return t.objectTree
}

func (t *treeImporter) State() (*state.State, error) {
	st, _, err := source.BuildState(nil, t.objectTree, "")
	if err != nil {
		return nil, err
	}

	if _, _, err = state.ApplyStateFast(st); err != nil {
		return nil, err
	}
	return st, nil
}

func (t *treeImporter) Import(beforeId string) (err error) {
	params := exporter.TreeImportParams{
		ListStorage:     t.listStorage,
		TreeStorage:     t.treeStorage,
		BeforeId:        beforeId,
		IncludeBeforeId: true,
	}
	t.objectTree, err = exporter.ImportHistoryTree(params)
	return
}

func (t *treeImporter) ChangeAt(idx int) (idCh IdChange, err error) {
	i := 1
	err = t.objectTree.IterateRoot(func(decrypted []byte) (any, error) {
		ch := &pb.Change{}
		err := proto.Unmarshal(decrypted, ch)
		if err != nil {
			return nil, err
		}
		return ch, nil
	}, func(change *objecttree.Change) bool {
		defer func() { i++ }()
		if change.Id == t.objectTree.Id() {
			return true
		}
		model := change.Model.(*pb.Change)
		if i == idx {
			idCh.Model = model
			idCh.Id = change.Id
			return false
		}
		return true
	},
	)
	return
}
