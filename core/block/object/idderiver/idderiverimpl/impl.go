package idderiverimpl

import (
	"context"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/block/object/idderiver"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/space"
)

const CName = "object-id-deriver"

type Deriver struct {
	spaceService space.Service
}

func New() idderiver.Deriver {
	return &Deriver{}
}

func (d *Deriver) Name() string {
	return CName
}

func (d *Deriver) Init(a *app.App) error {
	d.spaceService = app.MustComponent[space.Service](a)
	return nil
}

func (d *Deriver) DeriveObjectId(ctx context.Context, spaceId string, key domain.UniqueKey) (id string, err error) {
	spc, err := d.spaceService.Get(ctx, spaceId)
	if err != nil {
		return "", err
	}

	return spc.DeriveObjectID(ctx, key)
}
