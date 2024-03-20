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
	info spaceinfo.SpacePersistentInfo,
	a *app.App) (spacecontroller.SpaceController, error) {
	s := &spaceController{
		spaceId:           spaceId,
		status:            spacestatus.New(spaceId, info.AccountStatus, info.AclHeadId),
		lastUpdatedStatus: info.AccountStatus,
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

func (s *spaceController) SetInfo(ctx context.Context, info spaceinfo.SpacePersistentInfo) error {
	s.status.Lock()
	err := s.status.SetPersistentInfo(ctx, info)
	if err != nil {
		s.status.Unlock()
		return err
	}
	s.status.Unlock()
	return s.UpdateInfo(ctx, info)
}

func (s *spaceController) UpdateInfo(ctx context.Context, info spaceinfo.SpacePersistentInfo) error {
	s.status.Lock()
	if s.lastUpdatedStatus == info.AccountStatus || (s.lastUpdatedStatus == spaceinfo.AccountStatusDeleted && info.AccountStatus == spaceinfo.AccountStatusRemoving) {
		s.status.Unlock()
		return nil
	}
	s.lastUpdatedStatus = info.AccountStatus
	s.status.Unlock()
	updateStatus := func(mode mode.Mode) error {
		s.status.Lock()
		s.status.UpdatePersistentInfo(ctx, info)
		s.status.Unlock()
		_, err := s.sm.ChangeMode(mode)
		return err
	}
	switch info.AccountStatus {
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

func (s *spaceController) UpdateRemoteStatus(ctx context.Context, status spaceinfo.SpaceRemoteStatusInfo) error {
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
