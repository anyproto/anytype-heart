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

type workspace struct {
	spaceService space.Service
}

func newWorkspace(spaceService space.Service) *workspace {
	return &workspace{spaceService: spaceService}
}

func (w *workspace) GetIDAndPayload(ctx context.Context, spaceID string, _ *common.Snapshot, _ time.Time, _ bool, _ objectorigin.ObjectOrigin) (string, treestorage.TreeStorageCreatePayload, error) {
	spc, err := w.spaceService.Get(ctx, spaceID)
	if err != nil {
		return "", treestorage.TreeStorageCreatePayload{}, fmt.Errorf("get space : %w", err)
	}
	return spc.DerivedIDs().Workspace, treestorage.TreeStorageCreatePayload{}, nil
}
