package dataview

import (
	"github.com/anytypeio/go-anytype-middleware/core/session"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

func (d *sdataview) AddFilter(
	ctx *session.Context,
	blockId string,
	viewId string,
	filter *model.BlockContentDataviewFilter,
) (err error) {
	s := d.NewStateCtx(ctx)
	dv, err := getDataviewBlock(s, blockId)
	if err != nil {
		return err
	}

	if err = dv.AddFilter(viewId, filter); err != nil {
		return err
	}

	return d.Apply(s)
}

func (d *sdataview) RemoveFilters(
	ctx *session.Context,
	blockId string,
	viewId string,
	filterIDs []string,
) (err error) {
	s := d.NewStateCtx(ctx)
	dv, err := getDataviewBlock(s, blockId)
	if err != nil {
		return err
	}

	if err = dv.RemoveFilters(viewId, filterIDs); err != nil {
		return err
	}

	return d.Apply(s)
}

func (d *sdataview) ReplaceFilter(
	ctx *session.Context,
	blockId string,
	viewId string,
	filterID string,
	filter *model.BlockContentDataviewFilter,
) (err error) {
	s := d.NewStateCtx(ctx)
	dv, err := getDataviewBlock(s, blockId)
	if err != nil {
		return err
	}

	if err = dv.ReplaceFilter(viewId, filterID, filter); err != nil {
		return err
	}

	return d.Apply(s)
}

func (d *sdataview) ReorderFilters(
	ctx *session.Context,
	blockId string,
	viewId string,
	filterIDs []string,
) (err error) {
	s := d.NewStateCtx(ctx)
	dv, err := getDataviewBlock(s, blockId)
	if err != nil {
		return err
	}

	if err = dv.ReorderFilters(viewId, filterIDs); err != nil {
		return err
	}

	return d.Apply(s)
}
