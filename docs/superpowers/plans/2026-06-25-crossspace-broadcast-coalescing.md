# Cross-Space Broadcast Coalescing — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Coalesce and grace-delay the event broadcasts of the client-facing cross-space subscription (`ObjectCrossSpaceSearchSubscribe`) to shrink the subscribe-response/event race window and the startup redraw storm, and fix two adjacent pre-existing bugs.

**Architecture:** Separate the buffering *policy* from the delivery *mechanism*. A pure, synchronous `coalescer` (timestamps passed in, no goroutines, no real time) owns the window/grace/size-cap/counters-last-wins logic and is exhaustively unit-tested. A thin run-loop (a drainer goroutine feeding a `select` over the message channel and an injected-clock timer) drives the coalescer and calls `eventSender.Broadcast`. Only the `internalQueue == nil` (broadcast) path is changed; the nested-forward path is untouched.

**Tech Stack:** Go; `github.com/cheggaaa/mb/v3` (message buffer queue); `github.com/anyproto/anytype-heart/pb` (event protos); testify (`assert`/`require`/`mock`); existing `mock_event.MockSender` and `mock_subscription.MockService`.

## Global Constraints

- Commit message prefix: `GO-7334` for coalescing work; `GO-7336` for the resubscribe-leak fix; `GO-7337` for the counter-resurrection fix. Format: `GO-NNNN Short description`.
- Wrap every non-trivial error with context: `fmt.Errorf("operation: %w", err)`. Never return a bare `err` — wrap it. Do not change a bare `return` to `return err`; change it to `return fmt.Errorf("context: %w", err)`.
- All work targets the `develop` branch (branch off `develop`).
- The full `crossspacesub` and `core/subscription` suites must pass under `-race`.
- Scope is `core/subscription/crossspacesub` only. Do not touch the regular subscription engine, the event sender, or the wire protocol.
- Package consts (in `core/subscription/crossspacesub`): `defaultInitialGrace = 200 * time.Millisecond`, `defaultWindow = 50 * time.Millisecond`, `maxFlushSize = 500`.

---

## File Structure

- **Create** `core/subscription/crossspacesub/coalescer.go` — pure policy: `coalescer` type (`push`/`ready`/`nextDeadline`), `coalesceCounters`, `laterOf`.
- **Create** `core/subscription/crossspacesub/coalescer_test.go` — exhaustive pure unit tests.
- **Create** `core/subscription/crossspacesub/clock.go` — `clock` interface + `realClock`.
- **Modify** `core/subscription/crossspacesub/crossspacesub.go` — add timing/clock fields to `crossSpaceSubscription`; rewrite the broadcast branch of `run`; add `drain`; counter-resurrection fix in `updateTotalCount`/finalize/`removeSpace` (Task 4).
- **Modify** `core/subscription/crossspacesub/service.go` — add `initialGrace`/`window`/`clk` to `service`, default them in `Init`, pass to `newCrossSpaceSubscription`; resubscribe-replace in `Subscribe` (Task 3).
- **Modify** `core/subscription/crossspacesub/crossspacesub_test.go` — update `newBareCrossSpaceSub` for new fields; add Task 4 unit test; add a fake clock helper.
- **Modify** `core/subscription/crossspacesub/service_test.go` — set `initialGrace=0`/`window=0` in `newFixture`; add Task 2 wiring tests and Task 3 resubscribe test.

---

## Task 1: Pure coalescer policy

**Files:**
- Create: `core/subscription/crossspacesub/coalescer.go`
- Test: `core/subscription/crossspacesub/coalescer_test.go`

**Interfaces:**
- Produces:
  - `type coalescer struct { ... }` with constructor `newCoalescer(createdAt time.Time, grace, window time.Duration, maxFlush int) *coalescer`
  - `func (c *coalescer) push(now time.Time, msgs []*pb.EventMessage)`
  - `func (c *coalescer) ready(now time.Time) [][]*pb.EventMessage`
  - `func (c *coalescer) nextDeadline() time.Time` (zero value when buffer empty)
  - `func coalesceCounters(msgs []*pb.EventMessage) []*pb.EventMessage`
  - `func laterOf(a, b time.Time) time.Time`
- Consumes: `*pb.EventMessage`, `pb.EventMessageValueOfSubscriptionCounters` from `github.com/anyproto/anytype-heart/pb`.

- [ ] **Step 1: Write the failing test for `coalesceCounters`**

