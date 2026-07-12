package device

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/net/pool/mock_pool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/device/mock_device"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

// spaceSyncerStub is a minimal spaceHeadSyncer that counts SyncAllSpaceHeads calls.
type spaceSyncerStub struct {
	mu     sync.Mutex
	called int
}

func (s *spaceSyncerStub) SyncAllSpaceHeads() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.called++
}
func (s *spaceSyncerStub) Name() string        { return "spaceSyncerStub" }
func (s *spaceSyncerStub) Init(*app.App) error { return nil }
func (s *spaceSyncerStub) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.called
}

type networkStateFixture struct {
	*networkState
	a             *app.App
	mockRefresher *mock_device.MockopenedObjectRefresher
	mockPool      *mock_pool.MockService
	syncer        *spaceSyncerStub
}

var ctx = context.Background()

func newNetworkStateFixture(t *testing.T) *networkStateFixture {
	ctrl := gomock.NewController(t)
	mockRefresher := mock_device.NewMockopenedObjectRefresher(t)
	mockPool := mock_pool.NewMockService(ctrl)
	syncer := &spaceSyncerStub{}
	a := &app.App{}
	ns := New().(*networkState)
	a.Register(testutil.PrepareMock(ctx, a, mockRefresher)).
		Register(testutil.PrepareMock(ctx, a, mockPool)).
		Register(syncer).
		Register(ns)
	require.NoError(t, a.Start(ctx))
	t.Cleanup(func() { _ = ns.Close(ctx) })
	return &networkStateFixture{
		networkState:  ns,
		a:             a,
		mockRefresher: mockRefresher,
		mockPool:      mockPool,
		syncer:        syncer,
	}
}

func TestNetworkState_SetDeviceState(t *testing.T) {
	// foreground resume always refreshes opened objects; the connectivity
	// recovery (pool flush + head-sync) is gated by how long the app was
	// backgrounded.
	t.Run("backgrounded longer than recoverAfter: flush pool and sync all heads", func(t *testing.T) {
		startTime := time.Now()
		getTime = func() time.Time { return startTime }
		defer func() { getTime = time.Now }()
		fx := newNetworkStateFixture(t)
		fx.StateChange(int(domain.CompStateAppWentBackground))
		startTime = startTime.Add(recoverAfter + time.Second)
		fx.mockRefresher.EXPECT().RefreshOpenedObjects(mock.Anything).Times(1)
		fx.mockPool.EXPECT().Flush(gomock.Any()).Times(1)
		fx.StateChange(int(domain.CompStateAppWentForeground))
		assert.Equal(t, 1, fx.syncer.callCount())
	})
	t.Run("backgrounded less than recoverAfter: no flush, no head-sync", func(t *testing.T) {
		startTime := time.Now()
		getTime = func() time.Time { return startTime }
		defer func() { getTime = time.Now }()
		fx := newNetworkStateFixture(t)
		fx.StateChange(int(domain.CompStateAppWentBackground))
		startTime = startTime.Add(recoverAfter - time.Second)
		fx.mockRefresher.EXPECT().RefreshOpenedObjects(mock.Anything).Times(1)
		fx.StateChange(int(domain.CompStateAppWentForeground))
		assert.Equal(t, 0, fx.syncer.callCount())
	})
}

func TestNetworkState_SetNetworkState(t *testing.T) {
	t.Run("set network state", func(t *testing.T) {
		// given
		state := &networkState{}

		// when
		state.SetNetworkState(model.DeviceNetworkType_CELLULAR, "")

		// then
		assert.Equal(t, model.DeviceNetworkType_CELLULAR, state.networkState)
	})
	t.Run("update network state", func(t *testing.T) {
		// given
		state := &networkState{}

		// when
		state.SetNetworkState(model.DeviceNetworkType_CELLULAR, "")
		state.SetNetworkState(model.DeviceNetworkType_WIFI, "")

		// then
		assert.Equal(t, model.DeviceNetworkType_WIFI, state.networkState)
	})
	t.Run("update network state with hook", func(t *testing.T) {
		// given
		state := &networkState{}
		var hookState model.DeviceNetworkType
		h := func(state model.DeviceNetworkType) {
			hookState = state
		}
		state.RegisterHook(h)

		// when
		state.SetNetworkState(model.DeviceNetworkType_CELLULAR, "")
		state.SetNetworkState(model.DeviceNetworkType_WIFI, "")

		// then
		assert.Equal(t, model.DeviceNetworkType_WIFI, state.networkState)
		assert.Equal(t, model.DeviceNetworkType_WIFI, hookState)
	})
	t.Run("same value is a cheap no-op (hook not called)", func(t *testing.T) {
		// given: default state is WIFI(0)
		state := &networkState{}
		var calls int
		var last model.DeviceNetworkType
		state.RegisterHook(func(n model.DeviceNetworkType) {
			calls++
			last = n
		})

		// when/then: setting the same-as-current value must not fire the hook
		state.SetNetworkState(model.DeviceNetworkType_WIFI, "")
		assert.Equal(t, 0, calls, "same-as-default value must not fire the hook")

		// a real change fires exactly once
		state.SetNetworkState(model.DeviceNetworkType_CELLULAR, "")
		assert.Equal(t, 1, calls)
		assert.Equal(t, model.DeviceNetworkType_CELLULAR, last)

		// repeating the same value is a no-op — no extra hook calls
		state.SetNetworkState(model.DeviceNetworkType_CELLULAR, "")
		state.SetNetworkState(model.DeviceNetworkType_CELLULAR, "")
		assert.Equal(t, 1, calls, "repeated same value must be a no-op")

		// switching back fires once more
		state.SetNetworkState(model.DeviceNetworkType_WIFI, "")
		assert.Equal(t, 2, calls)
		assert.Equal(t, model.DeviceNetworkType_WIFI, last)
	})
}

