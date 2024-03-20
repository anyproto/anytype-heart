package space

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/anyproto/any-sync/accountservice"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/commonspace/object/tree/treechangeproto"
	"github.com/anyproto/any-sync/commonspace/spacesyncproto"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/gogo/protobuf/types"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/internal/spacecontroller"
	"github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/space/spacefactory"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

const CName = "client.space"

var log = logger.NewNamed(CName)

var (
	ErrIncorrectSpaceID = errors.New("incorrect space id")
	ErrSpaceNotExists   = errors.New("space not exists")
	ErrSpaceDeleted     = errors.New("space is deleted")
	ErrSpaceIsClosing   = errors.New("space is closing")
	ErrFailedToLoad     = errors.New("failed to load space")
)

func New() Service {
	return &service{}
}

type isNewAccount interface {
	IsNewAccount() bool
	app.Component
}

type Service interface {
	Create(ctx context.Context) (space clientspace.Space, err error)

	Join(ctx context.Context, id, aclHeadId string) error
	CancelLeave(ctx context.Context, id string) (err error)
	Get(ctx context.Context, id string) (space clientspace.Space, err error)
	Delete(ctx context.Context, id string) (err error)
	TechSpaceId() string
	TechSpace() *clientspace.TechSpace
	GetPersonalSpace(ctx context.Context) (space clientspace.Space, err error)
	GetTechSpace(ctx context.Context) (space clientspace.Space, err error)
	SpaceViewId(spaceId string) (spaceViewId string, err error)
	AccountMetadataSymKey() crypto.SymKey
	AccountMetadataPayload() []byte

	app.ComponentRunnable
}

type service struct {
	techSpace      *clientspace.TechSpace
	factory        spacefactory.SpaceFactory
	spaceCore      spacecore.SpaceCoreService
	accountService accountservice.Service
	config         *config.Config

	personalSpaceId        string
	techSpaceId            string
	newAccount             bool
	spaceControllers       map[string]spacecontroller.SpaceController
	waiting                map[string]controllerWaiter
	accountMetadataSymKey  crypto.SymKey
	accountMetadataPayload []byte
	repKey                 uint64

	mu        sync.Mutex
	ctx       context.Context
	ctxCancel context.CancelFunc
	isClosing atomic.Bool
}

func (s *service) Delete(ctx context.Context, id string) (err error) {
	if s.isClosing.Load() {
		return ErrSpaceIsClosing
	}
	s.mu.Lock()
	ctrl := s.spaceControllers[id]
	s.mu.Unlock()
	del, ok := ctrl.(spacecontroller.DeleteController)
	if !ok {
		return ErrSpaceNotExists
	}
	err = del.Delete(ctx)
	if err != nil {
		return fmt.Errorf("delete space: %w", err)
	}
	return nil
}

func (s *service) TechSpace() *clientspace.TechSpace {
	return s.techSpace
}

func (s *service) Init(a *app.App) (err error) {
	s.newAccount = app.MustComponent[isNewAccount](a).IsNewAccount()
	s.factory = app.MustComponent[spacefactory.SpaceFactory](a)
	s.spaceCore = app.MustComponent[spacecore.SpaceCoreService](a)
	s.accountService = app.MustComponent[accountservice.Service](a)
	s.config = app.MustComponent[*config.Config](a)
	s.spaceControllers = make(map[string]spacecontroller.SpaceController)
	s.waiting = make(map[string]controllerWaiter)
	s.personalSpaceId, err = s.spaceCore.DeriveID(context.Background(), spacecore.SpaceType)
	if err != nil {
		return
	}
	s.techSpaceId, err = s.spaceCore.DeriveID(context.Background(), spacecore.TechSpaceType)
	if err != nil {
		return
	}
	accountMetadata, metadataSymKey, err := deriveMetadata(s.accountService.Account().SignKey)
	if err != nil {
		return
	}
	s.accountMetadataSymKey = metadataSymKey
	s.accountMetadataPayload, err = accountMetadata.Marshal()
	if err != nil {
		return fmt.Errorf("marshal account metadata: %w", err)
	}

	s.repKey, err = getRepKey(s.personalSpaceId)
	s.ctx, s.ctxCancel = context.WithCancel(context.Background())
	return err
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) Run(ctx context.Context) (err error) {
	err = s.initMarketplaceSpace(ctx)
	if err != nil {
		return fmt.Errorf("init marketplace space: %w", err)
	}
	err = s.initTechSpace()
	if err != nil {
		return fmt.Errorf("init tech space: %w", err)
	}
	err = s.initPersonalSpace()
	if err != nil {
		if errors.Is(err, spacesyncproto.ErrSpaceMissing) || errors.Is(err, treechangeproto.ErrGetTree) {
			err = ErrSpaceNotExists
		}
		// fix for the users that have wrong network id stored in the folder
		err2 := s.config.ResetStoredNetworkId()
		if err2 != nil {
			log.Error("reset network id", zap.Error(err2))
		}
		return fmt.Errorf("init personal space: %w", err)
	}
	// only persist networkId after successful space init
	err = s.config.PersistAccountNetworkId()
	if err != nil {
		log.Error("persist network id to config", zap.Error(err))
	}
	return nil
}

