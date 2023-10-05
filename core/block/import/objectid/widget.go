package objectid

import (
	"context"
	"time"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"

	"github.com/anyproto/anytype-heart/core/block/import/converter"
	"github.com/anyproto/anytype-heart/space"
)

type widget struct {
	spaceService space.SpaceService
}

func newWidget(spaceService space.SpaceService) *widget {
	return &widget{spaceService: spaceService}
}

func (w widget) GetIDAndPayload(ctx context.Context, spaceID string, _ *converter.Snapshot, _ time.Time, _ bool) (string, treestorage.TreeStorageCreatePayload, error) {
	ds, err := w.spaceService.DerivedIDs(ctx, spaceID)
	if err != nil {
		return "", treestorage.TreeStorageCreatePayload{}, err
	}
	return ds.Widgets, treestorage.TreeStorageCreatePayload{}, nil
}
