package shareablespace

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/internal/components/spacestatus"
	"github.com/anyproto/anytype-heart/space/internal/spaceprocess/initial"
	"github.com/anyproto/anytype-heart/space/internal/spaceprocess/mode"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/space/techspace"
)

func TestSpaceController_InvitingLoading(t *testing.T) {
	fx := newFixture(t, spaceinfo.AccountStatusJoining)
	defer fx.stop()
	err := fx.ctrl.Start(context.Background())
	require.NoError(t, err)
	require.Equal(t, mode.ModeJoining, fx.ctrl.Mode())
	time.Sleep(100 * time.Millisecond)
	fx.reg.Lock()
	defer fx.reg.Unlock()
	require.Equal(t, []mode.Mode{mode.ModeJoining, mode.ModeLoading}, fx.reg.modes)
}

func TestSpaceController_LoadingDeleting(t *testing.T) {
	fx := newFixture(t, spaceinfo.AccountStatusUnknown)
	defer fx.stop()
	err := fx.ctrl.Start(context.Background())
	require.NoError(t, err)
	require.Equal(t, mode.ModeLoading, fx.ctrl.Mode())
	err = fx.ctrl.SetPersistentInfo(context.Background(), makePersistentInfo("spaceId", spaceinfo.AccountStatusDeleted))
	err = fx.ctrl.Update()
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
			err := fx.ctrl.SetPersistentInfo(context.Background(), makePersistentInfo("spaceId", spaceinfo.AccountStatusDeleted))
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
	err = fx.ctrl.SetPersistentInfo(context.Background(), makePersistentInfo("spaceId", spaceinfo.AccountStatusActive))
	require.Error(t, err)
	fx.reg.Lock()
	defer fx.reg.Unlock()
	require.Equal(t, []mode.Mode{mode.ModeOffloading}, fx.reg.modes)
}

func makePersistentInfo(spaceId string, status spaceinfo.AccountStatus) spaceinfo.SpacePersistentInfo {
	info := spaceinfo.NewSpacePersistentInfo(spaceId)
	info.SetAccountStatus(status)
	return info
}

type modeRegister struct {
	modes []mode.Mode
	sync.Mutex
}

func (m *modeRegister) register(mode mode.Mode) {
	m.Lock()
	m.modes = append(m.modes, mode)
	m.Unlock()
}

type spaceStatusStub struct {
	spaceId           string
	localStatus       spaceinfo.LocalStatus
	remoteStatus      spaceinfo.RemoteStatus
	accountStatus     spaceinfo.AccountStatus
	persistentUpdater func(status spaceinfo.AccountStatus)
	sync.Mutex
}

func (s *spaceStatusStub) Init(a *app.App) (err error) {
	return nil
}

func (s *spaceStatusStub) Name() (name string) {
	return spacestatus.CName
}

func (s *spaceStatusStub) SpaceId() string {
	return s.spaceId
}

func (s *spaceStatusStub) GetLocalStatus() spaceinfo.LocalStatus {
	s.Lock()
	defer s.Unlock()
	return s.localStatus
}

func (s *spaceStatusStub) SetOwner(ownerIdentity string, createdDate int64) (err error) {
	return
}

func (s *spaceStatusStub) GetRemoteStatus() spaceinfo.RemoteStatus {
	s.Lock()
	defer s.Unlock()
	return s.remoteStatus
}

func (s *spaceStatusStub) GetPersistentStatus() spaceinfo.AccountStatus {
	s.Lock()
	defer s.Unlock()
	return s.accountStatus
}

func (s *spaceStatusStub) Run(ctx context.Context) (err error) {
	return nil
}

func (s *spaceStatusStub) Close(ctx context.Context) (err error) {
	return nil
}

func (s *spaceStatusStub) SetPersistentStatus(status spaceinfo.AccountStatus) (err error) {
	s.Lock()
	defer s.Unlock()
	s.accountStatus = status
	if s.persistentUpdater != nil {
		s.persistentUpdater(status)
	}
	return nil
}

func (s *spaceStatusStub) SetPersistentInfo(info spaceinfo.SpacePersistentInfo) (err error) {
	s.Lock()
	defer s.Unlock()
	s.accountStatus = info.GetAccountStatus()
	return
}

func (s *spaceStatusStub) SetLocalStatus(status spaceinfo.LocalStatus) error {
	s.Lock()
	defer s.Unlock()
	s.localStatus = status
	return nil
}

func (s *spaceStatusStub) SetLocalInfo(info spaceinfo.SpaceLocalInfo) (err error) {
	s.Lock()
	defer s.Unlock()
	s.localStatus = info.GetLocalStatus()
	return
}

func (s *spaceStatusStub) SetAccessType(status spaceinfo.AccessType) (err error) {
	return
}

func (s *spaceStatusStub) SetAclInfo(isAclEmpty bool, pushKey crypto.PrivKey, pushEncryptionKey crypto.SymKey, spaceJoinedDate int64) (err error) {
	return
}

func (s *spaceStatusStub) GetLatestAclHeadId() string {
	return ""
}

func (s *spaceStatusStub) SetMyParticipantStatus(st model.ParticipantStatus) (err error) {
	return nil
}

func (s *spaceStatusStub) GetSpaceView() techspace.SpaceView {
	return nil
}

var _ spacestatus.SpaceStatus = (*spaceStatusStub)(nil)

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
		_ = i.status.SetPersistentStatus(spaceinfo.AccountStatusActive)
	}()
	i.reg.register(mode.ModeJoining)
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
	case mode.ModeJoining:
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
	s    *spaceStatusStub
	ctrl *spaceController
	reg  *modeRegister
}

func newFixture(t *testing.T, startStatus spaceinfo.AccountStatus) *fixture {
	reg := &modeRegister{}
	s := &spaceStatusStub{
		spaceId:       "spaceId",
		accountStatus: startStatus,
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
			err := controller.Update()
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
