package device

/*
AI generated

Name: Device Network and Foreground State Tracker
Scope: global

## Responsibility
- Tracks current network type (WiFi/Cellular/NotConnected) set by client
- Tracks app foreground/background state reported by the client
  (AppSetDeviceState); the initial "unreported" state is kept distinct from
  Background, and the background start is set only on a real transition.
  CompStateAppClosingInitiated sets a closing flag: queued recoveries then
  complete without flushing and skip hooks, head-sync and refresh
- Notifies registered hooks when network type changes
- Runs a connectivity-recovery pipeline (recoveryworker.go: flush connection
  pool, notify connectivity hooks, head-sync all spaces) whenever
  connectivity plausibly changed: network type switch, interface address
  change, process freeze (sleep/suspend), foreground resume after a long
  background. Admission never blocks; the pipeline runs on one serialized
  worker
- Opened-objects refresh on a Foreground transition is asynchronous to
  AppSetDeviceState when a recovery carries it (develop refreshed before the
  RPC returned): it runs after the flush of
  the job the transition enqueued, or right after the flush of a recovery
  that is already running; otherwise inline in StateChange (as before), even
  ahead of a merely queued flush
- Freeze-aware wake recovery (GO-7556, wake.go): every observer calls one
  locked observeGapLocked that opens a wake generation when the estimated
  freeze exceeds the platform threshold; desktop recovers any uncovered
  generation on its own (monitor fallback), mobile waits for the Foreground
  report. The long-background transition recovery is suppressed only when a
  wake generation observed since the last Foreground transition is covered
  by a recovery still queued/running or completed < wakeCoverWindow ago
- Mobile keeps the pre-GO-7556 flush budget: background duration on the
  monotonic clock (as before) and the old 30s sleep threshold
- Runs a background net monitor (netmonitor.go) so desktop gets recovery
  signals without any client RPC: interface-address diffing + the heartbeat
  sampler
- Exposes retained counters and the last wake/recovery result via debugstat
  (networkstate_stats.go)
- Known limits: a forward wall step > threshold on macOS/Linux is a false
  wake (one flush); a backward wall step and a sleep inside the same sample
  interval cancel out (the sleep is missed)
*/

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/debugstat"
	"github.com/anyproto/any-sync/net/pool"
	"go.uber.org/atomic"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/device/networkkey"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/net/addrs"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const CName = "networkState"

type NetworkState interface {
	app.Component
	app.ComponentStatable
	GetNetworkState() model.DeviceNetworkType
	// SetNetworkState records the network state reported by the client on
	// every OS path-change callback. networkId is the opaque identity of the
	// current path (may be empty when the client can't provide one): a change
	// of identity with an unchanged type (Wi-Fi to Wi-Fi switch, cellular
	// re-attach) still triggers connectivity recovery.
	SetNetworkState(networkState model.DeviceNetworkType, networkId string)
	RegisterHook(hook func(network model.DeviceNetworkType))
	// RegisterConnectivityHook registers a hook fired on every connectivity
	// recovery (network switch, interface change, wake, foreground resume),
	// after the connection pool has been flushed. online is false when the
	// device is known to be offline (reported NOT_CONNECTED or no usable
	// network interface).
	RegisterConnectivityHook(hook func(online bool))
	// IsOffline reports whether the device is known to have no connectivity:
	// the client reported NOT_CONNECTED or the net monitor sees no usable
	// interface address. False negatives are possible (a reachable-looking
	// interface with no real connectivity); callers must treat this as a hint
	// for backing off, not as a guarantee.
	IsOffline() bool
	// NetworkIdentity identifies the current network as the separately
	// observed parts it is made of. Callers use it to scope per-network
	// state like transport penalties, and must compare with
	// NetworkKey.SameNetwork rather than by equality: the parts arrive at
	// different times, so two observations of one network routinely differ.
	// ok is false while nothing identifies the network - nothing reported,
	// no interface snapshot, or the device is offline - and such a key must
	// be neither compared nor persisted.
	NetworkIdentity() (key NetworkKey, ok bool)
}

type openedObjectRefresher interface {
	app.Component
	RefreshOpenedObjects(ctx context.Context)
}

type spaceHeadSyncer interface {
	app.Component
	// SyncAllSpaceHeads is fire-and-forget: the head-sync runs in the background
	// on the syncer's own lifecycle context.
	SyncAllSpaceHeads()
}

const (
	// recoverAfter gates the foreground-resume recovery: short app switches
	// keep their connections, so flushing would only cause churn (GO-7302).
	recoverAfter = time.Second * 15
)

