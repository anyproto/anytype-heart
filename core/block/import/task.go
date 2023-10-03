package importer

import (
	"context"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/import/converter"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type DataObject struct {
	oldIDtoNew     map[string]string
	createPayloads map[string]treestorage.TreeStorageCreatePayload
	fileIDs        []string
	ctx            context.Context
	origin         model.ObjectOrigin
	spaceID        string
}

type Result struct {
	details *types.Struct
	newID   string
	err     error
}

func NewDataObject(ctx context.Context,
	oldIDtoNew map[string]string,
	createPayloads map[string]treestorage.TreeStorageCreatePayload,
	filesIDs []string,
	origin model.ObjectOrigin,
	spaceID string,
) *DataObject {
	return &DataObject{
		oldIDtoNew:     oldIDtoNew,
		createPayloads: createPayloads,
		fileIDs:        filesIDs,
		ctx:            ctx,
		origin:         origin,
		spaceID:        spaceID,
	}
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
	details, newID, err := t.oc.Create(dataObject, t.sn)
	return &Result{
		details: details,
		newID:   newID,
		err:     err,
	}
}
