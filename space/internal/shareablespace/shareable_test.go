package shareablespace

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/space/internal/components/spacestatus"
	"github.com/anyproto/anytype-heart/space/internal/spaceprocess/initial"
	"github.com/anyproto/anytype-heart/space/internal/spaceprocess/mode"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

type modeRegister struct {
	modes []mode.Mode
	sync.Mutex
}

func (m *modeRegister) register(mode mode.Mode) {
	m.Lock()
	m.modes = append(m.modes, mode)
	m.Unlock()
}

type spaceStatusMock struct {
	sync.Mutex
	spaceId           string
	localStatus       spaceinfo.LocalStatus
	remoteStatus      spaceinfo.RemoteStatus
	accountStatus     spaceinfo.AccountStatus
	persistentUpdater func(status spaceinfo.AccountStatus)
}

func (s *spaceStatusMock) Init(a *app.App) (err error) {
	return nil
}

func (s *spaceStatusMock) Name() (name string) {
	return spacestatus.CName
}

func (s *spaceStatusMock) Lock() {
	s.Mutex.Lock()
}

func (s *spaceStatusMock) Unlock() {
	s.Mutex.Unlock()
}

func (s *spaceStatusMock) SpaceId() string {
	return s.spaceId
}

func (s *spaceStatusMock) GetLocalStatus() spaceinfo.LocalStatus {
	return s.localStatus
}

func (s *spaceStatusMock) GetRemoteStatus() spaceinfo.RemoteStatus {
	return s.remoteStatus
}

func (s *spaceStatusMock) GetPersistentStatus() spaceinfo.AccountStatus {
	return s.accountStatus
}

func (s *spaceStatusMock) UpdatePersistentStatus(ctx context.Context, status spaceinfo.AccountStatus) {
	s.accountStatus = status
}

func (s *spaceStatusMock) SetRemoteStatus(ctx context.Context, status spaceinfo.RemoteStatus) error {
	s.remoteStatus = status
	return nil
}

func (s *spaceStatusMock) SetPersistentStatus(ctx context.Context, status spaceinfo.AccountStatus) (err error) {
	s.accountStatus = status
	if s.persistentUpdater != nil {
		s.persistentUpdater(status)
	}
	return nil
}

func (s *spaceStatusMock) SetLocalStatus(ctx context.Context, status spaceinfo.LocalStatus) error {
	s.localStatus = status
	return nil
}

func (s *spaceStatusMock) SetLocalInfo(ctx context.Context, info spaceinfo.SpaceLocalInfo) (err error) {
	s.localStatus = info.LocalStatus
	s.remoteStatus = info.RemoteStatus
	return nil
}

type inviting struct {
	inviteReceived atomic.Bool
	status         spacestatus.SpaceStatus
	reg            *modeRegister
}

func newInviting(status spacestatus.SpaceStatus, reg *modeRegister) mode.Process {
	return &inviting{
		status: status,
		reg:    reg,
	}
}

func (i *inviting) Start(ctx context.Context) error {
	go func() {
		i.inviteReceived.Store(true)
		i.status.Lock()
		i.status.SetPersistentStatus(ctx, spaceinfo.AccountStatusLoading)
		i.status.Unlock()
	}()
	i.reg.register(mode.ModeInviting)
	return nil
}

func (i *inviting) Close(ctx context.Context) error {
	return nil
}

func (i *inviting) CanTransition(next mode.Mode) bool {
	if next == mode.ModeLoading && !i.inviteReceived.Load() {
		return false
	}
	return true
}

type loading struct {
	status spacestatus.SpaceStatus
	reg    *modeRegister
}

func newLoading(status spacestatus.SpaceStatus, reg *modeRegister) mode.Process {
	return &loading{
		status: status,
		reg:    reg,
	}
}

func (l *loading) Start(ctx context.Context) error {
	l.reg.register(mode.ModeLoading)
	return nil
}

func (l *loading) Close(ctx context.Context) error {
	return nil
}

func (l *loading) CanTransition(next mode.Mode) bool {
	return true
}

type offloading struct {
	status spacestatus.SpaceStatus
	reg    *modeRegister
}

func newOffloading(status spacestatus.SpaceStatus, reg *modeRegister) mode.Process {
	return &offloading{
		status: status,
		reg:    reg,
	}
}

func (l *offloading) Start(ctx context.Context) error {
	l.reg.register(mode.ModeOffloading)
	return nil
}

func (l *offloading) Close(ctx context.Context) error {
	return nil
}

func (l *offloading) CanTransition(next mode.Mode) bool {
	return false
}

type factory struct {
	status spacestatus.SpaceStatus
	reg    *modeRegister
}

