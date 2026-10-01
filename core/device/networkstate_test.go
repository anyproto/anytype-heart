package device

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/net/pool/mock_pool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/device/mock_device"
	"github.com/anyproto/anytype-heart/core/device/networkkey"
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

// fakeClock drives both the wall and the monotonic clock. During sleep the
// monotonic clock pauses on macOS/iOS/Linux/Android and keeps counting on
// Windows; tests pick the shape explicitly.
type fakeClock struct {
	mu   sync.Mutex
	wall time.Time
	mono time.Duration
}

func newFakeClock() *fakeClock {
	return &fakeClock{wall: time.Unix(1_700_000_000, 0), mono: time.Hour}
}

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.wall
}

func (c *fakeClock) elapsed() time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.mono
}

func (c *fakeClock) advance(wall, mono time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.wall = c.wall.Add(wall)
	c.mono += mono
}

type fixtureOpts struct {
	mobile          bool
	monoCountsSleep bool // Windows
	liveWorker      bool // run the real worker and monitor goroutines
	heartbeatEvery  time.Duration
}

type networkStateFixture struct {
	*networkState
	t             *testing.T
	a             *app.App
	clock         *fakeClock
	mockRefresher *mock_device.MockopenedObjectRefresher
	mockPool      *mock_pool.MockService
	syncer        *spaceSyncerStub

	flushes    atomic.Int64 // completed Flush calls
	flushCalls atomic.Int64 // started Flush calls
	flushErr   error
	// flushHook, if set, runs inside Flush (to block or to move the clock)
	flushHook func()

	schedMu   sync.Mutex
	scheduled []func()
	delays    []time.Duration

	addrsMu sync.Mutex
	addrs   []net.Addr

	orderMu sync.Mutex
	order   []string // "flush", "refresh", "hook", "sync"
}

func (fx *networkStateFixture) record(ev string) {
	fx.orderMu.Lock()
	defer fx.orderMu.Unlock()
	fx.order = append(fx.order, ev)
}

func (fx *networkStateFixture) events() []string {
	fx.orderMu.Lock()
	defer fx.orderMu.Unlock()
	return append([]string(nil), fx.order...)
}

var ctx = context.Background()

// stubAddrs is a deterministic interface set for the net monitor so tests
// never depend on the host machine's real network state.
func stubAddrs() (addrs.InterfacesAddrs, error) {
	_, ipn, _ := net.ParseCIDR("192.0.2.10/24")
	return addrs.InterfacesAddrs{Addrs: []net.Addr{ipn}}, nil
}

func newNetworkStateFixture(t *testing.T) *networkStateFixture {
	return newNetworkStateFixtureOpts(t, fixtureOpts{})
}

// newNetworkStateFixtureOpts configures clocks, platform and scheduling
// BEFORE the component starts, so nothing races the live goroutines. By
// default the monitor and the recovery worker are not started at all
// (manualDrive): tests call sample/drain explicitly.
func newNetworkStateFixtureOpts(t *testing.T, opts fixtureOpts) *networkStateFixture {
	ctrl := gomock.NewController(t)
	fx := &networkStateFixture{
		t:             t,
		clock:         newFakeClock(),
		mockRefresher: mock_device.NewMockopenedObjectRefresher(t),
		mockPool:      mock_pool.NewMockService(ctrl),
		syncer:        &spaceSyncerStub{},
	}
	_, ipn, _ := net.ParseCIDR("192.0.2.10/24")
	fx.addrs = []net.Addr{ipn}
	fx.mockRefresher.EXPECT().RefreshOpenedObjects(mock.Anything).Run(func(context.Context) { fx.record("refresh") }).Return().Maybe()
	fx.mockPool.EXPECT().Flush(gomock.Any()).DoAndReturn(func(context.Context) error {
		fx.flushCalls.Inc()
		if fx.flushHook != nil {
			fx.flushHook()
		}
		fx.record("flush")
		fx.flushes.Inc()
		return fx.flushErr
	}).AnyTimes()

	a := &app.App{}
	ns := New().(*networkState)
	ns.mobile = opts.mobile
	ns.monoCountsSleep = opts.monoCountsSleep
	ns.now = fx.clock.now
	ns.elapsed = fx.clock.elapsed
	ns.manualDrive = !opts.liveWorker
	ns.heartbeatEvery = opts.heartbeatEvery
	ns.monitorGetAddrs = func() (addrs.InterfacesAddrs, error) {
		fx.addrsMu.Lock()
		defer fx.addrsMu.Unlock()
		return addrs.InterfacesAddrs{Addrs: fx.addrs}, nil
	}
	ns.scheduleAfter = func(d time.Duration, f func()) *time.Timer {
		fx.schedMu.Lock()
		defer fx.schedMu.Unlock()
		fx.scheduled = append(fx.scheduled, f)
		fx.delays = append(fx.delays, d)
		return time.NewTimer(time.Hour) // never fires in test
	}
	fx.networkState = ns
	a.Register(testutil.PrepareMock(ctx, a, fx.mockRefresher)).
		Register(testutil.PrepareMock(ctx, a, fx.mockPool)).
		Register(fx.syncer).
		Register(ns)
	require.NoError(t, a.Start(ctx))
	t.Cleanup(func() { _ = ns.Close(ctx) })
	fx.a = a
	if opts.liveWorker {
		require.Eventually(t, func() bool { return ns.monitorSnapshot.Load() != "" }, time.Second, time.Millisecond)
	} else {
		require.NotEmpty(t, ns.monitorSnapshot.Load())
	}
	return fx
}

// tick advances both clocks by one normal heartbeat and samples.
func (fx *networkStateFixture) tick() {
	fx.clock.advance(heartbeatInterval, heartbeatInterval)
	fx.onHeartbeat()
}

func (fx *networkStateFixture) ticks(n int) {
	for i := 0; i < n; i++ {
		fx.tick()
	}
}

// sleepFor freezes the process for d: the wall clock always jumps; the
// monotonic clock jumps only on Windows-style platforms.
func (fx *networkStateFixture) sleepFor(d time.Duration) {
	if fx.monoCountsSleep {
		fx.clock.advance(d, d)
	} else {
		fx.clock.advance(d, 0)
	}
}

// drain runs queued recoveries, failing fast instead of hanging.
func (fx *networkStateFixture) drain() {
	fx.t.Helper()
	done := make(chan struct{})
	go func() { fx.drainRecoveries(); close(done) }()
	waitFor(fx.t, done, "drain")
}

func (fx *networkStateFixture) lastDelay() time.Duration {
	fx.schedMu.Lock()
	defer fx.schedMu.Unlock()
	if len(fx.delays) == 0 {
		return 0
	}
	return fx.delays[len(fx.delays)-1]
}

// fireScheduled runs every pending trailing-run timer.
func (fx *networkStateFixture) fireScheduled() int {
	fx.schedMu.Lock()
	fs := fx.scheduled
	fx.scheduled = nil
	fx.schedMu.Unlock()
	for _, f := range fs {
		f()
	}
	return len(fs)
}

func (fx *networkStateFixture) scheduledCount() int {
	fx.schedMu.Lock()
	defer fx.schedMu.Unlock()
	return len(fx.scheduled)
}

func (fx *networkStateFixture) stat() networkStateStat {
	return fx.ProvideStat().(networkStateStat)
}

func (fx *networkStateFixture) refreshCalls() int {
	n := 0
	for _, c := range fx.mockRefresher.Calls {
		if c.Method == "RefreshOpenedObjects" {
			n++
		}
	}
	return n
}

// foregroundSettled reports the first Foreground (account start: one
// recovery), runs it and lets the suppression window pass with normal ticks.
// Returns the flush count after settling.
func (fx *networkStateFixture) foregroundSettled() int64 {
	fx.StateChange(int(domain.CompStateAppWentForeground))
	fx.drain()
	require.Equal(fx.t, int64(1), fx.flushes.Load(), "unreported -> Foreground keeps today's single recovery")
	fx.ticks(2)
	fx.drain()
	return fx.flushes.Load()
}

// waitFor fails the test instead of hanging when ch never closes.
func waitFor(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
}

func (fx *networkStateFixture) setAddrs(cidrs ...string) {
	var list []net.Addr
	for _, c := range cidrs {
		_, ipn, _ := net.ParseCIDR(c)
		list = append(list, ipn)
	}
	fx.addrsMu.Lock()
	fx.addrs = list
	fx.addrsMu.Unlock()
}

func TestNetworkState_SetDeviceState(t *testing.T) {
	// foreground resume always refreshes opened objects; the connectivity
	// recovery (pool flush + head-sync) is gated by how long the app was
	// backgrounded.
	t.Run("backgrounded longer than recoverAfter: flush pool and sync all heads", func(t *testing.T) {
		fx := newNetworkStateFixtureOpts(t, fixtureOpts{mobile: true})
		fx.StateChange(int(domain.CompStateAppWentBackground))
		fx.ticks(int(recoverAfter/heartbeatInterval) + 1)
		fx.StateChange(int(domain.CompStateAppWentForeground))
		fx.drain()
		assert.Equal(t, int64(1), fx.flushes.Load())
		assert.Equal(t, 1, fx.syncer.callCount())
		assert.Equal(t, 1, fx.refreshCalls())
		assert.Equal(t, triggerTransition, fx.stat().LastRecoveryTrigger)
	})
	t.Run("backgrounded less than recoverAfter: no flush, no head-sync", func(t *testing.T) {
		fx := newNetworkStateFixtureOpts(t, fixtureOpts{mobile: true})
		fx.StateChange(int(domain.CompStateAppWentBackground))
		fx.ticks(2)
		fx.StateChange(int(domain.CompStateAppWentForeground))
		fx.drain()
		assert.Equal(t, int64(0), fx.flushes.Load())
		assert.Equal(t, 0, fx.syncer.callCount())
		assert.Equal(t, 1, fx.refreshCalls())
	})
	t.Run("unreported -> Foreground runs exactly one recovery", func(t *testing.T) {
		fx := newNetworkStateFixture(t)
		fx.foregroundSettled()
		fx.StateChange(int(domain.CompStateAppWentForeground)) // redundant
		fx.drain()
		assert.Equal(t, int64(1), fx.flushes.Load())
		assert.Equal(t, 1, fx.refreshCalls(), "redundant foreground doesn't refresh objects")
	})
	t.Run("short app switch and redundant foreground: no recovery", func(t *testing.T) {
		for _, mobile := range []bool{false, true} {
			fx := newNetworkStateFixtureOpts(t, fixtureOpts{mobile: mobile})
			base := fx.foregroundSettled()
			fx.StateChange(int(domain.CompStateAppWentBackground))
			fx.ticks(1)
			fx.StateChange(int(domain.CompStateAppWentForeground))
			fx.StateChange(int(domain.CompStateAppWentForeground))
			fx.ticks(3)
			fx.drain()
			assert.Equal(t, base, fx.flushes.Load(), "mobile=%v", mobile)
			assert.Equal(t, int64(1), fx.stat().ForegroundDuplicate)
		}
	})
	t.Run("duplicate Background reports keep backgroundSince", func(t *testing.T) {
		fx := newNetworkStateFixtureOpts(t, fixtureOpts{mobile: true})
		base := fx.foregroundSettled()
		fx.StateChange(int(domain.CompStateAppWentBackground))
		fx.ticks(2) // 10s
		fx.StateChange(int(domain.CompStateAppWentBackground))
		assert.Equal(t, "foreground", fx.stat().PrevDeviceState, "duplicates don't overwrite the previous state")
		fx.ticks(2) // 20s since the real transition, 10s since the duplicate
		fx.StateChange(int(domain.CompStateAppWentForeground))
		fx.drain()
		assert.Equal(t, base+1, fx.flushes.Load())
		st := fx.stat()
		assert.Equal(t, int64(1), st.BackgroundDuplicate)
		assert.Equal(t, int64(1), st.BackgroundEvents)
	})
	t.Run("Android-style long background with ticks running recovers", func(t *testing.T) {
		fx := newNetworkStateFixtureOpts(t, fixtureOpts{mobile: true})
		base := fx.foregroundSettled()
		fx.StateChange(int(domain.CompStateAppWentBackground))
		fx.ticks(12) // 60s, no freeze
		fx.drain()
		assert.Equal(t, base, fx.flushes.Load())
		fx.StateChange(int(domain.CompStateAppWentForeground))
		fx.drain()
		assert.Equal(t, base+1, fx.flushes.Load())
		st := fx.stat()
		assert.Equal(t, int64(0), st.WakeGenObserved)
		assert.Equal(t, triggerTransition, st.LastRecoveryTrigger)
	})
}

