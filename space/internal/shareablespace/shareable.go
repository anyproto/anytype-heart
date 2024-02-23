package shareablespace

import (
	"context"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/space/internal/components/spacestatus"
	"github.com/anyproto/anytype-heart/space/internal/spacecontroller"
	"github.com/anyproto/anytype-heart/space/internal/spaceprocess/initial"
	"github.com/anyproto/anytype-heart/space/internal/spaceprocess/joiner"
	"github.com/anyproto/anytype-heart/space/internal/spaceprocess/loader"
	"github.com/anyproto/anytype-heart/space/internal/spaceprocess/mode"
	"github.com/anyproto/anytype-heart/space/internal/spaceprocess/offloader"
	"github.com/anyproto/anytype-heart/space/internal/spaceprocess/remover"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

var log = logger.NewNamed("common.space.shareablespace")

type spaceController struct {
	spaceId           string
	app               *app.App
	status            spacestatus.SpaceStatus
	lastUpdatedStatus spaceinfo.AccountStatus

	sm *mode.StateMachine
}

func NewSpaceController(
	spaceId string,
	status spaceinfo.AccountStatus,
	a *app.App) (spacecontroller.SpaceController, error) {
	s := &spaceController{
		spaceId:           spaceId,
		status:            spacestatus.New(spaceId, status),
		lastUpdatedStatus: status,
		app:               a,
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
	switch s.status.GetPersistentStatus() {
	case spaceinfo.AccountStatusDeleted:
		_, err := s.sm.ChangeMode(mode.ModeOffloading)
		return err
	case spaceinfo.AccountStatusJoining:
		_, err := s.sm.ChangeMode(mode.ModeJoining)
		return err
	case spaceinfo.AccountStatusRemoving:
		_, err := s.sm.ChangeMode(mode.ModeRemoving)
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

func (s *spaceController) SetStatus(ctx context.Context, status spaceinfo.AccountStatus) error {
	s.status.Lock()
	err := s.status.SetPersistentStatus(ctx, status)
	if err != nil {
		s.status.Unlock()
		return err
	}
	s.status.Unlock()
	return s.UpdateStatus(ctx, status)
}

func (s *spaceController) UpdateStatus(ctx context.Context, status spaceinfo.AccountStatus) error {
	s.status.Lock()
	if s.lastUpdatedStatus == status || (s.lastUpdatedStatus == spaceinfo.AccountStatusDeleted && status == spaceinfo.AccountStatusRemoving) {
		s.status.Unlock()
		return nil
	}
	s.lastUpdatedStatus = status
	s.status.Unlock()
	updateStatus := func(mode mode.Mode) error {
		s.status.Lock()
		s.status.UpdatePersistentStatus(ctx, status)
		s.status.Unlock()
		_, err := s.sm.ChangeMode(mode)
		return err
	}
	switch status {
	case spaceinfo.AccountStatusDeleted:
		return updateStatus(mode.ModeOffloading)
	case spaceinfo.AccountStatusJoining:
		return updateStatus(mode.ModeJoining)
	case spaceinfo.AccountStatusRemoving:
		return updateStatus(mode.ModeRemoving)
	default:
		return updateStatus(mode.ModeLoading)
	}
}

func (s *spaceController) UpdateRemoteStatus(ctx context.Context, status spaceinfo.RemoteStatus) error {
	s.status.Lock()
	defer s.status.Unlock()
	return s.status.SetRemoteStatus(ctx, status)
}

func (s *spaceController) Delete(ctx context.Context) error {
	offloading, err := s.sm.ChangeMode(mode.ModeOffloading)
	if err != nil {
		return err
	}
	of := offloading.(offloader.Offloader)
	return of.WaitOffload(ctx)
}

func (s *spaceController) Process(md mode.Mode) mode.Process {
	switch md {
	case mode.ModeInitial:
		return initial.New()
	case mode.ModeLoading:
		return loader.New(s.app, loader.Params{
			SpaceId: s.spaceId,
			Status:  s.status,
		})
	case mode.ModeOffloading:
		return offloader.New(s.app, offloader.Params{
			Status: s.status,
		})
	case mode.ModeRemoving:
		return remover.New(s.app, remover.Params{
			SpaceId: s.spaceId,
			Status:  s.status,
		})
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
	return nil
}
