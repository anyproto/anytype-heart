package spacefactory

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/anyproto/any-sync/accountservice"
	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/block/object/objectcache"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/components/dependencies"
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
	NewShareableSpace(ctx context.Context, id string, status spaceinfo.AccountStatus) (spacecontroller.SpaceController, error)
	CreateMarketplaceSpace(ctx context.Context) (sp spacecontroller.SpaceController, err error)
	CreateAndSetTechSpace(ctx context.Context) (*clientspace.TechSpace, error)
}

const CName = "client.space.spacefactory"

type spaceFactory struct {
	app             *app.App
	spaceCore       spacecore.SpaceCoreService
	techSpace       techspace.TechSpace
	accountService  accountservice.Service
	objectFactory   objectcache.ObjectFactory
	indexer         dependencies.SpaceIndexer
	installer       dependencies.BundledObjectsInstaller
	personalSpaceId string
	metadataPayload []byte
	repKey          uint64
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

func (s *spaceFactory) CreateAndSetTechSpace(ctx context.Context) (*clientspace.TechSpace, error) {
	techSpace := techspace.New()
	techCoreSpace, err := s.spaceCore.Derive(ctx, spacecore.TechSpaceType)
	if err != nil {
		return nil, fmt.Errorf("derive tech space: %w", err)
	}
	deps := clientspace.TechSpaceDeps{
		CommonSpace:     techCoreSpace,
		ObjectFactory:   s.objectFactory,
		AccountService:  s.accountService,
		PersonalSpaceId: s.personalSpaceId,
		Indexer:         s.indexer,
		Installer:       s.installer,
		TechSpace:       techSpace,
	}
	ts := clientspace.NewTechSpace(deps)
	err = s.techSpace.Run(techCoreSpace, ts.Cache)
	if err != nil {
		return nil, fmt.Errorf("run tech space: %w", err)
	}

	s.techSpace = ts
	s.app = s.app.ChildApp()
	s.app.Register(s.techSpace)
	return ts, nil
}

func (s *spaceFactory) NewShareableSpace(ctx context.Context, id string, status spaceinfo.AccountStatus) (spacecontroller.SpaceController, error) {
	ctrl, err := shareablespace.NewSpaceController(id, false, status, s.app)
	if err != nil {
		return nil, err
	}
	err = ctrl.Start(ctx)
	return ctrl, err
}

func (s *spaceFactory) CreateShareableSpace(ctx context.Context) (sp spacecontroller.SpaceController, err error) {
	coreSpace, err := s.spaceCore.Create(ctx, s.repKey, s.metadataPayload)
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
	if err != nil {
		return
	}
	s.metadataPayload, err = deriveAccountMetadata(s.accountService.Account().SignKey)
	if err != nil {
		return
	}
	s.repKey, err = getRepKey(s.personalSpaceId)
	return
}

func (s *spaceFactory) Name() (name string) {
	return CName
}

func getRepKey(spaceId string) (uint64, error) {
	sepIdx := strings.Index(spaceId, ".")
	if sepIdx == -1 {
		return 0, fmt.Errorf("space id is incorrect")
	}
	return strconv.ParseUint(spaceId[sepIdx+1:], 36, 64)
}