func TestNetworkState_WakeRecovery(t *testing.T) {
	t.Run("Windows-style gap: both clocks jump", func(t *testing.T) {
		fx := newNetworkStateFixtureOpts(t, fixtureOpts{monoCountsSleep: true})
		base := fx.foregroundSettled()
		fx.sleepFor(10 * time.Minute)
		fx.onHeartbeat()
		fx.drain()
		assert.Equal(t, base+1, fx.flushes.Load())
		st := fx.stat()
		assert.Equal(t, int64(1), st.WakeGenObserved)
		assert.Equal(t, int64(1), st.WakeGenCompleted)
		assert.Equal(t, int64((10*time.Minute-heartbeatInterval)/time.Millisecond), st.LastWakeFrozenForMs)
		assert.Equal(t, triggerDesktopFallback, st.LastRecoveryTrigger)
	})
	t.Run("macOS/Linux-style gap: monotonic paused, wall jumped", func(t *testing.T) {
		fx := newNetworkStateFixture(t)
		base := fx.foregroundSettled()
		fx.sleepFor(10 * time.Minute)
		fx.onHeartbeat()
		fx.drain()
		assert.Equal(t, base+1, fx.flushes.Load())
		st := fx.stat()
		assert.Equal(t, int64(1), st.WakeGenObserved)
		assert.Equal(t, int64(10*time.Minute/time.Millisecond), st.LastWakeFrozenForMs)
		assert.Equal(t, observeSourceSampler, st.LastWakeSource)
	})
	t.Run("freeze at the threshold is not a wake", func(t *testing.T) {
		fx := newNetworkStateFixture(t)
		base := fx.foregroundSettled()
		fx.sleepFor(wakeGapThreshold)
		fx.onHeartbeat()
		fx.drain()
		assert.Equal(t, base, fx.flushes.Load())
		assert.Equal(t, int64(0), fx.stat().WakeGenObserved)
	})
	t.Run("lost suspend: Foreground->Foreground plus a gap recovers once", func(t *testing.T) {
		for _, mobile := range []bool{false, true} {
			fx := newNetworkStateFixtureOpts(t, fixtureOpts{mobile: mobile})
			base := fx.foregroundSettled()
			fx.sleepFor(10 * time.Minute)
			// the resume RPC arrives before the next sample
			fx.StateChange(int(domain.CompStateAppWentForeground))
			fx.drain()
			fx.ticks(3)
			fx.drain()
			assert.Equal(t, base+1, fx.flushes.Load(), "mobile=%v", mobile)
			st := fx.stat()
			assert.Equal(t, triggerDuplicateForeground, st.LastRecoveryTrigger)
			assert.Equal(t, observeSourceStateChange, st.LastWakeSource)
		}
	})
	t.Run("lost resume on desktop: monitor fallback runs exactly one recovery", func(t *testing.T) {
		fx := newNetworkStateFixture(t)
		base := fx.foregroundSettled()
		fx.StateChange(int(domain.CompStateAppWentBackground)) // suspend delivered
		fx.sleepFor(10 * time.Minute)
		fx.onHeartbeat() // no Foreground ever arrives
		fx.drain()
		fx.ticks(10)
		fx.drain()
		assert.Equal(t, base+1, fx.flushes.Load())
		assert.Equal(t, int64(1), fx.stat().ExecutedByTrigger[triggerDesktopFallback])
	})
	t.Run("lost suspend on desktop: sampler first, then duplicate Foreground", func(t *testing.T) {
		fx := newNetworkStateFixture(t)
		base := fx.foregroundSettled()
		fx.sleepFor(10 * time.Minute)
		fx.onHeartbeat()
		fx.drain()
		fx.ticks(2)
		fx.StateChange(int(domain.CompStateAppWentForeground))
		fx.drain()
		assert.Equal(t, base+1, fx.flushes.Load())
	})
	t.Run("RPC then tick, more than 6s apart, no pre-seeded generation: one flush", func(t *testing.T) {
		for _, mobile := range []bool{false, true} {
			fx := newNetworkStateFixtureOpts(t, fixtureOpts{mobile: mobile})
			base := fx.foregroundSettled()
			fx.StateChange(int(domain.CompStateAppWentBackground))
			fx.sleepFor(10 * time.Minute)
			fx.StateChange(int(domain.CompStateAppWentForeground))
			fx.drain()
			fx.clock.advance(7*time.Second, 7*time.Second)
			fx.onHeartbeat()
			fx.ticks(3)
			fx.drain()
			assert.Equal(t, base+1, fx.flushes.Load(), "mobile=%v", mobile)
			assert.Equal(t, int64(1), fx.stat().WakeGenObserved)
		}
	})
	t.Run("tick then RPC, more than 6s apart, no pre-seeded generation: one flush", func(t *testing.T) {
		for _, mobile := range []bool{false, true} {
			fx := newNetworkStateFixtureOpts(t, fixtureOpts{mobile: mobile})
			base := fx.foregroundSettled()
			fx.StateChange(int(domain.CompStateAppWentBackground))
			fx.sleepFor(10 * time.Minute)
			fx.onHeartbeat()
			fx.drain()
			fx.clock.advance(7*time.Second, 7*time.Second)
			fx.StateChange(int(domain.CompStateAppWentForeground))
			fx.drain()
			fx.ticks(3)
			fx.drain()
			assert.Equal(t, base+1, fx.flushes.Load(), "mobile=%v", mobile)
			assert.Equal(t, int64(1), fx.stat().WakeGenObserved)
		}
	})
	t.Run("desktop fallback flushes while Background, delayed Foreground: no second flush", func(t *testing.T) {
		fx := newNetworkStateFixture(t)
		base := fx.foregroundSettled()
		fx.StateChange(int(domain.CompStateAppWentBackground))
		fx.sleepFor(time.Hour)
		fx.onHeartbeat()
		fx.drain()
		require.Equal(t, base+1, fx.flushes.Load())
		fx.ticks(4) // 20s later, still inside wakeCoverWindow
		fx.StateChange(int(domain.CompStateAppWentForeground))
		fx.drain()
		assert.Equal(t, base+1, fx.flushes.Load())
		assert.Equal(t, int64(1), fx.stat().ForegroundSuppressed)
		assert.Equal(t, 2, fx.refreshCalls(), "the Foreground transition still refreshes opened objects")
	})
	t.Run("Foreground while the fallback recovery is still queued: no second flush", func(t *testing.T) {
		fx := newNetworkStateFixture(t)
		base := fx.foregroundSettled()
		fx.StateChange(int(domain.CompStateAppWentBackground))
		fx.sleepFor(time.Hour)
		fx.onHeartbeat() // fallback queued, not yet run
		fx.StateChange(int(domain.CompStateAppWentForeground))
		fx.drain()
		assert.Equal(t, base+1, fx.flushes.Load())
	})
	t.Run("late Background report observes the gap first (desktop)", func(t *testing.T) {
		fx := newNetworkStateFixture(t)
		base := fx.foregroundSettled()
		fx.sleepFor(10 * time.Minute)
		fx.StateChange(int(domain.CompStateAppWentBackground)) // suspend delivered after the wake
		assert.True(t, fx.stat().RecoveryQueued, "the late Background report itself admits the fallback")
		fx.drain()
		fx.ticks(3)
		fx.StateChange(int(domain.CompStateAppWentForeground)) // resume
		fx.drain()
		assert.Equal(t, base+1, fx.flushes.Load())
		assert.Equal(t, int64(1), fx.stat().ExecutedByTrigger[triggerDesktopFallback])
	})
	t.Run("interface event first, then the sampler: one flush", func(t *testing.T) {
		fx := newNetworkStateFixture(t)
		base := fx.foregroundSettled()
		fx.sleepFor(10 * time.Minute)
		fx.setAddrs("198.51.100.7/24") // re-associated with a new address
		fx.monitor.checkInterfaces()
		fx.drain()
		fx.onHeartbeat()
		fx.ticks(3)
		fx.drain()
		assert.Equal(t, base+1, fx.flushes.Load())
		st := fx.stat()
		assert.Equal(t, observeSourceAdmission, st.LastWakeSource)
		assert.Equal(t, int64(1), st.WakeGenCompleted)
		assert.Equal(t, int64(0), st.ExecutedByTrigger[triggerDesktopFallback])
	})
	t.Run("recovery timer armed before sleep fires first, then the sampler: one flush", func(t *testing.T) {
		fx := newNetworkStateFixture(t)
		base := fx.foregroundSettled()
		fx.SetNetworkState(model.DeviceNetworkType_CELLULAR, "") // first report
		fx.SetNetworkState(model.DeviceNetworkType_WIFI, "")     // leading
		fx.SetNetworkState(model.DeviceNetworkType_CELLULAR, "") // coalesced -> timer
		fx.SetNetworkState(model.DeviceNetworkType_WIFI, "")     // back: fingerprint unchanged
		fx.drain()
		require.Equal(t, base+1, fx.flushes.Load())
		require.Equal(t, 1, fx.scheduledCount())

		fx.sleepFor(10 * time.Minute)
		fx.fireScheduled() // the pre-sleep timer fires first after the wake
		fx.drain()
		fx.onHeartbeat()
		fx.ticks(3)
		fx.drain()
		assert.Equal(t, base+2, fx.flushes.Load(), "the trailing run covers the wake; the sampler adds nothing")
		st := fx.stat()
		assert.Equal(t, int64(1), st.TrailingRuns)
		assert.Equal(t, int64(0), st.ExecutedByTrigger[triggerDesktopFallback])
		assert.Equal(t, observeSourceTrailing, st.LastWakeSource, "the timer itself opens the generation")
	})
	t.Run("recovery just before sleep, wake within 6s monotonic: still recovers", func(t *testing.T) {
		fx := newNetworkStateFixture(t)
		base := fx.foregroundSettled()
		fx.SetNetworkState(model.DeviceNetworkType_CELLULAR, "")
		fx.SetNetworkState(model.DeviceNetworkType_WIFI, "") // recovery right before the lid closes
		fx.drain()
		require.Equal(t, base+1, fx.flushes.Load())

		fx.clock.advance(10*time.Minute, 2*time.Second) // macOS: monotonic barely moved
		fx.onHeartbeat()                                // coalesced into a trailing run
		fx.drain()
		require.Equal(t, base+1, fx.flushes.Load())
		require.Equal(t, 1, fx.fireScheduled())
		fx.drain()
		assert.Equal(t, base+2, fx.flushes.Load(), "fingerprint unchanged, but the wake generation is uncovered")
		fx.ticks(3)
		fx.drain()
		assert.Equal(t, base+2, fx.flushes.Load())
		assert.Equal(t, int64(1), fx.stat().TrailingRuns)
	})
	t.Run("second wake during a running recovery stays pending and runs once", func(t *testing.T) {
		fx := newNetworkStateFixtureOpts(t, fixtureOpts{monoCountsSleep: true})
		base := fx.foregroundSettled()
		entered, release := make(chan struct{}), make(chan struct{})
		var once sync.Once
		fx.flushHook = func() {
			once.Do(func() {
				close(entered)
				<-release
			})
		}
		fx.sleepFor(10 * time.Minute)
		fx.onHeartbeat()
		done := make(chan struct{})
		go func() { fx.drainRecoveries(); close(done) }()
		waitFor(t, entered, "flush to start")

		fx.sleepFor(10 * time.Minute) // second freeze while Flush runs
		fx.onHeartbeat()
		fx.ticks(2)
		st := fx.stat()
		assert.Equal(t, int64(2), st.WakeGenObserved)
		assert.Equal(t, int64(1), st.WakeGenRunning)
		assert.True(t, st.RecoveryRunning)
		close(release)
		waitFor(t, done, "drain")
		fx.fireScheduled()
		fx.drain()
		fx.ticks(3)
		fx.fireScheduled()
		fx.drain()
		assert.Equal(t, base+2, fx.flushes.Load())
		assert.Equal(t, int64(2), fx.stat().WakeGenCompleted)
	})
	t.Run("slow recovery work: no false gap", func(t *testing.T) {
		fx := newNetworkStateFixture(t)
		base := fx.foregroundSettled()
		entered, release := make(chan struct{}), make(chan struct{})
		var once sync.Once
		fx.flushHook = func() {
			once.Do(func() {
				close(entered)
				<-release
			})
		}
		fx.SetNetworkState(model.DeviceNetworkType_CELLULAR, "")
		fx.SetNetworkState(model.DeviceNetworkType_WIFI, "")
		done := make(chan struct{})
		go func() { fx.drainRecoveries(); close(done) }()
		waitFor(t, entered, "flush to start")
		fx.ticks(12) // the flush takes a minute; the sampler keeps sampling
		close(release)
		waitFor(t, done, "drain")
		fx.ticks(2)
		fx.drain()
		assert.Equal(t, base+1, fx.flushes.Load())
		assert.Equal(t, int64(0), fx.stat().WakeGenObserved)
	})
	t.Run("suspend during interface enumeration: the gap is still detected", func(t *testing.T) {
		fx := newNetworkStateFixture(t)
		base := fx.foregroundSettled()
		fx.monitor.getAddrs = func() (addrs.InterfacesAddrs, error) {
			fx.sleepFor(10 * time.Minute) // suspended inside the syscall
			return stubAddrs()
		}
		fx.monitor.checkInterfaces()
		fx.onHeartbeat()
		fx.drain()
		assert.Equal(t, base+1, fx.flushes.Load())
		assert.Equal(t, observeSourceSampler, fx.stat().LastWakeSource)
	})
	t.Run("mobile freeze while backgrounded waits for Foreground", func(t *testing.T) {
		fx := newNetworkStateFixtureOpts(t, fixtureOpts{mobile: true})
		base := fx.foregroundSettled()
		fx.StateChange(int(domain.CompStateAppWentBackground))
		fx.sleepFor(10 * time.Minute)
		fx.onHeartbeat()
		fx.ticks(20)
		fx.drain()
		assert.Equal(t, base, fx.flushes.Load(), "no recovery while backgrounded on mobile")
		st := fx.stat()
		assert.Equal(t, int64(1), st.WakeGenObserved)
		assert.Equal(t, "pendingForeground", st.LastWakeOutcome)
		fx.StateChange(int(domain.CompStateAppWentForeground))
		fx.drain()
		assert.Equal(t, base+1, fx.flushes.Load())
		assert.Equal(t, int64(1), fx.stat().WakeGenCompleted)
	})
	t.Run("mobile freeze while backgrounded: one transition recovery covers the wake", func(t *testing.T) {
		// the freeze advances the wall clock, so the transition branch
		// qualifies; the uncovered wake is covered by that one recovery and
		// nothing else runs afterwards
		fx := newNetworkStateFixtureOpts(t, fixtureOpts{mobile: true})
		base := fx.foregroundSettled()
		fx.StateChange(int(domain.CompStateAppWentBackground))
		fx.sleepFor(time.Minute)
		fx.StateChange(int(domain.CompStateAppWentForeground))
		fx.drain()
		fx.ticks(5)
		fx.drain()
		assert.Equal(t, base+1, fx.flushes.Load())
	})
	t.Run("mobile network-change recovery while backgrounded doesn't swallow a later long-background Foreground", func(t *testing.T) {
		fx := newNetworkStateFixtureOpts(t, fixtureOpts{mobile: true})
		fx.SetNetworkState(model.DeviceNetworkType_WIFI, "wifi-1") // first report
		base := fx.foregroundSettled()
		fx.StateChange(int(domain.CompStateAppWentBackground))
		fx.sleepFor(10 * time.Minute)
		fx.onHeartbeat()                                           // wake pending
		fx.SetNetworkState(model.DeviceNetworkType_WIFI, "wifi-2") // background network change
		fx.drain()
		require.Equal(t, base+1, fx.flushes.Load())
		require.Equal(t, int64(1), fx.stat().WakeGenCompleted)
		fx.ticks(8) // 40s > wakeCoverWindow
		fx.StateChange(int(domain.CompStateAppWentForeground))
		fx.drain()
		assert.Equal(t, base+2, fx.flushes.Load(), "GO-7302 long-background recovery must still run")
		assert.Equal(t, int64(0), fx.stat().ForegroundSuppressed)
	})
	t.Run("covering recovery less than 30s before Foreground suppresses the transition recovery", func(t *testing.T) {
		fx := newNetworkStateFixtureOpts(t, fixtureOpts{mobile: true})
		fx.SetNetworkState(model.DeviceNetworkType_WIFI, "wifi-1")
		base := fx.foregroundSettled()
		fx.StateChange(int(domain.CompStateAppWentBackground))
		fx.ticks(4) // mobile measures the background on the monotonic clock
		fx.sleepFor(10 * time.Minute)
		fx.onHeartbeat()
		fx.SetNetworkState(model.DeviceNetworkType_WIFI, "wifi-2")
		fx.drain()
		fx.ticks(2) // 10s
		fx.StateChange(int(domain.CompStateAppWentForeground))
		fx.drain()
		assert.Equal(t, base+1, fx.flushes.Load())
		assert.Equal(t, int64(1), fx.stat().ForegroundSuppressed)
	})
	t.Run("network-change recovery without a wake never suppresses the transition", func(t *testing.T) {
		fx := newNetworkStateFixtureOpts(t, fixtureOpts{mobile: true})
		fx.SetNetworkState(model.DeviceNetworkType_WIFI, "wifi-1")
		base := fx.foregroundSettled()
		fx.StateChange(int(domain.CompStateAppWentBackground))
		fx.ticks(4)
		fx.SetNetworkState(model.DeviceNetworkType_WIFI, "wifi-2")
		fx.drain()
		fx.ticks(2)
		fx.StateChange(int(domain.CompStateAppWentForeground))
		fx.drain()
		assert.Equal(t, base+2, fx.flushes.Load())
	})
	t.Run("forward and backward wall-clock steps", func(t *testing.T) {
		t.Run("forward step on macOS/Linux is a (tolerated) false wake: one flush", func(t *testing.T) {
			fx := newNetworkStateFixture(t)
			base := fx.foregroundSettled()
			fx.clock.advance(time.Hour, heartbeatInterval)
			fx.onHeartbeat()
			fx.ticks(3)
			fx.drain()
			assert.Equal(t, base+1, fx.flushes.Load())
		})
		t.Run("forward step on Windows is ignored (monotonic-only detection)", func(t *testing.T) {
			fx := newNetworkStateFixtureOpts(t, fixtureOpts{monoCountsSleep: true})
			base := fx.foregroundSettled()
			fx.clock.advance(time.Hour, heartbeatInterval)
			fx.onHeartbeat()
			fx.drain()
			assert.Equal(t, base, fx.flushes.Load())
			assert.Equal(t, int64(0), fx.stat().WakeGenObserved)
		})
		t.Run("backward step: no wake, and a later sleep is still detected", func(t *testing.T) {
			fx := newNetworkStateFixture(t)
			base := fx.foregroundSettled()
			fx.clock.advance(-time.Hour, heartbeatInterval)
			fx.onHeartbeat()
			fx.ticks(2)
			fx.drain()
			assert.Equal(t, base, fx.flushes.Load())

			fx.sleepFor(10 * time.Minute)
			fx.onHeartbeat()
			fx.drain()
			assert.Equal(t, base+1, fx.flushes.Load())
		})
		t.Run("backward step does not shrink a background below monotonic time", func(t *testing.T) {
			fx := newNetworkStateFixtureOpts(t, fixtureOpts{mobile: true})
			base := fx.foregroundSettled()
			fx.StateChange(int(domain.CompStateAppWentBackground))
			fx.clock.advance(-time.Hour, heartbeatInterval)
			fx.ticks(4)
			fx.StateChange(int(domain.CompStateAppWentForeground))
			fx.drain()
			assert.Equal(t, base+1, fx.flushes.Load())
		})
	})
	t.Run("failed Flush is counted and still completes the generation", func(t *testing.T) {
		fx := newNetworkStateFixture(t)
		base := fx.foregroundSettled()
		fx.flushErr = errors.New("flush failed")
		fx.sleepFor(10 * time.Minute)
		fx.onHeartbeat()
		fx.drain()
		fx.ticks(5)
		fx.fireScheduled()
		fx.drain()
		assert.Equal(t, base+1, fx.flushes.Load(), "no retry loop on a failed flush")
		st := fx.stat()
		assert.Equal(t, int64(1), st.FlushErrors)
		assert.Equal(t, "flush failed", st.LastFlushError)
		assert.Equal(t, int64(1), st.WakeGenCompleted)
		assert.Equal(t, int64(1), st.LastRecoveryWakeGen)
		assert.Equal(t, triggerDesktopFallback, st.LastRecoveryTrigger)
	})
}