func TestNetworkState_ConnectivityRecovery(t *testing.T) {
	t.Run("first report does not recover, a later switch does", func(t *testing.T) {
		fx := newNetworkStateFixture(t)
		var hookCalls []bool
		fx.RegisterConnectivityHook(func(online bool) { hookCalls = append(hookCalls, online) })

		// first report: initial state, connections from startup are good
		fx.SetNetworkState(model.DeviceNetworkType_CELLULAR, "")
		assert.Empty(t, hookCalls)
		assert.Equal(t, 0, fx.syncer.callCount())

		// a real switch flushes the pool, fires hooks and head-syncs
		fx.mockPool.EXPECT().Flush(gomock.Any()).Times(1)
		fx.SetNetworkState(model.DeviceNetworkType_WIFI, "")
		assert.Equal(t, []bool{true}, hookCalls)
		assert.Equal(t, 1, fx.syncer.callCount())
	})
	t.Run("same type, different networkId triggers recovery", func(t *testing.T) {
		fx := newNetworkStateFixture(t)
		fx.SetNetworkState(model.DeviceNetworkType_CELLULAR, "cell-1") // first report

		// same type, new path identity (e.g. Wi-Fi->Wi-Fi or PDP re-attach)
		fx.mockPool.EXPECT().Flush(gomock.Any()).Times(1)
		fx.SetNetworkState(model.DeviceNetworkType_CELLULAR, "cell-2")
		assert.Equal(t, 1, fx.syncer.callCount())

		// repeating the same id is a no-op
		fx.SetNetworkState(model.DeviceNetworkType_CELLULAR, "cell-2")
		assert.Equal(t, 1, fx.syncer.callCount())

		// an empty id from an older client does not count as a change
		fx.SetNetworkState(model.DeviceNetworkType_CELLULAR, "")
		assert.Equal(t, 1, fx.syncer.callCount())
	})
	t.Run("switch to NOT_CONNECTED flushes but does not head-sync", func(t *testing.T) {
		fx := newNetworkStateFixture(t)
		var hookCalls []bool
		fx.RegisterConnectivityHook(func(online bool) { hookCalls = append(hookCalls, online) })
		fx.SetNetworkState(model.DeviceNetworkType_CELLULAR, "") // first report

		fx.mockPool.EXPECT().Flush(gomock.Any()).Times(1)
		fx.SetNetworkState(model.DeviceNetworkType_NOT_CONNECTED, "")
		assert.Equal(t, []bool{false}, hookCalls)
		assert.Equal(t, 0, fx.syncer.callCount())
		assert.True(t, fx.IsOffline())

		// back online: recovery is suppressed (within the window) but coalesced —
		// covered by the coalescing test; here just check the offline flag clears
		fx.networkMu.Lock()
		fx.networkState.networkState = model.DeviceNetworkType_WIFI
		fx.networkMu.Unlock()
		assert.False(t, fx.IsOffline())
	})
	t.Run("burst coalesces into one trailing recovery", func(t *testing.T) {
		now := time.Now()
		getTime = func() time.Time { return now }
		defer func() { getTime = time.Now }()
		var scheduled []func()
		scheduleAfter = func(d time.Duration, f func()) *time.Timer {
			scheduled = append(scheduled, f)
			return time.NewTimer(time.Hour) // never fires in test
		}
		defer func() { scheduleAfter = time.AfterFunc }()

		fx := newNetworkStateFixture(t)
		fx.SetNetworkState(model.DeviceNetworkType_CELLULAR, "") // first report, no recovery

		// three rapid switches: first runs immediately, the rest coalesce into
		// exactly one scheduled trailing run
		fx.mockPool.EXPECT().Flush(gomock.Any()).Times(1)
		fx.SetNetworkState(model.DeviceNetworkType_WIFI, "")
		fx.SetNetworkState(model.DeviceNetworkType_CELLULAR, "")
		fx.SetNetworkState(model.DeviceNetworkType_WIFI, "")
		assert.Equal(t, 1, fx.syncer.callCount())
		require.Len(t, scheduled, 1)

		// the trailing run executes the full pipeline once more
		now = now.Add(recoverySuppressWindow + time.Second)
		fx.mockPool.EXPECT().Flush(gomock.Any()).Times(1)
		scheduled[0]()
		assert.Equal(t, 2, fx.syncer.callCount())
	})
}

func TestNetworkState_GetNetworkState(t *testing.T) {
	t.Run("get default network state", func(t *testing.T) {
		// given
		state := New()

		// when
		networkType := state.GetNetworkState()

		// then
		assert.Equal(t, model.DeviceNetworkType_WIFI, networkType)
	})
	t.Run("get updated network state", func(t *testing.T) {
		// given
		state := New()

		// when
		state.SetNetworkState(model.DeviceNetworkType_CELLULAR, "")
		networkType := state.GetNetworkState()

		// then
		assert.Equal(t, model.DeviceNetworkType_CELLULAR, networkType)
	})
}
