package space

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/coordinator/coordinatorclient"
	"github.com/gogo/protobuf/types"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/internal/spacecontroller"
	"github.com/anyproto/anytype-heart/space/internal/spaceprocess/loader"
	"github.com/anyproto/anytype-heart/space/spacefactory"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

const CName = "client.space"

var log = logger.NewNamed(CName)

var (
	ErrIncorrectSpaceID = errors.New("incorrect space id")
	ErrSpaceNotExists   = errors.New("space not exists")
	ErrSpaceDeleted     = errors.New("space is deleted")
	ErrStatusUnkown     = errors.New("space status is unknown")
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

	Get(ctx context.Context, id string) (space clientspace.Space, err error)
	Delete(ctx context.Context, id string) (err error)
	GetPersonalSpace(ctx context.Context) (space clientspace.Space, err error)
	SpaceViewId(spaceId string) (spaceViewId string, err error)

	app.ComponentRunnable
}

type service struct {
	techSpace *clientspace.TechSpace
	factory   spacefactory.SpaceFactory

	delController *deletionController

	personalSpaceID  string
	newAccount       bool
	spaceControllers map[string]spacecontroller.SpaceController

	mu        sync.Mutex
	ctx       context.Context
	ctxCancel context.CancelFunc
}

func (s *service) Delete(ctx context.Context, id string) (err error) {
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
	s.spaceControllers = make(map[string]spacecontroller.SpaceController)
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
		return fmt.Errorf("init personal space: %w", err)
	}
	s.delController.Run()
	return nil
}

func (s *service) Create(ctx context.Context) (clientspace.Space, error) {
	ctrl, err := s.factory.CreateShareableSpace(ctx)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.spaceControllers[ctrl.SpaceId()] = ctrl
	s.mu.Unlock()
	return ctrl.Current().(loader.LoadWaiter).WaitLoad(ctx)
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
	return s.Get(ctx, s.personalSpaceID)
}

func (s *service) IsPersonal(id string) bool {
	return s.personalSpaceID == id
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

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, ctrl := range s.spaceControllers {
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
	for id, _ := range s.spaceControllers {
		if id != addr.AnytypeMarketplaceWorkspace {
			ids = append(ids, id)
		}
	}
	return
}
