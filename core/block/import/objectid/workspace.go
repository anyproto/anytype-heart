package objectid

import (
	"context"
	"fmt"
	"time"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"

	"github.com/anyproto/anytype-heart/core/block/import/converter"
	"github.com/anyproto/anytype-heart/space"
)

type workspace struct {
	spaceService space.SpaceService
}

func newWorkspace(spaceService space.SpaceService) *workspace {
	return &workspace{spaceService: spaceService}
}

func (w *workspace) GetIDAndPayload(ctx context.Context, spaceID string, _ *converter.Snapshot, _ time.Time, _ bool) (string, treestorage.TreeStorageCreatePayload, error) {
	spc, err := w.spaceService.Get(ctx, spaceID)
	if err != nil {
		return "", treestorage.TreeStorageCreatePayload{}, fmt.Errorf("get space : %w", err)
	}
	return spc.DerivedIDs().Workspace, treestorage.TreeStorageCreatePayload{}, nil
}