func TestNetworkState_RecoveryWorker(t *testing.T) {
	t.Run("live worker runs queued recoveries off the caller's goroutine", func(t *testing.T) {
		fx := newNetworkStateFixtureOpts(t, fixtureOpts{liveWorker: true})
		fx.SetNetworkState(model.DeviceNetworkType_CELLULAR, "")
		fx.SetNetworkState(model.DeviceNetworkType_WIFI, "")
		require.Eventually(t, func() bool { return fx.syncer.callCount() == 1 }, time.Second, time.Millisecond)
		assert.Equal(t, int64(1), fx.flushes.Load())
	})
	t.Run("admission never blocks on a running recovery", func(t *testing.T) {
		fx := newNetworkStateFixtureOpts(t, fixtureOpts{liveWorker: true})
		entered, release := make(chan struct{}), make(chan struct{})
		var once sync.Once
		fx.flushHook = func() {
			once.Do(func() {
				close(entered)
				<-release
			})
		}
		fx.StateChange(int(domain.CompStateAppWentForeground))
		waitFor(t, entered, "flush to start")
		// past the suppression window: the switch below is a leading run that
		// must queue behind the running one
		fx.clock.advance(recoverySuppressWindow+time.Second, recoverySuppressWindow+time.Second)
		returned := make(chan struct{})
		go func() {
			fx.StateChange(int(domain.CompStateAppWentBackground))
			fx.SetNetworkState(model.DeviceNetworkType_CELLULAR, "")
			fx.SetNetworkState(model.DeviceNetworkType_WIFI, "")
			close(returned)
		}()
		select {
		case <-returned:
		case <-time.After(5 * time.Second):
			t.Fatal("admission blocked on the running recovery")
		}
		close(release)
		require.Eventually(t, func() bool { return fx.flushes.Load() == 2 }, time.Second, time.Millisecond)
	})
	t.Run("shutdown stops the worker", func(t *testing.T) {
		fx := newNetworkStateFixtureOpts(t, fixtureOpts{liveWorker: true})
		done := fx.workerDone
		require.NotNil(t, done)
		require.NoError(t, fx.Close(ctx))
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("worker did not stop")
		}
		// admission after close is a no-op
		fx.SetNetworkState(model.DeviceNetworkType_CELLULAR, "")
		fx.SetNetworkState(model.DeviceNetworkType_WIFI, "")
		fx.StateChange(int(domain.CompStateAppWentForeground))
		fx.onHeartbeat()
		assert.False(t, fx.processQueuedRecovery())
		assert.Equal(t, int64(0), fx.flushes.Load())
	})
	t.Run("shutdown waits for a running recovery, bounded", func(t *testing.T) {
		fx := newNetworkStateFixtureOpts(t, fixtureOpts{liveWorker: true})
		entered, release := make(chan struct{}), make(chan struct{})
		var once sync.Once
		fx.RegisterConnectivityHook(func(bool) {
			once.Do(func() {
				close(entered)
				<-release // a hook stuck past Close
			})
		})
		defer close(release)
		fx.closeBound = 100 * time.Millisecond
		fx.StateChange(int(domain.CompStateAppWentForeground))
		waitFor(t, entered, "hook to start")
		start := time.Now()
		closed := make(chan struct{})
		go func() { _ = fx.Close(ctx); close(closed) }()
		select {
		case <-closed:
			t.Fatal("Close returned without waiting for the running recovery")
		case <-time.After(20 * time.Millisecond):
		}
		waitFor(t, closed, "Close")
		assert.Less(t, time.Since(start), 3*time.Second)
		assert.Equal(t, int64(1), fx.stat().CloseWaitTimeouts)
	})
	t.Run("Close cancels a stuck Flush and returns within the bound", func(t *testing.T) {
		fx := newNetworkStateFixtureOpts(t, fixtureOpts{liveWorker: true})
		entered, stuck := make(chan struct{}), make(chan struct{})
		defer close(stuck)
		var once sync.Once
		fx.flushHook = func() {
			once.Do(func() { close(entered) })
			<-stuck // ignores its context entirely
		}
		fx.StateChange(int(domain.CompStateAppWentForeground))
		waitFor(t, entered, "flush to start")
		start := time.Now()
		require.NoError(t, fx.Close(ctx))
		assert.Less(t, time.Since(start), time.Second, "Close must not wait for the abandoned Flush")
		waitFor(t, fx.workerDone, "worker exit")
		assert.Equal(t, int64(0), fx.stat().CloseWaitTimeouts)
	})
}