Create `core/subscription/crossspacesub/coalescer_test.go`:

```go
package crossspacesub

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"
)

func addMsg(id string) *pb.EventMessage {
	return event.NewMessage("s", &pb.EventMessageValueOfSubscriptionAdd{
		SubscriptionAdd: &pb.EventObjectSubscriptionAdd{Id: id, SubId: "cs"},
	})
}

func countersMsg(total int64) *pb.EventMessage {
	return event.NewMessage("s", &pb.EventMessageValueOfSubscriptionCounters{
		SubscriptionCounters: &pb.EventObjectSubscriptionCounters{Total: total, SubId: "cs"},
	})
}

func countersTotals(msgs []*pb.EventMessage) []int64 {
	var out []int64
	for _, m := range msgs {
		if c := m.GetSubscriptionCounters(); c != nil {
			out = append(out, c.Total)
		}
	}
	return out
}

func TestCoalesceCounters(t *testing.T) {
	t.Run("keeps only the last counters, at the tail", func(t *testing.T) {
		in := []*pb.EventMessage{countersMsg(1), addMsg("a"), countersMsg(2), addMsg("b"), countersMsg(3)}
		out := coalesceCounters(in)
		// 2 adds + exactly 1 counters
		require.Len(t, out, 3)
		assert.Equal(t, []int64{3}, countersTotals(out))
		assert.Nil(t, out[len(out)-1].GetSubscriptionAdd(), "counters must be last")
		assert.NotNil(t, out[len(out)-1].GetSubscriptionCounters())
	})
	t.Run("no counters: unchanged", func(t *testing.T) {
		in := []*pb.EventMessage{addMsg("a"), addMsg("b")}
		assert.Equal(t, in, coalesceCounters(in))
	})
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./core/subscription/crossspacesub/ -run TestCoalesceCounters -v`
Expected: FAIL — `undefined: coalesceCounters`.

- [ ] **Step 3: Implement `coalescer.go`**

Create `core/subscription/crossspacesub/coalescer.go`:

```go
package crossspacesub

import (
	"time"

	"github.com/anyproto/anytype-heart/pb"
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
// latest matters; its position relative to adds/removes is irrelevant.
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
	for i, m := range msgs {
		if m.GetSubscriptionCounters() != nil && i != last {
			continue
		}
		if i == last {
			continue // append survivor at the tail below
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
```

- [ ] **Step 4: Run `TestCoalesceCounters` to verify it passes**

Run: `go test ./core/subscription/crossspacesub/ -run TestCoalesceCounters -v`
Expected: PASS.

- [ ] **Step 5: Add the policy tests (grace, size cap, window)**

Append to `coalescer_test.go`:

```go
func adds(n int) []*pb.EventMessage {
	out := make([]*pb.EventMessage, n)
	for i := range out {
		out[i] = addMsg("x")
	}
	return out
}

func flatLen(batches [][]*pb.EventMessage) int {
	n := 0
	for _, b := range batches {
		n += len(b)
	}
	return n
}

func TestCoalescer(t *testing.T) {
	t0 := time.Unix(1000, 0)
	grace := 200 * time.Millisecond
	window := 50 * time.Millisecond

	t.Run("first flush held until createdAt+grace", func(t *testing.T) {
		c := newCoalescer(t0, grace, window, maxFlushSize)
		c.push(t0, adds(3))
		assert.Nil(t, c.ready(t0), "must hold during grace")
		assert.Nil(t, c.ready(t0.Add(grace-time.Millisecond)), "still within grace")
		out := c.ready(t0.Add(grace))
		assert.Equal(t, 3, flatLen(out), "flush at grace deadline")
	})

	t.Run("large first wave still held for grace (M1 regression)", func(t *testing.T) {
		c := newCoalescer(t0, grace, window, maxFlushSize)
		c.push(t0, adds(maxFlushSize+50)) // > cap
		assert.Nil(t, c.ready(t0.Add(grace-time.Millisecond)), "size cap must not pre-empt grace")
		out := c.ready(t0.Add(grace))
		require.Len(t, out, 2)
		assert.Len(t, out[0], maxFlushSize)
		assert.Len(t, out[1], 50)
	})

	t.Run("window coalesces waves that arrive close together", func(t *testing.T) {
		c := newCoalescer(t0, 0, window, maxFlushSize) // grace 0: exercise steady-state window
		c.push(t0, adds(2))
		assert.Equal(t, 2, flatLen(c.ready(t0)), "grace 0 -> deadline = now+window; now==deadline? no")
		// With grace 0 the first push deadline = now+window (future), so nothing yet:
		c2 := newCoalescer(t0, 0, window, maxFlushSize)
		c2.push(t0, adds(2))
		assert.Nil(t, c2.ready(t0), "within window")
		c2.push(t0.Add(window/2), adds(3)) // second wave inside the window
		assert.Nil(t, c2.ready(t0.Add(window/2)))
		out := c2.ready(t0.Add(window))
		assert.Equal(t, 5, flatLen(out), "both waves in one flush")
	})

	t.Run("size cap chunks to a hard bound; sub-cap remainder waits for window", func(t *testing.T) {
		c := newCoalescer(t0, 0, window, maxFlushSize)
		c.firstDone = true // later flush: no grace
		c.push(t0, adds(maxFlushSize+120))
		out := c.ready(t0) // not timeUp, but over cap -> one full chunk only
		require.Len(t, out, 1)
		assert.Len(t, out[0], maxFlushSize)
		assert.Nil(t, c.ready(t0), "remainder (120) waits for window")
		out = c.ready(t0.Add(window))
		require.Len(t, out, 1)
		assert.Len(t, out[0], 120)
	})

	t.Run("empty buffer: no batches, zero deadline", func(t *testing.T) {
		c := newCoalescer(t0, grace, window, maxFlushSize)
		assert.Nil(t, c.ready(t0))
		assert.True(t, c.nextDeadline().IsZero())
	})
}
```

