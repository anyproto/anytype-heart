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

func (s *Service) RemoveDataviewFilters(
	ctx *session.Context,
	contextId string,
	blockId string,
	viewId string,
	filterIDs []string,
) (err error) {
	return Do(s, contextId, func(b dataview.Dataview) error {
		return b.RemoveFilters(ctx, blockId, viewId, filterIDs)
	})
}

func (s *Service) UpdateDataviewFilter(
	ctx *session.Context,
	contextId string,
	blockId string,
	viewId string,
	filter *model.BlockContentDataviewFilter,
) (err error) {
	return Do(s, contextId, func(b dataview.Dataview) error {
		return b.UpdateFilter(ctx, blockId, viewId, filter)
	})
}

func (s *Service) ReorderDataviewFilters(
	ctx *session.Context,
	contextId string,
	blockId string,
	viewId string,
	filterIDs []string,
) (err error) {
	return Do(s, contextId, func(b dataview.Dataview) error {
		return b.ReorderFilters(ctx, blockId, viewId, filterIDs)
	})
}
