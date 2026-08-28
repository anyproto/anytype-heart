package device

/*
AI generated

Name: Device Network and Foreground State Tracker
Scope: global

## Responsibility
- Tracks current network type (WiFi/Cellular/NotConnected) set by client
- Tracks app foreground/background state transitions
- Notifies registered hooks when network type changes
- Runs a connectivity-recovery pipeline (flush connection pool, notify
  connectivity hooks, head-sync all spaces) whenever connectivity plausibly
  changed: network type switch, interface address change, wake from sleep,
  foreground resume after a long background
- Runs a background net monitor (netmonitor.go) so desktop gets recovery
  signals without any client RPC: interface-address diffing + clock-jump
  (sleep) detection
*/

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/debugstat"
	"github.com/anyproto/any-sync/net/pool"
	"go.uber.org/atomic"
	"go.uber.org/zap"

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
	// NetworkIdentity is an opaque key identifying the current network:
	// client-reported type and path id (mobile) combined with the net
	// monitor's interface snapshot (desktop). Equal keys mean the device is
	// still on the same network; callers use it to scope per-network state
	// like transport penalties.
	NetworkIdentity() string
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
	// recoverySuppressWindow bounds how often the recovery pipeline can run.
	// Signals arrive in bursts (wake fires the clock-jump detector, the
	// interface diff and the client's foreground RPC within seconds); the
	// first one runs immediately, the rest coalesce into at most one trailing
	// run so fresh connections aren't torn down repeatedly. Deliberately NOT
	// equal to netMonitorTickInterval: with equal values, an event observed
	// exactly one tick after a recovery lands on the window boundary and
	// leading-vs-coalesced is decided by sub-millisecond races.
	recoverySuppressWindow = time.Second * 6
)

type networkState struct {
	networkState          model.DeviceNetworkType
	networkId             string
	networkStateReported  bool
	objectsRefresher      openedObjectRefresher
	networkMu             sync.Mutex
	lastDeviceState       domain.CompState
	lastDeviceStateChange time.Time

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
	monitorGen      atomic.Int64
	recoveryMu      sync.Mutex
	lastRecoveryAt  time.Time
	recoveryPending bool
	pendingReason   string
	pendingTimer    *time.Timer
	closed          bool
	// lastRecoveredFingerprint is the connectivity state the last recovery
	// acted on; a trailing coalesced run with an identical fingerprint is a
	// duplicate signal of the same physical event (wake fires the clock-jump
	// detector, the interface diff and the foreground RPC within seconds) and
	// must not tear down the connections the leading run just re-established.
	lastRecoveredFingerprint string

	monitor     *netMonitor
	runCancel   context.CancelFunc
	statService debugstat.StatService

	stats recoveryStats

	// test hooks; nil means the real thing (bare-struct construction in tests
	// must stay safe, hence the nil-tolerant accessors below)
	now             func() time.Time
	scheduleAfter   func(d time.Duration, f func()) *time.Timer
	monitorGetAddrs func() (addrs.InterfacesAddrs, error)
}

// recoveryStats counts how the recovery mechanism is exercised. Every update
// is a single atomic op on an event-driven path (signals arrive at human
// timescales — network switches, wakes, RPCs), and the JSON snapshot is built
// only when the debug stat endpoint asks, so this adds no steady-state
// overhead.
type recoveryStats struct {
	networkReports atomic.Int64
	// networkReportsDuplicate counts no-op reports (same type and id). The
	// ratio to networkReports shows the client's call pattern: near-100%
	// duplicates is a healthy client reporting on every OS path callback as
	// documented; 0 raw reports means the client isn't wired up at all.
	networkReportsDuplicate atomic.Int64
	foregroundEvents        atomic.Int64
	backgroundEvents        atomic.Int64

	signalsByType          atomic.Int64
	signalsByPath          atomic.Int64
	signalsByForeground    atomic.Int64
	signalsByWake          atomic.Int64
	signalsByIfaceLost     atomic.Int64
	signalsByIfaceRegained atomic.Int64
	signalsOther           atomic.Int64
	signalsCoalesced       atomic.Int64

	trailingRuns    atomic.Int64
	trailingSkipped atomic.Int64

	recoveries         atomic.Int64
	recoveriesOffline  atomic.Int64
	lastRecoveryUnix   atomic.Int64
	lastRecoveryReason atomic.String
}

func (s *recoveryStats) countSignal(reason string) {
	switch {
	case strings.HasPrefix(reason, "network type"):
		s.signalsByType.Inc()
	case strings.HasPrefix(reason, "network path"):
		s.signalsByPath.Inc()
	case reason == "foreground resume":
		s.signalsByForeground.Inc()
	case strings.HasPrefix(reason, "wake from sleep"):
		s.signalsByWake.Inc()
	case strings.HasPrefix(reason, "interface addresses lost"):
		s.signalsByIfaceLost.Inc()
	case reason == "interface addresses regained":
		s.signalsByIfaceRegained.Inc()
	default:
		s.signalsOther.Inc()
	}
}