type networkState struct {
	networkState         model.DeviceNetworkType
	networkId            string
	networkStateReported bool
	objectsRefresher     openedObjectRefresher
	networkMu            sync.Mutex

	onNetworkUpdateHooks []func(network model.DeviceNetworkType)
	connectivityHooks    []func(online bool)
	hookMu               sync.Mutex
	pool                 pool.Service
	spaceSyncer          spaceHeadSyncer

	linkDown        atomic.Bool
	monitorSnapshot atomic.String
	// monitorGen counts observed monitor-state changes. The recovery
	// fingerprint uses it instead of the raw snapshot values: a link that
	// flapped down and back between two recovery signals yields an identical
	// snapshot string but a different generation, and the trailing run must
	// fire then — the leading run acted while the link was down and its dials
	// failed. Single writer (the monitor goroutine).
	monitorGen atomic.Int64

	// recoveryMu guards the device state, the freeze detector, admission
	// (suppression window, pending trailing run) and the recovery queue, so
	// "observe gap, decide, enqueue" is one atomic step for every caller.
	recoveryMu sync.Mutex
	// device state reported via AppSetDeviceState; deviceStateReported
	// distinguishes the initial unreported state from Background (both are
	// the zero CompState).
	lastDeviceState     domain.CompState
	deviceStateReported bool
	prevDeviceState     string
	// closing is set by CompStateAppClosingInitiated (sent before app.Close)
	// and by Close: queued recoveries then complete without flushing and skip
	// hooks, head-sync and refresh, whose components may already be closing.
	closing bool
	// background start, set only on a real transition into Background
	backgroundSinceWall time.Time
	backgroundSinceMono time.Duration
	// foregroundGenBase is the wake generation at the last Foreground
	// transition; backgroundGenBase copies it when a background interval
	// starts, so a wake the sampler saw shortly before a late Background
	// report still counts as observed within that interval.
	foregroundGenBase int64
	backgroundGenBase int64
	wake              wakeTracker

	hasLastRecovery  bool
	lastRecoveryMono time.Duration
	recoveryPending  bool
	pendingReason    string
	pendingTrigger   string
	pendingTimer     *time.Timer
	closed           bool
	// lastRecoveredFingerprint is the connectivity state the last recovery
	// acted on; a trailing coalesced run with an identical fingerprint is a
	// duplicate signal of the same physical event (wake fires the freeze
	// detector, the interface diff and the foreground RPC within seconds) and
	// must not tear down the connections the leading run just re-established
	// — unless a wake generation is still uncompleted.
	lastRecoveredFingerprint string

	// serialized recovery worker
	queuedJob    *recoveryJob
	recoveryBusy bool
	// refreshAfterRunning: a Foreground transition arrived while a recovery
	// ran; the worker refreshes opened objects once that run finishes
	refreshAfterRunning bool
	workerKick          chan struct{}
	workerDone          chan struct{}

	monitor     *netMonitor
	runCtx      context.Context
	runCancel   context.CancelFunc
	statService debugstat.StatService

	stats recoveryStats

	// mobile disables the desktop-only monitor fallback (iOS/Android recover
	// on Foreground reports instead) and keeps mobile's pre-GO-7556 flush
	// budget. monoCountsSleep selects the Windows freeze estimate (the
	// monotonic clock includes sleep there).
	mobile          bool
	monoCountsSleep bool

	testHooks
}

// testHooks are test-only overrides; the zero value means the real thing
// (bare-struct construction in tests must stay safe, hence the nil-tolerant
// accessors below).
type testHooks struct {
	now             func() time.Time
	elapsed         func() time.Duration
	scheduleAfter   func(d time.Duration, f func()) *time.Timer
	monitorGetAddrs func() (addrs.InterfacesAddrs, error)
	heartbeatEvery  time.Duration
	flushBound      time.Duration
	refreshBound    time.Duration
	closeBound      time.Duration
	// beforeFlushWait runs after Flush is started, before the worker waits
	// for it (lets tests make the result and the deadline ready together)
	beforeFlushWait func()
	// manualDrive: Run starts neither the monitor goroutines nor the recovery
	// worker; tests drive onHeartbeat, monitor.checkInterfaces and
	// drainRecoveries themselves.
	manualDrive bool
}

func (n *networkState) timeNow() time.Time {
	if n.now != nil {
		return n.now()
	}
	return time.Now()
}

// wallNow is the wall clock without its monotonic reading.
func (n *networkState) wallNow() time.Time {
	return n.timeNow().Round(0)
}

