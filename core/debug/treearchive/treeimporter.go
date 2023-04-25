package treearchive

import (
	"errors"
	"fmt"
	"github.com/anytypeio/any-sync/commonspace/object/acl/list"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"

	"github.com/anytypeio/any-sync/commonspace/object/acl/liststorage"
	"github.com/anytypeio/any-sync/commonspace/object/tree/objecttree"
	"github.com/anytypeio/any-sync/commonspace/object/tree/treestorage"
	"github.com/gogo/protobuf/proto"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/source"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

var ErrCantRequestHeaderModel = errors.New("can't request header model")

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
	Import(fromRoot bool, beforeId string) error
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

func (t *treeImporter) Import(fullTree bool, beforeId string) (err error) {
	aclList, err := list.BuildAclList(t.listStorage)
	if err != nil {
		return
	}
	t.objectTree, err = objecttree.BuildNonVerifiableHistoryTree(objecttree.HistoryTreeParams{
		TreeStorage:     t.treeStorage,
		AclList:         aclList,
		BeforeId:        beforeId,
		IncludeBeforeId: true,
		BuildFullTree:   fullTree,
	})
	return
}

func (t *treeImporter) Json() (treeJson TreeJson, err error) {
	treeJson = TreeJson{
		Id: t.objectTree.Id(),
	}
	i := 0
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
	if idx == 0 && t.objectTree.Root().Id == t.objectTree.Id() {
		err = ErrCantRequestHeaderModel
		return
	}
	i := 0
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
	})
	if err != nil {
		return
	}
	if idCh.Model == nil {
		err = fmt.Errorf("no such index in tree")
	}
	return
}
