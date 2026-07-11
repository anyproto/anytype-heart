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
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/net/pool"
	"go.uber.org/atomic"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const CName = "networkState"

type NetworkState interface {
	app.Component
	app.ComponentStatable
	GetNetworkState() model.DeviceNetworkType
	SetNetworkState(networkState model.DeviceNetworkType)
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
	// run so fresh connections aren't torn down repeatedly.
	recoverySuppressWindow = time.Second * 5
)

type networkState struct {
	networkState          model.DeviceNetworkType
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
	recoveryMu      sync.Mutex
	lastRecoveryAt  time.Time
	recoveryPending bool

	monitor   *netMonitor
	runCancel context.CancelFunc
}

var (
	getTime       = time.Now       // for testing purposes
	scheduleAfter = time.AfterFunc // for testing purposes
)

func (n *networkState) StateChange(state int) {
	n.hookMu.Lock()
	var (
		curTime    = getTime()
		curState   = domain.CompState(state)
		oldState   = n.lastDeviceState
		timePassed = curTime.Sub(n.lastDeviceStateChange)
	)
	n.lastDeviceStateChange = curTime
	n.lastDeviceState = curState
	n.hookMu.Unlock()
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
	return
}

func (n *networkState) Run(ctx context.Context) (err error) {
	var runCtx context.Context
	runCtx, n.runCancel = context.WithCancel(context.Background())
	n.monitor = newNetMonitor(n.triggerRecovery, n.linkDown.Store)
	go n.monitor.run(runCtx)
	return
}

func (n *networkState) Close(ctx context.Context) (err error) {
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

func (n *networkState) SetNetworkState(networkState model.DeviceNetworkType) {
	n.networkMu.Lock()
	first := !n.networkStateReported
	n.networkStateReported = true
	changed := n.networkState != networkState
	n.networkState = networkState
	n.networkMu.Unlock()

	if !changed {
		// to avoid unnecessary hook calls
		return
	}
	n.runOnNetworkUpdateHook(networkState)
	// The first report is the client telling us the initial state, not a
	// switch — connections established during startup are still good.
	if !first {
		n.triggerRecovery("network type changed to " + networkState.String())
	}
}

func (n *networkState) IsOffline() bool {
	return n.linkDown.Load() || n.GetNetworkState() == model.DeviceNetworkType_NOT_CONNECTED
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
	n.recoveryMu.Lock()
	now := getTime()
	since := now.Sub(n.lastRecoveryAt)
	if !n.lastRecoveryAt.IsZero() && since < recoverySuppressWindow {
		if !n.recoveryPending {
			n.recoveryPending = true
			scheduleAfter(recoverySuppressWindow-since, func() {
				n.runPendingRecovery(reason)
			})
		}
		n.recoveryMu.Unlock()
		log.Info("connectivity recovery coalesced", zap.String("reason", reason))
		return
	}
	n.lastRecoveryAt = now
	n.recoveryMu.Unlock()
	n.recover(reason)
}

func (n *networkState) runPendingRecovery(reason string) {
	n.recoveryMu.Lock()
	n.recoveryPending = false
	n.lastRecoveryAt = getTime()
	n.recoveryMu.Unlock()
	n.recover("coalesced: " + reason)
}

func (n *networkState) recover(reason string) {
	online := !n.IsOffline()
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
