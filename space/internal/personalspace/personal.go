package personalspace

import (
	"context"

	"github.com/anyproto/any-sync/app"
	"go.uber.org/multierr"

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

func makeStatusApp(a *app.App, spaceId string) (*app.App, error) {
	newApp := a.ChildApp()
	newApp.Register(spacestatus.New(spaceId))
	err := newApp.Start(context.Background())
	if err != nil {
		return nil, err
	}
	return newApp, nil
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
	exists, err := s.techSpace.SpaceViewExists(ctx, s.spaceId)
	// This could happen for old accounts
	if !exists || err != nil {
		info := spaceinfo.NewSpacePersistentInfo(s.spaceId)
		info.SetAccountStatus(spaceinfo.AccountStatusUnknown)
		err = s.techSpace.SpaceViewCreate(ctx, s.spaceId, false, info)
		if err != nil {
			return
		}
	}
	s.app, err = makeStatusApp(s.app, s.spaceId)
	if err != nil {
		return
	}
	s.loader = s.newLoader()
	return s.loader.Start(ctx)
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
		SpaceId:       s.spaceId,
		IsPersonal:    true,
		OwnerMetadata: s.metadata,
	})
}

func (s *spaceController) Update() error {
	// TODO: [PS] Implement for deletion
	return nil
}

func (s *spaceController) SetPersistentInfo(ctx context.Context, info spaceinfo.SpacePersistentInfo) error {
	// TODO: [PS] Implement for deletion
	return nil
}

func (s *spaceController) SetLocalInfo(ctx context.Context, info spaceinfo.SpaceLocalInfo) error {
	// TODO: [PS] Implement for deletion
	return nil
}

func (s *spaceController) Close(ctx context.Context) error {
	if s.loader == nil {
		return nil
	}
	loaderErr := s.loader.Close(ctx)
	appErr := s.app.Close(ctx)
	return multierr.Combine(loaderErr, appErr)
}

func (s *spaceController) GetStatus() spaceinfo.AccountStatus {
	return spaceinfo.AccountStatusUnknown
}

func (s *spaceController) GetLocalStatus() spaceinfo.LocalStatus {
	return spaceinfo.LocalStatusOk
}
