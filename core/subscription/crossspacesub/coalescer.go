package crossspacesub

import (
	"time"

	"github.com/anyproto/anytype-heart/pb"
)

const (
	// defaultInitialGrace is how long the first broadcast for a new subId is
	// held after the subscription was created, to let the client apply the
	// subscribe response before live events land.
	defaultInitialGrace = 200 * time.Millisecond
	// defaultWindow is the steady-state coalescing window measured from the
	// first buffered message of a flush.
	defaultWindow = 50 * time.Millisecond
	// maxFlushSize is the hard upper bound on messages per Broadcast.
	maxFlushSize = 500
)

// coalescer is the pure, synchronous buffering policy for the client-facing
// cross-space broadcast path. It owns the coalescing window, the once-per-sub
// initial grace, the per-Broadcast size cap, and counters last-wins. It holds
// no goroutine and no real clock: callers pass `now` so it is fully
// deterministic and unit-testable. The run loop (crossspacesub.go) drives it.
type coalescer struct {
	createdAt time.Time
	grace     time.Duration
	window    time.Duration
	maxFlush  int

	firstDone bool
	buf       []*pb.EventMessage
	deadline  time.Time // zero when buf is empty
}

func newCoalescer(createdAt time.Time, grace, window time.Duration, maxFlush int) *coalescer {
	if maxFlush <= 0 {
		maxFlush = maxFlushSize // guard: a non-positive cap would spin the chunk loop
	}
	return &coalescer{createdAt: createdAt, grace: grace, window: window, maxFlush: maxFlush}
}

// push appends already-patched messages. It arms the flush deadline when the
// buffer transitions from empty to non-empty: the first flush is held until
// max(createdAt+grace, now+window); later flushes use now+window.
func (c *coalescer) push(now time.Time, msgs []*pb.EventMessage) {
	if len(msgs) == 0 {
		return
	}
	if len(c.buf) == 0 {
		if !c.firstDone {
			c.deadline = laterOf(c.createdAt.Add(c.grace), now.Add(c.window))
		} else {
			c.deadline = now.Add(c.window)
		}
	}
	c.buf = append(c.buf, msgs...)
}

// ready returns the batches to broadcast now, each already counters-coalesced
// and never larger than maxFlush. Rules:
//   - first-flush grace: while !firstDone and now < deadline, hold everything
//     (even a buffer over the size cap — the grace must not be pre-empted);
//   - size cap: once grace is satisfied (or on a later flush), emit full
//     maxFlush-sized chunks whenever the buffer is over the cap;
//   - window: a sub-cap remainder is emitted only once the deadline passes.
//
// A sub-cap remainder left after size-triggered chunking keeps its existing
// (future) deadline, so it still flushes on time.
func (c *coalescer) ready(now time.Time) [][]*pb.EventMessage {
	if len(c.buf) == 0 {
		c.deadline = time.Time{}
		return nil
	}
	if !c.firstDone && now.Before(c.deadline) {
		return nil // first-flush grace not met: hold
	}
	timeUp := !now.Before(c.deadline)

	var out [][]*pb.EventMessage
	for len(c.buf) >= c.maxFlush {
		out = append(out, coalesceCounters(c.buf[:c.maxFlush:c.maxFlush]))
		c.buf = c.buf[c.maxFlush:]
		c.firstDone = true
	}
	if timeUp && len(c.buf) > 0 {
		out = append(out, coalesceCounters(c.buf))
		c.buf = nil
		c.firstDone = true
	}
	if len(c.buf) == 0 {
		c.deadline = time.Time{}
	}
	return out
}

// nextDeadline is when the run loop should wake to flush; zero when idle.
func (c *coalescer) nextDeadline() time.Time { return c.deadline }

// coalesceCounters drops every SubscriptionCounters message except the last and
// appends that survivor at the tail. After patchEvent each counters message
// carries the cross-space subId and an absolute aggregate total, so only the
// latest matters; its position relative to adds/removes is irrelevant. With ≤1
// counters message it returns the input unchanged (no relocation). It runs per
// emitted chunk, so a flush split across multiple Broadcasts can carry one
// counters message per chunk — each self-consistent, last-wins across the
// ordered Broadcasts.
func coalesceCounters(msgs []*pb.EventMessage) []*pb.EventMessage {
	last := -1
	count := 0
	for i, m := range msgs {
		if m.GetSubscriptionCounters() != nil {
			last = i
			count++
		}
	}
	if count <= 1 {
		return msgs
	}
	out := make([]*pb.EventMessage, 0, len(msgs)-count+1)
	for _, m := range msgs {
		if m.GetSubscriptionCounters() != nil {
			continue // skip all counters here; append the survivor at the tail below
		}
		out = append(out, m)
	}
	return append(out, msgs[last])
}

func laterOf(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}
