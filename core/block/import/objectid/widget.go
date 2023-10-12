package objectid

import (
	"context"
	"fmt"
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
	spc, err := w.spaceService.Get(ctx, spaceID)
	if err != nil {
		return "", treestorage.TreeStorageCreatePayload{}, fmt.Errorf("get space : %w", err)
	}
	return spc.DerivedIDs().Widgets, treestorage.TreeStorageCreatePayload{}, nil
}
