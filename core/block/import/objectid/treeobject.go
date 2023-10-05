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

type treeObject struct {
	existingObject *existingObject
	objectCache    objectcache.Cache
}

func newTreeObject(existingObject *existingObject, objectCache objectcache.Cache) *treeObject {
	return &treeObject{existingObject: existingObject, objectCache: objectCache}
}

func (t *treeObject) GetIDAndPayload(ctx context.Context, spaceID string, sn *converter.Snapshot, createdTime time.Time, getExisting bool) (string, treestorage.TreeStorageCreatePayload, error) {
	id, payload, err := t.existingObject.GetIDAndPayload(ctx, spaceID, sn, createdTime, getExisting)
	if err != nil {
		return "", treestorage.TreeStorageCreatePayload{}, err
	}
	if id != "" {
		return id, payload, nil
	}
	payload, err = t.objectCache.CreateTreePayload(ctx, spaceID, payloadcreator.PayloadCreationParams{
		Time:           createdTime,
		SmartblockType: sn.SbType,
	})
	if err != nil {
		return "", treestorage.TreeStorageCreatePayload{}, fmt.Errorf("create tree payload: %w", err)
	}
	return payload.RootRawChange.Id, payload, nil
}
