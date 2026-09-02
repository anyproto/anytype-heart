package recovery

import (
	"time"

	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"
)

// coalesceWindow is the coalescing period: repeats of the same state for the
// same peer or space within it collapse to one trailing event.
const coalesceWindow = 250 * time.Millisecond

// coalesceKey identifies the level a coalesced payload replaces: one per
// (kind, peer or space).
type coalesceKey struct {
	kind string
	id   string
}

type timer interface {
	Stop() bool
}

type clock interface {
	Now() time.Time
	AfterFunc(d time.Duration, f func()) timer
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

func (realClock) AfterFunc(d time.Duration, f func()) timer { return time.AfterFunc(d, f) }

// markLocked publishes or schedules one payload. key == nil is an edge and
// publishes at once, together with whatever the window still holds, so
// ordering stays causal. A keyed payload publishes at once when the window is
// idle (the first of a burst) and otherwise replaces the pending level under
// its key, to go out on the trailing edge.
func (t *Tracker) markLocked(payload pb.IsEventAccountRecoveryUpdatePayload, key *coalesceKey) {
	t.emitLocked(payload, key, false)
}

func (t *Tracker) emitLocked(payload pb.IsEventAccountRecoveryUpdatePayload, key *coalesceKey, force bool) {
	if !t.run.started || (t.run.closed && !force) {
		return
	}
	now := t.clock.Now()
	if key == nil || !now.Before(t.nextAt) {
		t.publishLocked(payload)
		return
	}
	if _, ok := t.pending[*key]; !ok {
		t.pendingOrder = append(t.pendingOrder, *key)
	}
	t.pending[*key] = payload
	if t.timer == nil {
		// the trailing edge: whatever the window swallowed must still arrive
		t.timer = t.clock.AfterFunc(t.nextAt.Sub(now), t.onTimer)
	}
}

// publishLocked assigns ids — at publication, never at arrival — and sends the
// pending window plus extra as ONE pb.Event. The send happens under the mutex
// so ids and delivery order cannot disagree; that is safe because the sender
// is non-blocking by contract (core/event/event_grpc.go sendEvent enqueues
// onto a bounded per-session queue).
func (t *Tracker) publishLocked(extra pb.IsEventAccountRecoveryUpdatePayload) {
	now := t.clock.Now()
	msgs := make([]*pb.EventMessage, 0, len(t.pendingOrder)+1)
	for _, key := range t.pendingOrder {
		msgs = append(msgs, t.wrapLocked(now, t.pending[key]))
	}
	if extra != nil {
		msgs = append(msgs, t.wrapLocked(now, extra))
	}
	t.pending = map[coalesceKey]pb.IsEventAccountRecoveryUpdatePayload{}
	t.pendingOrder = nil
	t.nextAt = now.Add(t.window)
	if len(msgs) == 0 || t.sender == nil {
		return
	}
	t.sender.Broadcast(&pb.Event{Messages: msgs})
}

func (t *Tracker) wrapLocked(now time.Time, payload pb.IsEventAccountRecoveryUpdatePayload) *pb.EventMessage {
	update := &pb.EventAccountRecoveryUpdate{
		RunId:       t.run.runId,
		Id:          t.nextId,
		TimestampMs: now.UnixMilli(),
		Payload:     payload,
	}
	t.nextId++
	return event.NewMessage("", &pb.EventMessageValueOfAccountRecoveryUpdate{AccountRecoveryUpdate: update})
}

func (t *Tracker) onTimer() {
	defer containTelemetry("timer")
	t.mu.Lock()
	defer t.mu.Unlock()
	t.timer = nil
	if t.run.closed || len(t.pendingOrder) == 0 {
		return
	}
	t.publishLocked(nil)
}
