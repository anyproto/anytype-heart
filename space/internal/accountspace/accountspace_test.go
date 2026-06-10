package accountspace

import (
	"context"
	"errors"
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
	// the joining stub flips the status to Active, which must converge to loading
	fx.waitModes(t, mode.ModeJoining, mode.ModeLoading)
}

func TestSpaceController_LoadingDeleting(t *testing.T) {
	fx := newFixture(t, spaceinfo.AccountStatusUnknown)
	defer fx.stop()
	err := fx.ctrl.Start(context.Background())
	require.NoError(t, err)
	require.Equal(t, mode.ModeLoading, fx.ctrl.Mode())
	err = fx.ctrl.SetPersistentInfo(context.Background(), makePersistentInfo("spaceId", spaceinfo.AccountStatusDeleted))
	require.NoError(t, err)
	fx.waitModes(t, mode.ModeLoading, mode.ModeOffloading)
}

func TestSpaceController_LoadingDeletingMultipleUpdates(t *testing.T) {
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
	fx.waitModes(t, mode.ModeLoading, mode.ModeOffloading)
}

func TestSpaceController_Deleting(t *testing.T) {
	fx := newFixture(t, spaceinfo.AccountStatusDeleted)
	defer fx.stop()
	err := fx.ctrl.Start(context.Background())
	require.NoError(t, err)
	require.Equal(t, mode.ModeOffloading, fx.ctrl.Mode())
	fx.waitModes(t, mode.ModeOffloading)
}

func TestSpaceController_DeletedThenActiveReloads(t *testing.T) {
	// offloading is not terminal: when the status returns to Active
	// (e.g. CancelLeave), the reconciler loads the space again
	fx := newFixture(t, spaceinfo.AccountStatusDeleted)
	defer fx.stop()
	err := fx.ctrl.Start(context.Background())
	require.NoError(t, err)
	require.Equal(t, mode.ModeOffloading, fx.ctrl.Mode())
	err = fx.ctrl.SetPersistentInfo(context.Background(), makePersistentInfo("spaceId", spaceinfo.AccountStatusActive))
	require.NoError(t, err)
	fx.waitModes(t, mode.ModeOffloading, mode.ModeLoading)
}

func TestSpaceController_LatestWinsCoalescing(t *testing.T) {
	fx := newFixture(t, spaceinfo.AccountStatusUnknown)
	defer fx.stop()
	err := fx.ctrl.Start(context.Background())
	require.NoError(t, err)
	// rapid flips must converge on the last written status without losing it
	for i := 0; i < 5; i++ {
		require.NoError(t, fx.ctrl.SetPersistentInfo(context.Background(), makePersistentInfo("spaceId", spaceinfo.AccountStatusDeleted)))
		require.NoError(t, fx.ctrl.SetPersistentInfo(context.Background(), makePersistentInfo("spaceId", spaceinfo.AccountStatusActive)))
	}
	require.NoError(t, fx.ctrl.SetPersistentInfo(context.Background(), makePersistentInfo("spaceId", spaceinfo.AccountStatusDeleted)))
	require.Eventually(t, func() bool {
		return fx.ctrl.Mode() == mode.ModeOffloading
	}, time.Second, 5*time.Millisecond)
}

func TestSpaceController_StartFailureSurfacesErrorAndRetries(t *testing.T) {
	startErr := errors.New("process start failed")
	fx := newFixture(t, spaceinfo.AccountStatusUnknown)
	defer fx.stop()
	fx.f.failLoading.Store(&startErr)

	err := fx.ctrl.Start(context.Background())
	require.ErrorIs(t, err, startErr)
	require.Equal(t, mode.ModeInitial, fx.ctrl.Mode())

	// next input change clears the failure and retries
	fx.f.failLoading.Store(nil)
	err = fx.ctrl.SetPersistentInfo(context.Background(), makePersistentInfo("spaceId", spaceinfo.AccountStatusActive))
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		return fx.ctrl.Mode() == mode.ModeLoading
	}, time.Second, 5*time.Millisecond)
}

func TestSpaceController_DormantStaysIdleUntilDemand(t *testing.T) {
	fx := newFixture(t, spaceinfo.AccountStatusUnknown)
	defer fx.stop()
	// registration alone (no Start) must not load anything
	time.Sleep(50 * time.Millisecond)
	require.Equal(t, mode.ModeInitial, fx.ctrl.Mode())
	require.Empty(t, fx.reg.snapshot())

	fx.ctrl.Demand()
	fx.waitModes(t, mode.ModeLoading)
}