- [ ] **Step 6: Run the full coalescer suite**

Run: `go test ./core/subscription/crossspacesub/ -run 'TestCoalesce|TestCoalescer' -v`
Expected: PASS (all subtests).

- [ ] **Step 7: Commit**

```bash
git add core/subscription/crossspacesub/coalescer.go core/subscription/crossspacesub/coalescer_test.go
git commit -m "GO-7334 crossspacesub: add pure coalescer policy (window/grace/size-cap/counters)"
```

---

## Task 2: Clock seam + wire the coalescer into `run`

**Files:**
- Create: `core/subscription/crossspacesub/clock.go`
- Modify: `core/subscription/crossspacesub/crossspacesub.go` (struct fields, `run`, new `drain`)
- Modify: `core/subscription/crossspacesub/service.go` (service fields/defaults, `newCrossSpaceSubscription` signature + call site)
- Modify: `core/subscription/crossspacesub/crossspacesub_test.go` (`newBareCrossSpaceSub` fields, fake clock helper)
- Modify: `core/subscription/crossspacesub/service_test.go` (zero timing in `newFixture`, wiring tests)

**Interfaces:**
- Consumes: `coalescer` from Task 1.
- Produces:
  - `type clock interface { now() time.Time; after(d time.Duration) <-chan time.Time }`
  - `realClock` implementing it.
  - `crossSpaceSubscription` gains fields `createdAt time.Time`, `initialGrace, window time.Duration`, `clk clock`.
  - `newCrossSpaceSubscription(subId string, request subscriptionservice.SubscribeRequest, eventSender event.Sender, subscriptionService subscriptionservice.Service, loadedSpaceIds []string, pendingSpaceIds []string, predicate Predicate, clk clock, grace, window time.Duration)` (extended signature).
  - `service` gains `initialGrace, window time.Duration`, `clk clock`.

- [ ] **Step 1: Create the clock seam**

Create `core/subscription/crossspacesub/clock.go`:

```go
package crossspacesub

import "time"

// clock is the timing seam so the broadcast run loop is deterministic in tests.
// Production uses realClock; tests inject a fake whose after() channel they fire
// manually. It is a struct field (never a package var) to stay -race clean.
type clock interface {
	now() time.Time
	after(d time.Duration) <-chan time.Time
}

type realClock struct{}

func (realClock) now() time.Time                         { return time.Now() }
func (realClock) after(d time.Duration) <-chan time.Time { return time.After(d) }
```

- [ ] **Step 2: Add timing consts and struct fields**

In `core/subscription/crossspacesub/crossspacesub.go`, add `"time"` to imports, add consts near the top, and the fields to `crossSpaceSubscription`:

```go
const (
	defaultInitialGrace = 200 * time.Millisecond
	defaultWindow       = 50 * time.Millisecond
	maxFlushSize        = 500
)
```