// monoNow is the monotonic clock (pauses during sleep except on Windows).
func (n *networkState) monoNow() time.Duration {
	if n.elapsed != nil {
		return n.elapsed()
	}
	return time.Since(monoEpoch)
}

func durationOr(d, def time.Duration) time.Duration {
	if d > 0 {
		return d
	}
	return def
}

func (n *networkState) heartbeatPeriod() time.Duration {
	return durationOr(n.heartbeatEvery, heartbeatInterval)
}
func (n *networkState) flushTimeout() time.Duration { return durationOr(n.flushBound, flushTimeout) }
func (n *networkState) refreshTimeout() time.Duration {
	return durationOr(n.refreshBound, refreshTimeout)
}
func (n *networkState) closeTimeout() time.Duration { return durationOr(n.closeBound, closeTimeout) }

// workCtx is cancelled by Close (Background before Run).
func (n *networkState) workCtx() context.Context {
	if n.runCtx != nil {
		return n.runCtx
	}
	return context.Background()
}

// sleepAwareSince is the time elapsed since (wall, mono) as the larger of the
// two deltas: the monotonic clock pauses during suspend on macOS, iOS, Linux
// and Android, and a backward wall step must not shrink it below monotonic.
func (n *networkState) sleepAwareSince(wall time.Time, mono time.Duration) time.Duration {
	return max(n.wallNow().Sub(wall), n.monoNow()-mono)
}

// backgroundDuration: desktop counts sleep (max of both clocks); mobile keeps
// the pre-GO-7556 monotonic measure, so a short device sleep while locked
// doesn't add a flush (longer sleeps open a wake generation instead).
func (n *networkState) backgroundDuration() time.Duration {
	if n.mobile {
		return n.monoNow() - n.backgroundSinceMono
	}
	return n.sleepAwareSince(n.backgroundSinceWall, n.backgroundSinceMono)
}

func (n *networkState) schedule(d time.Duration, f func()) *time.Timer {
	if n.scheduleAfter != nil {
		return n.scheduleAfter(d, f)
	}
	return time.AfterFunc(d, f)
}

func (n *networkState) StateChange(state int) {
	curState := domain.CompState(state)
	if curState == domain.CompStateAppClosingInitiated {
		n.recoveryMu.Lock()
		n.closing = true
		n.recoveryMu.Unlock()
		return
	}
	if curState != domain.CompStateAppWentForeground && curState != domain.CompStateAppWentBackground {
		return
	}
	n.recoveryMu.Lock()
	newGen := n.observeGapLocked(observeSourceStateChange)
	oldState, reported := n.lastDeviceState, n.deviceStateReported
	if !reported || oldState != curState {
		n.prevDeviceState = deviceStateName(oldState, reported)
	}
	n.lastDeviceState, n.deviceStateReported = curState, true

	if curState == domain.CompStateAppWentBackground {
		if !reported || oldState != domain.CompStateAppWentBackground {
			// only a real change starts the background interval; duplicate
			// Background reports must not reset it
			n.stats.backgroundEvents.Inc()
			n.backgroundSinceWall, n.backgroundSinceMono = n.wallNow(), n.monoNow()
			n.backgroundGenBase = n.foregroundGenBase
		} else {
			n.stats.backgroundDuplicate.Inc()
		}
		// a late Background report can be the first to see the gap
		n.desktopFallbackLocked(newGen)
		n.recoveryMu.Unlock()
		return
	}

	transitioned := !reported || oldState != domain.CompStateAppWentForeground
	var backgroundedFor time.Duration
	if transitioned {
		n.stats.foregroundEvents.Inc()
		if reported {
			backgroundedFor = n.backgroundDuration()
		} else {
			// unreported -> Foreground (account start): today's single
			// recovery, the initial state used to be the zero-value Background
			backgroundedFor = time.Duration(1<<63 - 1)
		}
	} else {
		n.stats.foregroundDuplicate.Inc()
	}
	longBackground := transitioned && backgroundedFor > recoverAfter
	suppressed := false
	if longBackground && n.coveredByRecentWakeRecoveryLocked() {
		longBackground, suppressed = false, true
		n.stats.foregroundSuppressed.Inc()
	}
	uncovered := n.wake.uncovered()
	wakeGen := n.wake.observed
	var trigger string
	enqueued := false
	switch {
	case longBackground:
		trigger = triggerTransition
		enqueued = n.admitLocked("foreground resume", trigger, true)
	case uncovered && transitioned:
		trigger = triggerForegroundWake
		enqueued = n.admitLocked("foreground resume (wake)", trigger, true)
	case uncovered:
		trigger = triggerDuplicateForeground
		n.admitLocked("foreground resume (duplicateForeground)", trigger, false)
	}
	// Opened-objects refresh. A job this Foreground enqueued carries it (its
	// flush runs first). Otherwise, if a recovery is running, it attaches to
	// that run and goes out right after its flush (at most flushTimeout
	// later). Otherwise it runs inline now, as before GO-7556 — even if a job
	// is only queued: waiting behind a queued flush could be much later than
	// before, so it costs one extra request round instead.
	refreshNow := false
	if transitioned && !enqueued {
		if n.recoveryBusy {
			n.refreshAfterRunning = true
		} else {
			refreshNow = true
		}
	}
	if transitioned {
		n.foregroundGenBase = n.wake.observed
	}
	n.recoveryMu.Unlock()

	if transitioned {
		// Anchor log for measuring how fast per-space diffsync reacts to a wakeup (GO-7302).
		lvl := log.Info
		if suppressed || trigger == triggerForegroundWake {
			lvl = log.Warn // a wake decision: keep it visible in user builds
		}
		lvl("app went foreground",
			zap.Duration("backgroundedFor", backgroundedFor),
			zap.Bool("wasReported", reported),
			zap.Int64("wakeGen", wakeGen),
			zap.Bool("suppressedByWakeRecovery", suppressed),
			zap.String("trigger", trigger),
			zap.Bool("refreshInline", refreshNow))
		if refreshNow {
			n.objectsRefresher.RefreshOpenedObjects(context.Background())
		}
	} else if trigger != "" {
		log.Warn("duplicateForeground with an uncovered wake", zap.Int64("wakeGen", wakeGen))
	}
}

