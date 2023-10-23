package objectid

import (
	"context"
	"fmt"
	"time"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"

	"github.com/anyproto/anytype-heart/core/block/import/converter"
	"github.com/anyproto/anytype-heart/core/block/object/payloadcreator"
	"github.com/anyproto/anytype-heart/space"
)

type treeObject struct {
	existingObject *existingObject
	spaceService   space.Service
}

func newTreeObject(existingObject *existingObject, spaceService space.Service) *treeObject {
	return &treeObject{existingObject: existingObject, spaceService: spaceService}
}

func (t *treeObject) GetIDAndPayload(ctx context.Context, spaceID string, sn *converter.Snapshot, createdTime time.Time, getExisting bool) (string, treestorage.TreeStorageCreatePayload, error) {
	id, payload, err := t.existingObject.GetIDAndPayload(ctx, spaceID, sn, createdTime, getExisting)
	if err != nil {
		return "", treestorage.TreeStorageCreatePayload{}, err
	}
	if id != "" {
		return id, payload, nil
	}
	spc, err := t.spaceService.Get(ctx, spaceID)
	if err != nil {
		return "", treestorage.TreeStorageCreatePayload{}, fmt.Errorf("get space : %w", err)
	}
	payload, err = spc.CreateTreePayload(ctx, payloadcreator.PayloadCreationParams{
		Time:           createdTime,
		SmartblockType: sn.SbType,
	})
	if err != nil {
		return "", treestorage.TreeStorageCreatePayload{}, fmt.Errorf("create tree payload: %w", err)
	}
	return payload.RootRawChange.Id, payload, nil
}