type networkStateStat struct {
	NetworkType      string `json:"networkType"`
	NetworkId        string `json:"networkId,omitempty"`
	ReportedByClient bool   `json:"reportedByClient"`
	LinkDown         bool   `json:"linkDown"`
	MonitorSnapshot  string `json:"monitorSnapshot"`
	MonitorGen       int64  `json:"monitorGeneration"`
	Offline          bool   `json:"offline"`

	NetworkReports          int64 `json:"networkReports"`
	NetworkReportsDuplicate int64 `json:"networkReportsDuplicate"`
	ForegroundEvents        int64 `json:"foregroundEvents"`
	BackgroundEvents        int64 `json:"backgroundEvents"`

	SignalsNetworkTypeChange int64 `json:"signalsNetworkTypeChange"`
	SignalsNetworkPathChange int64 `json:"signalsNetworkPathChange"`
	SignalsForegroundResume  int64 `json:"signalsForegroundResume"`
	SignalsWakeFromSleep     int64 `json:"signalsWakeFromSleep"`
	SignalsInterfaceLost     int64 `json:"signalsInterfaceLost"`
	SignalsInterfaceRegained int64 `json:"signalsInterfaceRegained"`
	SignalsOther             int64 `json:"signalsOther"`
	SignalsCoalesced         int64 `json:"signalsCoalesced"`

	TrailingRuns    int64 `json:"trailingRuns"`
	TrailingSkipped int64 `json:"trailingSkipped"`

	Recoveries         int64  `json:"recoveries"`
	RecoveriesOffline  int64  `json:"recoveriesOffline"`
	LastRecoveryUnix   int64  `json:"lastRecoveryUnix,omitempty"`
	LastRecoveryReason string `json:"lastRecoveryReason,omitempty"`
}

func (n *networkState) ProvideStat() any {
	n.networkMu.Lock()
	state, id, reported := n.networkState, n.networkId, n.networkStateReported
	n.networkMu.Unlock()
	return networkStateStat{
		NetworkType:      state.String(),
		NetworkId:        id,
		ReportedByClient: reported,
		LinkDown:         n.linkDown.Load(),
		MonitorSnapshot:  n.monitorSnapshot.Load(),
		MonitorGen:       n.monitorGen.Load(),
		Offline:          n.IsOffline(),

		NetworkReports:          n.stats.networkReports.Load(),
		NetworkReportsDuplicate: n.stats.networkReportsDuplicate.Load(),
		ForegroundEvents:        n.stats.foregroundEvents.Load(),
		BackgroundEvents:        n.stats.backgroundEvents.Load(),

		SignalsNetworkTypeChange: n.stats.signalsByType.Load(),
		SignalsNetworkPathChange: n.stats.signalsByPath.Load(),
		SignalsForegroundResume:  n.stats.signalsByForeground.Load(),
		SignalsWakeFromSleep:     n.stats.signalsByWake.Load(),
		SignalsInterfaceLost:     n.stats.signalsByIfaceLost.Load(),
		SignalsInterfaceRegained: n.stats.signalsByIfaceRegained.Load(),
		SignalsOther:             n.stats.signalsOther.Load(),
		SignalsCoalesced:         n.stats.signalsCoalesced.Load(),

		TrailingRuns:    n.stats.trailingRuns.Load(),
		TrailingSkipped: n.stats.trailingSkipped.Load(),

		Recoveries:         n.stats.recoveries.Load(),
		RecoveriesOffline:  n.stats.recoveriesOffline.Load(),
		LastRecoveryUnix:   n.stats.lastRecoveryUnix.Load(),
		LastRecoveryReason: n.stats.lastRecoveryReason.Load(),
	}
}

func (n *networkState) StatId() string { return CName }

func (n *networkState) StatType() string { return CName }

func (n *networkState) timeNow() time.Time {
	if n.now != nil {
		return n.now()
	}
	return time.Now()
}

func (n *networkState) schedule(d time.Duration, f func()) *time.Timer {
	if n.scheduleAfter != nil {
		return n.scheduleAfter(d, f)
	}
	return time.AfterFunc(d, f)
}

func (n *networkState) StateChange(state int) {
	n.hookMu.Lock()
	var (
		curTime    = n.timeNow()
		curState   = domain.CompState(state)
		oldState   = n.lastDeviceState
		timePassed = curTime.Sub(n.lastDeviceStateChange)
	)
	n.lastDeviceStateChange = curTime
	n.lastDeviceState = curState
	n.hookMu.Unlock()
	if oldState != curState {
		switch curState {
		case domain.CompStateAppWentForeground:
			n.stats.foregroundEvents.Inc()
		case domain.CompStateAppWentBackground:
			n.stats.backgroundEvents.Inc()
		}
	}
	if oldState != curState && curState == domain.CompStateAppWentForeground {
		// Anchor log for measuring how fast per-space diffsync reacts to a wakeup (GO-7302).
		log.Info("app went foreground", zap.Duration("backgroundedFor", timePassed))
		if timePassed > recoverAfter {
			n.triggerRecovery("foreground resume")
		}
		n.objectsRefresher.RefreshOpenedObjects(context.Background())
	}
}

