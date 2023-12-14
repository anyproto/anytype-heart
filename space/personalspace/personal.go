package personalspace

import (
	"context"
	"errors"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/space/components/spacecontroller"
	"github.com/anyproto/anytype-heart/space/components/spaceloader"
	"github.com/anyproto/anytype-heart/space/components/spacestatus"
	"github.com/anyproto/anytype-heart/space/process/loader"
	"github.com/anyproto/anytype-heart/space/process/modechanger"
	"github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/space/techspace"
)

func NewSpaceController(
	spaceId string,
	justCreated bool,
	a *app.App) spacecontroller.SpaceController {
	techSpace := a.MustComponent(techspace.CName).(techspace.TechSpace)
	spaceCore := a.MustComponent(spacecore.CName).(spacecore.SpaceCoreService)
	return &spaceController{
		app:         a,
		spaceId:     spaceId,
		justCreated: justCreated,
		techSpace:   techSpace,
		spaceCore:   spaceCore,
	}
}

type spaceController struct {
	app         *app.App
	spaceId     string
	justCreated bool

	loader    loader.Loader
	spaceCore spacecore.SpaceCoreService
	techSpace techspace.TechSpace
}

func (s *spaceController) Start(ctx context.Context) (err error) {
	// Check that space exists. If not, probably user is migrating from legacy version
	_, err = s.spaceCore.Get(ctx, s.spaceId)
	if err != nil {
		return
	}

	s.loader = s.newLoader()
	err = s.loader.Start(ctx)
	// This could happen for old accounts
	if errors.Is(err, spaceloader.ErrSpaceNotExists) {
		err = s.techSpace.SpaceViewCreate(ctx, s.spaceId, false)
		if err != nil {
			return
		}
		err = s.loader.Close(ctx)
		if err != nil {
			return
		}
		s.loader = s.newLoader()
		err = s.loader.Start(ctx)
		if err != nil {
			return
		}
	}
	if err != nil {
		return
	}

	return err
}

func (s *spaceController) Mode() modechanger.Mode {
	return modechanger.ModeLoading
}

func (s *spaceController) Current() any {
	return s.loader
}

func (s *spaceController) SpaceId() string {
	return s.spaceId
}

func (s *spaceController) newLoader() loader.Loader {
	return loader.New(s.app, loader.Params{
		JustCreated: s.justCreated,
		SpaceId:     s.spaceId,
		Status:      spacestatus.New(s.spaceId, spaceinfo.AccountStatusUnknown),
	})
}

func (s *spaceController) UpdateStatus(ctx context.Context, status spaceinfo.AccountStatus) error {
	return nil
}

func (s *spaceController) Close(ctx context.Context) error {
	return s.loader.Close(ctx)
}
