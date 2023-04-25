package treearchive

import (
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"

	"github.com/anytypeio/any-sync/commonspace/object/acl/liststorage"
	"github.com/anytypeio/any-sync/commonspace/object/tree/exporter"
	"github.com/anytypeio/any-sync/commonspace/object/tree/objecttree"
	"github.com/anytypeio/any-sync/commonspace/object/tree/treestorage"
	"github.com/gogo/protobuf/proto"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

type IdChange struct {
	Model *pb.Change
	Id    string
}

type TreeJson struct {
	Changes []JsonChange `json:"changes"`
	Id      string       `json:"id"`
}

type JsonChange struct {
	Id     string               `json:"id"`
	Ord    int                  `json:"ord"`
	Change MarshalledJsonChange `json:"change"`
}

type MarshalledJsonChange struct {
	JsonString string
}

func (m MarshalledJsonChange) MarshalJSON() ([]byte, error) {
	return []byte(m.JsonString), nil
}

type TreeImporter interface {
	ObjectTree() objecttree.ReadableObjectTree
	State() (*state.State, error)
	Import(beforeId string) error
	Json() (TreeJson, error)
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
	st, _, _, err := source.BuildState(nil, t.objectTree, "")
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

func (t *treeImporter) Json() (treeJson TreeJson, err error) {
	treeJson = TreeJson{
		Id: t.objectTree.Id(),
	}
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
			// probably this will never be the case, because all our trees should have snapshots, and not root change
			return true
		}
		model := change.Model.(*pb.Change)
		ch := JsonChange{
			Id:     change.Id,
			Ord:    i,
			Change: MarshalledJsonChange{JsonString: pbtypes.Sprint(model)},
		}
		treeJson.Changes = append(treeJson.Changes, ch)
		return true
	})
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
			// probably this will never be the case, because all our trees should have snapshots, and not root change
			return true
		}
		model := change.Model.(*pb.Change)
		if i == idx {
			idCh.Model = model
			idCh.Id = change.Id
			return false
		}
		return true
	})
	if err != nil {
		return
	}
	if idCh.Model == nil {
		err = fmt.Errorf("no such index in tree")
	}
	return
}