func New() NetworkState {
	return &networkState{
		mobile:          runtime.GOOS == "android" || runtime.GOOS == "ios",
		monoCountsSleep: runtime.GOOS == "windows",
		workerKick:      make(chan struct{}, 1),
	}
}

func (n *networkState) Init(a *app.App) (err error) {
	n.pool = app.MustComponent[pool.Service](a)
	n.objectsRefresher = app.MustComponent[openedObjectRefresher](a)
	n.spaceSyncer = app.MustComponent[spaceHeadSyncer](a)
	if statService, err := app.GetComponent[debugstat.StatService](a); err == nil {
		n.statService = statService
		statService.AddProvider(n)
	}
	return
}

func (n *networkState) Run(ctx context.Context) (err error) {
	n.runCtx, n.runCancel = context.WithCancel(context.Background())
	n.monitor = newNetMonitor(n.triggerRecovery, n.onMonitorSnapshot, n.onHeartbeat, n.monitorGetAddrs)
	n.monitor.heartbeatEvery = n.heartbeatPeriod()

	n.recoveryMu.Lock()
	n.observeGapLocked(observeSourceStart) // baseline for the first sample
	if n.workerKick == nil {
		n.workerKick = make(chan struct{}, 1)
	}
	if !n.manualDrive {
		n.workerDone = make(chan struct{})
	}
	n.recoveryMu.Unlock()

	if n.manualDrive {
		n.monitor.checkInterfaces()
		return
	}
	go n.runRecoveryWorker(n.runCtx)
	go n.monitor.run(n.runCtx)
	return
}

func (n *networkState) Close(ctx context.Context) (err error) {
	n.recoveryMu.Lock()
	n.closed = true
	n.closing = true
	if n.pendingTimer != nil {
		n.pendingTimer.Stop()
	}
	// drop the queued job (the worker also refuses to start one once closed)
	n.queuedJob = nil
	done := n.workerDone
	n.recoveryMu.Unlock()
	if n.statService != nil {
		n.statService.RemoveProvider(n)
	}
	if n.runCancel != nil {
		// also cancels a running Flush and refresh
		n.runCancel()
	}
	// wait (bounded) for a running recovery, so it doesn't touch the pool
	// after it closes; never long enough to trip app.Close's stop deadline
	if done != nil {
		timer := time.NewTimer(n.closeTimeout())
		defer timer.Stop()
		select {
		case <-done:
		case <-ctx.Done():
		case <-timer.C:
			n.stats.closeWaitTimeouts.Inc()
			log.Warn("network state close: recovery worker still running", zap.Duration("waited", n.closeTimeout()))
		}
	}
	return
}

func (n *networkState) Name() (name string) {
	return CName
}

