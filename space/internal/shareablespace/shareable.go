package shareablespace

import (
	"context"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/space/deletioncontroller"
	"github.com/anyproto/anytype-heart/space/internal/components/spacestatus"
	"github.com/anyproto/anytype-heart/space/internal/spacecontroller"
	"github.com/anyproto/anytype-heart/space/internal/spaceprocess/initial"
	"github.com/anyproto/anytype-heart/space/internal/spaceprocess/joiner"
	"github.com/anyproto/anytype-heart/space/internal/spaceprocess/loader"
	"github.com/anyproto/anytype-heart/space/internal/spaceprocess/mode"
	"github.com/anyproto/anytype-heart/space/internal/spaceprocess/offloader"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

var log = logger.NewNamed("common.space.shareablespace")

type statusUpdater interface {
	UpdateCoordinatorStatus()
}

type spaceController struct {
	spaceId           string
	app               *app.App
	status            spacestatus.SpaceStatus
	lastUpdatedStatus spaceinfo.AccountStatus
	updater           statusUpdater
	mx                sync.Mutex

	sm *mode.StateMachine
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

func NewSpaceController(
	spaceId string,
	info spaceinfo.SpacePersistentInfo,
	a *app.App) (spacecontroller.SpaceController, error) {
	newApp, err := makeStatusApp(a, spaceId)
	if err != nil {
		return nil, err
	}
	s := &spaceController{
		spaceId:           spaceId,
		status:            newApp.MustComponent(spacestatus.CName).(spacestatus.SpaceStatus),
		lastUpdatedStatus: info.GetAccountStatus(),
		app:               newApp,
	}

	// this is done for tests to not complicate them :-)
	if updater, ok := a.Component(deletioncontroller.CName).(statusUpdater); ok {
		s.updater = updater
	}
	sm, err := mode.NewStateMachine(s, log.With(zap.String("spaceId", spaceId)))
	if err != nil {
		return nil, err
	}
	s.sm = sm
	return s, nil
}

func (s *spaceController) SpaceId() string {
	return s.spaceId
}

func (s *spaceController) Start(ctx context.Context) error {
	defer func() {
		if s.updater != nil {
			s.updater.UpdateCoordinatorStatus()
		}
	}()
	switch s.status.GetPersistentStatus() {
	case spaceinfo.AccountStatusDeleted:
		_, err := s.sm.ChangeMode(mode.ModeOffloading)
		return err
	case spaceinfo.AccountStatusJoining:
		_, err := s.sm.ChangeMode(mode.ModeJoining)
		return err
	case spaceinfo.AccountStatusRemoving:
		_, err := s.sm.ChangeMode(mode.ModeOffloading)
		return err
	default:
		_, err := s.sm.ChangeMode(mode.ModeLoading)
		return err
	}
}

func (s *spaceController) Mode() mode.Mode {
	return s.sm.GetMode()
}

func (s *spaceController) Current() any {
	return s.sm.GetProcess()
}

func (s *spaceController) SetPersistentInfo(ctx context.Context, info spaceinfo.SpacePersistentInfo) error {
	err := s.status.SetPersistentInfo(info)
	if err != nil {
		return err
	}
	return s.Update()
}

func (s *spaceController) SetLocalInfo(ctx context.Context, info spaceinfo.SpaceLocalInfo) error {
	return s.status.SetLocalInfo(info)
}

func (s *spaceController) Update() error {
	s.mx.Lock()
	status := s.status.GetPersistentStatus()
	if s.lastUpdatedStatus == status {
		s.mx.Unlock()
		return nil
	}
	s.lastUpdatedStatus = status
	s.mx.Unlock()
	updateStatus := func(mode mode.Mode) error {
		_, err := s.sm.ChangeMode(mode)
		return err
	}
	switch status {
	case spaceinfo.AccountStatusDeleted:
		return updateStatus(mode.ModeOffloading)
	case spaceinfo.AccountStatusJoining:
		return updateStatus(mode.ModeJoining)
	case spaceinfo.AccountStatusRemoving:
		return updateStatus(mode.ModeOffloading)
	default:
		return updateStatus(mode.ModeLoading)
	}
}

func (s *spaceController) Process(md mode.Mode) mode.Process {
	switch md {
	case mode.ModeInitial:
		return initial.New()
	case mode.ModeLoading:
		return loader.New(s.app, loader.Params{
			SpaceId: s.spaceId,
		})
	case mode.ModeOffloading:
		return offloader.New(s.app)
	case mode.ModeJoining:
		return joiner.New(s.app, joiner.Params{
			SpaceId: s.spaceId,
			Status:  s.status,
			Log:     log,
		})
	default:
		return initial.New()
	}
}

func (s *spaceController) Close(ctx context.Context) error {
	s.sm.Close()
	// this closes status
	return s.app.Close(ctx)
}

func (s *spaceController) GetStatus() spaceinfo.AccountStatus {
	return s.status.GetPersistentStatus()
}

func (s *spaceController) GetLocalStatus() spaceinfo.LocalStatus {
	return s.status.GetLocalStatus()
}
