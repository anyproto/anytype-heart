package objectid

import (
	"context"
	"fmt"
	"time"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"

	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/space"
)

type widget struct {
	spaceService space.Service
}

func newWidget(spaceService space.Service) *widget {
	return &widget{spaceService: spaceService}
}

func (w widget) GetIDAndPayload(ctx context.Context, spaceID string, _ *common.Snapshot, _ time.Time, _ bool, _ objectorigin.ObjectOrigin) (string, treestorage.TreeStorageCreatePayload, error) {
	spc, err := w.spaceService.Get(ctx, spaceID)
	if err != nil {
		return "", treestorage.TreeStorageCreatePayload{}, fmt.Errorf("get space : %w", err)
	}
	return spc.DerivedIDs().Widgets, treestorage.TreeStorageCreatePayload{}, nil
}
