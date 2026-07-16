package device

import (
	"context"
	"net"
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
	"github.com/anyproto/anytype-heart/net/addrs"
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

// stubAddrs is a deterministic interface set for the net monitor so tests
// never depend on the host machine's real network state.
func stubAddrs() (addrs.InterfacesAddrs, error) {
	_, ipn, _ := net.ParseCIDR("192.0.2.10/24")
	return addrs.InterfacesAddrs{Addrs: []net.Addr{ipn}}, nil
}

func newNetworkStateFixture(t *testing.T) *networkStateFixture {
	ctrl := gomock.NewController(t)
	mockRefresher := mock_device.NewMockopenedObjectRefresher(t)
	mockPool := mock_pool.NewMockService(ctrl)
	syncer := &spaceSyncerStub{}
	a := &app.App{}
	ns := New().(*networkState)
	ns.monitorGetAddrs = stubAddrs
	a.Register(testutil.PrepareMock(ctx, a, mockRefresher)).
		Register(testutil.PrepareMock(ctx, a, mockPool)).
		Register(syncer).
		Register(ns)
	require.NoError(t, a.Start(ctx))
	t.Cleanup(func() { _ = ns.Close(ctx) })
	// wait for the monitor's initial snapshot so recovery fingerprints are
	// deterministic from the first assertion on
	require.Eventually(t, func() bool { return ns.monitorSnapshot.Load() != "" }, time.Second, time.Millisecond)
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
		fx := newNetworkStateFixture(t)
		startTime := time.Now()
		fx.networkState.now = func() time.Time { return startTime }
		fx.StateChange(int(domain.CompStateAppWentBackground))
		startTime = startTime.Add(recoverAfter + time.Second)
		fx.mockRefresher.EXPECT().RefreshOpenedObjects(mock.Anything).Times(1)
		fx.mockPool.EXPECT().Flush(gomock.Any()).Times(1)
		fx.StateChange(int(domain.CompStateAppWentForeground))
		assert.Equal(t, 1, fx.syncer.callCount())
	})
	t.Run("backgrounded less than recoverAfter: no flush, no head-sync", func(t *testing.T) {
		fx := newNetworkStateFixture(t)
		startTime := time.Now()
		fx.networkState.now = func() time.Time { return startTime }
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
	t.Run("burst coalesces into one trailing recovery when the network really changed", func(t *testing.T) {
		fx := newNetworkStateFixture(t)
		now := time.Now()
		fx.networkState.now = func() time.Time { return now }
		var scheduled []func()
		fx.networkState.scheduleAfter = func(d time.Duration, f func()) *time.Timer {
			scheduled = append(scheduled, f)
			return time.NewTimer(time.Hour) // never fires in test
		}
		fx.SetNetworkState(model.DeviceNetworkType_CELLULAR, "") // first report, no recovery

		// rapid switches: the first runs immediately, the second coalesces into
		// exactly one scheduled trailing run
		fx.mockPool.EXPECT().Flush(gomock.Any()).Times(1)
		fx.SetNetworkState(model.DeviceNetworkType_WIFI, "")     // leading recovery at WIFI
		fx.SetNetworkState(model.DeviceNetworkType_CELLULAR, "") // suppressed
		assert.Equal(t, 1, fx.syncer.callCount())
		require.Len(t, scheduled, 1)
		fx.recoveryMu.Lock()
		assert.Contains(t, fx.pendingReason, "CELLULAR", "trailing run must report the latest coalesced reason")
		fx.recoveryMu.Unlock()

		// state at trailing time (CELLULAR) differs from what the leading run
		// acted on (WIFI) -> the trailing run executes the full pipeline
		now = now.Add(recoverySuppressWindow + time.Second)
		fx.mockPool.EXPECT().Flush(gomock.Any()).Times(1)
		scheduled[0]()
		assert.Equal(t, 2, fx.syncer.callCount())
	})
	t.Run("trailing run fires when the link flapped even if addresses end up identical", func(t *testing.T) {
		// a short outage bracketing a wake: the leading run acted while the
		// link was down (its dials failed), so an identical final address set
		// must NOT be treated as "already handled" — the monitor generation
		// makes the flap visible to the fingerprint
		fx := newNetworkStateFixture(t)
		now := time.Now()
		fx.networkState.now = func() time.Time { return now }
		var scheduled []func()
		fx.networkState.scheduleAfter = func(d time.Duration, f func()) *time.Timer {
			scheduled = append(scheduled, f)
			return time.NewTimer(time.Hour)
		}
		fx.SetNetworkState(model.DeviceNetworkType_CELLULAR, "") // first report

		fx.mockPool.EXPECT().Flush(gomock.Any()).Times(1)
		fx.SetNetworkState(model.DeviceNetworkType_WIFI, "") // leading recovery

		// link drops and comes back with the same address while suppressed
		key := fx.networkState.monitorSnapshot.Load()
		fx.onMonitorSnapshot("", true)
		fx.onMonitorSnapshot(key, false)
		fx.triggerRecovery("interface addresses regained") // suppressed -> pending
		require.Len(t, scheduled, 1)

		// same type, same id, same final snapshot — but the generation moved,
		// so the trailing run must execute the full pipeline
		now = now.Add(recoverySuppressWindow + time.Second)
		fx.mockPool.EXPECT().Flush(gomock.Any()).Times(1)
		scheduled[0]()
		assert.Equal(t, 2, fx.syncer.callCount(), "flapped link must not be swallowed by the skip")
	})
	t.Run("trailing run is skipped when the network is unchanged (duplicate signals)", func(t *testing.T) {
		// a wake fires the clock-jump detector, the interface diff and the
		// foreground RPC within seconds: the duplicates must not flush the
		// connections the leading run just re-established
		fx := newNetworkStateFixture(t)
		now := time.Now()
		fx.networkState.now = func() time.Time { return now }
		var scheduled []func()
		fx.networkState.scheduleAfter = func(d time.Duration, f func()) *time.Timer {
			scheduled = append(scheduled, f)
			return time.NewTimer(time.Hour)
		}
		fx.SetNetworkState(model.DeviceNetworkType_CELLULAR, "") // first report

		fx.mockPool.EXPECT().Flush(gomock.Any()).Times(1)
		fx.SetNetworkState(model.DeviceNetworkType_WIFI, "")     // leading recovery at WIFI
		fx.SetNetworkState(model.DeviceNetworkType_CELLULAR, "") // suppressed
		fx.SetNetworkState(model.DeviceNetworkType_WIFI, "")     // back to the recovered state
		require.Len(t, scheduled, 1)

		// trailing fingerprint equals the leading one -> no second teardown
		now = now.Add(recoverySuppressWindow + time.Second)
		scheduled[0]()
		assert.Equal(t, 1, fx.syncer.callCount(), "duplicate trailing run must be skipped")
	})
	t.Run("close cancels a pending trailing recovery", func(t *testing.T) {
		fx := newNetworkStateFixture(t)
		now := time.Now()
		fx.networkState.now = func() time.Time { return now }
		var scheduled []func()
		fx.networkState.scheduleAfter = func(d time.Duration, f func()) *time.Timer {
			scheduled = append(scheduled, f)
			return time.NewTimer(time.Hour)
		}
		fx.SetNetworkState(model.DeviceNetworkType_CELLULAR, "")
		fx.mockPool.EXPECT().Flush(gomock.Any()).Times(1)
		fx.SetNetworkState(model.DeviceNetworkType_WIFI, "")     // immediate recovery
		fx.SetNetworkState(model.DeviceNetworkType_CELLULAR, "") // suppressed -> pending
		require.Len(t, scheduled, 1)

		require.NoError(t, fx.Close(ctx))
		scheduled[0]() // must be a no-op after close: no flush, no head-sync
		assert.Equal(t, 1, fx.syncer.callCount())
	})
}

func TestNetworkState_IsOffline(t *testing.T) {
	t.Run("explicit online report overrides linkDown", func(t *testing.T) {
		// the interface heuristic must not wedge a device offline when the
		// client's OS callbacks say it is connected (e.g. Android getter that
		// doesn't enumerate cellular interfaces)
		state := &networkState{}
		state.linkDown.Store(true)
		assert.True(t, state.IsOffline(), "no report yet: linkDown decides")

		state.SetNetworkState(model.DeviceNetworkType_CELLULAR, "")
		assert.False(t, state.IsOffline(), "explicit CELLULAR report must win over linkDown")
	})
	t.Run("reported NOT_CONNECTED is offline regardless of interfaces", func(t *testing.T) {
		state := &networkState{}
		state.SetNetworkState(model.DeviceNetworkType_WIFI, "")
		state.SetNetworkState(model.DeviceNetworkType_NOT_CONNECTED, "")
		assert.True(t, state.IsOffline())
	})
	t.Run("without reports linkDown decides (desktop)", func(t *testing.T) {
		state := &networkState{}
		assert.False(t, state.IsOffline())
		state.linkDown.Store(true)
		assert.True(t, state.IsOffline())
		state.linkDown.Store(false)
		assert.False(t, state.IsOffline())
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
