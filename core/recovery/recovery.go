// Package recovery is the account start-up status stream (GO-7471): a
// recovery-scoped, monotonic event log covering every app open — from the
// first millisecond of AccountSelect until every space has published its load
// result — plus the folded Snapshot the Rpc.Account.RecoveryState RPC serves.
//
// One Tracker lives for the process (owned by core/application) and runs one
// run per app start. Every producer feeds it through a narrow method; it
// assigns monotonic ids, derives the coarse phase, coalesces bursts and
// broadcasts, all from one state under one mutex, so the pushed events and the
// pulled snapshot cannot drift. Everything here is advisory: a slow or failing
// tracker must never affect a start, which is why every entry point is
// panic-contained and nothing under the mutex does I/O or calls into any-sync.
//
// Design: docs/superpowers/specs/2026-09-02-cold-sync-recovery-events-design.md
package recovery

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/net/peerobserver"
	"github.com/anyproto/any-sync/nodeconf"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/space/spacecore/localdiscovery"
	"github.com/anyproto/anytype-heart/space/spacecore/peerstore"
)

const CName = "core.recovery"

var log = logging.Logger(CName)

// Run describes one app start. Sender is the application's event sender,
// available before the app exists; Init re-binds it from the app.
type Run struct {
	Mode   pb.EventAccountRecoveryMode
	Sender event.Sender
}

// Tracker is the fold. It is an app.ComponentRunnable registered by
// core/application ahead of every bootstrap component, so its Init runs before
// any dial or space load.
type Tracker struct {
	mu     sync.Mutex
	clock  clock
	window time.Duration
	sender event.Sender
	begun  bool
	// nodeTypes classifies peers (NetworkNode vs LocalPeer); nil until Init
	nodeTypes nodeTypesResolver

	nextId  int64
	run     runState
	phase   phaseState
	net     netState
	account accountState
	spaces  map[string]*spaceState
	views   viewGate

	pending      map[coalesceKey]pb.IsEventAccountRecoveryUpdatePayload
	pendingOrder []coalesceKey
	nextAt       time.Time
	timer        timer
	// outageTimer fires waitingForNetworkAfter into an outage
	outageTimer timer
}

func New() *Tracker {
	return newTracker(realClock{}, coalesceWindow)
}

func newTracker(c clock, window time.Duration) *Tracker {
	t := &Tracker{clock: c, window: window}
	t.resetLocked()
	return t
}

// Begin opens a new run: it resets every piece of state, assigns a fresh
// runId and records the start instant. Started itself is published from Init,
// where the network id is known (config has run its Init by then); Fail
// publishes it first if Init never ran.
func (t *Tracker) Begin(run Run) {
	defer containTelemetry("begin")
	t.mu.Lock()
	defer t.mu.Unlock()
	t.beginLocked(run)
}

func (t *Tracker) beginLocked(run Run) {
	t.resetLocked()
	t.begun = true
	if run.Sender != nil {
		t.sender = run.Sender
	}
	now := t.clock.Now()
	t.run = runState{runId: newRunId(), mode: run.Mode, startedAt: now}
	t.phase.startedAt = now
}

func (t *Tracker) resetLocked() {
	if t.timer != nil {
		t.timer.Stop()
		t.timer = nil
	}
	t.stopOutageTimerLocked()
	t.run = runState{}
	t.phase = phaseState{current: pb.EventAccountRecovery_LookingForPeers}
	t.net = netState{peers: map[string]*peerState{}}
	t.account = accountState{}
	t.spaces = map[string]*spaceState{}
	t.views = viewGate{unresolved: map[string]struct{}{}, seen: map[string]struct{}{}, expected: map[string]struct{}{}}
	t.pending = map[coalesceKey]pb.IsEventAccountRecoveryUpdatePayload{}
	t.pendingOrder = nil
	t.nextId = 1
	t.nextAt = time.Time{}
}

// Fail ends the run with an account-level verdict: AccountSelect itself
// failed. It is the run's terminal, so it publishes even after Close — a
// failed app.Start closes the components it initialised before returning the
// error — and publishes Started first when Init never ran. A cancelled start
// (context.Canceled) is a deliberate stop, not a failure: it has no verdict
// to publish, so the run is closed instead and reads idle from here on.
func (t *Tracker) Fail(err error) {
	defer containTelemetry("fail")
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.begun || t.terminalLocked() {
		return
	}
	if errors.Is(err, context.Canceled) {
		// app.Start's unwind closes the tracker itself when the failure came
		// after its Init; this covers a cancel that struck before it
		t.closeLocked()
		return
	}
	info := classifyAccount(err)
	if info == nil {
		return
	}
	t.phase.failed = info
	t.publishStartedLocked()
	t.refreshPhaseLocked(true)
}