Add to the `crossSpaceSubscription` struct (after `queue`):

```go
	clk          clock
	createdAt    time.Time
	initialGrace time.Duration
	window       time.Duration
```

- [ ] **Step 3: Extend `newCrossSpaceSubscription` to accept and stamp timing**

In `crossspacesub.go`, change the signature and set the fields. Add to the struct literal in `newCrossSpaceSubscription`: `clk: clk, initialGrace: grace, window: window,`. Stamp `createdAt` as late as possible — immediately before the final `return`:

```go
func newCrossSpaceSubscription(subId string, request subscriptionservice.SubscribeRequest, eventSender event.Sender, subscriptionService subscriptionservice.Service, loadedSpaceIds []string, pendingSpaceIds []string, predicate Predicate, clk clock, grace, window time.Duration) (*crossSpaceSubscription, *subscriptionservice.SubscribeResponse, error) {
	ctx, ctxCancel := context.WithCancel(context.Background())
	s := &crossSpaceSubscription{
		ctx:                   ctx,
		ctxCancel:             ctxCancel,
		subId:                 subId,
		request:               request,
		eventSender:           eventSender,
		spacePredicate:        predicate,
		subscriptionService:   subscriptionService,
		perSpaceSubscriptions: make(map[string]string),
		inflightSpaceIds:      make(map[string]uint64),
		pendingSpaceIds:       make(map[string]struct{}, len(pendingSpaceIds)),
		totalCounts:           map[string]int64{},
		queue:                 mb.New[*pb.EventMessage](0),
		clk:                   clk,
		initialGrace:          grace,
		window:                window,
	}
	// ... existing body unchanged through wg.Wait() ...

	s.createdAt = s.clk.now() // anchor grace as late as the server can (≈ response send)
	return s, aggregatedResp, resErr
}
```

- [ ] **Step 4: Rewrite the broadcast branch of `run`; add `drain`**

In `crossspacesub.go`, replace `run` with the version below. The nested (`internalQueue != nil`) path keeps the existing per-message forward loop; the broadcast path uses the drainer + select + coalescer.

```go
func (s *crossSpaceSubscription) run(internalQueue *mb.MB[*pb.EventMessage]) {
	if internalQueue != nil {
		s.runNested(internalQueue)
		return
	}
	s.runBroadcast()
}

// runNested forwards each patched message to the parent queue (unchanged behavior).
func (s *crossSpaceSubscription) runNested(internalQueue *mb.MB[*pb.EventMessage]) {
	for {
		msgs, err := s.queue.Wait(s.ctx)
		if errors.Is(err, context.Canceled) {
			return
		}
		if err != nil {
			log.Error("wait messages", zap.Error(err), zap.String("subId", s.subId))
		}
		for _, msg := range msgs {
			s.patchEvent(msg)
			if aerr := internalQueue.Add(s.ctx, msg); aerr != nil {
				log.Error("add to internal queue", zap.Error(aerr), zap.String("subId", s.subId))
			}
		}
	}
}

// runBroadcast coalesces patched messages and broadcasts them, holding the
// first emission for the initial grace. A drainer goroutine converts the
// blocking queue into a channel so the loop can select message-vs-timer.
func (s *crossSpaceSubscription) runBroadcast() {
	msgCh := make(chan []*pb.EventMessage)
	go s.drain(msgCh)

	c := newCoalescer(s.createdAt, s.initialGrace, s.window, maxFlushSize)
	flush := func() {
		for _, batch := range c.ready(s.clk.now()) {
			s.eventSender.Broadcast(&pb.Event{Messages: batch})
		}
	}
	for {
		var timerC <-chan time.Time
		if d := c.nextDeadline(); !d.IsZero() {
			timerC = s.clk.after(d.Sub(s.clk.now()))
		}
		select {
		case <-s.ctx.Done():
			return
		case msgs, ok := <-msgCh:
			if !ok {
				return
			}
			for _, m := range msgs {
				s.patchEvent(m)
			}
			c.push(s.clk.now(), msgs)
			flush()
		case <-timerC:
			flush()
		}
	}
}

// drain reads bounded batches from the queue and hands them to runBroadcast,
// stopping when the queue closes or the subscription is cancelled.
func (s *crossSpaceSubscription) drain(out chan<- []*pb.EventMessage) {
	defer close(out)
	cond := s.queue.NewCond().WithMax(maxFlushSize)
	for {
		msgs, err := s.queue.WaitCond(s.ctx, cond)
		if err != nil {
			return
		}
		select {
		case out <- msgs:
		case <-s.ctx.Done():
			return
		}
	}
}
```

