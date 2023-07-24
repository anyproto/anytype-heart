package importer

import (
	"context"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/import/converter"
)

type DataObject struct {
	oldIDtoNew     map[string]string
	createPayloads map[string]treestorage.TreeStorageCreatePayload
	fileIDs        []string
	ctx            context.Context
}

type Result struct {
	details *types.Struct
	newID   string
	err     error
}

func NewDataObject(oldIDtoNew map[string]string,
	createPayloads map[string]treestorage.TreeStorageCreatePayload,
	filesIDs []string,
	ctx context.Context) *DataObject {
	return &DataObject{oldIDtoNew: oldIDtoNew, createPayloads: createPayloads, fileIDs: filesIDs, ctx: ctx}
}

type Task struct {
	spaceID string
	sn      *converter.Snapshot
	oc      Creator
}

func NewTask(spaceID string, sn *converter.Snapshot, oc Creator) *Task {
	return &Task{sn: sn, oc: oc, spaceID: spaceID}
}

func (t *Task) Execute(data interface{}) interface{} {
	dataObject := data.(*DataObject)
	details, newID, err := t.oc.Create(dataObject.ctx, t.spaceID, t.sn, dataObject.oldIDtoNew, dataObject.createPayloads, dataObject.fileIDs)
	return &Result{
		details: details,
		newID:   newID,
		err:     err,
	}
}
