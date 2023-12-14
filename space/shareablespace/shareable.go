package shareablespace

import (
	"context"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/space/components/spacecontroller"
	"github.com/anyproto/anytype-heart/space/components/spacestatus"
	"github.com/anyproto/anytype-heart/space/process/initial"
	"github.com/anyproto/anytype-heart/space/process/loader"
	"github.com/anyproto/anytype-heart/space/process/modechanger"
	"github.com/anyproto/anytype-heart/space/process/offloader"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

type spaceController struct {
	spaceId     string
	app         *app.App
	justCreated bool
	status      *spacestatus.SpaceStatus

	sm *modechanger.StateMachine
}

func NewSpaceController(
	spaceId string,
	justCreated bool,
	status spaceinfo.AccountStatus,
	a *app.App) (spacecontroller.SpaceController, error) {
	s := &spaceController{
		spaceId:     spaceId,
		justCreated: justCreated,
		status:      spacestatus.New(spaceId, status),
		app:         a,
	}
	sm, err := modechanger.NewStateMachine(s)
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
	switch s.status.AccountStatus {
	case spaceinfo.AccountStatusDeleted:
		_, err := s.sm.ChangeMode(modechanger.ModeOffloading)
		return err
	default:
		_, err := s.sm.ChangeMode(modechanger.ModeLoading)
		return err
	}
}

func (s *spaceController) Mode() modechanger.Mode {
	return s.sm.GetMode()
}

func (s *spaceController) Current() any {
	return s.sm.GetProcess()
}

func (s *spaceController) UpdateStatus(ctx context.Context, status spaceinfo.AccountStatus) error {
	s.status.Lock()
	curStatus := s.status.GetPersistentStatus()
	if curStatus == status {
		s.status.Unlock()
		return nil
	}
	s.status.Unlock()
	switch status {
	case spaceinfo.AccountStatusDeleted:
		s.status.Lock()
		s.status.UpdatePersistentStatus(ctx, status)
		s.status.Unlock()
		_, err := s.sm.ChangeMode(modechanger.ModeOffloading)
		return err
	default:
		return modechanger.ErrInvalidTransition
	}
}

func (s *spaceController) UpdateRemoteStatus(ctx context.Context, status spaceinfo.RemoteStatus) error {
	s.status.Lock()
	defer s.status.Unlock()
	return s.status.SetRemoteStatus(ctx, status)
}

func (s *spaceController) Delete(ctx context.Context) error {
	offloading, err := s.sm.ChangeMode(modechanger.ModeOffloading)
	if err != nil {
		return err
	}
	of := offloading.(offloader.Offloader)
	return of.WaitOffload(ctx)
}

func (s *spaceController) Process(mode modechanger.Mode) modechanger.Process {
	switch mode {
	case modechanger.ModeInitial:
		return initial.New()
	case modechanger.ModeLoading:
		return loader.New(s.app, loader.Params{
			JustCreated: s.justCreated,
			SpaceId:     s.spaceId,
			Status:      s.status,
		})
	case modechanger.ModeOffloading:
		return offloader.New(s.app, offloader.Params{
			Status: s.status,
		})
	default:
		panic("unknown mode")
	}
}

func (s *spaceController) Close(ctx context.Context) error {
	s.sm.Close()
	return nil
}
