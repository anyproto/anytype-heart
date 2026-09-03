package recovery

import (
	"sync"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/net/peerobserver"
	"github.com/anyproto/any-sync/nodeconf"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/space/spacecore/localdiscovery"
	"github.com/anyproto/anytype-heart/space/spacecore/peerstore"
)

// recordingSender captures everything the tracker sends, in order.
type recordingSender struct {
	mu         sync.Mutex
	broadcasts []*pb.Event
	sessions   map[string][]*pb.Event
	panicOnce  bool
}

func (r *recordingSender) Broadcast(ev *pb.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.panicOnce {
		r.panicOnce = false
		panic("sender panic")
	}
	r.broadcasts = append(r.broadcasts, ev)
}

func (r *recordingSender) SendToSession(token string, ev *pb.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sessions == nil {
		r.sessions = map[string][]*pb.Event{}
	}
	r.sessions[token] = append(r.sessions[token], ev)
}

func (r *recordingSender) BroadcastToOtherSessions(string, *pb.Event) {}

func (r *recordingSender) Init(*app.App) error { return nil }

func (r *recordingSender) Name() string { return "eventSender" }

func (r *recordingSender) IsActive(string) bool { return true }

func (r *recordingSender) BroadcastExceptSessions(*pb.Event, []string) {}

// updates flattens every broadcast into the recovery updates it carried.
func (r *recordingSender) updates() []*pb.EventAccountRecoveryUpdate {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*pb.EventAccountRecoveryUpdate
	for _, ev := range r.broadcasts {
		out = append(out, updatesOf(ev)...)
	}
	return out
}

func updatesOf(ev *pb.Event) []*pb.EventAccountRecoveryUpdate {
	var out []*pb.EventAccountRecoveryUpdate
	for _, msg := range ev.Messages {
		if v, ok := msg.Value.(*pb.EventMessageValueOfAccountRecoveryUpdate); ok {
			out = append(out, v.AccountRecoveryUpdate)
		}
	}
	return out
}

type fakeTimer struct {
	at      time.Time
	f       func()
	stopped bool
}

func (t *fakeTimer) Stop() bool {
	was := !t.stopped
	t.stopped = true
	return was
}

// fakeClock is a manual clock: Advance moves time and fires due timers in
// order, outside any tracker lock (as the real time.AfterFunc would).
type fakeClock struct {
	mu     sync.Mutex
	now    time.Time
	timers []*fakeTimer
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) AfterFunc(d time.Duration, f func()) timer {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &fakeTimer{at: c.now.Add(d), f: f}
	c.timers = append(c.timers, t)
	return t
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	var due, rest []*fakeTimer
	for _, t := range c.timers {
		if !t.stopped && !t.at.After(c.now) {
			due = append(due, t)
		} else if !t.stopped {
			rest = append(rest, t)
		}
	}
	c.timers = rest
	c.mu.Unlock()
	for _, t := range due {
		t.f()
	}
}

func (c *fakeClock) pendingTimers() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, t := range c.timers {
		if !t.stopped {
			n++
		}
	}
	return n
}

// fakeNodeConf answers NodeTypes from a map, under nodeconf's component name.
type fakeNodeConf struct {
	types map[string][]nodeconf.NodeType
}

func (f *fakeNodeConf) Init(*app.App) error { return nil }

func (f *fakeNodeConf) Name() string { return nodeconf.CName }

func (f *fakeNodeConf) NodeTypes(id string) []nodeconf.NodeType { return f.types[id] }

// fakeMux stands in for net/peerobservermux under the peerobserver slot.
type fakeMux struct {
	observers []peerobserver.Observer
}

func (f *fakeMux) Init(*app.App) error { return nil }

func (f *fakeMux) Name() string { return peerobserver.CName }

func (f *fakeMux) Add(o peerobserver.Observer) { f.observers = append(f.observers, o) }

// fakeDiscovery captures the possibility hook under localdiscovery's name.
type fakeDiscovery struct {
	hooks []func(localdiscovery.DiscoveryPossibility)
}

func (f *fakeDiscovery) Init(*app.App) error { return nil }

func (f *fakeDiscovery) Name() string { return localdiscovery.CName }

func (f *fakeDiscovery) RegisterDiscoveryPossibilityHook(hook func(localdiscovery.DiscoveryPossibility)) {
	f.hooks = append(f.hooks, hook)
}

func (f *fakeDiscovery) set(state localdiscovery.DiscoveryPossibility) {
	for _, hook := range f.hooks {
		hook(state)
	}
}

// fakeNetwork captures the connectivity hook under core/device's name.
type fakeNetwork struct {
	offline bool
	hooks   []func(online bool)
}

func (f *fakeNetwork) Init(*app.App) error { return nil }

func (f *fakeNetwork) Name() string { return deviceNetworkStateCName }

