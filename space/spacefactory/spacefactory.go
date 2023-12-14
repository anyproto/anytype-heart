package spacefactory

import (
	"context"
	"errors"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/space/components/spacecontroller"
	"github.com/anyproto/anytype-heart/space/marketplacespace"
	"github.com/anyproto/anytype-heart/space/personalspace"
	"github.com/anyproto/anytype-heart/space/shareablespace"
	"github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/space/techspace"
)

type SpaceFactory interface {
	CreatePersonalSpace(ctx context.Context) (sp spacecontroller.SpaceController, err error)
	NewPersonalSpace(ctx context.Context) (spacecontroller.SpaceController, error)
	CreateShareableSpace(ctx context.Context) (sp spacecontroller.SpaceController, err error)
	NewShareableSpace(ctx context.Context, status spaceinfo.AccountStatus) (spacecontroller.SpaceController, error)
	CreateMarketplaceSpace(ctx context.Context) (sp spacecontroller.SpaceController, err error)
}

const CName = "client.space.spacefactory"

type spaceFactory struct {
	app             *app.App
	spaceCore       spacecore.SpaceCoreService
	techSpace       techspace.TechSpace
	personalSpaceId string
}

func New() SpaceFactory {
	return &spaceFactory{}
}

func (s *spaceFactory) CreatePersonalSpace(ctx context.Context) (sp spacecontroller.SpaceController, err error) {
	coreSpace, err := s.spaceCore.Derive(ctx, spacecore.SpaceType)
	if err != nil {
		return
	}
	if err := s.techSpace.SpaceViewCreate(ctx, coreSpace.Id(), true); err != nil {
		if errors.Is(err, techspace.ErrSpaceViewExists) {
			return s.NewPersonalSpace(ctx)
		}
		return nil, err
	}
	ctrl := personalspace.NewSpaceController(coreSpace.Id(), true, s.app)
	err = ctrl.Start(ctx)
	return ctrl, err
}

func (s *spaceFactory) NewPersonalSpace(ctx context.Context) (spacecontroller.SpaceController, error) {
	coreSpace, err := s.spaceCore.Derive(ctx, spacecore.SpaceType)
	if err != nil {
		return nil, err
	}
	ctrl := personalspace.NewSpaceController(coreSpace.Id(), false, s.app)
	err = ctrl.Start(ctx)
	return ctrl, err
}

func (s *spaceFactory) NewShareableSpace(ctx context.Context, status spaceinfo.AccountStatus) (spacecontroller.SpaceController, error) {
	coreSpace, err := s.spaceCore.Derive(ctx, spacecore.SpaceType)
	if err != nil {
		return nil, err
	}
	ctrl, err := shareablespace.NewSpaceController(coreSpace.Id(), false, status, s.app)
	if err != nil {
		return nil, err
	}
	err = ctrl.Start(ctx)
	return ctrl, err
}

func (s *spaceFactory) CreateShareableSpace(ctx context.Context) (sp spacecontroller.SpaceController, err error) {
	coreSpace, err := s.spaceCore.Derive(ctx, spacecore.SpaceType)
	if err != nil {
		return
	}
	if err := s.techSpace.SpaceViewCreate(ctx, coreSpace.Id(), true); err != nil {
		return nil, err
	}
	ctrl, err := shareablespace.NewSpaceController(coreSpace.Id(), false, spaceinfo.AccountStatusUnknown, s.app)
	if err != nil {
		return nil, err
	}
	err = ctrl.Start(ctx)
	return ctrl, err
}

func (s *spaceFactory) CreateMarketplaceSpace(ctx context.Context) (sp spacecontroller.SpaceController, err error) {
	ctrl := marketplacespace.NewSpaceController(s.app, s.personalSpaceId)
	err = ctrl.Start(ctx)
	return ctrl, err
}

func (s *spaceFactory) Init(a *app.App) (err error) {
	s.app = a
	s.techSpace = a.MustComponent(techspace.CName).(techspace.TechSpace)
	s.spaceCore = a.MustComponent(spacecore.CName).(spacecore.SpaceCoreService)
	s.personalSpaceId, err = s.spaceCore.DeriveID(context.Background(), spacecore.SpaceType)
	return
}

func (s *spaceFactory) Name() (name string) {
	return CName
}
