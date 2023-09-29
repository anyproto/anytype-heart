package objectid

import (
	"time"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"

	"github.com/anyproto/anytype-heart/core/block/import/converter"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
)

type Widget struct {
	core core.Service
}

func NewWidget(core core.Service) *Widget {
	return &Widget{core: core}
}

func (w Widget) GetID(spaceID string, _ *converter.Snapshot, _ time.Time, _ bool) (string, treestorage.TreeStorageCreatePayload, error) {
	widgetID := w.core.PredefinedObjects(spaceID).Widgets
	return widgetID, treestorage.TreeStorageCreatePayload{}, nil
}