func TestSpaceController_DormantOffloadsWithoutDemand(t *testing.T) {
	// deletion outranks demand: a never-demanded space still offloads
	fx := newFixture(t, spaceinfo.AccountStatusDeleted)
	defer fx.stop()
	fx.waitModes(t, mode.ModeOffloading)
}

func TestSpaceController_WaitLoadFailsWhenOffloading(t *testing.T) {
	fx := newFixture(t, spaceinfo.AccountStatusDeleted)
	defer fx.stop()
	fx.waitModes(t, mode.ModeOffloading)
	_, err := fx.ctrl.WaitLoad(context.Background())
	require.ErrorIs(t, err, ErrModeUnreachable)
}

func TestSpaceController_CloseUnblocksWaiters(t *testing.T) {
	fx := newFixture(t, spaceinfo.AccountStatusUnknown)
	fx.f.blockLoading = make(chan struct{})

	startDone := make(chan error, 1)
	go func() {
		startDone <- fx.ctrl.Start(context.Background())
	}()
	// wait until the loading process is blocked in Start
	require.Eventually(t, func() bool {
		return fx.f.loadingStarted.Load()
	}, time.Second, time.Millisecond)

	closeDone := make(chan error, 1)
	go func() {
		closeDone <- fx.ctrl.Close(context.Background())
	}()
	close(fx.f.blockLoading)

	select {
	case err := <-startDone:
		// either outcome is fine (converged just before close, or unblocked by
		// close), but the waiter must not hang
		_ = err
	case <-time.After(time.Second):
		t.Fatal("Start blocked after Close")
	}
	select {
	case err := <-closeDone:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("Close hung")
	}
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

func (m *modeRegister) snapshot() []mode.Mode {
	m.Lock()
	defer m.Unlock()
	return append([]mode.Mode{}, m.modes...)
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

type joining struct {
	status spacestatus.SpaceStatus
	reg    *modeRegister
}

func (i *joining) Start(ctx context.Context) error {
	go func() {
		_ = i.status.SetPersistentStatus(spaceinfo.AccountStatusActive)
	}()
	i.reg.register(mode.ModeJoining)
	return nil
}

func (i *joining) Close(ctx context.Context) error {
	return nil
}

type loading struct {
	f   *factory
	reg *modeRegister
}

func (l *loading) Start(ctx context.Context) error {
	l.f.loadingStarted.Store(true)
	if l.f.blockLoading != nil {
		<-l.f.blockLoading
	}
	if errp := l.f.failLoading.Load(); errp != nil && *errp != nil {
		return *errp
	}
	l.reg.register(mode.ModeLoading)
	return nil
}

func (l *loading) Close(ctx context.Context) error {
	return nil
}

type offloading struct {
	reg *modeRegister
}

func (l *offloading) Start(ctx context.Context) error {
	l.reg.register(mode.ModeOffloading)
	return nil
}

func (l *offloading) Close(ctx context.Context) error {
	return nil
}

type factory struct {
	status spacestatus.SpaceStatus
	reg    *modeRegister

	failLoading    atomic.Pointer[error]
	blockLoading   chan struct{}
	loadingStarted atomic.Bool
}

func (f *factory) Process(md mode.Mode) mode.Process {
	switch md {
	case mode.ModeInitial:
		return initial.New()
	case mode.ModeJoining:
		return &joining{status: f.status, reg: f.reg}
	case mode.ModeLoading:
		return &loading{f: f, reg: f.reg}
	case mode.ModeOffloading:
		return &offloading{reg: f.reg}
	default:
		panic("unhandled default case")
	}
}

type fixture struct {
	f    *factory
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
	f := &factory{
		status: s,
		reg:    reg,
	}
	controller := &spaceController{
		spaceId: "spaceId",
		status:  s,
		app:     &app.App{},
	}
	controller.rec = newReconciler(f, log)
	// mirror NewSpaceController: seed the desired status at registration
	controller.rec.setStatus(s.GetPersistentStatus())
	s.persistentUpdater = func(status spaceinfo.AccountStatus) {
		go func() {
			err := controller.Update()
			require.NoError(t, err)
		}()
	}
	return &fixture{
		f:    f,
		s:    s,
		ctrl: controller,
		reg:  reg,
	}
}

// waitModes asserts the registered mode sequence converges to want.
func (fx *fixture) waitModes(t *testing.T, want ...mode.Mode) {
	require.Eventually(t, func() bool {
		got := fx.reg.snapshot()
		if len(got) != len(want) {
			return false
		}
		for i := range want {
			if got[i] != want[i] {
				return false
			}
		}
		return true
	}, time.Second, 5*time.Millisecond, "modes: %v", fx.reg.snapshot())
}

func (fx *fixture) stop() {
	fx.ctrl.rec.close(context.Background())
}
