package personalspace

import (
	"context"

	"github.com/anyproto/anytype-heart/space/internal/spaceprocess/loader"
	"github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/space/techspace"
)

type personalLoader struct {
	loader.Loader
	spaceId   string
	spaceCore spacecore.SpaceCoreService
	techSpace techspace.TechSpace
	newLoader func() loader.Loader
}

func (p *personalLoader) Start(ctx context.Context) (err error) {
	_, err = p.spaceCore.Get(ctx, p.spaceId)
	if err != nil {
		return
	}
	exists, err := p.techSpace.SpaceViewExists(ctx, p.spaceId)
	// This could happen for old accounts
	if !exists || err != nil {
		info := spaceinfo.NewSpacePersistentInfo(p.spaceId)
		info.SetAccountStatus(spaceinfo.AccountStatusUnknown)
		err = p.techSpace.SpaceViewCreate(ctx, p.spaceId, false, info)
		if err != nil {
			return
		}
	}
	p.Loader = p.newLoader()
	return p.Loader.Start(ctx)
}
