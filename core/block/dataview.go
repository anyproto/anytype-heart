package block

import (
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/dataview"
	"github.com/anytypeio/go-anytype-middleware/core/session"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

func (s *Service) AddDataviewFilter(
	ctx *session.Context,
	contextId string,
	blockId string,
	viewId string,
	filter *model.BlockContentDataviewFilter,
) (err error) {
	return Do(s, contextId, func(b dataview.Dataview) error {
		return b.AddFilter(ctx, blockId, viewId, filter)
	})
}
