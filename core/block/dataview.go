package block

import (
	"github.com/anyproto/anytype-heart/core/block/editor/dataview"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func (s *Service) AddDataviewFilter(
	ctx session.Context,
	contextID string,
	blockID string,
	viewID string,
	filter *model.BlockContentDataviewFilter,
) (err error) {
	return DoStateCtx(s, ctx, contextID, func(s *state.State, d dataview.Dataview) error {
		dv, err := d.GetDataviewBlock(s, blockID)
		if err != nil {
			return err
		}

		return dv.AddFilter(viewID, filter)
	})
}

func (s *Service) RemoveDataviewFilters(
	ctx session.Context,
	contextID string,
	blockID string,
	viewID string,
	filterIDs []string,
) (err error) {
	return DoStateCtx(s, ctx, contextID, func(s *state.State, d dataview.Dataview) error {
		dv, err := d.GetDataviewBlock(s, blockID)
		if err != nil {
			return err
		}

		return dv.RemoveFilters(viewID, filterIDs)
	})
}

func (s *Service) ReplaceDataviewFilter(
	ctx session.Context,
	contextID string,
	blockID string,
	viewID string,
	filterID string,
	filter *model.BlockContentDataviewFilter,
) (err error) {
	return DoStateCtx(s, ctx, contextID, func(s *state.State, d dataview.Dataview) error {
		dv, err := d.GetDataviewBlock(s, blockID)
		if err != nil {
			return err
		}

		return dv.ReplaceFilter(viewID, filterID, filter)
	})
}

func (s *Service) ReorderDataviewFilters(
	ctx session.Context,
	contextID string,
	blockID string,
	viewID string,
	filterIDs []string,
) (err error) {
	return DoStateCtx(s, ctx, contextID, func(s *state.State, d dataview.Dataview) error {
		dv, err := d.GetDataviewBlock(s, blockID)
		if err != nil {
			return err
		}

		return dv.ReorderFilters(viewID, filterIDs)
	})
}

func (s *Service) AddDataviewSort(
	ctx session.Context,
	contextID string,
	blockID string,
	viewID string,
	sort *model.BlockContentDataviewSort,
) (err error) {
	return DoStateCtx(s, ctx, contextID, func(s *state.State, d dataview.Dataview) error {
		dv, err := d.GetDataviewBlock(s, blockID)
		if err != nil {
			return err
		}

		return dv.AddSort(viewID, sort)
	})
}

func (s *Service) RemoveDataviewSorts(
	ctx session.Context,
	contextID string,
	blockID string,
	viewID string,
	ids []string,
) (err error) {
	return DoStateCtx(s, ctx, contextID, func(s *state.State, d dataview.Dataview) error {
		dv, err := d.GetDataviewBlock(s, blockID)
		if err != nil {
			return err
		}

		return dv.RemoveSorts(viewID, ids)
	})
}

func (s *Service) ReplaceDataviewSort(
	ctx session.Context,
	contextID string,
	blockID string,
	viewID string,
	id string,
	sort *model.BlockContentDataviewSort,
) (err error) {
	return DoStateCtx(s, ctx, contextID, func(s *state.State, d dataview.Dataview) error {
		dv, err := d.GetDataviewBlock(s, blockID)
		if err != nil {
			return err
		}

		return dv.ReplaceSort(viewID, id, sort)
	})
}

func (s *Service) ReorderDataviewSorts(
	ctx session.Context,
	contextID string,
	blockID string,
	viewID string,
	ids []string,
) (err error) {
	return DoStateCtx(s, ctx, contextID, func(s *state.State, d dataview.Dataview) error {
		dv, err := d.GetDataviewBlock(s, blockID)
		if err != nil {
			return err
		}

		return dv.ReorderSorts(viewID, ids)
	})
}

func (s *Service) AddDataviewViewRelation(
	ctx session.Context,
	contextID string,
	blockID string,
	viewID string,
	relation *model.BlockContentDataviewRelation,
) (err error) {
	return DoStateCtx(s, ctx, contextID, func(s *state.State, d dataview.Dataview) error {
		dv, err := d.GetDataviewBlock(s, blockID)
		if err != nil {
			return err
		}

		return dv.AddViewRelation(viewID, relation)
	})
}

func (s *Service) RemoveDataviewViewRelations(
	ctx session.Context,
	contextID string,
	blockID string,
	viewID string,
	relationKeys []string,
) (err error) {
	return DoStateCtx(s, ctx, contextID, func(s *state.State, d dataview.Dataview) error {
		dv, err := d.GetDataviewBlock(s, blockID)
		if err != nil {
			return err
		}

		return dv.RemoveViewRelations(viewID, relationKeys)
	})
}

func (s *Service) ReplaceDataviewViewRelation(
	ctx session.Context,
	contextID string,
	blockID string,
	viewID string,
	relationKey string,
	relation *model.BlockContentDataviewRelation,
) (err error) {
	return DoStateCtx(s, ctx, contextID, func(s *state.State, d dataview.Dataview) error {
		dv, err := d.GetDataviewBlock(s, blockID)
		if err != nil {
			return err
		}

		return dv.ReplaceViewRelation(viewID, relationKey, relation)
	})
}

func (s *Service) ReorderDataviewViewRelations(
	ctx session.Context,
	contextID string,
	blockID string,
	viewID string,
	relationKeys []string,
) (err error) {
	return DoStateCtx(s, ctx, contextID, func(s *state.State, d dataview.Dataview) error {
		dv, err := d.GetDataviewBlock(s, blockID)
		if err != nil {
			return err
		}

		return dv.ReorderViewRelations(viewID, relationKeys)
	})
}