func (f factory) Process(md mode.Mode) mode.Process {
	switch md {
	case mode.ModeInitial:
		return initial.New()
	case mode.ModeInviting:
		return newInviting(f.status, f.reg)
	case mode.ModeLoading:
		return newLoading(f.status, f.reg)
	case mode.ModeOffloading:
		return newOffloading(f.status, f.reg)
	default:
		panic("unhandled default case")
	}
}

type fixture struct {
	f    factory
	s    *spaceStatusMock
	ctrl *spaceController
	reg  *modeRegister
}

func newFixture(t *testing.T, startStatus spaceinfo.AccountStatus) *fixture {
	reg := &modeRegister{}
	s := &spaceStatusMock{
		spaceId:       "spaceId",
		accountStatus: startStatus,
		Mutex:         sync.Mutex{},
	}
	f := factory{
		status: s,
		reg:    reg,
	}
	sm, err := mode.NewStateMachine(f, log)
	require.NoError(t, err)
	controller := &spaceController{
		spaceId:           "spaceId",
		status:            s,
		app:               &app.App{},
		lastUpdatedStatus: startStatus,
		sm:                sm,
	}
	s.persistentUpdater = func(status spaceinfo.AccountStatus) {
		go func() {
			err := controller.UpdateStatus(context.Background(), status)
			require.NoError(t, err)
		}()
	}
	return &fixture{
		f: factory{
			status: s,
		},
		s:    s,
		ctrl: controller,
		reg:  reg,
	}
}

func (fx *fixture) stop() {
	fx.ctrl.sm.Close()
}

func TestSpaceController_InvitingLoading(t *testing.T) {
	fx := newFixture(t, spaceinfo.AccountStatusInviting)
	defer fx.stop()
	err := fx.ctrl.Start(context.Background())
	require.NoError(t, err)
	require.Equal(t, mode.ModeInviting, fx.ctrl.Mode())
	time.Sleep(100 * time.Millisecond)
	fx.reg.Lock()
	defer fx.reg.Unlock()
	require.Equal(t, []mode.Mode{mode.ModeInviting, mode.ModeLoading}, fx.reg.modes)
}

func TestSpaceController_LoadingDeleting(t *testing.T) {
	fx := newFixture(t, spaceinfo.AccountStatusUnknown)
	defer fx.stop()
	err := fx.ctrl.Start(context.Background())
	require.NoError(t, err)
	require.Equal(t, mode.ModeLoading, fx.ctrl.Mode())
	err = fx.ctrl.UpdateStatus(context.Background(), spaceinfo.AccountStatusDeleted)
	require.NoError(t, err)
	fx.reg.Lock()
	defer fx.reg.Unlock()
	require.Equal(t, []mode.Mode{mode.ModeLoading, mode.ModeOffloading}, fx.reg.modes)
}

func TestSpaceController_LoadingDeletingMultipleWaiters(t *testing.T) {
	fx := newFixture(t, spaceinfo.AccountStatusUnknown)
	defer fx.stop()
	err := fx.ctrl.Start(context.Background())
	require.NoError(t, err)
	require.Equal(t, mode.ModeLoading, fx.ctrl.Mode())
	wg := sync.WaitGroup{}
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			err := fx.ctrl.UpdateStatus(context.Background(), spaceinfo.AccountStatusDeleted)
			require.NoError(t, err)
			wg.Done()
		}()
	}
	wg.Wait()
	fx.reg.Lock()
	defer fx.reg.Unlock()
	require.Equal(t, []mode.Mode{mode.ModeLoading, mode.ModeOffloading}, fx.reg.modes)
}

func TestSpaceController_Deleting(t *testing.T) {
	fx := newFixture(t, spaceinfo.AccountStatusDeleted)
	defer fx.stop()
	err := fx.ctrl.Start(context.Background())
	require.NoError(t, err)
	require.Equal(t, mode.ModeOffloading, fx.ctrl.Mode())
	time.Sleep(100 * time.Millisecond)
	fx.reg.Lock()
	defer fx.reg.Unlock()
	require.Equal(t, []mode.Mode{mode.ModeOffloading}, fx.reg.modes)
}

func TestSpaceController_DeletingInvalid(t *testing.T) {
	fx := newFixture(t, spaceinfo.AccountStatusDeleted)
	defer fx.stop()
	err := fx.ctrl.Start(context.Background())
	require.NoError(t, err)
	require.Equal(t, mode.ModeOffloading, fx.ctrl.Mode())
	err = fx.ctrl.UpdateStatus(context.Background(), spaceinfo.AccountStatusLoading)
	require.Error(t, err)
	fx.reg.Lock()
	defer fx.reg.Unlock()
	require.Equal(t, []mode.Mode{mode.ModeOffloading}, fx.reg.modes)
}