(Remove the old single `run` body. `patchEvent` is now called in `runBroadcast`/`runNested`; keep `patchEvent`/`patchAll` semantics — patching still happens on the single run-loop goroutine, before `push`.)

- [ ] **Step 5: Add service fields, defaults, and pass-through**

In `core/subscription/crossspacesub/service.go`: add `"time"` to imports; add fields to `service`:

```go
	initialGrace time.Duration
	window       time.Duration
	clk          clock
```

In `Init`, set defaults:

```go
	s.initialGrace = defaultInitialGrace
	s.window = defaultWindow
	s.clk = realClock{}
```

In `Subscribe`, pass them to the constructor:

```go
	spaceSub, resp, err := newCrossSpaceSubscription(req.SubId, req, s.eventSender, s.subscriptionService, loadedIds, pendingIds, spaceViewPredicate, s.clk, s.initialGrace, s.window)
```

- [ ] **Step 6: Update `newBareCrossSpaceSub` and add a fake clock helper**

In `crossspacesub_test.go`, set the new fields in `newBareCrossSpaceSub` struct literal: `clk: realClock{}, initialGrace: 0, window: 0,`. Add a fake clock at the end of the file:

```go
type fakeClock struct {
	mu      sync.Mutex
	t       time.Time
	waiters []fakeWaiter
}

type fakeWaiter struct {
	at time.Time
	ch chan time.Time
}

func newFakeClock() *fakeClock { return &fakeClock{t: time.Unix(1000, 0)} }

func (c *fakeClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) after(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	ch := make(chan time.Time, 1)
	if d <= 0 {
		ch <- c.t
		return ch
	}
	c.waiters = append(c.waiters, fakeWaiter{at: c.t.Add(d), ch: ch})
	return ch
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
	kept := c.waiters[:0]
	for _, w := range c.waiters {
		if !w.at.After(c.t) {
			w.ch <- c.t
		} else {
			kept = append(kept, w)
		}
	}
	c.waiters = kept
}

func (c *fakeClock) numWaiters() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.waiters)
}
```

- [ ] **Step 7: Zero the timing in the integration fixture**

In `service_test.go` `newFixture`, after `a.Start` succeeds and before `return`, neutralize timing so existing exact-sequence tests are unaffected and fast:

```go
	svc := s.(*service)
	svc.initialGrace = 0
	svc.window = 0
```

(Use `svc` in the returned `service:` field.)

- [ ] **Step 8: Run the existing integration suite to verify no regressions**

Run: `go test ./core/subscription/crossspacesub/ -run TestSubscribe -race -v`
Expected: PASS — coalescing with grace=0/window=0 flushes each wave immediately, so the asserted message sequences are unchanged.

- [ ] **Step 9: Add a wiring test (grace actually delays via the injected clock)**

Add to `crossspacesub_test.go`:

```go
func TestRunBroadcast_graceDelaysFirstFlush(t *testing.T) {
	ms := mock_subscription.NewMockService(t)
	s := newBareCrossSpaceSub(t, ms)
	fc := newFakeClock()
	s.clk = fc
	s.initialGrace = 200 * time.Millisecond
	s.window = 50 * time.Millisecond
	s.createdAt = fc.now()

	var mu sync.Mutex
	var broadcasts int
	sender := mock_event.NewMockSender(t)
	sender.EXPECT().Broadcast(mock.Anything).Run(func(*pb.Event) {
		mu.Lock()
		broadcasts++
		mu.Unlock()
	}).Maybe()
	s.eventSender = sender

	go s.runBroadcast()

	require.NoError(t, s.queue.Add(context.Background(), addMsg("a")))
	// run loop must arm the grace timer before we advance the clock
	require.Eventually(t, func() bool { return fc.numWaiters() >= 1 }, time.Second, time.Millisecond)
	mu.Lock()
	assert.Equal(t, 0, broadcasts, "no broadcast before grace deadline")
	mu.Unlock()

	fc.advance(200 * time.Millisecond)
	require.Eventually(t, func() bool { mu.Lock(); defer mu.Unlock(); return broadcasts == 1 },
		time.Second, time.Millisecond, "exactly one broadcast after grace")
}
```

(`s.eventSender` is set directly; ensure the field is assignable — it is exported within the package.)

