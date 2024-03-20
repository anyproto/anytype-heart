package personalspace

import (
	"context"
	"errors"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/space/internal/components/spaceloader"
	"github.com/anyproto/anytype-heart/space/internal/components/spacestatus"
	"github.com/anyproto/anytype-heart/space/internal/spacecontroller"
	"github.com/anyproto/anytype-heart/space/internal/spaceprocess/loader"
	"github.com/anyproto/anytype-heart/space/internal/spaceprocess/mode"
	"github.com/anyproto/anytype-heart/space/internal/techspace"
	"github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

func NewSpaceController(spaceId string, metadata []byte, a *app.App) spacecontroller.SpaceController {
	techSpace := a.MustComponent(techspace.CName).(techspace.TechSpace)
	spaceCore := a.MustComponent(spacecore.CName).(spacecore.SpaceCoreService)
	return &spaceController{
		app:       a,
		spaceId:   spaceId,
		techSpace: techSpace,
		spaceCore: spaceCore,
		metadata:  metadata,
	}
}

type spaceController struct {
	app      *app.App
	spaceId  string
	metadata []byte

	loader    loader.Loader
	spaceCore spacecore.SpaceCoreService
	techSpace techspace.TechSpace
}

func (s *spaceController) UpdateRemoteStatus(ctx context.Context, status spaceinfo.SpaceRemoteStatusInfo) error {
	return nil
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
		err = s.techSpace.SpaceViewCreate(ctx, s.spaceId, false, spaceinfo.SpacePersistentInfo{AccountStatus: spaceinfo.AccountStatusUnknown})
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

func (s *spaceController) Mode() mode.Mode {
	return mode.ModeLoading
}

func (s *spaceController) Current() any {
	return s.loader
}

func (s *spaceController) SpaceId() string {
	return s.spaceId
}

func (s *spaceController) newLoader() loader.Loader {
	return loader.New(s.app, loader.Params{
		SpaceId:             s.spaceId,
		Status:              spacestatus.New(s.spaceId, spaceinfo.AccountStatusUnknown, ""),
		StopIfMandatoryFail: true,
		OwnerMetadata:       s.metadata,
	})
}

func (s *spaceController) UpdateInfo(ctx context.Context, info spaceinfo.SpacePersistentInfo) error {
	return nil
}

func (s *spaceController) SetInfo(ctx context.Context, info spaceinfo.SpacePersistentInfo) error {
	return nil
}

func (s *spaceController) Close(ctx context.Context) error {
	return s.loader.Close(ctx)
}
