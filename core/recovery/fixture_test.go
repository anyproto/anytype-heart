package recovery

import (
	"sync"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
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

type fixture struct {
	*Tracker
	sender *recordingSender
	clock  *fakeClock
	hooks  session.HookRunner
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
	}
	fx.Tracker = newTracker(fx.clock, coalesceWindow)
	fx.Begin(Run{Mode: mode, Sender: fx.sender})
	return fx
}

// init runs the component Init against a minimal app carrying the session
// hook runner (no config: networkId stays empty, no sender: Begin's is kept).
func (fx *fixture) init(t *testing.T) {
	t.Helper()
	a := new(app.App)
	a.Register(fx.hooks)
	require.NoError(t, fx.Init(a))
}

func (fx *fixture) lastUpdate(t *testing.T) *pb.EventAccountRecoveryUpdate {
	t.Helper()
	ups := fx.sender.updates()
	require.NotEmpty(t, ups)
	return ups[len(ups)-1]
}