func (t *Tracker) Init(a *app.App) error {
	defer containTelemetry("init")
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.begun {
		// registered without Begin (tests, or a future caller): still a run
		t.beginLocked(Run{Mode: pb.EventAccountRecovery_WarmStart})
	}
	if snd, err := app.GetComponent[event.Sender](a); err == nil {
		t.sender = snd
	}
	if cfg, ok := a.Component(config.CName).(*config.Config); ok {
		t.run.networkId = cfg.GetNodeConf().NetworkId
		t.run.localOnly = cfg.GetNetworkMode() == pb.RpcAccount_LocalOnly
	}
	if hooks, err := app.GetComponent[session.HookRunner](a); err == nil {
		hooks.RegisterHook(t.sendSnapshotToSession)
	}
	// peer layer: every lookup is optional so the tracker degrades to a
	// phase-only stream when a producer is absent (tests, future hosts)
	if resolver, ok := a.Component(nodeconf.CName).(nodeTypesResolver); ok {
		t.nodeTypes = resolver
	}
	if mux, ok := a.Component(peerobserver.CName).(observerRegistry); ok {
		mux.Add(t)
	}
	if discovery, ok := a.Component(localdiscovery.CName).(discoveryHookRegistrar); ok {
		discovery.RegisterDiscoveryPossibilityHook(t.onDiscoveryPossibility)
	}
	if network, ok := a.Component(deviceNetworkStateCName).(networkConnectivity); ok {
		t.net.deviceOffline = network.IsOffline()
		network.RegisterConnectivityHook(t.onConnectivity)
	}
	if store, ok := a.Component(peerstore.CName).(peerStoreObservable); ok {
		store.AddObserver(t.onLocalPeerSpaces)
	}
	t.publishStartedLocked()
	t.evaluateWaitingLocked()
	t.refreshPhaseLocked(false)
	return nil
}

func (t *Tracker) Name() string { return CName }

func (t *Tracker) Run(_ context.Context) error { return nil }

// Close silences the run. Only Fail may still publish after it — a failed
// app.Start closes the components it initialised before returning the error —
// and its verdict carries whatever the coalescing window still holds. A run
// closed without a verdict is over and reads idle; the levels its window held
// are dropped with it, so the stream says nothing more about a run the
// snapshot no longer reports.
func (t *Tracker) Close(_ context.Context) error {
	defer containTelemetry("close")
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closeLocked()
	return nil
}

func (t *Tracker) closeLocked() {
	if t.timer != nil {
		t.timer.Stop()
		t.timer = nil
	}
	t.stopOutageTimerLocked()
	t.run.closed = true
}

// Snapshot is the pull half: the same builder the push side uses over the same
// state. It is total: before the first Begin it is the idle snapshot, a run in
// progress or ended with a verdict reports itself until the next Begin, and a
// run that ended without one reads idle again — so a client can call the RPC
// at any moment, in any order relative to AccountSelect, always gets an
// answer, and never takes a dead run for a live one.
func (t *Tracker) Snapshot() *pb.EventAccountRecoverySnapshot {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.begun || t.abandonedLocked() {
		return IdleSnapshot()
	}
	return t.buildSnapshotLocked()
}

// IdleSnapshot is the answer when no recovery run is in progress and none has
// left a verdict: an empty runId is the discriminator clients check, and the
// phase is the explicit NotStarted rather than the zero value, which would
// read as LookingForPeers before anything is happening.
func IdleSnapshot() *pb.EventAccountRecoverySnapshot {
	return &pb.EventAccountRecoverySnapshot{
		Phase:      pb.EventAccountRecovery_NotStarted,
		Mode:       pb.EventAccountRecovery_ModeUnknown,
		LocalPeers: pb.EventAccountRecovery_NoLocalPeers,
	}
}

// sendSnapshotToSession is the session hook: snapshot-on-subscribe, so a UI
// attaching mid-run renders correctly without replaying history. The update
// carries the last published id, so the client applies deltas from id+1.
func (t *Tracker) sendSnapshotToSession(ctx session.Context) error {
	defer containTelemetry("session hook")
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.begun || !t.run.started || t.abandonedLocked() || t.sender == nil {
		return nil
	}
	update := &pb.EventAccountRecoveryUpdate{
		RunId:       t.run.runId,
		Id:          t.nextId - 1,
		TimestampMs: t.clock.Now().UnixMilli(),
		Payload:     &pb.EventAccountRecoveryUpdatePayloadOfSnapshot{Snapshot: t.buildSnapshotLocked()},
	}
	t.sender.SendToSession(ctx.ID(), event.NewEventSingleMessage("", &pb.EventMessageValueOfAccountRecoveryUpdate{
		AccountRecoveryUpdate: update,
	}))
	return nil
}

func (t *Tracker) publishStartedLocked() {
	if t.run.started {
		return
	}
	t.run.started = true
	t.publishLocked(&pb.EventAccountRecoveryUpdatePayloadOfStarted{Started: &pb.EventAccountRecoveryStarted{
		Mode:      t.run.mode,
		NetworkId: t.run.networkId,
	}})
}

func (t *Tracker) terminalLocked() bool {
	return t.phase.finished || t.phase.failed != nil
}

// abandonedLocked reports a run that ended without a verdict: its start was
// cancelled, or the app was closed before Done. There is nothing left to say
// about it, and presenting it as live would make a client that attaches
// later — and pulls the snapshot — arm a Cancel for a start that is over.
func (t *Tracker) abandonedLocked() bool {
	return t.run.closed && !t.terminalLocked()
}

func newRunId() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return hex.EncodeToString([]byte(time.Now().String()))[:16]
	}
	return hex.EncodeToString(b[:])
}

// containTelemetry is the enforcement of the package promise: a status
// surface must never break an account start. Deferred directly so its
// recover() is the deferred call; state may be left half-updated, which is a
// telemetry cost the start does not pay.
func containTelemetry(where string) {
	if rec := recover(); rec != nil {
		log.Errorf("recovery telemetry panicked at %s and was contained: %v", where, rec)
	}
}