func New() NetworkState {
	return &networkState{}
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
	var runCtx context.Context
	runCtx, n.runCancel = context.WithCancel(context.Background())
	n.monitor = newNetMonitor(n.triggerRecovery, n.onMonitorSnapshot, n.monitorGetAddrs)
	go n.monitor.run(runCtx)
	return
}

func (n *networkState) Close(ctx context.Context) (err error) {
	n.recoveryMu.Lock()
	n.closed = true
	if n.pendingTimer != nil {
		n.pendingTimer.Stop()
	}
	n.recoveryMu.Unlock()
	if n.statService != nil {
		n.statService.RemoveProvider(n)
	}
	if n.runCancel != nil {
		n.runCancel()
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
	reason := "network type changed to " + networkState.String()
	if !typeChanged {
		reason = "network path changed (same type " + networkState.String() + ")"
	}
	n.triggerRecovery(reason)
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

func (n *networkState) NetworkIdentity() string {
	n.networkMu.Lock()
	state, id := n.networkState, n.networkId
	n.networkMu.Unlock()
	return fmt.Sprintf("%d|%s|%s", state, id, n.monitorSnapshot.Load())
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

// triggerRecovery runs the connectivity-recovery pipeline, leading-edge with a
// suppression window: the first signal runs immediately; signals inside the
// window coalesce into a single trailing run (a second real change right after
// the first must not be lost, but fresh connections must not be flushed over
// and over during an event burst).
func (n *networkState) triggerRecovery(reason string) {
	n.stats.countSignal(reason)
	n.recoveryMu.Lock()
	if n.closed {
		n.recoveryMu.Unlock()
		return
	}
	now := n.timeNow()
	since := now.Sub(n.lastRecoveryAt)
	if !n.lastRecoveryAt.IsZero() && since < recoverySuppressWindow {
		n.stats.signalsCoalesced.Inc()
		// remember the latest reason so the trailing run reports what actually
		// coalesced last, not the first suppressed signal
		n.pendingReason = reason
		if !n.recoveryPending {
			n.recoveryPending = true
			n.pendingTimer = n.schedule(recoverySuppressWindow-since, n.runPendingRecovery)
		}
		n.recoveryMu.Unlock()
		log.Info("connectivity recovery coalesced", zap.String("reason", reason))
		return
	}
	n.lastRecoveryAt = now
	n.recoveryMu.Unlock()
	n.recover(reason)
}

func (n *networkState) runPendingRecovery() {
	n.recoveryMu.Lock()
	if n.closed {
		n.recoveryMu.Unlock()
		return
	}
	reason := n.pendingReason
	lastFingerprint := n.lastRecoveredFingerprint
	n.recoveryPending = false
	n.lastRecoveryAt = n.timeNow()
	n.recoveryMu.Unlock()
	// A trailing run only makes sense when the network actually changed since
	// the leading run (e.g. Wi-Fi->cellular right after a wake). Duplicate
	// signals of the same physical event must not flush the connections the
	// leading run just re-established.
	if n.fingerprint() == lastFingerprint {
		n.stats.trailingSkipped.Inc()
		log.Info("connectivity recovery skipped: no network change since the last run",
			zap.String("reason", reason))
		return
	}
	n.stats.trailingRuns.Inc()
	n.recover("coalesced: " + reason)
}

func (n *networkState) recover(reason string) {
	n.recoveryMu.Lock()
	n.lastRecoveredFingerprint = n.fingerprint()
	n.recoveryMu.Unlock()
	online := !n.IsOffline()
	n.stats.recoveries.Inc()
	if !online {
		n.stats.recoveriesOffline.Inc()
	}
	n.stats.lastRecoveryUnix.Store(n.timeNow().Unix())
	n.stats.lastRecoveryReason.Store(reason)
	log.Info("connectivity recovery", zap.String("reason", reason), zap.Bool("online", online))
	// Flush drops every pooled connection (closing the underlying sockets), so
	// the next Get re-dials instead of serving a connection that died with the
	// old network path. Flushing while offline is still right: it kills dead
	// sockets that would otherwise block writers for the transport-timeout
	// window.
	if n.pool != nil {
		if err := n.pool.Flush(context.Background()); err != nil {
			log.Debug("flush pool on connectivity recovery", zap.Error(err))
		}
	}
	n.runConnectivityHooks(online)
	if online && n.spaceSyncer != nil {
		n.spaceSyncer.SyncAllSpaceHeads()
	}
}
