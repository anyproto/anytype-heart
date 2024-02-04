package publicspace

import (
	"context"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/util/crypto"

	"github.com/anyproto/anytype-heart/space/internal/components/spacestatus"
	"github.com/anyproto/anytype-heart/space/internal/spacecontroller"
	"github.com/anyproto/anytype-heart/space/internal/spaceprocess/loader"
	"github.com/anyproto/anytype-heart/space/internal/spaceprocess/mode"
	"github.com/anyproto/anytype-heart/space/internal/techspace"
	"github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

func NewSpaceController(spaceId string, privKey crypto.PrivKey, a *app.App) spacecontroller.SpaceController {
	techSpace := a.MustComponent(techspace.CName).(techspace.TechSpace)
	spaceCore := a.MustComponent(spacecore.CName).(spacecore.SpaceCoreService)
	return &spaceController{
		app:       a,
		spaceId:   spaceId,
		techSpace: techSpace,
		spaceCore: spaceCore,
		guestKey:  privKey,
	}
}

type spaceController struct {
	app      *app.App
	spaceId  string
	metadata []byte

	loader    loader.Loader
	spaceCore spacecore.SpaceCoreService
	techSpace techspace.TechSpace
	guestKey  crypto.PrivKey
}

func (s *spaceController) UpdateRemoteStatus(ctx context.Context, status spaceinfo.RemoteStatus) error {
	return nil
}

func (s *spaceController) Start(ctx context.Context) (err error) {
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
		SpaceId:  s.spaceId,
		Status:   spacestatus.New(s.spaceId, spaceinfo.AccountStatusUnknown),
		GuestKey: s.guestKey,
	})
}

func (s *spaceController) UpdateStatus(ctx context.Context, status spaceinfo.AccountStatus) error {
	return nil
}

func (s *spaceController) SetStatus(ctx context.Context, status spaceinfo.AccountStatus) error {
	return nil
}

func (s *spaceController) Close(ctx context.Context) error {
	return s.loader.Close(ctx)
}