func TestNetworkState_FlushBound(t *testing.T) {
	t.Run("a stuck Flush is abandoned after the bound and still completes its generation", func(t *testing.T) {
		fx := newNetworkStateFixture(t)
		base := fx.foregroundSettled()
		stuck := make(chan struct{})
		defer close(stuck)
		fx.flushHook = func() { <-stuck }
		fx.flushBound = 20 * time.Millisecond
		fx.sleepFor(10 * time.Minute)
		fx.onHeartbeat()
		start := time.Now()
		fx.drain()
		assert.Less(t, time.Since(start), time.Second, "the worker must not wait for the default 10s bound")
		st := fx.stat()
		assert.Equal(t, int64(1), st.FlushTimeouts)
		assert.Equal(t, int64(1), st.WakeGenCompleted)
		assert.Contains(t, st.LastFlushError, "abandoned")
		assert.Equal(t, base+1, fx.flushCalls.Load())
	})
	t.Run("the next flush waits for an abandoned one: flushes never overlap", func(t *testing.T) {
		fx := newNetworkStateFixture(t)
		fx.SetNetworkState(model.DeviceNetworkType_WIFI, "w1")
		base := fx.foregroundSettled()
		release := make(chan struct{})
		var calls atomic.Int32
		fx.flushHook = func() {
			if calls.Inc() == 1 {
				<-release
			}
		}
		fx.flushBound = 20 * time.Millisecond
		fx.SetNetworkState(model.DeviceNetworkType_WIFI, "w2")
		fx.drain()                      // abandons flush #1
		fx.flushBound = 2 * time.Second // the wait for #1 is bounded by this
		fx.ticks(2)
		fx.SetNetworkState(model.DeviceNetworkType_WIFI, "w3")
		done := make(chan struct{})
		go func() { fx.drainRecoveries(); close(done) }()
		select {
		case <-done:
			t.Fatal("second recovery ran while the abandoned flush was still running")
		case <-time.After(50 * time.Millisecond):
		}
		assert.Equal(t, base+1, fx.flushCalls.Load(), "no second Flush while the first is in flight")
		close(release)
		waitFor(t, done, "second drain")
		ev := fx.events()
		assert.Equal(t, []string{"flush", "flush"}, ev[len(ev)-2:])
		assert.Equal(t, base+2, fx.flushes.Load())
	})
}

func TestNetworkState_FlushBoundStuckForever(t *testing.T) {
	// Q2: a Flush that never returns must not wedge recovery until Close
	fx := newNetworkStateFixtureOpts(t, fixtureOpts{mobile: true})
	fx.SetNetworkState(model.DeviceNetworkType_WIFI, "w1")
	fx.foregroundSettled()
	stuck := make(chan struct{})
	defer close(stuck)
	var calls atomic.Int32
	fx.flushHook = func() {
		if calls.Inc() == 1 {
			<-stuck
		}
	}
	fx.flushBound = 20 * time.Millisecond
	fx.SetNetworkState(model.DeviceNetworkType_WIFI, "w2")
	fx.drain() // abandons flush #1 for good
	refreshes, syncs := fx.refreshCalls(), fx.syncer.callCount()

	fx.ticks(2)
	fx.StateChange(int(domain.CompStateAppWentBackground))
	fx.sleepFor(time.Hour)
	fx.ticks(4)
	fx.StateChange(int(domain.CompStateAppWentForeground)) // foregroundWake/transition with refresh
	fx.drain()                                             // waits one bound, then skips its Flush
	st := fx.stat()
	assert.Equal(t, int64(1), st.FlushSkippedAbandoned)
	assert.Equal(t, int64(1), st.WakeGenCompleted, "the generation still completes")
	assert.Equal(t, refreshes+1, fx.refreshCalls(), "the Foreground refresh still runs")
	assert.Equal(t, syncs+1, fx.syncer.callCount(), "head-sync still runs")
	assert.Equal(t, int32(1), calls.Load(), "no overlapping Flush started")
}

func TestNetworkState_RefreshOrdering(t *testing.T) {
	t.Run("transition recovery: refresh runs in the worker after the flush", func(t *testing.T) {
		fx := newNetworkStateFixtureOpts(t, fixtureOpts{mobile: true})
		fx.foregroundSettled()
		fx.StateChange(int(domain.CompStateAppWentBackground))
		fx.ticks(4)
		fx.StateChange(int(domain.CompStateAppWentForeground))
		ev := fx.events()
		assert.Equal(t, "refresh", ev[len(ev)-1], "settle refresh only; the new one waits for the worker")
		n := len(ev)
		fx.drain()
		assert.Equal(t, []string{"flush", "refresh"}, fx.events()[n:])
	})
	t.Run("no recovery admitted: refresh inline", func(t *testing.T) {
		fx := newNetworkStateFixtureOpts(t, fixtureOpts{mobile: true})
		fx.foregroundSettled()
		n := len(fx.events())
		fx.StateChange(int(domain.CompStateAppWentBackground))
		fx.ticks(1)
		fx.StateChange(int(domain.CompStateAppWentForeground))
		assert.Equal(t, []string{"refresh"}, fx.events()[n:])
		fx.drain()
		assert.Equal(t, []string{"refresh"}, fx.events()[n:])
	})
	t.Run("transition coalesced into a trailing run that is skipped still refreshes", func(t *testing.T) {
		fx := newNetworkStateFixtureOpts(t, fixtureOpts{mobile: true})
		fx.SetNetworkState(model.DeviceNetworkType_WIFI, "w1")
		fx.foregroundSettled()
		fx.StateChange(int(domain.CompStateAppWentBackground))
		fx.ticks(4)
		fx.SetNetworkState(model.DeviceNetworkType_WIFI, "w2") // leading
		fx.drain()
		n := len(fx.events())
		fx.StateChange(int(domain.CompStateAppWentForeground)) // coalesced into the trailing run
		require.Equal(t, 1, fx.fireScheduled())
		fx.drain()
		assert.Equal(t, []string{"refresh"}, fx.events()[n:])
		assert.Equal(t, int64(1), fx.stat().TrailingSkipped)
	})
	t.Run("closing: queued recovery completes without flush, hooks, head-sync or refresh", func(t *testing.T) {
		fx := newNetworkStateFixtureOpts(t, fixtureOpts{mobile: true})
		hooks := 0
		fx.RegisterConnectivityHook(func(bool) { hooks++ })
		fx.sleepFor(time.Hour)
		fx.StateChange(int(domain.CompStateAppWentForeground)) // queued with refresh, covers gen 1
		fx.StateChange(int(domain.CompStateAppClosingInitiated))
		fx.drain()
		assert.Empty(t, fx.events())
		assert.Equal(t, int64(1), fx.stat().WakeGenCompleted)
		assert.Equal(t, 0, hooks)
		assert.Equal(t, 0, fx.syncer.callCount())
		st := fx.stat()
		assert.True(t, st.Closing)
	})
}

func TestNetworkState_FreezeEstimate(t *testing.T) {
	t.Run("macOS sleep: wall +60s, mono +5s is a wake", func(t *testing.T) {
		fx := newNetworkStateFixture(t)
		base := fx.foregroundSettled()
		fx.clock.advance(60*time.Second, 5*time.Second)
		fx.onHeartbeat()
		fx.drain()
		assert.Equal(t, base+1, fx.flushes.Load())
		assert.Equal(t, int64(55_000), fx.stat().LastWakeFrozenForMs)
	})
	t.Run("macOS starvation: both clocks +60s is not a wake", func(t *testing.T) {
		fx := newNetworkStateFixture(t)
		base := fx.foregroundSettled()
		fx.clock.advance(60*time.Second, 60*time.Second)
		fx.onHeartbeat()
		fx.drain()
		assert.Equal(t, base, fx.flushes.Load())
		assert.Equal(t, int64(0), fx.stat().WakeGenObserved)
	})
	t.Run("a 15s freeze is not a wake", func(t *testing.T) {
		fx := newNetworkStateFixture(t)
		base := fx.foregroundSettled()
		fx.sleepFor(15 * time.Second)
		fx.onHeartbeat()
		fx.drain()
		assert.Equal(t, base, fx.flushes.Load())
		assert.Equal(t, int64(0), fx.stat().WakeGenObserved)
	})
	t.Run("a 25s freeze is a wake (threshold is 20s)", func(t *testing.T) {
		fx := newNetworkStateFixture(t)
		base := fx.foregroundSettled()
		fx.sleepFor(25 * time.Second)
		fx.onHeartbeat()
		fx.drain()
		assert.Equal(t, base+1, fx.flushes.Load())
	})
	t.Run("Windows sleep: mono +60s is a wake", func(t *testing.T) {
		fx := newNetworkStateFixtureOpts(t, fixtureOpts{monoCountsSleep: true})
		base := fx.foregroundSettled()
		fx.clock.advance(60*time.Second, 60*time.Second)
		fx.onHeartbeat()
		fx.drain()
		assert.Equal(t, base+1, fx.flushes.Load())
		assert.Equal(t, int64(55_000), fx.stat().LastWakeFrozenForMs)
	})
	t.Run("Windows: a starved sampler is indistinguishable from sleep (accepted false wake)", func(t *testing.T) {
		fx := newNetworkStateFixtureOpts(t, fixtureOpts{monoCountsSleep: true})
		base := fx.foregroundSettled()
		fx.clock.advance(30*time.Second, 30*time.Second) // 25s over one period
		fx.onHeartbeat()
		fx.drain()
		assert.Equal(t, base+1, fx.flushes.Load())
	})
	t.Run("backward step and sleep in the same interval cancel out (known limit)", func(t *testing.T) {
		fx := newNetworkStateFixture(t)
		base := fx.foregroundSettled()
		fx.clock.advance(-time.Hour+5*time.Minute, 0) // -1h step, then a 5 min sleep
		fx.onHeartbeat()
		fx.drain()
		assert.Equal(t, base, fx.flushes.Load())
	})
}

