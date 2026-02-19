package objectcreator

import (
	"context"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"

	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
)

type DataObject struct {
	oldIDtoNew           map[string]string
	createPayloads       map[string]treestorage.TreeStorageCreatePayload
	relationKeysToFormat map[domain.RelationKey]int32
	ctx                  context.Context
	origin               objectorigin.ObjectOrigin
	spaceID              string

	newIdsSet map[string]struct{}
}

type Result struct {
	Details *domain.Details
	NewID   string
	Err     error
}

func NewDataObject(
	ctx context.Context,
	oldIDtoNew map[string]string,
	createPayloads map[string]treestorage.TreeStorageCreatePayload,
	relationKeysToFormat map[domain.RelationKey]int32,
	origin objectorigin.ObjectOrigin,
	spaceID string,
) *DataObject {
	newIdsSet := make(map[string]struct{}, len(oldIDtoNew))
	for _, newId := range oldIDtoNew {
		newIdsSet[newId] = struct{}{}
	}
	return &DataObject{
		oldIDtoNew:           oldIDtoNew,
		createPayloads:       createPayloads,
		relationKeysToFormat: relationKeysToFormat,
		ctx:                  ctx,
		origin:               origin,
		spaceID:              spaceID,
		newIdsSet:            newIdsSet,
	}
}

type Task struct {
	sn *common.Snapshot
	oc Service
}

func NewTask(sn *common.Snapshot, oc Service) *Task {
	return &Task{sn: sn, oc: oc}
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
