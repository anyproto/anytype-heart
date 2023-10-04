package objectid

import (
	"context"
	"fmt"
	"time"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"

	"github.com/anyproto/anytype-heart/core/block/import/converter"
	"github.com/anyproto/anytype-heart/core/block/object/objectcache"
	"github.com/anyproto/anytype-heart/core/block/object/payloadcreator"
)

type TreeObject struct {
	existingObject *ExistingObject
	objectCache    objectcache.Cache
}

func NewTreeObject(existingObject *ExistingObject, objectCache objectcache.Cache) *TreeObject {
	return &TreeObject{existingObject: existingObject, objectCache: objectCache}
}

func (t *TreeObject) GetID(spaceID string, sn *converter.Snapshot, createdTime time.Time, getExisting bool) (string, treestorage.TreeStorageCreatePayload, error) {
	id, payload, err := t.existingObject.GetID(spaceID, sn, createdTime, getExisting)
	if err != nil {
		return "", treestorage.TreeStorageCreatePayload{}, err
	}
	if id != "" {
		return id, payload, nil
	}
	payload, err = t.objectCache.CreateTreePayload(context.Background(), spaceID, payloadcreator.PayloadCreationParams{
		Time:           createdTime,
		SmartblockType: sn.SbType,
	})
	if err != nil {
		return "", treestorage.TreeStorageCreatePayload{}, fmt.Errorf("create tree payload: %w", err)
	}
	return payload.RootRawChange.Id, payload, nil
}
