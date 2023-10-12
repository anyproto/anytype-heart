package space

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/anyproto/any-sync/accountservice"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/gogo/protobuf/types"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/object/objectcache"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/space/techspace"
)

const CName = "client.space"

var log = logger.NewNamed(CName)

var (
	ErrIncorrectSpaceID = errors.New("incorrect space id")
	ErrSpaceNotExists   = errors.New("space not exists")
)

func New() SpaceService {
	return &service{}
}

type spaceIndexer interface {
	ReindexCommonObjects() error
	ReindexSpace(spaceID string) error
}

type isNewAccount interface {
	IsNewAccount() bool
	app.Component
}

type SpaceService interface {
	Create(ctx context.Context) (space Space, err error)

	Get(ctx context.Context, id string) (space Space, err error)
	GetPersonalSpace(ctx context.Context) (space Space, err error)

	app.ComponentRunnable
}

type service struct {
	indexer          spaceIndexer
	spaceCore        spacecore.SpaceCoreService
	techSpace        techspace.TechSpace
	marketplaceSpace *space

	bundledObjectsInstaller bundledObjectsInstaller
	accountService          accountservice.Service
	objectFactory           objectcache.ObjectFactory

	personalSpaceID string

	newAccount bool

	statuses map[string]spaceinfo.SpaceInfo
	loading  map[string]*loadingSpace
	loaded   map[string]Space

	mu sync.Mutex

	ctx       context.Context
	ctxCancel context.CancelFunc

	repKey uint64
}

func (s *service) Init(a *app.App) (err error) {
	s.indexer = app.MustComponent[spaceIndexer](a)
	s.spaceCore = app.MustComponent[spacecore.SpaceCoreService](a)
	s.objectFactory = app.MustComponent[objectcache.ObjectFactory](a)
	s.accountService = app.MustComponent[accountservice.Service](a)
	s.bundledObjectsInstaller = app.MustComponent[bundledObjectsInstaller](a)
	s.newAccount = app.MustComponent[isNewAccount](a).IsNewAccount()
	s.techSpace = techspace.New()
	s.bundledObjectsInstaller = app.MustComponent[bundledObjectsInstaller](a)

	s.statuses = map[string]spaceinfo.SpaceInfo{}
	s.loading = map[string]*loadingSpace{}
	s.loaded = map[string]Space{}

	return err
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) Run(ctx context.Context) (err error) {
	s.marketplaceSpace, err = s.newMarketplaceSpace(ctx)
	if err != nil {
		return
	}

	err = s.initTechSpace()
	if err != nil {
		return fmt.Errorf("init tech space: %w", err)
	}
	s.ctx, s.ctxCancel = context.WithCancel(context.Background())

	s.personalSpaceID, err = s.spaceCore.DeriveID(s.ctx, spacecore.SpaceType)
	if err != nil {
		return
	}

	// TODO: move this logic to any-sync
	s.repKey, err = getRepKey(s.personalSpaceID)
	if err != nil {
		return
	}

	err = s.indexer.ReindexCommonObjects()
	if err != nil {
		return
	}

	if s.newAccount {
		return s.createPersonalSpace(s.ctx)
	}
	return s.loadPersonalSpace(s.ctx)
}

func (s *service) initTechSpace() error {
	techCoreSpace, err := s.spaceCore.Derive(context.Background(), spacecore.TechSpaceType)
	if err != nil {
		return fmt.Errorf("derive tech space: %w", err)
	}

	sp := &space{
		service:                s,
		AnySpace:               techCoreSpace,
		loadMandatoryObjectsCh: make(chan struct{}),
		installer:              s.bundledObjectsInstaller,
	}
	sp.Cache = objectcache.New(techCoreSpace, s.accountService, s.objectFactory, s.personalSpaceID, sp)

	err = s.techSpace.Run(techCoreSpace, sp.Cache)

	s.preLoad(techCoreSpace.Id(), sp)
	if err != nil {
		return fmt.Errorf("run tech space: %w", err)
	}
	return nil
}

func (s *service) Create(ctx context.Context) (Space, error) {
	coreSpace, err := s.spaceCore.Create(ctx, s.repKey)
	if err != nil {
		return nil, err
	}
	return s.create(ctx, coreSpace)
}

func (s *service) Get(ctx context.Context, spaceID string) (sp Space, err error) {
	if spaceID == addr.AnytypeMarketplaceWorkspace {
		return s.marketplaceSpace, nil
	}
	if err = s.startLoad(ctx, spaceID); err != nil {
		return nil, err
	}
	return s.waitLoad(ctx, spaceID)
}

func (s *service) GetPersonalSpace(ctx context.Context) (sp Space, err error) {
	return s.Get(ctx, s.personalSpaceID)
}

func (s *service) open(ctx context.Context, spaceID string) (sp Space, err error) {
	coreSpace, err := s.spaceCore.Get(ctx, spaceID)
	if err != nil {
		return nil, err
	}
	return s.newSpace(ctx, coreSpace)
}

func (s *service) createPersonalSpace(ctx context.Context) (err error) {
	coreSpace, err := s.spaceCore.Derive(ctx, spacecore.SpaceType)
	if err != nil {
		return
	}
	_, err = s.create(ctx, coreSpace)
	if err == nil {
		return
	}
	if errors.Is(err, techspace.ErrSpaceViewExists) {
		return s.loadPersonalSpace(ctx)
	}
	return
}

func (s *service) loadPersonalSpace(ctx context.Context) (err error) {
	err = s.startLoad(ctx, s.personalSpaceID)
	// This could happen for old accounts
	if errors.Is(err, ErrSpaceNotExists) {
		err = s.techSpace.SpaceViewCreate(ctx, s.personalSpaceID)
		if err != nil {
			return err
		}
		err = s.startLoad(ctx, s.personalSpaceID)
		if err != nil {
			return err
		}
	}
	if err != nil {
		return
	}

	_, err = s.waitLoad(ctx, s.personalSpaceID)
	return err
}

func (s *service) IsPersonal(id string) bool {
	return s.personalSpaceID == id
}

func (s *service) OnViewCreated(spaceID string) {
	go func() {
		if err := s.startLoad(s.ctx, spaceID); err != nil {
			log.Warn("OnViewCreated.startLoad error", zap.Error(err))
		}
	}()
}

func (s *service) OnWorkspaceChanged(spaceId string, details *types.Struct) {
	go func() {
		if err := s.techSpace.SpaceViewSetData(s.ctx, spaceId, details); err != nil {
			log.Warn("OnWorkspaceChanged error", zap.Error(err))
		}
	}()
}

func (s *service) Close(ctx context.Context) (err error) {
	if s.ctxCancel != nil {
		s.ctxCancel()
	}
	return nil
}

func getRepKey(spaceID string) (uint64, error) {
	sepIdx := strings.Index(spaceID, ".")
	if sepIdx == -1 {
		return 0, ErrIncorrectSpaceID
	}
	return strconv.ParseUint(spaceID[sepIdx+1:], 36, 64)
}
