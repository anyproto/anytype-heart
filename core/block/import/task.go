package importer

import (
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/import/converter"
	"github.com/anyproto/anytype-heart/core/session"
)

type Task struct {
	sn *converter.Snapshot
	oc Creator
}

type DataObject struct {
	oldIDtoNew     map[string]string
	createPayloads map[string]treestorage.TreeStorageCreatePayload
	fileKeys       []string
	ctx            *session.Context
}

type Result struct {
	details *types.Struct
	newID   string
	err     error
}

func NewDataObject(oldIDtoNew map[string]string,
	createPayloads map[string]treestorage.TreeStorageCreatePayload,
	keys []string,
	ctx *session.Context) *DataObject {
	return &DataObject{oldIDtoNew: oldIDtoNew, createPayloads: createPayloads, fileKeys: keys, ctx: ctx}
}

func NewTask(sn *converter.Snapshot, oc Creator) *Task {
	return &Task{sn: sn, oc: oc}
}

func (t *Task) Execute(data interface{}) interface{} {
	dataObject := data.(*DataObject)
	details, newID, err := t.oc.Create(dataObject.ctx, t.sn, dataObject.oldIDtoNew, dataObject.createPayloads, dataObject.fileKeys)
	return &Result{
		details: details,
		newID:   newID,
		err:     err,
	}
}
