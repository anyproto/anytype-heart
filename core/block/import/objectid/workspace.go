package objectid

import (
	"time"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"

	"github.com/anyproto/anytype-heart/core/block/import/converter"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
)

type Workspace struct {
	core core.Service
}

func NewWorkspace(core core.Service) *Workspace {
	return &Workspace{core: core}
}

func (w *Workspace) GetID(spaceID string, _ *converter.Snapshot, _ time.Time, _ bool) (string, treestorage.TreeStorageCreatePayload, error) {
	workspaceID := w.core.PredefinedObjects(spaceID).Workspace
	return workspaceID, treestorage.TreeStorageCreatePayload{}, nil
}