func (n *networkState) GetNetworkState() model.DeviceNetworkType {
	n.networkMu.Lock()
	defer n.networkMu.Unlock()
	return n.networkState
}

func (n *networkState) SetNetworkState(networkState model.DeviceNetworkType, networkId string) {
	n.stats.networkReports.Inc()
	n.networkMu.Lock()
	first := !n.networkStateReported
	n.networkStateReported = true
	typeChanged := n.networkState != networkState
	// The identity only counts as changed when known both before and after —
	// an empty id (older clients, callbacks without one) keeps type-only
	// semantics instead of registering spurious changes.
	idChanged := networkId != "" && n.networkId != "" && n.networkId != networkId
	if networkId != "" {
		n.networkId = networkId
	}
	n.networkState = networkState
	n.networkMu.Unlock()

	if !typeChanged && !idChanged {
		// to avoid unnecessary hook calls
		n.stats.networkReportsDuplicate.Inc()
		return
	}
	if typeChanged {
		n.runOnNetworkUpdateHook(networkState)
	}
	// The first report is the client telling us the initial state, not a
	// switch — connections established during startup are still good.
	if first {
		return
	}
	reason, trigger := "network type changed to "+networkState.String(), triggerNetworkType
	if !typeChanged {
		reason, trigger = "network path changed (same type "+networkState.String()+")", triggerNetworkPath
	}
	n.triggerRecovery(reason, trigger)
}

func (n *networkState) onMonitorSnapshot(key string, down bool) {
	// note: no short-circuit — both atomics must be updated every call
	keyChanged := n.monitorSnapshot.Swap(key) != key
	downChanged := n.linkDown.Swap(down) != down
	if keyChanged || downChanged {
		n.monitorGen.Inc()
	}
}

// fingerprint captures the connectivity state a recovery acts on: the
// client-reported type and path id plus the monitor generation. Equal
// fingerprints mean nothing was observed to change since the last recovery —
// the generation (rather than the raw snapshot) makes a down-and-back link
// flap visible even when the address set ends up identical.
func (n *networkState) fingerprint() string {
	n.networkMu.Lock()
	state, id := n.networkState, n.networkId
	n.networkMu.Unlock()
	return fmt.Sprintf("%d|%s|%d", state, id, n.monitorGen.Load())
}

// NetworkKey is the network identity; see networkkey.Key.
type NetworkKey = networkkey.Key

func (n *networkState) NetworkIdentity() (NetworkKey, bool) {
	n.networkMu.Lock()
	state, id, reported := n.networkState, n.networkId, n.networkStateReported
	n.networkMu.Unlock()
	// Offline says nothing about which network the device is on.
	if state == model.DeviceNetworkType_NOT_CONNECTED {
		return NetworkKey{}, false
	}
	key := NetworkKey{
		Reported: reported,
		Type:     int32(state),
		PathId:   id,
		Snapshot: n.monitorSnapshot.Load(),
	}
	if !key.Known() {
		return NetworkKey{}, false
	}
	return key, true
}

func (n *networkState) IsOffline() bool {
	n.networkMu.Lock()
	reported := n.networkStateReported
	state := n.networkState
	n.networkMu.Unlock()
	if state == model.DeviceNetworkType_NOT_CONNECTED {
		return true
	}
	// A client that actively reports a connected type (mobile OS callbacks) is
	// authoritative: the interface heuristic must not be able to wedge the
	// device "offline" when e.g. Android's injected getter doesn't enumerate
	// cellular interfaces. linkDown only decides when no client reports
	// (desktop) — there the monitor is the sole signal source.
	if reported {
		return false
	}
	return n.linkDown.Load()
}

func (n *networkState) RegisterHook(hook func(network model.DeviceNetworkType)) {
	n.hookMu.Lock()
	defer n.hookMu.Unlock()
	n.onNetworkUpdateHooks = append(n.onNetworkUpdateHooks, hook)
}

func (n *networkState) RegisterConnectivityHook(hook func(online bool)) {
	n.hookMu.Lock()
	defer n.hookMu.Unlock()
	n.connectivityHooks = append(n.connectivityHooks, hook)
}

func (n *networkState) runOnNetworkUpdateHook(state model.DeviceNetworkType) {
	n.hookMu.Lock()
	defer n.hookMu.Unlock()
	for _, hook := range n.onNetworkUpdateHooks {
		hook(state)
	}
}

func (n *networkState) runConnectivityHooks(online bool) {
	n.hookMu.Lock()
	defer n.hookMu.Unlock()
	for _, hook := range n.connectivityHooks {
		hook(online)
	}
}