func TestNetworkState_WakeCoverage(t *testing.T) {
	t.Run("macOS second wake during a running Flush: the coalesced trailing run still flushes", func(t *testing.T) {
		fx := newNetworkStateFixture(t)
		base := fx.foregroundSettled()
		entered, release := make(chan struct{}), make(chan struct{})
		var once sync.Once
		fx.flushHook = func() { once.Do(func() { close(entered); <-release }) }
		fx.sleepFor(10 * time.Minute)
		fx.onHeartbeat()
		done := make(chan struct{})
		go func() { fx.drainRecoveries(); close(done) }()
		waitFor(t, entered, "flush")
		fx.sleepFor(10 * time.Minute) // mono paused: still inside the 6s window
		fx.onHeartbeat()
		st := fx.stat()
		assert.Equal(t, int64(2), st.WakeGenQueued, "the pending trailing run covers gen 2")
		assert.Equal(t, int64(1), st.WakeGenRunning)
		assert.Equal(t, "queued", st.LastWakeOutcome)
		close(release)
		waitFor(t, done, "drain")
		fx.onHeartbeat()
		assert.Equal(t, base+1, fx.flushCalls.Load(), "gen 2 is queued: no second admission")
		fx.fireScheduled()
		fx.drain()
		assert.Equal(t, base+2, fx.flushes.Load())
		assert.Equal(t, int64(2), fx.stat().WakeGenCompleted)
		assert.Equal(t, "completed", fx.stat().LastWakeOutcome)
	})
	t.Run("desktop fallback whose Flush takes >6s, then Foreground: no second flush", func(t *testing.T) {
		fx := newNetworkStateFixture(t)
		base := fx.foregroundSettled()
		entered, release := make(chan struct{}), make(chan struct{})
		var once sync.Once
		fx.flushHook = func() { once.Do(func() { close(entered); <-release }) }
		fx.StateChange(int(domain.CompStateAppWentBackground))
		fx.sleepFor(time.Hour)
		fx.onHeartbeat()
		done := make(chan struct{})
		go func() { fx.drainRecoveries(); close(done) }()
		waitFor(t, entered, "flush")
		fx.ticks(2) // flush is slow: 10s
		fx.StateChange(int(domain.CompStateAppWentForeground))
		assert.Equal(t, int64(1), fx.stat().ForegroundSuppressed, "running recovery covers the wake")
		close(release)
		waitFor(t, done, "drain")
		fx.fireScheduled()
		fx.drain()
		assert.Equal(t, base+1, fx.flushes.Load())
	})
	t.Run("a wake before the last Foreground transition doesn't suppress a later transition", func(t *testing.T) {
		fx := newNetworkStateFixtureOpts(t, fixtureOpts{mobile: true})
		base := fx.foregroundSettled()
		fx.StateChange(int(domain.CompStateAppWentBackground))
		fx.sleepFor(10 * time.Minute)
		fx.StateChange(int(domain.CompStateAppWentForeground)) // wake recovered at this transition
		fx.drain()
		require.Equal(t, base+1, fx.flushes.Load())
		fx.ticks(1)
		fx.StateChange(int(domain.CompStateAppWentBackground))
		fx.ticks(4) // 20s background, recovery completed 25s ago (< 30s)
		fx.StateChange(int(domain.CompStateAppWentForeground))
		fx.drain()
		assert.Equal(t, base+2, fx.flushes.Load(), "the earlier wake belongs to the previous interval")
	})
	t.Run("late Background after the sampler opened the generation: one flush for one wake", func(t *testing.T) {
		fx := newNetworkStateFixture(t)
		base := fx.foregroundSettled()
		fx.sleepFor(10 * time.Minute)
		fx.onHeartbeat() // fallback
		fx.drain()
		fx.StateChange(int(domain.CompStateAppWentBackground)) // late suspend report
		fx.ticks(4)
		fx.StateChange(int(domain.CompStateAppWentForeground)) // 20s later
		fx.drain()
		assert.Equal(t, base+1, fx.flushes.Load())
		assert.Equal(t, int64(1), fx.stat().ForegroundSuppressed)
	})
	t.Run("freeze between enqueue and execution start: bound at start, one flush", func(t *testing.T) {
		fx := newNetworkStateFixture(t)
		base := fx.foregroundSettled()
		fx.triggerRecovery("network type changed to X", triggerNetworkType) // queued, not run
		fx.sleepFor(time.Hour)
		fx.drain() // the worker runs before any sample
		st := fx.stat()
		assert.Equal(t, observeSourceRecoveryStart, st.LastWakeSource)
		assert.Equal(t, int64(1), st.WakeGenCompleted)
		fx.ticks(3)
		fx.fireScheduled()
		fx.drain()
		assert.Equal(t, base+1, fx.flushes.Load())
	})
	t.Run("generation bound at execution start counts as running: no second admission", func(t *testing.T) {
		fx := newNetworkStateFixture(t)
		base := fx.foregroundSettled()
		entered, release := make(chan struct{}), make(chan struct{})
		var once sync.Once
		fx.flushHook = func() { once.Do(func() { close(entered); <-release }) }
		fx.ticks(2)
		fx.triggerRecovery("network type changed to X", triggerNetworkType) // queued before the freeze
		fx.sleepFor(time.Hour)
		done := make(chan struct{})
		go func() { fx.drainRecoveries(); close(done) }() // binds gen 1 at start
		waitFor(t, entered, "flush")
		st := fx.stat()
		require.Equal(t, int64(0), st.WakeGenQueued)
		require.Equal(t, int64(1), st.WakeGenRunning)
		fx.onHeartbeat() // desktop fallback must see gen 1 as covered
		fx.StateChange(int(domain.CompStateAppWentForeground))
		assert.False(t, fx.stat().RecoveryQueued)
		close(release)
		waitFor(t, done, "drain")
		fx.drain()
		assert.Equal(t, base+1, fx.flushCalls.Load())
	})
	t.Run("a leading admission replaces a queued trailing run", func(t *testing.T) {
		fx := newNetworkStateFixtureOpts(t, fixtureOpts{mobile: true})
		fx.SetNetworkState(model.DeviceNetworkType_WIFI, "w1")
		base := fx.foregroundSettled()
		fx.StateChange(int(domain.CompStateAppWentBackground))
		fx.ticks(4)
		fx.SetNetworkState(model.DeviceNetworkType_WIFI, "w2") // leading
		fx.drain()
		fx.triggerRecovery("interface addresses regained", triggerInterfaceRegained) // coalesced
		require.Equal(t, 1, fx.fireScheduled())                                      // trailing queued, not run
		fx.clock.advance(7*time.Second, 7*time.Second)
		fx.StateChange(int(domain.CompStateAppWentForeground)) // leading transition
		fx.drain()
		assert.Equal(t, base+2, fx.flushes.Load(), "the trailing run would be skipped; the transition must run")
		assert.Equal(t, triggerTransition, fx.stat().LastRecoveryTrigger)
	})
	t.Run("foregroundWake: short background after a pending mobile wake", func(t *testing.T) {
		fx := newNetworkStateFixtureOpts(t, fixtureOpts{mobile: true})
		base := fx.foregroundSettled()
		fx.sleepFor(10 * time.Minute)
		fx.onHeartbeat() // mobile: pending
		fx.StateChange(int(domain.CompStateAppWentBackground))
		fx.ticks(1)
		fx.StateChange(int(domain.CompStateAppWentForeground))
		fx.drain()
		assert.Equal(t, base+1, fx.flushes.Load())
		st := fx.stat()
		assert.Equal(t, triggerForegroundWake, st.LastRecoveryTrigger)
		assert.Equal(t, int64(1), st.ExecutedByTrigger[triggerForegroundWake])
	})
	t.Run("a leading run queued behind a running one is not replaced by a trailing run", func(t *testing.T) {
		fx := newNetworkStateFixture(t)
		base := fx.foregroundSettled()
		fx.StateChange(int(domain.CompStateAppWentBackground))
		fx.ticks(10) // 50s background, no freeze
		fx.triggerRecovery("interface addresses regained", triggerInterfaceRegained)
		first := true
		fx.flushHook = func() {
			if !first {
				return
			}
			first = false
			fx.clock.advance(7*time.Second, 7*time.Second)
			fx.StateChange(int(domain.CompStateAppWentForeground))                       // leading (window passed)
			fx.triggerRecovery("interface addresses regained", triggerInterfaceRegained) // coalesced -> pending
			fx.fireScheduled()                                                           // trailing merges into the slot
		}
		fx.drain()
		assert.Equal(t, base+2, fx.flushes.Load())
		assert.Equal(t, triggerTransition, fx.stat().LastRecoveryTrigger)
	})
}

func TestNetworkState_LiveWake(t *testing.T) {
	// real sampler and worker goroutines; only the clock is fake
	fx := newNetworkStateFixtureOpts(t, fixtureOpts{liveWorker: true, heartbeatEvery: 2 * time.Millisecond})
	fx.StateChange(int(domain.CompStateAppWentForeground))
	require.Eventually(t, func() bool { return fx.flushes.Load() == 1 }, time.Second, time.Millisecond)
	fx.clock.advance(time.Minute, time.Minute) // let the window pass
	time.Sleep(20 * time.Millisecond)
	fx.sleepFor(10 * time.Minute)
	require.Eventually(t, func() bool { return fx.stat().WakeGenCompleted == 1 }, 2*time.Second, time.Millisecond)
	time.Sleep(50 * time.Millisecond) // many more samples
	assert.Equal(t, int64(2), fx.flushes.Load())
	assert.Equal(t, int64(1), fx.stat().WakeGenObserved)
}

