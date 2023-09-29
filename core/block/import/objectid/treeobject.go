package objectid

import (
	"context"
	"fmt"
	"time"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/import/converter"
)

type TreeObject struct {
	existingObject *ExistingObject
	service        *block.Service
}

func NewTreeObject(existingObject *ExistingObject, service *block.Service) *TreeObject {
	return &TreeObject{existingObject: existingObject, service: service}
}

func (t *TreeObject) GetID(spaceID string, sn *converter.Snapshot, createdTime time.Time, getExisting bool) (string, treestorage.TreeStorageCreatePayload, error) {
	id, payload, err := t.existingObject.GetID(spaceID, sn, createdTime, getExisting)
	if err != nil {
		return "", treestorage.TreeStorageCreatePayload{}, err
	}
	if id != "" {
		return id, payload, nil
	}
	payload, err = t.service.CreateTreePayload(context.Background(), spaceID, sn.SbType, createdTime)
	if err != nil {
		return "", treestorage.TreeStorageCreatePayload{}, fmt.Errorf("create tree payload: %w", err)
	}
	return payload.RootRawChange.Id, payload, nil
}