func (s *service) Create(ctx context.Context) (clientspace.Space, error) {
	if s.isClosing.Load() {
		return nil, ErrSpaceIsClosing
	}
	return s.create(ctx)
}

func (s *service) Get(ctx context.Context, spaceId string) (sp clientspace.Space, err error) {
	if spaceId == s.techSpace.TechSpaceId() {
		return s.techSpace, nil
	}
	ctrl, err := s.getStatus(ctx, spaceId)
	if err != nil {
		return nil, err
	}
	return s.waitLoad(ctx, ctrl)
}

func (s *service) GetPersonalSpace(ctx context.Context) (sp clientspace.Space, err error) {
	return s.Get(ctx, s.personalSpaceId)
}

func (s *service) GetTechSpace(ctx context.Context) (sp clientspace.Space, err error) {
	return s.Get(ctx, s.techSpaceId)
}

func (s *service) IsPersonal(id string) bool {
	return s.personalSpaceId == id
}

func (s *service) OnViewUpdated(info spaceinfo.SpacePersistentInfo) {
	go func() {
		ctrl, err := s.startStatus(s.ctx, info)
		if err != nil && !errors.Is(err, ErrSpaceDeleted) {
			log.Warn("OnViewUpdated.startStatus error", zap.Error(err))
			return
		}
		err = ctrl.UpdateInfo(s.ctx, info)
		if err != nil {
			log.Warn("OnViewCreated.UpdateStatus error", zap.Error(err))
			return
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

func (s *service) AccountMetadataSymKey() crypto.SymKey {
	return s.accountMetadataSymKey
}

func (s *service) AccountMetadataPayload() []byte {
	return s.accountMetadataPayload
}

func (s *service) UpdateRemoteStatus(ctx context.Context, status spaceinfo.SpaceRemoteStatusInfo) error {
	s.mu.Lock()
	ctrl := s.spaceControllers[status.SpaceId]
	s.mu.Unlock()
	if ctrl == nil {
		return fmt.Errorf("no such space: %s", status.SpaceId)
	}
	err := ctrl.UpdateRemoteStatus(ctx, status)
	if err != nil {
		return fmt.Errorf("updateRemoteStatus: %w", err)
	}
	if !status.IsOwned && status.RemoteStatus == spaceinfo.RemoteStatusDeleted {
		return ctrl.SetInfo(ctx, spaceinfo.SpacePersistentInfo{
			SpaceID:       status.SpaceId,
			AccountStatus: spaceinfo.AccountStatusRemoving,
		})
	}
	return nil
}

func (s *service) SpaceViewId(spaceId string) (spaceViewId string, err error) {
	return s.techSpace.SpaceViewId(spaceId)
}

func (s *service) Close(ctx context.Context) error {
	if s.ctxCancel != nil {
		s.ctxCancel()
	}
	s.isClosing.Store(true)
	s.mu.Lock()
	ctrls := make([]spacecontroller.SpaceController, 0, len(s.spaceControllers))
	for _, ctrl := range s.spaceControllers {
		ctrls = append(ctrls, ctrl)
	}
	s.mu.Unlock()

	for _, ctrl := range ctrls {
		err := ctrl.Close(ctx)
		if err != nil {
			log.Error("close space", zap.String("spaceId", ctrl.SpaceId()), zap.Error(err))
		}
	}
	err := s.techSpace.Close(ctx)
	if err != nil {
		log.Error("close tech space", zap.Error(err))
	}
	return nil
}

func (s *service) AllSpaceIds() (ids []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id := range s.spaceControllers {
		if id == addr.AnytypeMarketplaceWorkspace {
			continue
		}
		ids = append(ids, id)
	}
	return
}

func (s *service) TechSpaceId() string {
	return s.techSpace.TechSpaceId()
}
