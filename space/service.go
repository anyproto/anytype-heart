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
	"github.com/anyproto/any-sync/coordinator/coordinatorclient"
	"github.com/gogo/protobuf/types"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/internal/spacecontroller"
	"github.com/anyproto/anytype-heart/space/internal/spaceprocess/mode"
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

	Join(ctx context.Context, id string) (err error)
	Get(ctx context.Context, id string) (space clientspace.Space, err error)
	Delete(ctx context.Context, id string) (err error)
	GetPersonalSpace(ctx context.Context) (space clientspace.Space, err error)
	SpaceViewId(spaceId string) (spaceViewId string, err error)
	AccountMetadata() []byte

	app.ComponentRunnable
}

type service struct {
	techSpace      *clientspace.TechSpace
	factory        spacefactory.SpaceFactory
	spaceCore      spacecore.SpaceCoreService
	accountService accountservice.Service

	delController *deletionController

	personalSpaceId  string
	newAccount       bool
	spaceControllers map[string]spacecontroller.SpaceController
	waiting          map[string]controllerWaiter
	metadataPayload  []byte
	repKey           uint64

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

func (s *service) Init(a *app.App) (err error) {
	s.newAccount = app.MustComponent[isNewAccount](a).IsNewAccount()
	coordClient := app.MustComponent[coordinatorclient.CoordinatorClient](a)
	s.delController = newDeletionController(s, coordClient)
	s.factory = app.MustComponent[spacefactory.SpaceFactory](a)
	s.spaceCore = app.MustComponent[spacecore.SpaceCoreService](a)
	s.accountService = app.MustComponent[accountservice.Service](a)
	s.spaceControllers = make(map[string]spacecontroller.SpaceController)
	s.waiting = make(map[string]controllerWaiter)
	s.personalSpaceId, err = s.spaceCore.DeriveID(context.Background(), spacecore.SpaceType)
	if err != nil {
		return
	}
	s.metadataPayload, err = deriveMetadata(s.accountService.Account().SignKey)
	if err != nil {
		return
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
		return fmt.Errorf("init personal space: %w", err)
	}
	s.delController.Run()
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
	ctrl, err := s.startStatus(ctx, spaceId, spaceinfo.AccountStatusUnknown)
	if err != nil {
		return nil, err
	}
	return s.waitLoad(ctx, ctrl)
}

func (s *service) GetPersonalSpace(ctx context.Context) (sp clientspace.Space, err error) {
	return s.Get(ctx, s.personalSpaceId)
}

func (s *service) IsPersonal(id string) bool {
	return s.personalSpaceId == id
}

func (s *service) OnViewUpdated(info spaceinfo.SpacePersistentInfo) {
	go func() {
		ctrl, err := s.startStatus(s.ctx, info.SpaceID, info.AccountStatus)
		if err != nil && !errors.Is(err, ErrSpaceDeleted) {
			log.Warn("OnViewUpdated.startStatus error", zap.Error(err))
			return
		}
		err = ctrl.UpdateStatus(s.ctx, info.AccountStatus)
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

func (s *service) AccountMetadata() []byte {
	return s.metadataPayload
}

func (s *service) updateRemoteStatus(ctx context.Context, spaceId string, status spaceinfo.RemoteStatus) error {
	s.mu.Lock()
	ctrl := s.spaceControllers[spaceId]
	s.mu.Unlock()
	if ctrl == nil {
		return fmt.Errorf("no such space: %s", spaceId)
	}
	err := ctrl.UpdateRemoteStatus(ctx, status)
	if err != nil {
		return fmt.Errorf("updateRemoteStatus: %w", err)
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
	s.delController.Close()
	return nil
}

func (s *service) allIDs() (ids []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, sc := range s.spaceControllers {
		if id == addr.AnytypeMarketplaceWorkspace || sc.Mode() != mode.ModeLoading {
			continue
		}
		ids = append(ids, id)
	}
	return
}