func TestNetworkState_RefreshTiming(t *testing.T) {
	t.Run("P1 foregroundWake: refresh after its flush", func(t *testing.T) {
		fx := newNetworkStateFixtureOpts(t, fixtureOpts{mobile: true})
		fx.foregroundSettled()
		fx.sleepFor(10 * time.Minute)
		fx.onHeartbeat()
		fx.StateChange(int(domain.CompStateAppWentBackground))
		fx.ticks(1)
		n := len(fx.events())
		fx.StateChange(int(domain.CompStateAppWentForeground))
		assert.Empty(t, fx.events()[n:], "deferred: a flush is queued")
		fx.drain()
		assert.Equal(t, []string{"flush", "refresh"}, fx.events()[n:])
	})
	t.Run("P2 iOS resume: path callback leads, Foreground right after refreshes immediately", func(t *testing.T) {
		fx := newNetworkStateFixtureOpts(t, fixtureOpts{mobile: true})
		fx.SetNetworkState(model.DeviceNetworkType_WIFI, "w1")
		fx.foregroundSettled()
		fx.StateChange(int(domain.CompStateAppWentBackground))
		fx.ticks(60)
		fx.SetNetworkState(model.DeviceNetworkType_CELLULAR, "c1") // leading recovery
		fx.drain()
		n := len(fx.events())
		fx.StateChange(int(domain.CompStateAppWentForeground)) // coalesced, leading already done
		assert.Equal(t, []string{"refresh"}, fx.events()[n:], "no 6s wait for the trailing timer")
		fx.SetNetworkState(model.DeviceNetworkType_CELLULAR, "c2") // another coalesced signal
		fx.fireScheduled()
		fx.drain()
		assert.Equal(t, []string{"refresh", "flush"}, fx.events()[n:], "the refresh is not repeated")
	})
	t.Run("P3 leading run merges into a queued transition and keeps its refresh", func(t *testing.T) {
		fx := newNetworkStateFixtureOpts(t, fixtureOpts{mobile: true})
		fx.SetNetworkState(model.DeviceNetworkType_WIFI, "w1")
		fx.foregroundSettled()
		fx.StateChange(int(domain.CompStateAppWentBackground))
		fx.ticks(4)
		n := len(fx.events())
		fx.StateChange(int(domain.CompStateAppWentForeground)) // queued, refresh
		fx.clock.advance(7*time.Second, 7*time.Second)
		fx.SetNetworkState(model.DeviceNetworkType_WIFI, "w2") // leading, merges
		fx.drain()
		assert.Equal(t, []string{"flush", "refresh"}, fx.events()[n:])
	})
	t.Run("transition coalesced while the leading run is only queued: refresh inline, not behind the queue", func(t *testing.T) {
		fx := newNetworkStateFixtureOpts(t, fixtureOpts{mobile: true})
		fx.SetNetworkState(model.DeviceNetworkType_WIFI, "w1")
		fx.foregroundSettled()
		fx.StateChange(int(domain.CompStateAppWentBackground))
		fx.ticks(4)
		n := len(fx.events())
		fx.SetNetworkState(model.DeviceNetworkType_WIFI, "w2") // leading, queued
		fx.StateChange(int(domain.CompStateAppWentForeground)) // coalesced
		assert.Equal(t, []string{"refresh"}, fx.events()[n:], "as before GO-7556: immediately")
		fx.drain()
		assert.Equal(t, []string{"refresh", "flush"}, fx.events()[n:])
	})
	t.Run("suppressed transition during a running wake recovery: refresh after that flush", func(t *testing.T) {
		fx := newNetworkStateFixture(t)
		base := fx.foregroundSettled()
		entered, release := make(chan struct{}), make(chan struct{})
		var once sync.Once
		fx.flushHook = func() { once.Do(func() { close(entered); <-release }) }
		fx.StateChange(int(domain.CompStateAppWentBackground))
		fx.sleepFor(time.Hour)
		fx.onHeartbeat() // desktop fallback queued
		done := make(chan struct{})
		go func() { fx.drainRecoveries(); close(done) }()
		waitFor(t, entered, "flush")
		n := len(fx.events())
		fx.StateChange(int(domain.CompStateAppWentForeground)) // suppressed: covered by the running run
		assert.Empty(t, fx.events()[n:], "must not refresh on the pre-wake connections")
		close(release)
		waitFor(t, done, "drain")
		assert.Equal(t, []string{"flush", "refresh"}, fx.events()[n:])
		assert.Equal(t, base+1, fx.flushes.Load())
		assert.Equal(t, int64(1), fx.stat().ForegroundSuppressed)
	})
	t.Run("suppressed transition while the wake recovery is only queued: refresh inline", func(t *testing.T) {
		fx := newNetworkStateFixture(t)
		fx.foregroundSettled()
		fx.StateChange(int(domain.CompStateAppWentBackground))
		fx.sleepFor(time.Hour)
		fx.onHeartbeat()
		n := len(fx.events())
		fx.StateChange(int(domain.CompStateAppWentForeground))
		assert.Equal(t, []string{"refresh"}, fx.events()[n:])
		fx.drain()
		assert.Equal(t, []string{"refresh", "flush"}, fx.events()[n:], "no second refresh")
	})
	t.Run("a stuck refresh doesn't wedge the worker", func(t *testing.T) {
		fx := newNetworkStateFixtureOpts(t, fixtureOpts{mobile: true})
		stuck := make(chan struct{})
		defer close(stuck)
		fx.refreshBound = 20 * time.Millisecond
		fx.mockRefresher.ExpectedCalls = nil
		fx.mockRefresher.EXPECT().RefreshOpenedObjects(mock.Anything).Run(func(context.Context) { <-stuck }).Return().Maybe()
		start := time.Now()
		fx.StateChange(int(domain.CompStateAppWentForeground))
		fx.drain()
		assert.Less(t, time.Since(start), time.Second)
		assert.Equal(t, int64(1), fx.stat().RefreshTimeouts)
		fx.ticks(2)
		fx.SetNetworkState(model.DeviceNetworkType_CELLULAR, "")
		fx.SetNetworkState(model.DeviceNetworkType_WIFI, "")
		fx.drain()
		assert.Equal(t, int64(2), fx.flushes.Load())
	})
	t.Run("Foreground with only a queued trailing run refreshes inline; the skip adds nothing", func(t *testing.T) {
		fx := newNetworkStateFixtureOpts(t, fixtureOpts{mobile: true})
		fx.SetNetworkState(model.DeviceNetworkType_WIFI, "w1")
		fx.foregroundSettled()
		fx.StateChange(int(domain.CompStateAppWentBackground))
		fx.ticks(4)
		fx.SetNetworkState(model.DeviceNetworkType_WIFI, "w2")
		fx.drain()
		fx.triggerRecovery("interface addresses regained", triggerInterfaceRegained)
		require.Equal(t, 1, fx.fireScheduled())
		n := len(fx.events())
		fx.StateChange(int(domain.CompStateAppWentForeground))
		assert.Equal(t, []string{"refresh"}, fx.events()[n:])
		fx.drain()
		assert.Equal(t, []string{"refresh"}, fx.events()[n:])
		assert.Equal(t, int64(1), fx.stat().TrailingSkipped)
	})
}

func TestNetworkState_Shutdown(t *testing.T) {
	t.Run("Close during a running Flush: no hooks, head-sync or refresh afterwards", func(t *testing.T) {
		fx := newNetworkStateFixtureOpts(t, fixtureOpts{liveWorker: true})
		var hooks atomic.Int64
		entered, release := make(chan struct{}), make(chan struct{})
		var once sync.Once
		fx.flushHook = func() { once.Do(func() { close(entered); <-release }) }
		fx.StateChange(int(domain.CompStateAppWentForeground))
		waitFor(t, entered, "flush")
		fx.RegisterConnectivityHook(func(bool) { hooks.Inc() })
		closed := make(chan struct{})
		go func() { _ = fx.Close(ctx); close(closed) }()
		waitFor(t, closed, "Close") // cancels the flush
		close(release)
		waitFor(t, fx.workerDone, "worker exit")
		assert.Equal(t, int64(0), hooks.Load())
		assert.Equal(t, int64(0), fx.stat().FlushErrors, "a Close-cancelled Flush is not a flush error")
		assert.Equal(t, int64(0), fx.stat().FlushTimeouts, "nor a timeout")
		assert.Empty(t, fx.stat().LastFlushError)
		assert.Equal(t, 0, fx.syncer.callCount())
		assert.Equal(t, 0, fx.refreshCalls())
	})
}

func TestNetworkState_MobileParity(t *testing.T) {
	// pre-GO-7556 develop: mobile background measured on the monotonic clock,
	// and only a >30s sleep counted as a wake
	t.Run("mobile: 25s device sleep while locked doesn't flush", func(t *testing.T) {
		fx := newNetworkStateFixtureOpts(t, fixtureOpts{mobile: true})
		base := fx.foregroundSettled()
		fx.StateChange(int(domain.CompStateAppWentBackground))
		fx.ticks(1)
		fx.sleepFor(25 * time.Second)
		fx.StateChange(int(domain.CompStateAppWentForeground))
		fx.drain()
		assert.Equal(t, base, fx.flushes.Load())
		assert.Equal(t, 1, fx.refreshCalls()-1, "refresh still inline")
	})
	t.Run("mobile: 40s device sleep while locked flushes once", func(t *testing.T) {
		fx := newNetworkStateFixtureOpts(t, fixtureOpts{mobile: true})
		base := fx.foregroundSettled()
		fx.StateChange(int(domain.CompStateAppWentBackground))
		fx.ticks(1)
		fx.sleepFor(40 * time.Second)
		fx.StateChange(int(domain.CompStateAppWentForeground))
		fx.drain()
		assert.Equal(t, base+1, fx.flushes.Load())
		assert.Equal(t, triggerForegroundWake, fx.stat().LastRecoveryTrigger)
	})
	t.Run("desktop: 25s sleep is a wake", func(t *testing.T) {
		fx := newNetworkStateFixture(t)
		base := fx.foregroundSettled()
		fx.StateChange(int(domain.CompStateAppWentBackground))
		fx.ticks(1)
		fx.sleepFor(25 * time.Second)
		fx.StateChange(int(domain.CompStateAppWentForeground))
		fx.drain()
		assert.Equal(t, base+1, fx.flushes.Load())
	})
}

func TestNetworkState_GenerationBaseAndTrailingDelay(t *testing.T) {
	t.Run("duplicate Foreground doesn't move the Foreground generation base", func(t *testing.T) {
		fx := newNetworkStateFixtureOpts(t, fixtureOpts{mobile: true})
		base := fx.foregroundSettled()
		fx.sleepFor(10 * time.Minute)
		fx.StateChange(int(domain.CompStateAppWentForeground)) // duplicate with a wake: recovers
		fx.drain()
		require.Equal(t, base+1, fx.flushes.Load())
		fx.StateChange(int(domain.CompStateAppWentBackground))
		fx.ticks(4) // 20s; recovery completed 20s ago
		fx.StateChange(int(domain.CompStateAppWentForeground))
		fx.drain()
		assert.Equal(t, base+1, fx.flushes.Load(), "the wake belongs to this background interval: covered")
	})
	t.Run("trailing timer is armed for the rest of the window", func(t *testing.T) {
		fx := newNetworkStateFixture(t)
		fx.SetNetworkState(model.DeviceNetworkType_CELLULAR, "")
		fx.SetNetworkState(model.DeviceNetworkType_WIFI, "") // leading
		fx.clock.advance(2*time.Second, 2*time.Second)
		fx.SetNetworkState(model.DeviceNetworkType_CELLULAR, "") // coalesced 2s later
		assert.Equal(t, recoverySuppressWindow-2*time.Second, fx.lastDelay())
	})
}

func TestNetworkState_CancellationAndBoundaries(t *testing.T) {
	t.Run("no Flush once the run context is cancelled", func(t *testing.T) {
		fx := newNetworkStateFixture(t)
		fx.SetNetworkState(model.DeviceNetworkType_CELLULAR, "")
		fx.SetNetworkState(model.DeviceNetworkType_WIFI, "") // queued
		fx.runCancel()                                       // Close racing the worker
		fx.drain()
		assert.Equal(t, int64(0), fx.flushCalls.Load())
		assert.Equal(t, int64(1), fx.stat().FlushSkippedClosed)
		assert.Equal(t, 0, fx.syncer.callCount())
	})
	t.Run("Close honours its context", func(t *testing.T) {
		fx := newNetworkStateFixtureOpts(t, fixtureOpts{liveWorker: true})
		entered, release := make(chan struct{}), make(chan struct{})
		defer close(release)
		var once sync.Once
		fx.RegisterConnectivityHook(func(bool) { once.Do(func() { close(entered); <-release }) })
		fx.closeBound = 3 * time.Second
		fx.StateChange(int(domain.CompStateAppWentForeground))
		waitFor(t, entered, "hook")
		cctx, cancel := context.WithCancel(ctx)
		cancel()
		start := time.Now()
		require.NoError(t, fx.Close(cctx))
		assert.Less(t, time.Since(start), time.Second)
	})
	t.Run("closing during a running recovery drops the attached refresh", func(t *testing.T) {
		fx := newNetworkStateFixture(t)
		fx.foregroundSettled()
		entered, release := make(chan struct{}), make(chan struct{})
		var once sync.Once
		fx.flushHook = func() { once.Do(func() { close(entered); <-release }) }
		fx.StateChange(int(domain.CompStateAppWentBackground))
		fx.sleepFor(time.Hour)
		fx.onHeartbeat()
		done := make(chan struct{})
		go func() { fx.drainRecoveries(); close(done) }()
		waitFor(t, entered, "flush")
		n := len(fx.events())
		fx.StateChange(int(domain.CompStateAppWentForeground)) // attaches to the running run
		fx.StateChange(int(domain.CompStateAppClosingInitiated))
		close(release)
		waitFor(t, done, "drain")
		assert.Equal(t, []string{"flush"}, fx.events()[n:])
	})
	t.Run("covering recovery exactly 30s before Foreground no longer suppresses", func(t *testing.T) {
		fx := newNetworkStateFixture(t)
		base := fx.foregroundSettled()
		fx.StateChange(int(domain.CompStateAppWentBackground))
		fx.sleepFor(time.Hour)
		fx.onHeartbeat()
		fx.drain()  // completes now
		fx.ticks(6) // exactly wakeCoverWindow
		fx.StateChange(int(domain.CompStateAppWentForeground))
		fx.drain()
		assert.Equal(t, base+2, fx.flushes.Load())
	})
	t.Run("desktop: backward wall step doesn't shrink a background below monotonic time", func(t *testing.T) {
		fx := newNetworkStateFixture(t)
		base := fx.foregroundSettled()
		fx.StateChange(int(domain.CompStateAppWentBackground))
		fx.clock.advance(-time.Hour, heartbeatInterval)
		fx.onHeartbeat()
		fx.ticks(4)
		fx.StateChange(int(domain.CompStateAppWentForeground))
		fx.drain()
		assert.Equal(t, base+1, fx.flushes.Load())
		assert.Equal(t, triggerTransition, fx.stat().LastRecoveryTrigger)
	})
}

