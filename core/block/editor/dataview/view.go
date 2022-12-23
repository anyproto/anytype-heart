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
