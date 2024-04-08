package personalspace

import (
	"context"
	"errors"

	"github.com/anyproto/any-sync/app"
	"go.uber.org/multierr"

	"github.com/anyproto/anytype-heart/space/internal/components/spaceloader"
	"github.com/anyproto/anytype-heart/space/internal/components/spacestatus"
	"github.com/anyproto/anytype-heart/space/internal/spacecontroller"
	"github.com/anyproto/anytype-heart/space/internal/spaceprocess/loader"
	"github.com/anyproto/anytype-heart/space/internal/spaceprocess/mode"
	"github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/space/techspace"
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

var makeStatusApp = func(a *app.App, spaceId string) *app.App {
	newApp := a.ChildApp()
	newApp.Register(spacestatus.New(spaceId))
	_ = newApp.Start(context.Background())
	return newApp
}

type spaceController struct {
	app      *app.App
	spaceId  string
	metadata []byte

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
		info := spaceinfo.NewSpacePersistentInfo(s.spaceId)
		info.SetAccountStatus(spaceinfo.AccountStatusUnknown)
		err = s.techSpace.SpaceViewCreate(ctx, s.spaceId, false, info)
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
	return loader.New(makeStatusApp(s.app, s.spaceId), loader.Params{
		SpaceId:       s.spaceId,
		IsPersonal:    true,
		OwnerMetadata: s.metadata,
	})
}

func (s *spaceController) Update() error {
	return nil
}

func (s *spaceController) SetPersistentInfo(ctx context.Context, info spaceinfo.SpacePersistentInfo) error {
	return nil
}

func (s *spaceController) SetLocalInfo(ctx context.Context, info spaceinfo.SpaceLocalInfo) error {
	return nil
}

func (s *spaceController) Close(ctx context.Context) error {
	loaderErr := s.loader.Close(ctx)
	appErr := s.app.Close(ctx)
	return multierr.Combine(loaderErr, appErr)
}

func (s *spaceController) GetStatus() spaceinfo.AccountStatus {
	return spaceinfo.AccountStatusUnknown
}
