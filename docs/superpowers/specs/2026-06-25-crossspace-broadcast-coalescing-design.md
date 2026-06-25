# Coalesced + grace-delayed broadcast for cross-space subscriptions

Date: 2026-06-25
Status: Implemented (branch go-7334-crossspace-broadcast-coalescing) — revised after a
3-reviewer design pass and a 10-reviewer post-implementation pass (see Post-implementation
review notes at the end)
Issue: GO-7334
Related (pre-existing bugs folded into implementation, also tracked separately):
GO-7336 (resubscribe leak), GO-7337 (counter resurrection)

## Problem

The lazy cross-space subscription work (GO-7288, `801cbbc3e` and follow-ups) made
`crossspacesub.Subscribe` lazy: on subscribe, only spaces whose objectstore is already open
are subscribed synchronously and their records are returned in the **RPC response**; spaces
whose store is not open yet are recorded as pending. As the background warm-up opens each
store, `service.onSpaceIndexOpened → PromotePending` fires an `asyncInit=true` per-space
subscribe whose records flow **as events** through `crossSpaceSubscription.run →
eventSender.Broadcast` (`core/subscription/crossspacesub/crossspacesub.go:113-138`).

### Scope: only the client-facing cross-space subscription broadcasts

This matters and was initially mis-stated. A cross-space subscription only calls
`eventSender.Broadcast` when `internalQueue == nil` (`crossspacesub.go:132-136`). The six
in-process consumers (`pushnotification`, `acl/participantsub`, `filedownloader`, the four
`api/service/cache.go` caches, `block/chats`) all pass an `InternalQueue` and take the
**nested** path — they consume the per-space stream in-process via
`objectsubscription.NewFromQueue` and never broadcast, so they produce **zero client
redraws**. The **only** broadcasting (client-facing) cross-space subscription is
`ObjectCrossSpaceSearchSubscribe` (`core/object.go:247-273`, no `InternalQueue`). This
design therefore affects exactly that one RPC. The other six benefit only indirectly, if at
all, via their own downstream client events — which is out of scope here.

### 1. Subscribe-response vs. event race (the main issue)

The Subscribe response and the live events travel on **two independent delivery paths with
no ordering guarantee between them**:

- the response is the synchronous return value of the `ObjectCrossSpaceSearchSubscribe` RPC;
- events arrive on the long-lived `ListenSessionEvents` stream (`eventSender.Broadcast`).

