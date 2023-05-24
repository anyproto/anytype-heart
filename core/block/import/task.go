package importer

import (
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/import/converter"
	"github.com/anyproto/anytype-heart/core/session"
)

type Task struct {
	sn        *converter.Snapshot
	relations []*converter.Relation
	existing  bool
	oc        Creator
}

type DataObject struct {
	oldIDtoNew     map[string]string
	createPayloads map[string]treestorage.TreeStorageCreatePayload
	ctx            *session.Context
}

type Result struct {
	details *types.Struct
	newID   string
	err     error
}

func NewDataObject(oldIDtoNew map[string]string, createPayloads map[string]treestorage.TreeStorageCreatePayload, ctx *session.Context) *DataObject {
	return &DataObject{oldIDtoNew: oldIDtoNew, createPayloads: createPayloads, ctx: ctx}
}

func NewTask(sn *converter.Snapshot, relations []*converter.Relation, existing bool, oc Creator) *Task {
	return &Task{sn: sn, relations: relations, existing: existing, oc: oc}
}

func (t *Task) Execute(data interface{}) interface{} {
	dataObject := data.(*DataObject)
	details, newID, err := t.oc.Create(dataObject.ctx, t.sn, t.relations, dataObject.oldIDtoNew, dataObject.createPayloads, t.existing)
	return &Result{
		details: details,
		newID:   newID,
		err:     err,
	}
}