- [ ] **Step 10: Run the wiring test**

Run: `go test ./core/subscription/crossspacesub/ -run TestRunBroadcast_graceDelaysFirstFlush -race -v`
Expected: PASS.

- [ ] **Step 11: Run the whole package under -race**

Run: `go test ./core/subscription/crossspacesub/ -race`
Expected: PASS.

- [ ] **Step 12: Commit**

```bash
git add core/subscription/crossspacesub/clock.go core/subscription/crossspacesub/crossspacesub.go core/subscription/crossspacesub/service.go core/subscription/crossspacesub/crossspacesub_test.go core/subscription/crossspacesub/service_test.go
git commit -m "GO-7334 crossspacesub: coalesce + grace-delay broadcasts on the client-facing path"
```

---

## Task 3: Resubscribe replace (GO-7336)

**Files:**
- Modify: `core/subscription/crossspacesub/service.go` (`Subscribe`)
- Test: `core/subscription/crossspacesub/service_test.go`

**Interfaces:**
- Consumes: existing `service.subscriptions`, `crossSpaceSubscription.close()`.

- [ ] **Step 1: Write the failing test**

Add to `service_test.go`:

```go
func TestSubscribe_resubscribeReplacesOldSub(t *testing.T) {
	fx := newFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	fx.objectStore.AddObjects(t, techSpaceId, []objectstore.TestObject{
		givenSpaceViewObject("spaceView1", "space1", model.SpaceStatus_SpaceActive, model.SpaceStatus_Ok),
	})

	req := givenRequest()
	req.SubId = "fixed-sub-id"
	_, err := fx.Subscribe(req, NoOpPredicate())
	require.NoError(t, err)
	// resubscribe with the SAME subId
	_, err = fx.Subscribe(req, NoOpPredicate())
	require.NoError(t, err)

	// exactly one live subscription is registered for that subId
	fx.service.lock.Lock()
	_, ok := fx.service.subscriptions[req.SubId]
	n := len(fx.service.subscriptions)
	fx.service.lock.Unlock()
	assert.True(t, ok)
	assert.Equal(t, 1, n, "resubscribe must not leave two subs for one subId")

	// a single object change is broadcast once, not twice (no orphaned old loop)
	obj := objectstore.TestObject{
		bundle.RelationKeyId:             domain.String("participant1"),
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_participant)),
	}
	fx.objectStore.AddObjects(t, "space1", []objectstore.TestObject{obj})

	msgs, err := fx.eventQueue.NewCond().WithMin(3).Wait(ctx)
	require.NoError(t, err)
	adds := 0
	for _, m := range msgs {
		if m.GetSubscriptionAdd() != nil && m.GetSubscriptionAdd().Id == "participant1" {
			adds++
		}
	}
	assert.Equal(t, 1, adds, "object added exactly once (old sub did not also broadcast)")
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./core/subscription/crossspacesub/ -run TestSubscribe_resubscribeReplacesOldSub -race -v`
Expected: FAIL — two subs registered and/or the add is broadcast twice.

- [ ] **Step 3: Implement the replace in `Subscribe`**

In `service.go` `Subscribe`, after `req.SubId` is finalized and `s.lock` is held (it already is via `defer`), but **before** building the new sub, close any existing one:

```go
	s.lock.Lock()
	defer s.lock.Unlock()
	if existing, ok := s.subscriptions[req.SubId]; ok {
		if cerr := existing.close(); cerr != nil {
			log.Error("close existing subscription on resubscribe",
				zap.String("subId", req.SubId), zap.Error(cerr))
		}
		delete(s.subscriptions, req.SubId)
	}
	var loadedIds, pendingIds []string
	// ... existing body ...
```

(`crossSpaceSubscription.close()` cancels its ctx and closes its queue; it does not take `s.lock`, so closing under `s.lock` is safe. The old `run`/`drain` goroutines exit; the old per-space subscriptions are released by the cancelled context path. Place this block right after `s.lock.Lock()` / `defer s.lock.Unlock()` at `service.go:170-171`.)

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./core/subscription/crossspacesub/ -run TestSubscribe_resubscribeReplacesOldSub -race -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add core/subscription/crossspacesub/service.go core/subscription/crossspacesub/service_test.go
git commit -m "GO-7336 crossspacesub: close existing sub on resubscribe with same subId (fix leak)"
```

---

## Task 4: Counter resurrection fix (GO-7337)

**Files:**
- Modify: `core/subscription/crossspacesub/crossspacesub.go` (`updateTotalCount`, finalize sites, `removeSpace`)
- Test: `core/subscription/crossspacesub/crossspacesub_test.go`

**Interfaces:**
- Produces: `crossSpaceSubscription` gains `activeInternalSubs map[string]struct{}` (the set of internal per-space subIds whose totals may be counted).

- [ ] **Step 1: Write the failing test**

Add to `crossspacesub_test.go`:

```go
func countersValue(m *pb.EventMessage) (int64, bool) {
	if c := m.GetSubscriptionCounters(); c != nil {
		return c.Total, true
	}
	return 0, false
}

