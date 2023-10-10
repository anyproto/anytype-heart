package space

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/app/ocache"
	"github.com/gogo/protobuf/types"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/object/objectcache"
	"github.com/anyproto/anytype-heart/pkg/lib/threads"
	"github.com/anyproto/anytype-heart/space/objectprovider"
	"github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/space/spacecore/storage"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/space/techspace"
)

const CName = "client.space"

var log = logger.NewNamed(CName)

var (
	ErrIncorrectSpaceID     = errors.New("incorrect space id")
	ErrSpaceNotExists       = errors.New("space not exists")
	ErrSpaceDeleted         = errors.New("space deleted")
	ErrSpaceDeletionPending = errors.New("space deletion pending")
	ErrSpaceIsActive        = errors.New("space is active")
)

func New() SpaceService {
	return &service{}
}

type spaceIndexer interface {
	ReindexCommonObjects() error
	ReindexSpace(spaceID string) error
	RemoveIndexes(spaceID string) error
}

type bundledObjectsInstaller interface {
	InstallBundledObjects(ctx context.Context, spaceID string, ids []string) ([]string, []*types.Struct, error)
	app.Component
}

type isNewAccount interface {
	IsNewAccount() bool
	app.Component
}

type SpaceService interface {
	Create(ctx context.Context) (space Space, err error)
	Get(ctx context.Context, id string) (space Space, err error)
	Delete(ctx context.Context, id string) (delStatus DeletionStatus, err error)
	RevertDeletion(ctx context.Context, id string) (err error)
	Offload(ctx context.Context, id string) (err error)
	Download(ctx context.Context, id string) (err error)
	DerivedIDs(ctx context.Context, spaceID string) (ids threads.DerivedSmartblockIds, err error)

	app.ComponentRunnable
}

type service struct {
	indexer        spaceIndexer
	spaceCore      spacecore.SpaceCoreService
	provider       objectprovider.ObjectProvider
	objectCache    objectcache.Cache
	techSpace      techspace.TechSpace
	storageService storage.ClientStorage

	personalSpaceID string

	newAccount bool

	statuses   map[string]spaceinfo.SpaceInfo
	loading    map[string]*loadingSpace
	loaded     map[string]Space
	offloading map[string]*offloadingSpace

	mu sync.Mutex

	ctx       context.Context
	ctxCancel context.CancelFunc

	derivedIDsCache ocache.OCache

	repKey uint64
}

func (s *service) Init(a *app.App) (err error) {
	s.indexer = app.MustComponent[spaceIndexer](a)
	s.spaceCore = app.MustComponent[spacecore.SpaceCoreService](a)
	s.objectCache = app.MustComponent[objectcache.Cache](a)
	installer := app.MustComponent[bundledObjectsInstaller](a)
	s.provider = objectprovider.NewObjectProvider(s.objectCache, installer)
	s.newAccount = app.MustComponent[isNewAccount](a).IsNewAccount()
	s.techSpace = app.MustComponent[techspace.TechSpace](a)
	s.storageService = app.MustComponent[storage.ClientStorage](a)

	s.statuses = map[string]spaceinfo.SpaceInfo{}
	s.loading = map[string]*loadingSpace{}
	s.offloading = map[string]*offloadingSpace{}

	s.loaded = map[string]Space{}

	s.derivedIDsCache = ocache.New(s.loadDerivedIDs)
	return nil
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) Run(_ context.Context) (err error) {
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

func (s *service) Delete(ctx context.Context, id string) (delStatus DeletionStatus, err error) {
	curStatus := s.getStatus(id)
	// TODO: add time for deletion
	networkStatus, err := s.spaceCore.Delete(ctx, id)
	if err != nil {
		return
	}
	err = s.setStatus(ctx, spaceinfo.SpaceInfo{
		SpaceID:      id,
		LocalStatus:  curStatus.LocalStatus,
		RemoteStatus: spaceinfo.RemoteStatusWaitingDeletion,
	})
	if err != nil {
		return
	}
	delStatus = DeletionStatus{
		Status:       spaceinfo.RemoteStatusWaitingDeletion,
		DeletionDate: networkStatus.DeletionDate,
	}
	return
}

func (s *service) RevertDeletion(ctx context.Context, id string) (err error) {
	curStatus := s.getStatus(id)
	err = s.spaceCore.RevertDeletion(ctx, id)
	if err != nil {
		return
	}
	return s.setStatus(ctx, spaceinfo.SpaceInfo{
		SpaceID:      id,
		LocalStatus:  curStatus.LocalStatus,
		RemoteStatus: spaceinfo.RemoteStatusOk,
	})
}

func (s *service) Offload(ctx context.Context, id string) (err error) {
	// TODO: move to offloading space
	sp, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	err = sp.Close()
	if err != nil {
		return
	}
	err = s.storageService.DeleteSpaceStorage(ctx, id)
	if err != nil {
		return
	}
	return s.indexer.RemoveIndexes(id)
}

func (s *service) Download(ctx context.Context, id string) (err error) {
	//TODO implement me
	panic("implement me")
}

func (s *service) Create(ctx context.Context) (Space, error) {
	coreSpace, err := s.spaceCore.Create(ctx, s.repKey)
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

func (s *service) open(ctx context.Context, spaceID string) (sp Space, err error) {
	coreSpace, err := s.spaceCore.Get(ctx, spaceID)
	if err != nil {
		return nil, err
	}
	derivedIDs, err := s.DerivedIDs(ctx, spaceID)
	if err != nil {
		return nil, err
	}
	sp = newSpace(s, coreSpace, derivedIDs)
	return
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
	if err = s.startLoad(ctx, s.personalSpaceID); err != nil {
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

func (s *service) Close(ctx context.Context) (err error) {
	if s.ctxCancel != nil {
		s.ctxCancel()
	}
	return s.derivedIDsCache.Close()
}

func getRepKey(spaceID string) (uint64, error) {
	sepIdx := strings.Index(spaceID, ".")
	if sepIdx == -1 {
		return 0, ErrIncorrectSpaceID
	}
	return strconv.ParseUint(spaceID[sepIdx+1:], 36, 64)
}
