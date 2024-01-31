package objectcreator

import (
	"context"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
)

type DataObject struct {
	oldIDtoNew     map[string]string
	createPayloads map[string]treestorage.TreeStorageCreatePayload
	fileIDs        []string
	ctx            context.Context
	origin         objectorigin.ObjectOrigin
	spaceID        string
}

type Result struct {
	Details *types.Struct
	NewID   string
	Err     error
}

func NewDataObject(ctx context.Context,
	oldIDtoNew map[string]string,
	createPayloads map[string]treestorage.TreeStorageCreatePayload,
	filesIDs []string,
	origin objectorigin.ObjectOrigin,
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
	sn      *common.Snapshot
	oc      Service
}

func NewTask(spaceID string, sn *common.Snapshot, oc Service) *Task {
	return &Task{sn: sn, oc: oc, spaceID: spaceID}
}

func (t *Task) Execute(data interface{}) interface{} {
	dataObject := data.(*DataObject)
	details, newID, err := t.oc.Create(dataObject, t.sn)
	return &Result{
		Details: details,
		NewID:   newID,
		Err:     err,
	}
}