The client *does* know the subId (it generates and sends it in the request — confirmed:
`pb/protos/service/service.proto` `ObjectCrossSpaceSearchSubscribe`, and `docs/Subscriptions.md`
notes the client ignores the response's subId and uses its own). So this is **not** an
"unknown subId" problem. The problem is that the client processes the *response* (which
carries the initial record set) in one place, and receives *events* for the same subId in
another, and **nothing sequences the two**. An event that the client handles before it has
finished applying the response's initial set corrupts or drops against a not-yet-populated
subscription.

The lazy change widened this window substantially: there is now almost always a burst of
promote events emitted immediately after `Subscribe` returns (one wave per space that opens
during warm-up), so the race fires with high probability on a busy startup rather than
rarely.

The authoritative fix is client-side (buffer events for a subId until its response has been
applied, then drain). That is real, awkward work and is deferred. Since this design adds a
server-side buffer for batching anyway, holding the *first* emission briefly is a cheap,
effective mitigation from the same buffer — there is no reason to fire an event microseconds
after the response.

### 2. Event storm / redraw avalanche on startup

A single `ObjectCrossSpaceSearchSubscribe` spanning N spaces receives one promote wave per
space as stores open during warm-up. Each wave is its own `Broadcast` today, and the client
redraws per delivered payload, so a wide cross-space search produces a redraw burst right
when the app is busiest. (`run` already coalesces a *single* wave's `Add×K + Counters` into
one `Broadcast` via one `s.queue.Wait`; the new coalescing additionally merges waves that
arrive close together — see "Open questions" on how much that buys, given warm-up spreads
opens over seconds.)

### Aggravating finding (out of scope, see Follow-ups)

`GrpcSender.sendEvent` spawns a `go func()` **per event** to call `server.Server.Send`
(`core/event/event_grpc.go:68-82`), and `Broadcast` fans out to every session. Separate
`Broadcast` calls are therefore not ordered with respect to each other on the wire, and
concurrent `Send` on a single gRPC stream is technically unsafe. Emitting fewer, larger
`Broadcast` calls reduces how often this bites, but the real fix (serialize sends per
session) is a separate change.

## Goals

- Collapse temporally-adjacent promote waves of `ObjectCrossSpaceSearchSubscribe` into fewer
  `Broadcast` calls, reducing client redraws on startup.
- Reduce the probability of the response-vs-event race by delaying the first emission for a
  new subId until the client has had time to apply the response — from the same buffer.
- Keep the change isolated to the cross-space broadcast path (`internalQueue == nil`); no
  change to the regular subscription engine, the event sender, or the wire protocol.
- Bounded added latency and bounded `pb.Event` payload size.
- Make the timing fully test-injectable (no real-time sleeps in tests).

## Non-goals

- **A deterministic fix for the race.** This is a server-side *mitigation*. The authoritative
  fix is the client-side event buffer (Follow-up 1). This design only shrinks the window and
  the redraw count for the one broadcasting consumer.
- **General subscription coalescing.** Scope is `ObjectCrossSpaceSearchSubscribe` only. The
  same two-path race exists for regular `ObjectSearchSubscribe` and is **not** addressed here
  — it remains exposed and is another reason the client-side fix is the real solution.
- **Op-aware coalescing** (Add+Remove cancellation, Amend/Set merging). During warm-up the
  storm is overwhelmingly pure `Add`s, so concatenation captures nearly all the benefit. Only
  counters are merged (last-wins). Deeper merging is a possible future refinement.
- **Serializing `GrpcSender` per-session sends** (the aggravating finding above).

## Background: where events are emitted today

`crossSpaceSubscription.run` (`crossspacesub.go:113-138`) consumes the internal queue
`s.queue` that all per-space subscriptions feed into:

```go
func (s *crossSpaceSubscription) run(internalQueue *mb.MB[*pb.EventMessage]) {
    for {
        msgs, err := s.queue.Wait(s.ctx)
        // ... ctx.Canceled -> return; other err -> log ...
        for _, msg := range msgs {
            s.patchEvent(msg)                 // rewrite per-space subId -> s.subId
            if internalQueue != nil {
                internalQueue.Add(s.ctx, msg) // nested sub: forward per-message, no broadcast
            }
        }
        if internalQueue == nil {
            s.eventSender.Broadcast(&pb.Event{Messages: msgs}) // client-facing path
        }
    }
}
```

- `s.queue` is `mb.MB[*pb.EventMessage]` (cheggaaa/mb v3.0.2; `go.mod:23`).
- `patchEvent` (`crossspacesub.go:140-172`) rewrites every message's per-space internal subId
  to the unified `s.subId`, and routes counters through `updateTotalCount` so each emitted
  `SubscriptionCounters` carries the aggregate absolute total for `s.subId`.
- The broadcast path is taken **only when `internalQueue == nil`**. The nested path
  (`internalQueue != nil`) must remain per-message and is untouched by this change.
- All events for the subscription funnel through `s.queue → run` (verified: the only
  `Broadcast` in the package is `crossspacesub.go:133`; `removeSpace` and
  `rollbackSubscription` only `s.queue.Add`). So coalescing in `run` covers every event path.

### Relevant `mb` v3 semantics (verified against v3.0.2 source)

- `MB.WaitCond(ctx, cond)` returns immediately with whatever is buffered (subject to `cond`),
  else blocks until messages arrive, the queue closes, or `ctx` is done.
- A standard `context.WithDeadline`/`WithTimeout` child works: on deadline `WaitCond` returns
  `(nil, context.DeadlineExceeded)` and **leaves buffered messages intact** — `releaseWaiter`
  (mb.go:259-265) prepends any delivered-but-unconsumed message back to `buf`, preserving
  FIFO. No event loss across the deadline.
- `NewCond().WithMax(n)` caps the messages a single `Wait` returns. `WaitCond` is a value type
  and `WithMax`/`WithMin` return copies, so a `cond` value is safe to reuse and re-derive
  across iterations. Default `Min<1` is promoted to 1, so a `Wait` with messages buffered
  returns them immediately.
- **Do not** use `mb.CtxWithTimeLimit`: when its timer fires on an empty buffer it resets the
  condition's `Min` to 1 and re-arms, so after one idle tick it stops coalescing. A
  self-managed deadline avoids this.

## Design

### 0. Timing seam (so tests are deterministic) — addresses review M5

`time.Until` and `context.WithTimeout` use the real wall clock; a fake `now()` alone controls
nothing. So the subscription takes an injected timing seam, set at construction, **stored as a
field (never a package var** — a package var would be read by the `run` goroutine while a test
writes it, a `-race` failure):

```go
type clock interface {
    now() time.Time
    // after returns a channel that fires once `d` has elapsed; tests drive it manually.
    after(d time.Duration) <-chan time.Time
}
```

The real implementation wraps `time.Now` / `time.After`. Tests inject a manual clock and a
fake `event.Sender` (the capturing `mock_event.MockSender` already used in
`crossspacesub` tests). `initialGrace` and `window` are **fields** on the subscription
(defaulted from the package consts, overridable in tests to ~0) rather than direct constant
reads, so existing exact-sequence tests are not slowed by 200ms or merged unexpectedly.

The coalescing loop blocks on `s.queue` for the first message, then selects between more
queue messages and the injected timer — instead of `context.WithTimeout` per iteration
(review m2) — so the deadline fires through the same injectable seam:

```go
firstFlush := true
for {
    first, err := s.queue.WaitOne(s.ctx)   // block for the first msg of a wave
    if err != nil {                         // ErrClosed / ctx.Canceled
        return                              // drop any (none here) and stop — see error handling
    }
    buf := s.patchAll([]*pb.EventMessage{first})

    var deadline time.Time
    if firstFlush {
        deadline = laterOf(s.createdAt.Add(s.initialGrace), s.clk.now().Add(s.window))
    } else {
        deadline = s.clk.now().Add(s.window)
    }
    timer := s.clk.after(deadline.Sub(s.clk.now()))

    for {
        graceUnmet := firstFlush && s.clk.now().Before(deadline)
        full := len(buf) >= maxFlushSize
        if full && !graceUnmet {
            break // size cap (grace satisfied, or a non-first flush)
        }
        if full { // buffer full but first-flush grace not yet met: hold, drain nothing
            select {
            case <-timer:        // grace elapsed -> flush this maxFlushSize batch
            case <-s.ctx.Done(): // sub closed -> drop and stop
                return
            }
            break
        }
        // WithMax(room) keeps buf <= maxFlushSize -> hard per-Broadcast bound (review m1)
        cond := s.queue.NewCond().WithMax(maxFlushSize - len(buf))
        more, werr := s.queue.WaitCond(ctxWithTimer(s.ctx, timer), cond)
        buf = append(buf, s.patchAll(more)...)
        if werr != nil {                    // timer fired or sub closed
            if s.ctx.Err() != nil {         // sub closed mid-window: drop and stop
                return
            }
            break                           // deadline reached: flush
        }
    }

    s.flushBroadcast(buf)
    firstFlush = false
}
```

`ctxWithTimer` is a tiny helper that yields a context cancelled either by `s.ctx` or by the
injected `timer` firing (so `WaitCond` returns `DeadlineExceeded`-equivalent through the seam).
`patchAll` applies the existing `patchEvent` to each message (subId rewrite + counter
aggregation happen exactly as today, before buffering). The nested `internalQueue != nil`
branch keeps the existing per-message forward loop verbatim — coalescing applies only to the
`internalQueue == nil` broadcast path (review #9).

### 1. Window and grace semantics — addresses review M1, M3

- **Steady-state window.** Each flush coalesces at most `window` of arrivals measured from the
  first buffered message, so promote waves that land close together merge. A delivered event
  waits at most `window`.
- **Size cap is a hard bound and never pre-empts the first grace.** `WithMax(maxFlushSize -
  len(buf))` clamps each `Wait` so `buf` never exceeds `maxFlushSize` (fixes the ~2× payload
  bound, review m1). On the **first** flush the size cap is suppressed until the grace deadline
  passes, so a large first wave (≥`maxFlushSize`) is still held for the grace and cannot fire
  ungraced (fixes review M1 — the original draft flushed the first 500 immediately and
  consumed `firstFlush`, defeating the mitigation in exactly the heavy case).
- **Initial grace (race mitigation).** The first flush is held until
  `max(createdAt + initialGrace, firstMsg + window)`, applied once per subscription.
- **Grace anchor (review M3).** `createdAt` is stamped as **late as the server can** — right
  before `Subscribe` returns the response / when `run` starts — not in
  `newCrossSpaceSubscription` (which precedes the synchronous loaded-space subscribes, the RPC
  marshal, the wire hop, and client deserialize). The server still cannot observe when the
  client actually applied the response, so `initialGrace` is a heuristic; its value must be
  calibrated against measured response-apply latency under startup load (see Open questions).

### 2. `flushBroadcast` — coalescing depth

```go
func (s *crossSpaceSubscription) flushBroadcast(buf []*pb.EventMessage) {
    if len(buf) == 0 { return }
    msgs := coalesceCounters(buf) // keep only the last SubscriptionCounters, at the tail
    s.eventSender.Broadcast(&pb.Event{Messages: msgs})
}
```

- **Concatenation in arrival order** for all kinds except counters (FIFO preserved; review
  confirmed ordering is safe and `patchEvent`'s in-place mutation does not alias another
  subscriber — each message is a fresh allocation routed to a single destination).
- **Counters last-wins.** After `patchEvent`, every `SubscriptionCounters` carries `s.subId`
  and an absolute aggregate total, so intermediate counters in a flush are redundant; keep
  only the last, appended at the tail.
  - Caveat (review M8, transient): when a flush is cut by the size cap after trailing `Add`s
    whose follow-on counter lands in the next flush, the kept counter can momentarily predate
    those adds. Self-corrects on the next flush; acceptable.
  - **Correctness depends on the GO-7337 fix** (below). Without it, counters last-wins can
    surface a *permanently* wrong total — not a coalescing artifact (the bug is at patch time,
    which batching does not change), but this design must not claim counter safety until
    GO-7337 lands. The two are folded into the same implementation.

### 3. Folded fix — resubscribe replace (GO-7336, review M7)

`service.Subscribe` currently does `s.subscriptions[req.SubId] = spaceSub` with no lookup, so
a resubscribe with an existing subId orphans the previous `crossSpaceSubscription`: its `run`
goroutine keeps broadcasting under the same subId, its `s.queue` is never closed, and its
per-space subs are never unsubscribed (leak). Resubscribe is a supported flow and is exactly
when the race/grace recurs.

Fix: in `Subscribe`, if `req.SubId` already exists, `close()` and remove the existing sub
before creating the new one (mirroring the engine's replace-on-resubscribe). Done under
`s.lock`; `crossSpaceSubscription.close()` does not acquire `s.lock`, so no deadlock. The new
sub gets a fresh `createdAt`, so the grace correctly re-applies to the new generation.

### 4. Folded fix — counter resurrection (GO-7337, review M2)

A removed space's *queued* per-space counter, patched after `removeSpace` already ran, calls
`updateTotalCount` (`crossspacesub.go:411`) and **re-inserts** the removed internal subId's
total that `removeTotalCount` (`:383`) just deleted. Interleaving: space A emits
`Add A1, Add A2, Counters(subId_A, 2)` into `s.queue`; `RemoveSpace(A)` enqueues `Remove A1,
Remove A2, Counters(s.subId, 0)`; `run` patches in queue order, re-inserting `subId_A=2`, so
the surviving last-wins counter reads **2 against 0 records**. Permanent wrong count.

Fix: aggregation must ignore counters for internal subIds that are no longer active. Track the
set of active internal subIds (or gate on `perSpaceSubscriptions` membership); `removeSpace`
marks the internal subId inactive so its later queued counters are dropped in `patchEvent`.
Implementation must handle the ordering subtlety where queued `Add`s for a just-removed space
are still ahead in the queue (they are already covered by the `Remove` ids
`UnsubscribeAndReturnIds` returns); this fix carries its own tests.

### 5. Configuration

Package-level defaults, copied into per-sub fields at construction (so tests can override):

```go
const (
    defaultInitialGrace = 200 * time.Millisecond // first-flush hold after createdAt
    defaultWindow       = 50 * time.Millisecond  // steady-state coalescing window (tune; see Open questions)
    maxFlushSize        = 500                     // hard cap on messages per Broadcast
)
```

Consistent with the package's no-per-request-tuning stance (`service.go:147-168` rejects
limit/sorts/pagination/etc.); not runtime-tunable, so a bad value ships in the binary —
hence the calibration note.

### 6. Observability — addresses review m4

Add debug-level counters: flushes emitted, messages per flush (avg/max), and grace-delayed
first-flush count, so the redraw-reduction win can be confirmed in production. `mb.Stats()`
(`mb.go:404`) is available for the queue side.

## Data flow

```
ObjectCrossSpaceSearchSubscribe(req)
   └─ loaded-space records ──▶ RPC response (synchronous)
   └─ createdAt = now()  (stamped just before response returns)
   └─ go run()  (broadcast path, internalQueue == nil)
         queue.WaitOne(ctx)                  ── blocks for the first promote/live event
         ── first wave ──▶ buf; deadline = max(createdAt+grace, firstMsg+window)
         WaitCond(timer, WithMax(cap-len)) loop  ── coalesce until deadline; size cap honored only after grace
         flushBroadcast(buf)                 ── one Broadcast (counters last-wins)
         ── repeat with window-only deadline ──

warm-up opens stores over seconds (bounded concurrency)
   └─ onSpaceIndexOpened → PromotePending → asyncInit per-space subscribe
         records ──▶ s.queue ──▶ coalesced into the flushes above
```

## Error handling

- `s.queue.WaitOne(s.ctx)` returning `ErrClosed`/`context.Canceled` (Unsubscribe/Close): `run`
  returns; buffer dropped. **Drop-on-close is the single consistent policy** for both the
  outer and inner cancel paths (fixes review M3 flush-vs-drop contradiction — the client has
  unsubscribed, so broadcasting is pointless).
- Inner `WaitCond` timer firing: window elapsed, flush and continue.
- Inner `WaitCond` error while `s.ctx` is done: drop and return.
- Preserve a log for any unexpected (non-`ErrClosed`/non-`Canceled`) error to keep today's
  diagnostic (review m7).
- `RemoveSpace`/rollback synthetic Remove + Counters flow through `s.queue` and are batched
  normally; counter correctness relies on the GO-7337 fix.

## Testing

Manual clock + capturing fake `event.Sender`; no real-time sleeps.

- **Storm coalescing:** several promote waves within `window` → a single `Broadcast`,
  arrival order preserved.
- **Initial grace, small wave:** events enqueued right after subscribe → no `Broadcast`
  before `createdAt + initialGrace`; first `Broadcast` carries all grace-window events.
- **Initial grace, large wave (review M1 regression):** ≥`maxFlushSize` enqueued immediately
  → still no `Broadcast` before the grace deadline (size cap suppressed during first grace).
- **Late first event:** first event after `createdAt + initialGrace` → flush after only
  `window`.
- **Hard size cap (review m1):** `buf` never exceeds `maxFlushSize` in a single `Broadcast`.
- **Counters last-wins:** a flush with multiple counters emits exactly one (latest total) at
  the tail.
- **Counter resurrection (GO-7337):** space added then removed across a window → final
  counter matches record count (regression test).
- **Resubscribe (GO-7336):** resubscribe with an existing subId closes the old sub (old
  `run` exits, old per-space subs unsubscribed, no leak) before the new one starts.
- **Close mid-window:** `close()` during an active window stops `run` without panic and drops
  the buffer.
- **Nested path unchanged:** `internalQueue != nil` still forwards per-message; nothing
  broadcast.
- Existing `crossspacesub` and subscription suites green under `-race` (per-sub grace/window
  set to ~0 so exact-sequence tests are unaffected).

## Open questions (measure before/with implementation)

1. **Is `ObjectCrossSpaceSearchSubscribe` startup-hot, and how wide?** The benefit is real
   only if the client opens broadcasting cross-space searches during the warm-up storm, over
   many spaces. Confirm volume.
2. **Promote arrival spacing.** Warm-up spreads opens over seconds (bounded concurrency ~4,
   ~78 spaces per the startup profile). If waves land >`window` apart, the new cross-wave
   coalescing buys little beyond what `run` already does per wave — `window` may need to be
   250–500 ms, or the storm-reduction gain is marginal (the race mitigation still stands).
3. **`initialGrace` calibration.** Measure client response-apply latency under startup load;
   200 ms is a starting guess, not a measured value.

## Follow-ups (tracked, out of scope here)

1. Client-side deterministic fix: buffer events for a subId until its response is applied,
   then drain. The authoritative fix; this server mitigation is interim.
2. Serialize `GrpcSender` per-session sends (remove the per-event `go func`) to fix the
   event-ordering / concurrent-`Send` hazard.
3. Optional op-aware coalescing (Add+Remove cancellation, Amend/Set merge) if profiling shows
   redundant churn beyond counters after this ships.
4. The same two-path race for regular `ObjectSearchSubscribe` is unaddressed; the client-side
   buffer (1) covers it generally.

## Post-implementation review notes (2026-06-25)

After implementation, a 10-agent adversarial review ran over the branch diff. The policy,
ordering, hard per-Broadcast bound, lifecycle (no leak/deadlock), single-goroutine patching,
and nested-path equivalence were all confirmed correct. Two real defects and several
robustness/clarity items were found and fixed; a few are documented as accepted tradeoffs.

### Defects found and fixed
- **Undercount (GO-7337 follow-up).** The `activeInternalSubs` gate dropped the async-init
  counter on the AddSpace/PromotePending path, because the internal subId was marked active
  *after* `subscribe()` returns while the snapshot counter is delivered asynchronously. Fix:
  `completeReservation` records the per-space total synchronously (idempotent, last-wins),
  mirroring the loaded-space path.
- **`close()` per-space leak (GO-7336 follow-up).** `close()` only cancelled ctx + closed the
  queue; the per-space subs use a caller-provided queue, so they were never released by the
  engine (slots, pinned objects, per-change CPU) — a pre-existing leak now hit on every
  resubscribe. Fix: `close()` joins the run goroutine (via `started`/`done`, so no stale tail
  broadcasts under the reused subId) and calls `UnsubscribeAndReturnIds` for each finalized
  per-space sub. `Subscribe` builds the new sub before closing the old one, so a failed rebuild
  no longer destroys the existing subscription.

### Clarifications to this spec
- **Size cap is a per-Broadcast hard bound, not a buffer bound.** §1/§2's "buf never exceeds
  maxFlushSize" is imprecise: during the initial grace the buffer deliberately holds everything
  (grace must not be pre-empted), so `coalescer.buf` may exceed `maxFlushSize`; what is bounded
  is each emitted `pb.Event` (≤ `maxFlushSize`), enforced by chunking in `ready()`.
- **Counters last-wins is per emitted chunk.** A flush split across multiple Broadcasts carries
  at most one counters message per chunk; each is self-consistent and the latest total wins
  across the ordered Broadcasts.
- **Observability (§6) is intentionally minimal:** a per-flush `log.Debug` of batch size, not
  the aggregate counters originally sketched.

### Accepted tradeoffs (not changed)
- **Transient buffer retention during grace.** A broad cold-start search can hold a multi-MB
  burst for up to `initialGrace` before the first flush. Not capped on purpose: capping during
  grace would re-expose the response-vs-event race for those records (which would ship to the
  client anyway). The spike is freed at the grace flush.
- **GrpcSender chunking.** A >`maxFlushSize` flush is emitted as N sequential `Broadcast`s,
  each a separate (unordered) `Send` on the stream — amplifying the pre-existing per-event
  `go func` hazard. Bounded and self-correcting because cross-space results are unsorted and
  the counter converges. The real fix (serialize per-session sends) remains Follow-up 2.

### Open question still outstanding
- **Window value (`defaultWindow=50ms`).** Warm-up opens spaces seconds apart, so cross-wave
  coalescing — and thus redraw reduction — is marginal at 50ms; the load-bearing wins are the
  grace (race mitigation) and the bounded payload. Measure promote-wave spacing and client
  redraw counts before raising the window or claiming a redraw reduction.