func (f *fakeNetwork) IsOffline() bool { return f.offline }

func (f *fakeNetwork) RegisterConnectivityHook(hook func(online bool)) {
	f.hooks = append(f.hooks, hook)
}

func (f *fakeNetwork) set(online bool) {
	f.offline = !online
	for _, hook := range f.hooks {
		hook(online)
	}
}

// fakePeerStore captures the exchange observer under peerstore's name.
type fakePeerStore struct {
	observers []peerstore.Observer
}

func (f *fakePeerStore) Init(*app.App) error { return nil }

func (f *fakePeerStore) Name() string { return peerstore.CName }

func (f *fakePeerStore) AddObserver(o peerstore.Observer) { f.observers = append(f.observers, o) }

// exchange simulates UpdateLocalPeer's notification after a space exchange.
func (f *fakePeerStore) exchange(peerId string, before, after []string, removed bool) {
	for _, o := range f.observers {
		o(peerId, before, after, removed)
	}
}

type fixture struct {
	*Tracker
	sender    *recordingSender
	clock     *fakeClock
	hooks     session.HookRunner
	nodes     *fakeNodeConf
	mux       *fakeMux
	discovery *fakeDiscovery
	network   *fakeNetwork
	peers     *fakePeerStore
}

var fixtureEpoch = time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)

// newFixture returns a tracker with a run begun but not initialised, so a
// test can choose what happens between Begin and Init.
func newFixture(t *testing.T, mode pb.EventAccountRecoveryMode) *fixture {
	t.Helper()
	fx := &fixture{
		sender: &recordingSender{},
		clock:  &fakeClock{now: fixtureEpoch},
		hooks:  session.NewHookRunner(),
		nodes: &fakeNodeConf{types: map[string][]nodeconf.NodeType{
			"node1": {nodeconf.NodeTypeTree},
			"coord": {nodeconf.NodeTypeCoordinator},
		}},
		mux:       &fakeMux{},
		discovery: &fakeDiscovery{},
		network:   &fakeNetwork{},
		peers:     &fakePeerStore{},
	}
	fx.Tracker = newTracker(fx.clock, coalesceWindow)
	fx.Begin(Run{Mode: mode, Sender: fx.sender})
	return fx
}

// init runs the component Init against a minimal app carrying the session
// hook runner and the peer-layer fakes (no config: networkId stays empty, no
// sender: Begin's is kept).
func (fx *fixture) init(t *testing.T) {
	t.Helper()
	a := new(app.App)
	a.Register(fx.hooks).Register(fx.nodes).Register(fx.mux).Register(fx.discovery).Register(fx.network).Register(fx.peers)
	require.NoError(t, fx.Init(a))
}

func (fx *fixture) dialStarted(peerId string, addrs int) {
	fx.ObservePeerEvent(peerobserver.Event{Kind: peerobserver.KindDialStarted, PeerId: peerId, AddrCount: addrs})
}

func (fx *fixture) connected(peerId, scheme string, inbound bool, dur time.Duration) {
	fx.ObservePeerEvent(peerobserver.Event{
		Kind: peerobserver.KindConnected, PeerId: peerId, Scheme: scheme, Inbound: inbound,
		Addr: "addr:" + peerId, ProtoVersion: 7, Dur: dur,
	})
}

func (fx *fixture) dialFailed(peerId string, err error, dur time.Duration) {
	fx.ObservePeerEvent(peerobserver.Event{Kind: peerobserver.KindDialFailed, PeerId: peerId, Err: err, Dur: dur})
}

func (fx *fixture) closed(peerId string, inbound bool) {
	fx.ObservePeerEvent(peerobserver.Event{Kind: peerobserver.KindClosed, PeerId: peerId, Inbound: inbound})
}

// flush advances past the coalescing window so pending levels publish.
func (fx *fixture) flush() {
	fx.clock.Advance(coalesceWindow)
}

func (fx *fixture) phaseChanges() []*pb.EventAccountRecoveryPhaseChanged {
	var out []*pb.EventAccountRecoveryPhaseChanged
	for _, u := range fx.sender.updates() {
		if p, ok := u.Payload.(*pb.EventAccountRecoveryUpdatePayloadOfPhaseChanged); ok {
			out = append(out, p.PhaseChanged)
		}
	}
	return out
}

func (fx *fixture) peer(t *testing.T, peerId string) *pb.EventAccountRecoverySnapshotPeer {
	t.Helper()
	for _, p := range fx.Snapshot().Peers {
		if p.PeerId == peerId {
			return p
		}
	}
	t.Fatalf("peer %s not in snapshot", peerId)
	return nil
}

func (fx *fixture) lastUpdate(t *testing.T) *pb.EventAccountRecoveryUpdate {
	t.Helper()
	ups := fx.sender.updates()
	require.NotEmpty(t, ups)
	return ups[len(ups)-1]
}