// A per-space counter still queued when its space is removed must not resurrect
// the removed space's total. GO-7337.
func TestPatchEvent_removedSpaceCounterDoesNotResurrectTotal(t *testing.T) {
	ms := mock_subscription.NewMockService(t)
	s := newBareCrossSpaceSub(t, ms)
	s.perSpaceSubscriptions["space1"] = "sub-A"
	s.activeInternalSubs["sub-A"] = struct{}{}
	ms.EXPECT().UnsubscribeAndReturnIds("space1", "sub-A").Return([]string{"obj1", "obj2"}, nil)

	// 1) per-space A's initial counter is still queued (unpatched)
	queuedCounter := event.NewMessage("space1", &pb.EventMessageValueOfSubscriptionCounters{
		SubscriptionCounters: &pb.EventObjectSubscriptionCounters{SubId: "sub-A", Total: 2},
	})

	// 2) space removed: marks sub-A inactive and zeroes its total
	s.RemoveSpace("space1")

	// 3) the stale queued counter is now patched
	s.patchEvent(queuedCounter)

	total, ok := countersValue(queuedCounter)
	require.True(t, ok)
	assert.Equal(t, int64(0), total, "removed space's queued counter must not re-add its total")
	s.lock.Lock()
	assert.Equal(t, int64(0), s.getTotalCount())
	s.lock.Unlock()
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./core/subscription/crossspacesub/ -run TestPatchEvent_removedSpaceCounterDoesNotResurrectTotal -v`
Expected: FAIL — total is `2` (resurrected), and/or `activeInternalSubs` is undefined.

- [ ] **Step 3: Add the `activeInternalSubs` set and gate `updateTotalCount`**

In `crossspacesub.go`:

Add the field to the struct: `activeInternalSubs map[string]struct{}`. Initialize it in both `newCrossSpaceSubscription` (`activeInternalSubs: map[string]struct{}{},`) and `newBareCrossSpaceSub` in the test helper (`activeInternalSubs: map[string]struct{}{},`).

Mark internal subs active where finalized:
- In `newCrossSpaceSubscription`, where loaded spaces finalize (`s.perSpaceSubscriptions[spaceId] = resp.SubId`), add `s.activeInternalSubs[resp.SubId] = struct{}{}` under the same `s.lock`.
- In `completeReservation`, where `s.perSpaceSubscriptions[spaceId] = resp.SubId` (the `ours && !closed` branch), add `s.activeInternalSubs[resp.SubId] = struct{}{}`.

Gate aggregation in `updateTotalCount`:

```go
func (s *crossSpaceSubscription) updateTotalCount(internalSubId string, perSpaceTotal int64) int64 {
	s.lock.Lock()
	defer s.lock.Unlock()

	if internalSubId == s.subId {
		// synthesized counters event (space removal/rollback): already aggregated
		return s.getTotalCount()
	}
	if _, active := s.activeInternalSubs[internalSubId]; !active {
		// counter for a removed/rolled-back per-space sub: ignore so a stale
		// queued counter cannot resurrect the removed space's total (GO-7337)
		return s.getTotalCount()
	}
	s.totalCounts[internalSubId] = perSpaceTotal
	return s.getTotalCount()
}
```

- [ ] **Step 4: Mark inactive on removal**

In `removeSpace`, where the sub is removed, delete from the active set so later queued counters are ignored. After `subId, ok := s.perSpaceSubscriptions[spaceId]` and inside the `if ok {` block (alongside `total := s.removeTotalCount(subId)`):

```go
		delete(s.activeInternalSubs, subId)
		total := s.removeTotalCount(subId)
```

Also handle rollback symmetry: in `rollbackSubscription` the sub was never added to `perSpaceSubscriptions`, so it was never marked active — `updateTotalCount` already ignores its counters. No change needed there; add a one-line comment noting it.

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./core/subscription/crossspacesub/ -run TestPatchEvent_removedSpaceCounterDoesNotResurrectTotal -v`
Expected: PASS.

- [ ] **Step 6: Run the whole package under -race**

Run: `go test ./core/subscription/crossspacesub/ -race`
Expected: PASS (existing counter/total tests still green; the active-set gate does not change live-space totals).

- [ ] **Step 7: Commit**

```bash
git add core/subscription/crossspacesub/crossspacesub.go core/subscription/crossspacesub/crossspacesub_test.go
git commit -m "GO-7337 crossspacesub: ignore counters for removed per-space subs (fix wrong total)"
```

---

## Task 5: Observability + full verification

**Files:**
- Modify: `core/subscription/crossspacesub/crossspacesub.go` (debug log in `runBroadcast` flush)

**Interfaces:** none new.

- [ ] **Step 1: Add a debug log of coalescing effect**

In `runBroadcast`, change the `flush` closure to log batch sizes at debug level (cheap visibility into the redraw win; no new metrics dependency):

```go
	flush := func() {
		for _, batch := range c.ready(s.clk.now()) {
			log.Debug("crossspacesub broadcast flush",
				zap.String("subId", s.subId), zap.Int("messages", len(batch)))
			s.eventSender.Broadcast(&pb.Event{Messages: batch})
		}
	}
```

- [ ] **Step 2: Build and vet the package**

Run: `go build ./core/subscription/... && go vet ./core/subscription/crossspacesub/`
Expected: no output (success).

- [ ] **Step 3: Run the full subscription suite under -race**

Run: `go test ./core/subscription/... -race`
Expected: PASS.

- [ ] **Step 4: Lint the changed files**

Run: `golangci-lint run core/subscription/crossspacesub/...`
Expected: no findings (errcheck/funlen/nestif/prealloc clean — match the existing package style).

- [ ] **Step 5: Commit**

```bash
git add core/subscription/crossspacesub/crossspacesub.go
git commit -m "GO-7334 crossspacesub: debug log coalesced broadcast batch sizes"
```

---

## Post-implementation (not code tasks)

- The three **Open questions** in the spec remain: confirm `ObjectCrossSpaceSearchSubscribe` is startup-hot and how wide; measure promote-wave spacing to validate/tune `defaultWindow`; calibrate `defaultInitialGrace` against client response-apply latency. Capture findings on GO-7334 before tuning the consts.
- Spec reference: `docs/superpowers/specs/2026-06-25-crossspace-broadcast-coalescing-design.md`.

---

## Self-Review

**Spec coverage:**
- Design §0 timing seam → Task 2 (clock.go, fake clock). ✓
- Design §1 window/grace/size-cap (M1, M3) → Task 1 (`push`/`ready`, grace-before-cap, hard chunk bound) + Task 2 (`createdAt` stamped late). ✓
- Design §2 `flushBroadcast` counters last-wins → Task 1 (`coalesceCounters`) + Task 2 (flush). ✓
- Design §3 resubscribe replace (GO-7336) → Task 3. ✓
- Design §4 counter resurrection (GO-7337) → Task 4. ✓
- Design §5 config consts → Task 2 (consts) + `newFixture` zeroing. ✓
- Design §6 observability → Task 5. ✓
- Error handling (drop-on-close single policy) → Task 2 (`runBroadcast`/`drain` return on `ctx.Done`/queue close, buffer dropped). ✓
- Nested path unchanged → Task 2 (`runNested`). ✓
- Testing section items → covered across Task 1 (policy), Task 2 (grace via fake clock, no-regression), Task 3 (resubscribe), Task 4 (counter). ✓

**Placeholder scan:** No TBD/TODO; every code and test step shows complete code and exact run commands with expected output. ✓

**Type consistency:** `clock.now()/after()` used identically in `realClock`, `fakeClock`, and `runBroadcast`. `coalescer` constructor/method names (`newCoalescer`, `push`, `ready`, `nextDeadline`, `coalesceCounters`, `laterOf`) match between Task 1 definition and Task 2 use. `newCrossSpaceSubscription` extended signature matches its `service.Subscribe` call site. `activeInternalSubs` defined (Task 4) and initialized in both constructors. `maxFlushSize`/`defaultInitialGrace`/`defaultWindow` consts used consistently. ✓
