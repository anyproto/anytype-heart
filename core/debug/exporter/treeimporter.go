package exporter

import (
	"errors"
	"fmt"

	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"

	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/source/sourceimpl"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/util/pbtypes"
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
	Json() (TreeJson, error)
	ChangeAt(idx int) (IdChange, error)
}

type treeImporter struct {
	objectTree objecttree.ReadableObjectTree
}

func NewTreeImporter(objectTree objecttree.ReadableObjectTree) TreeImporter {
	return &treeImporter{
		objectTree: objectTree,
	}
}

func (t *treeImporter) ObjectTree() objecttree.ReadableObjectTree {
	return t.objectTree
}

func (t *treeImporter) State() (*state.State, error) {
	var (
		st  *state.State
		err error
	)

	st, _, _, err = sourceimpl.BuildState("", nil, t.objectTree, true)
	if err != nil {
		return nil, err
	}

	if _, _, err = state.ApplyStateFast("", st); err != nil {
		return nil, err
	}
	return st, nil
}

func (t *treeImporter) Json() (treeJson TreeJson, err error) {
	treeJson = TreeJson{
		Id: t.objectTree.Id(),
	}
	i := 0
	err = t.objectTree.IterateRoot(sourceimpl.UnmarshalChange, func(change *objecttree.Change) bool {
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
	err = t.objectTree.IterateRoot(sourceimpl.UnmarshalChange, func(change *objecttree.Change) bool {
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
