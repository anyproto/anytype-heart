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
	"github.com/anyproto/any-sync/coordinator/coordinatorclient"
	"github.com/gogo/protobuf/types"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/object/objectcache"
	"github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/space/spacecore/storage"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/space/techspace"
)

const CName = "client.space"

var log = logger.NewNamed(CName)

var (
	ErrIncorrectSpaceID = errors.New("incorrect space id")
	ErrSpaceNotExists   = errors.New("space not exists")
	ErrSpaceDeleted     = errors.New("space is offloaded")
	ErrStatusUnkown     = errors.New("space status is unknown")
)

func New() Service {
	return &service{}
}

type spaceIndexer interface {
	ReindexMarketplaceSpace(space Space) error
	ReindexSpace(space Space) error
	RemoveIndexes(spaceID string) (err error)
}

type fileOffloader interface {
	FilesSpaceOffload(ctx context.Context, spaceID string) (err error)
}

type isNewAccount interface {
	IsNewAccount() bool
	app.Component
}

type Service interface {
	Create(ctx context.Context) (space Space, err error)

	Get(ctx context.Context, id string) (space Space, err error)
	Delete(ctx context.Context, id string) (err error)
	GetPersonalSpace(ctx context.Context) (space Space, err error)
	SpaceViewId(spaceId string) (spaceViewId string, err error)

	app.ComponentRunnable
}

type service struct {
	indexer          spaceIndexer
	spaceCore        spacecore.SpaceCoreService
	techSpace        techspace.TechSpace
	marketplaceSpace Space
	delController    *deletionController

	bundledObjectsInstaller bundledObjectsInstaller
	accountService          accountservice.Service
	objectFactory           objectcache.ObjectFactory
	storageService          storage.ClientStorage
	offloader               fileOffloader

	personalSpaceID string
	metadataPayload []byte

	newAccount bool

	createdSpaces map[string]struct{}
	statuses      map[string]spaceinfo.SpaceInfo
	loading       map[string]*loadingSpace
	offloading    map[string]*offloadingSpace
	offloaded     map[string]struct{}
	loaded        map[string]Space

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
	s.storageService = app.MustComponent[storage.ClientStorage](a)
	coordClient := app.MustComponent[coordinatorclient.CoordinatorClient](a)
	s.delController = newDeletionController(s, coordClient)
	s.offloader = app.MustComponent[fileOffloader](a)
	s.createdSpaces = map[string]struct{}{}
	s.statuses = map[string]spaceinfo.SpaceInfo{}
	s.loading = map[string]*loadingSpace{}
	s.offloading = map[string]*offloadingSpace{}
	s.loaded = map[string]Space{}
	s.offloaded = map[string]struct{}{}

	return err
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) Run(ctx context.Context) (err error) {
	s.ctx, s.ctxCancel = context.WithCancel(context.Background())
	s.metadataPayload, err = deriveAccountMetadata(s.accountService.Account().SignKey)
	if err != nil {
		return
	}
	err = s.initMarketplaceSpace()
	if err != nil {
		return fmt.Errorf("init marketplace space: %w", err)
	}
	err = s.initTechSpace()
	if err != nil {
		return fmt.Errorf("init tech space: %w", err)
	}
	err = s.initPersonalSpace()
	if err != nil {
		return fmt.Errorf("init personal space: %w", err)
	}
	s.delController.Run()
	return nil
}

func (s *service) Create(ctx context.Context) (Space, error) {
	coreSpace, err := s.spaceCore.Create(ctx, s.repKey, s.metadataPayload)
	if err != nil {
		return nil, err
	}
	return s.create(ctx, coreSpace)
}

func (s *service) Get(ctx context.Context, spaceID string) (sp Space, err error) {
	if err = s.startLoad(ctx, spaceID); err != nil {
		return nil, err
	}
	return s.waitLoad(ctx, spaceID)
}

func (s *service) GetPersonalSpace(ctx context.Context) (sp Space, err error) {
	return s.Get(ctx, s.personalSpaceID)
}

func (s *service) open(ctx context.Context, spaceID string, justCreated bool) (sp Space, err error) {
	coreSpace, err := s.spaceCore.Get(ctx, spaceID)
	if err != nil {
		return nil, err
	}
	return s.newSpace(ctx, coreSpace, justCreated)
}

func (s *service) IsPersonal(id string) bool {
	return s.personalSpaceID == id
}

func (s *service) OnViewCreated(info spaceinfo.SpaceInfo) {
	go func() {
		s.setViewCreatedInfo(info)
		err := s.startLoad(s.ctx, info.SpaceID)
		if err != nil && err != ErrSpaceDeleted {
			log.Warn("OnViewCreated.startLoad error", zap.Error(err))
		}
		if info.AccountStatus != spaceinfo.AccountStatusDeleted {
			return
		}
		err = s.startDelete(s.ctx, info.SpaceID)
		if err != nil {
			log.Warn("OnViewCreated.startDelete error", zap.Error(err))
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

func (s *service) SpaceViewId(spaceId string) (spaceViewId string, err error) {
	return s.techSpace.SpaceViewId(spaceId)
}

func (s *service) Close(ctx context.Context) (err error) {
	if s.ctxCancel != nil {
		s.ctxCancel()
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, sp := range s.loaded {
		err = sp.Close(ctx)
		if err != nil {
			log.Error("close space", zap.String("spaceId", sp.Id()), zap.Error(err))
		}
	}
	s.delController.Close()
	return
}

func getRepKey(spaceID string) (uint64, error) {
	sepIdx := strings.Index(spaceID, ".")
	if sepIdx == -1 {
		return 0, ErrIncorrectSpaceID
	}
	return strconv.ParseUint(spaceID[sepIdx+1:], 36, 64)
}