func TestWakeTracker_Observe(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0)
	t.Run("first observation only sets the baseline", func(t *testing.T) {
		var w wakeTracker
		assert.False(t, w.observe(t0, time.Hour, false, heartbeatInterval, wakeGapThreshold).newGen)
		assert.True(t, w.init)
	})
	t.Run("drift above the threshold opens a generation", func(t *testing.T) {
		var w wakeTracker
		w.observe(t0, time.Hour, false, heartbeatInterval, wakeGapThreshold)
		o := w.observe(t0.Add(30*time.Second), time.Hour+5*time.Second, false, heartbeatInterval, wakeGapThreshold)
		assert.True(t, o.newGen)
		assert.Equal(t, 25*time.Second, o.drift)
		assert.Equal(t, int64(1), w.observed)
	})
	t.Run("mobile threshold is 30s", func(t *testing.T) {
		var w wakeTracker
		w.observe(t0, time.Hour, false, heartbeatInterval, mobileWakeGapThreshold)
		assert.False(t, w.observe(t0.Add(25*time.Second), time.Hour, false, heartbeatInterval, mobileWakeGapThreshold).newGen)
	})
	t.Run("monotonic baseline never moves back", func(t *testing.T) {
		var w wakeTracker
		w.observe(t0, time.Hour, false, heartbeatInterval, wakeGapThreshold)
		w.observe(t0.Add(time.Second), time.Hour-time.Minute, false, heartbeatInterval, wakeGapThreshold)
		assert.Equal(t, time.Hour, w.baseMono)
	})
	t.Run("Windows subtracts one period", func(t *testing.T) {
		var w wakeTracker
		w.observe(t0, time.Hour, true, 10*time.Second, wakeGapThreshold)
		o := w.observe(t0, time.Hour+29*time.Second, true, 10*time.Second, wakeGapThreshold)
		assert.False(t, o.newGen)
		assert.Equal(t, 19*time.Second, o.frozen)
	})
	t.Run("outcome is derived from the watermarks", func(t *testing.T) {
		w := wakeTracker{observed: 2, queued: 2, completed: 1}
		assert.Equal(t, "queued", w.outcome(false))
		w.running = 2
		assert.Equal(t, "running", w.outcome(false))
		w.completed = 2
		assert.Equal(t, "completed", w.outcome(false))
		w = wakeTracker{observed: 1}
		assert.Equal(t, "pendingForeground", w.outcome(true))
		assert.Equal(t, "observed", w.outcome(false))
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
		fx.drain()
		assert.Empty(t, hookCalls)
		assert.Equal(t, 0, fx.syncer.callCount())

		// a real switch flushes the pool, fires hooks and head-syncs
		fx.SetNetworkState(model.DeviceNetworkType_WIFI, "")
		fx.drain()
		assert.Equal(t, int64(1), fx.flushes.Load())
		assert.Equal(t, []bool{true}, hookCalls)
		assert.Equal(t, 1, fx.syncer.callCount())
	})
	t.Run("same type, different networkId triggers recovery", func(t *testing.T) {
		fx := newNetworkStateFixture(t)
		fx.SetNetworkState(model.DeviceNetworkType_CELLULAR, "cell-1") // first report

		// same type, new path identity (e.g. Wi-Fi->Wi-Fi or PDP re-attach)
		fx.SetNetworkState(model.DeviceNetworkType_CELLULAR, "cell-2")
		fx.drain()
		assert.Equal(t, int64(1), fx.flushes.Load())
		assert.Equal(t, 1, fx.syncer.callCount())

		// repeating the same id is a no-op
		fx.SetNetworkState(model.DeviceNetworkType_CELLULAR, "cell-2")
		fx.drain()
		assert.Equal(t, 1, fx.syncer.callCount())

		// an empty id from an older client does not count as a change
		fx.SetNetworkState(model.DeviceNetworkType_CELLULAR, "")
		fx.drain()
		assert.Equal(t, 1, fx.syncer.callCount())
	})
	t.Run("switch to NOT_CONNECTED flushes but does not head-sync", func(t *testing.T) {
		fx := newNetworkStateFixture(t)
		var hookCalls []bool
		fx.RegisterConnectivityHook(func(online bool) { hookCalls = append(hookCalls, online) })
		fx.SetNetworkState(model.DeviceNetworkType_CELLULAR, "") // first report

		fx.SetNetworkState(model.DeviceNetworkType_NOT_CONNECTED, "")
		fx.drain()
		assert.Equal(t, int64(1), fx.flushes.Load())
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
		fx.SetNetworkState(model.DeviceNetworkType_CELLULAR, "") // first report, no recovery

		// rapid switches: the first runs immediately, the second coalesces into
		// exactly one scheduled trailing run
		fx.SetNetworkState(model.DeviceNetworkType_WIFI, "")     // leading recovery at WIFI
		fx.drain()                                               // leading run acts on WIFI
		fx.SetNetworkState(model.DeviceNetworkType_CELLULAR, "") // suppressed
		fx.drain()
		assert.True(t, fx.stat().RecoveryQueued, "a pending trailing run counts as queued")
		assert.Equal(t, 1, fx.syncer.callCount())
		require.Equal(t, 1, fx.scheduledCount())
		fx.recoveryMu.Lock()
		assert.Contains(t, fx.pendingReason, "CELLULAR", "trailing run must report the latest coalesced reason")
		fx.recoveryMu.Unlock()

		// state at trailing time (CELLULAR) differs from what the leading run
		// acted on (WIFI) -> the trailing run executes the full pipeline
		fx.clock.advance(recoverySuppressWindow, recoverySuppressWindow)
		fx.fireScheduled()
		fx.drain()
		assert.Equal(t, int64(2), fx.flushes.Load())
		assert.Equal(t, 2, fx.syncer.callCount())
	})
	t.Run("trailing run fires when the link flapped even if addresses end up identical", func(t *testing.T) {
		// a short outage bracketing a wake: the leading run acted while the
		// link was down (its dials failed), so an identical final address set
		// must NOT be treated as "already handled" — the monitor generation
		// makes the flap visible to the fingerprint
		fx := newNetworkStateFixture(t)
		fx.SetNetworkState(model.DeviceNetworkType_CELLULAR, "") // first report

		fx.SetNetworkState(model.DeviceNetworkType_WIFI, "") // leading recovery
		fx.drain()

		// link drops and comes back with the same address while suppressed
		key := fx.networkState.monitorSnapshot.Load()
		fx.onMonitorSnapshot("", true)
		fx.onMonitorSnapshot(key, false)
		fx.triggerRecovery("interface addresses regained", triggerInterfaceRegained) // suppressed -> pending
		require.Equal(t, 1, fx.scheduledCount())

		// same type, same id, same final snapshot — but the generation moved,
		// so the trailing run must execute the full pipeline
		fx.clock.advance(recoverySuppressWindow, recoverySuppressWindow)
		fx.fireScheduled()
		fx.drain()
		assert.Equal(t, 2, fx.syncer.callCount(), "flapped link must not be swallowed by the skip")
	})
	t.Run("trailing run is skipped when the network is unchanged (duplicate signals)", func(t *testing.T) {
		// a wake fires the freeze detector, the interface diff and the
		// foreground RPC within seconds: the duplicates must not flush the
		// connections the leading run just re-established
		fx := newNetworkStateFixture(t)
		fx.SetNetworkState(model.DeviceNetworkType_CELLULAR, "") // first report

		fx.SetNetworkState(model.DeviceNetworkType_WIFI, "")     // leading recovery at WIFI
		fx.SetNetworkState(model.DeviceNetworkType_CELLULAR, "") // suppressed
		fx.SetNetworkState(model.DeviceNetworkType_WIFI, "")     // back to the recovered state
		require.Equal(t, 1, fx.scheduledCount())
		fx.drain()

		// trailing fingerprint equals the leading one -> no second teardown
		fx.clock.advance(recoverySuppressWindow, recoverySuppressWindow)
		fx.fireScheduled()
		fx.drain()
		assert.Equal(t, 1, fx.syncer.callCount(), "duplicate trailing run must be skipped")
		assert.Equal(t, int64(1), fx.stat().TrailingSkipped)
	})
	t.Run("trailing run merges into a still-queued leading run", func(t *testing.T) {
		// the suppressed trailing run must not count its own queued work as
		// done, but once the leading run has completed an unchanged network
		// is a duplicate
		fx := newNetworkStateFixture(t)
		fx.SetNetworkState(model.DeviceNetworkType_CELLULAR, "")
		fx.SetNetworkState(model.DeviceNetworkType_WIFI, "")
		fx.triggerRecovery("interface addresses regained", triggerInterfaceRegained) // suppressed
		fx.fireScheduled()                                                           // trailing merges behind the queued leading run
		fx.drain()
		assert.Equal(t, int64(1), fx.flushes.Load())
	})
	t.Run("close cancels a pending trailing recovery", func(t *testing.T) {
		fx := newNetworkStateFixture(t)
		fx.SetNetworkState(model.DeviceNetworkType_CELLULAR, "")
		fx.SetNetworkState(model.DeviceNetworkType_WIFI, "") // immediate recovery
		fx.drain()
		fx.SetNetworkState(model.DeviceNetworkType_CELLULAR, "") // suppressed -> pending
		require.Equal(t, 1, fx.scheduledCount())

		require.NoError(t, fx.Close(ctx))
		fx.fireScheduled() // must be a no-op after close: no flush, no head-sync
		fx.drain()
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

func TestNetworkState_ProvideStat(t *testing.T) {
	fx := newNetworkStateFixture(t)
	fx.SetNetworkState(model.DeviceNetworkType_CELLULAR, "cell-1") // first report: no recovery
	fx.SetNetworkState(model.DeviceNetworkType_WIFI, "wifi-1")     // switch: recovery
	fx.drain()

	st := fx.stat()
	assert.Equal(t, model.DeviceNetworkType_WIFI.String(), st.NetworkType)
	assert.Equal(t, "wifi-1", st.NetworkId)
	assert.True(t, st.ReportedByClient)
	assert.False(t, st.Offline)
	assert.Equal(t, int64(2), st.NetworkReports)
	assert.Equal(t, int64(1), st.ExecutedByTrigger[triggerNetworkType])
	assert.Equal(t, int64(1), st.ExecutedByTrigger[triggerNetworkType])
	assert.Equal(t, int64(1), st.Recoveries)
	assert.Equal(t, int64(0), st.RecoveriesOffline)
	assert.Contains(t, st.LastRecoveryReason, "WIFI")
	assert.Equal(t, triggerNetworkType, st.LastRecoveryTrigger)
	assert.NotZero(t, st.LastRecoveryUnix)
	assert.NotEmpty(t, st.MonitorSnapshot)
	assert.Equal(t, "unreported", st.DeviceState)
	assert.False(t, st.RecoveryQueued)
	assert.False(t, st.RecoveryRunning)

	// duplicate report: counted as a report and as a duplicate, not as a
	// signal or recovery
	fx.SetNetworkState(model.DeviceNetworkType_WIFI, "wifi-1")
	fx.drain()
	st = fx.stat()
	assert.Equal(t, int64(3), st.NetworkReports)
	assert.Equal(t, int64(1), st.NetworkReportsDuplicate)
	assert.Equal(t, int64(1), st.Recoveries)

	// wake fields
	fx.ticks(2)
	fx.StateChange(int(domain.CompStateAppWentForeground))
	fx.drain()
	fx.ticks(2)
	fx.sleepFor(time.Minute)
	fx.onHeartbeat()
	fx.drain()
	st = fx.stat()
	assert.Equal(t, "foreground", st.DeviceState)
	assert.Equal(t, int64(1), st.WakeGenObserved)
	assert.Equal(t, int64(1), st.WakeGenQueued)
	assert.Equal(t, int64(1), st.WakeGenRunning)
	assert.Equal(t, int64(1), st.WakeGenCompleted)
	assert.Equal(t, int64(time.Minute/time.Millisecond), st.LastWakeFrozenForMs)
	assert.NotZero(t, st.LastWakeUnix)
	assert.Equal(t, int64(1), st.LastRecoveryWakeGen)
	assert.Equal(t, int64(1), st.ExecutedByTrigger[triggerDesktopFallback])
}

func TestNetworkState_NetworkIdentity(t *testing.T) {
	t.Run("identity combines type, path id and monitor snapshot", func(t *testing.T) {
		// given
		state := &networkState{}
		state.SetNetworkState(model.DeviceNetworkType_WIFI, "wifi-1")
		state.onMonitorSnapshot("10.0.0.5", false)

		// when
		identity, ok := state.NetworkIdentity()

		// then
		assert.True(t, ok)
		assert.Equal(t, networkkey.Key{Reported: true, Type: int32(model.DeviceNetworkType_WIFI), PathId: "wifi-1", Snapshot: "10.0.0.5"}, identity)
	})
	t.Run("identity changes when the network path changes", func(t *testing.T) {
		// given
		state := &networkState{}
		state.SetNetworkState(model.DeviceNetworkType_WIFI, "wifi-1")
		before, _ := state.NetworkIdentity()

		// when
		state.SetNetworkState(model.DeviceNetworkType_WIFI, "wifi-2")

		// then
		after, _ := state.NetworkIdentity()
		assert.NotEqual(t, before, after)
	})
	t.Run("identity stable across duplicate reports", func(t *testing.T) {
		// given
		state := &networkState{}
		state.SetNetworkState(model.DeviceNetworkType_WIFI, "wifi-1")
		before, _ := state.NetworkIdentity()

		// when
		state.SetNetworkState(model.DeviceNetworkType_WIFI, "wifi-1")

		// then
		after, _ := state.NetworkIdentity()
		assert.Equal(t, before, after)
	})
	t.Run("unknown before any report or snapshot", func(t *testing.T) {
		// given: the zero state, which formats as the real-looking "0||"
		state := &networkState{}

		// when
		identity, ok := state.NetworkIdentity()

		// then
		assert.False(t, ok)
		assert.Empty(t, identity)
	})
	t.Run("type alone is not an identity", func(t *testing.T) {
		// given: a client reporting without a path id and no interface
		// enumeration (Android without the getter)
		state := &networkState{}
		state.SetNetworkState(model.DeviceNetworkType_WIFI, "")

		// when
		_, ok := state.NetworkIdentity()

		// then
		assert.False(t, ok)
	})
	t.Run("client path id alone is an identity", func(t *testing.T) {
		// given
		state := &networkState{}
		state.SetNetworkState(model.DeviceNetworkType_CELLULAR, "cell-1")

		// when
		identity, ok := state.NetworkIdentity()

		// then
		assert.True(t, ok)
		assert.Equal(t, networkkey.Key{Reported: true, Type: int32(model.DeviceNetworkType_CELLULAR), PathId: "cell-1"}, identity)
	})
	t.Run("monitor snapshot alone is an identity", func(t *testing.T) {
		// given
		state := &networkState{}
		state.onMonitorSnapshot("10.0.0.5", false)

		// when
		identity, ok := state.NetworkIdentity()

		// then
		assert.True(t, ok)
		assert.Equal(t, networkkey.Key{Snapshot: "10.0.0.5"}, identity)
	})
	t.Run("offline is not an identity", func(t *testing.T) {
		// given: a known network, then the client reports the link gone
		state := &networkState{}
		state.SetNetworkState(model.DeviceNetworkType_WIFI, "wifi-1")
		state.onMonitorSnapshot("10.0.0.5", false)
		state.SetNetworkState(model.DeviceNetworkType_NOT_CONNECTED, "")

		// when
		_, ok := state.NetworkIdentity()

		// then
		assert.False(t, ok)

		// and the desktop shape of offline: no client report, no address
		state = &networkState{}
		state.onMonitorSnapshot("", true)
		_, ok = state.NetworkIdentity()
		assert.False(t, ok)
	})
}

// countingRefresher counts refreshes and records them in the fixture's event
// log; hook runs inside the n-th call.
type countingRefresher struct {
	fx    *networkStateFixture
	mu    sync.Mutex
	calls int
	hook  func(n int)
}

func (c *countingRefresher) Init(*app.App) error { return nil }
func (c *countingRefresher) Name() string        { return "countingRefresher" }
func (c *countingRefresher) RefreshOpenedObjects(context.Context) {
	c.mu.Lock()
	c.calls++
	n, hook := c.calls, c.hook
	c.mu.Unlock()
	c.fx.record("refresh")
	if hook != nil {
		hook(n)
	}
}
func (c *countingRefresher) count() int { c.mu.Lock(); defer c.mu.Unlock(); return c.calls }

func TestNetworkState_RefreshBookkeeping(t *testing.T) {
	t.Run("a normal refresh is not counted as abandoned", func(t *testing.T) {
		fx := newNetworkStateFixtureOpts(t, fixtureOpts{mobile: true})
		fx.foregroundSettled() // worker-run refresh
		for i := 0; i < 30; i++ {
			fx.StateChange(int(domain.CompStateAppWentBackground))
			fx.ticks(4)
			fx.StateChange(int(domain.CompStateAppWentForeground))
			fx.drain()
		}
		require.Equal(t, 31, fx.refreshCalls())
		// the abandon WARN is emitted only together with this counter
		assert.Equal(t, int64(0), fx.stat().RefreshTimeouts)
	})
	t.Run("a second Foreground during the job's own refresh gets its own refresh", func(t *testing.T) {
		fx := newNetworkStateFixtureOpts(t, fixtureOpts{mobile: true})
		fx.foregroundSettled()
		cr := &countingRefresher{fx: fx}
		inRefresh, release := make(chan struct{}), make(chan struct{})
		cr.hook = func(n int) {
			if n == 1 {
				close(inRefresh)
				<-release
			}
		}
		fx.objectsRefresher = cr
		fx.StateChange(int(domain.CompStateAppWentBackground))
		fx.ticks(4)
		fx.StateChange(int(domain.CompStateAppWentForeground)) // job with refresh
		done := make(chan struct{})
		go func() { fx.drainRecoveries(); close(done) }()
		waitFor(t, inRefresh, "job refresh")
		fx.StateChange(int(domain.CompStateAppWentBackground))
		fx.clock.advance(time.Second, time.Second)
		fx.StateChange(int(domain.CompStateAppWentForeground)) // attaches to the running job
		close(release)
		waitFor(t, done, "drain")
		assert.Equal(t, 2, cr.count())
	})
	t.Run("a Foreground during the job's flush is served by the job's own refresh", func(t *testing.T) {
		fx := newNetworkStateFixtureOpts(t, fixtureOpts{mobile: true})
		fx.foregroundSettled()
		entered, release := make(chan struct{}), make(chan struct{})
		var once sync.Once
		fx.flushHook = func() { once.Do(func() { close(entered); <-release }) }
		fx.StateChange(int(domain.CompStateAppWentBackground))
		fx.ticks(4)
		fx.StateChange(int(domain.CompStateAppWentForeground)) // job with refresh
		done := make(chan struct{})
		go func() { fx.drainRecoveries(); close(done) }()
		waitFor(t, entered, "flush")
		n := len(fx.events())
		fx.StateChange(int(domain.CompStateAppWentBackground))
		fx.clock.advance(time.Second, time.Second)
		fx.StateChange(int(domain.CompStateAppWentForeground)) // attaches; the job's refresh comes later anyway
		close(release)
		waitFor(t, done, "drain")
		assert.Equal(t, []string{"flush", "refresh"}, fx.events()[n:], "one refresh, not two")
	})
	t.Run("running + queued flushes: a no-recovery Foreground refreshes after the running flush only", func(t *testing.T) {
		fx := newNetworkStateFixtureOpts(t, fixtureOpts{mobile: true})
		fx.SetNetworkState(model.DeviceNetworkType_WIFI, "w1")
		fx.foregroundSettled()
		fx.StateChange(int(domain.CompStateAppWentBackground))
		entered, release := make(chan struct{}), make(chan struct{})
		var once sync.Once
		fx.flushHook = func() { once.Do(func() { close(entered); <-release }) }
		fx.SetNetworkState(model.DeviceNetworkType_WIFI, "w2") // running, blocked
		done := make(chan struct{})
		go func() { fx.drainRecoveries(); close(done) }()
		waitFor(t, entered, "flush")
		fx.ticks(2)
		fx.SetNetworkState(model.DeviceNetworkType_CELLULAR, "c1") // queued
		n := len(fx.events())
		fx.clock.advance(time.Second, time.Second)
		fx.StateChange(int(domain.CompStateAppWentForeground)) // ~11s background: no recovery of its own
		assert.Empty(t, fx.events()[n:], "not ahead of the running flush")
		close(release)
		waitFor(t, done, "drain")
		assert.Equal(t, []string{"flush", "refresh", "flush"}, fx.events()[n:])
	})
	t.Run("a Foreground merging into a queued non-refresh job keeps its refresh", func(t *testing.T) {
		fx := newNetworkStateFixtureOpts(t, fixtureOpts{mobile: true})
		fx.SetNetworkState(model.DeviceNetworkType_WIFI, "w1")
		fx.foregroundSettled()
		fx.StateChange(int(domain.CompStateAppWentBackground))
		fx.ticks(4)
		entered, release := make(chan struct{}), make(chan struct{})
		var once sync.Once
		fx.flushHook = func() { once.Do(func() { close(entered); <-release }) }
		fx.SetNetworkState(model.DeviceNetworkType_WIFI, "w2") // A: running, blocked
		done := make(chan struct{})
		go func() { fx.drainRecoveries(); close(done) }()
		waitFor(t, entered, "flush")
		fx.clock.advance(7*time.Second, 7*time.Second)
		fx.SetNetworkState(model.DeviceNetworkType_WIFI, "w3") // B: queued, no refresh
		fx.clock.advance(7*time.Second, 7*time.Second)
		n := len(fx.events())
		fx.StateChange(int(domain.CompStateAppWentForeground)) // leading transition, merges into B
		close(release)
		waitFor(t, done, "drain")
		assert.Equal(t, []string{"flush", "flush", "refresh"}, fx.events()[n:])
	})
	t.Run("the attach-to-running flag doesn't leak into later recoveries", func(t *testing.T) {
		fx := newNetworkStateFixtureOpts(t, fixtureOpts{mobile: true})
		fx.SetNetworkState(model.DeviceNetworkType_WIFI, "w1")
		fx.foregroundSettled()
		fx.StateChange(int(domain.CompStateAppWentBackground))
		entered, release := make(chan struct{}), make(chan struct{})
		var once sync.Once
		fx.flushHook = func() { once.Do(func() { close(entered); <-release }) }
		fx.SetNetworkState(model.DeviceNetworkType_WIFI, "w2")
		done := make(chan struct{})
		go func() { fx.drainRecoveries(); close(done) }()
		waitFor(t, entered, "flush")
		fx.ticks(1)
		fx.StateChange(int(domain.CompStateAppWentForeground)) // attaches to the running run
		close(release)
		waitFor(t, done, "drain")
		refreshes := fx.refreshCalls()
		fx.ticks(2)
		fx.SetNetworkState(model.DeviceNetworkType_WIFI, "w3") // unrelated recovery
		fx.drain()
		assert.Equal(t, refreshes, fx.refreshCalls())
	})
}
