# Subscription engine: design

Design for the from-scratch reimplementation of `core/subscription`. The behavioral contract
is [Subscriptions.md](Subscriptions.md) (§3 = requirements, §4 = do-not-build, §5 = checklist);
this document specifies *how* the engine fulfills it: architecture, data structures, event
pipeline, and the memory/concurrency model. Revised after independent architecture and
memory reviews; review-driven decisions are marked *(rev)*.

## 1. Design principle: one core primitive, an adapter ring around it

Most of the subscription surface is not live-query logic — it is Anytype-specific request
shaping (sources, collections, explicit id lists) and derived outputs (dependencies, kanban
groups, cross-space fan-out). The engine keeps those out of the core:

```
RPC / Go API
  → normalize    validation, subId, Source→filters, snapshot truncation
  → orchestrate  deps, groups, collections, async-init        ← adapters, one file each
  → core         per-space live query: scope + filters + order/window → transition stream
  → encode       neutral transitions → pb.EventMessage (wire-identical), off-mutex
  → route        Broadcast (client) | mb queue (Internal) | Go callback (adapters)
```

The **core primitive** is a single subscription shape:

> `{optional id-scope, compiled filters, optional order+window, requested keys}` over one
> space → initial snapshot + a stream of minimal transitions
> *(enter, leave, move, change-keys, total-changed)*.

Everything else is expressed through it:

| Surface | Expressed as |
|---|---|
| client search sub | ordered sub (order + window from sorts/limit/offset) |
| internal sub (§3.9) | unordered sub (no order ⇒ set semantics, whole set visible) |
| `SubscribeIds` | sub with a **fixed id scope** and no filters |
| `CollectionId` | sub with a **mutable id scope** (collection stream) + filters |
| `{subId}/dep` | child sub with a **derived mutable scope**, detail-events-only flag |
| groups (kanban) | adapter over a hidden unordered sub; core knows no groups |
| cross-space | existing `crossspacesub` layered on the public API (unchanged) |

Membership predicate of the core: `(scope == nil || id ∈ scope) && filter.FilterObject(details)`.

`crossspacesub` and `objectsubscription` stay as-is on top of the public API; they are
acceptance consumers, not engine parts.

## 2. What the core builds on

The store (`pkg/lib/localstore/objectstore/spaceindex`) provides exactly two primitives:

- **Snapshot**: `Query`/`QueryAndCount`/`QueryByIds` with filters/sorts compiled by
  `database.NewFilters` — which also owns default-filter injection (`isArchived`/`isDeleted`/
  `type`) with the documented `Condition: None` opt-out, quickOption date resolution, nested
  dot-keys, And/Or trees, and in-memory evaluation (`Filter.FilterObject(details)`,
  `Order.Compare(a, b)`). The engine compiles requests once through this same path for both
  snapshot and live matching, so the two can never disagree. *(Footnote: compiled filters are
  frozen — nested dot-key id sets and quickOption dates resolve at compile time and go stale
  together until re-subscribe. §4 forbids nested sub-subscriptions, so this is the accepted
  semantic.)*
- **Feed**: `SpaceIndex.SubscribeForAll(cb)` — a single per-space callback slot carrying the
  object's **full new details** (no prev/new pair). The engine is the only consumer. Hard
  deletes (`DeleteObject`) arrive as tombstone details (`isDeleted: true`) through the same
  feed; `DeleteDetails` (indexer-only) does **not** notify — a documented blind spot shared
  with reindex flows.

**Store prerequisite *(rev, was blocker B1)***: today the feed fires post-commit only for
plain `UpdateObjectDetails`. For tx-bearing writes (`ModifyObjectDetailsCtx` under `WriteTx`,
`DeleteObject`'s internal tx) the callback fires **before commit**, so a write could be
neither in the engine's wiring-then-snapshot window nor ever notified — silent data loss —
and a rolled-back tx emits phantom events. Phase 1 includes a store-side fix: notifications
raised inside a tx are buffered in the tx and flushed only on successful commit (the
`WriteTx` handle is wrapped; rollback discards). The §6.1 no-data-loss argument is stated
against this fixed semantics.

## 3. Core data structures and memory model

The memory rule (contract §3.10): full details are kept **only for objects a subscription
actually renders** — its *visible* members. Out-of-window members cost an id-set entry and
nothing more *(rev)*. `counters.total` is the member count, never derived from retained
details.

```go
type coreSub struct {
    subId, spaceId string
    scope   *idScope          // nil, or mutable id whitelist
    filter  database.Filter   // compiled once via database.NewFilters
    order   database.Order    // nil ⇒ unordered (set semantics)
    window  window            // offset/limit; limit 0 = unbounded
    keys    []domain.RelationKey

    members map[string]struct{} // full matching set: ids only (membership + total)
    visible *windowState        // ordered subs: sorted window entries; nil when unordered
    before  int                 // ordered subs with offset: members ranked before window
    detailEventsOnly bool       // dep subs: suppress Add/Remove/Position/Counters
    delivery deliveryTarget     // broadcast | queue | Go callback
}

// windowState: only the window (≤ limit, or everything when limit 0) is ordered.
type winEntry struct {
    id       string
    sortVals *domain.Details // sort keys only; nil when the order is the bare id tiebreak
    prev     *domain.Details // projection to requested keys (event diffing)
}
```

**Window-only ordering *(rev, memory review F1/F5/F9)***: the design deliberately does not
keep sort values or a global sorted list for the full matching set. Out-of-window sort
values are never needed: *total* is the size of `members`; *enter/leave* is a filter eval +
set lookup; *does an update cross the window?* is a comparison of the updated object's sort
values (available — the feed carries full details) against the window boundary entries;
*window underflow* (a member left/moved out of the window) is repaired by a windowed
re-query with margin — the same store read §5.2 already needs for slide-ins. `offset > 0`
(one cold client path) is served by the `before` counter, maintained by the same boundary
comparisons. A windowed sub over 100k matching objects costs ~the id set (≈10 MB) plus a
≤500-entry window instead of ~35 MB of per-member sort state. For `limit 0` subs the window
is the whole set — full details are then exactly what the client renders, which is the
contract's own floor. Unordered subs hold `members` plus per-member `prev` projections
(every member is visible).

State is **per-sub private**. No shared refcounted cache: sharing would save memory only
when many subs track the same object with overlapping key sets, which real usage barely has.
Private state makes ownership trivial — teardown is dropping the struct — and removes all
cross-sub invariants.

**Invariant *(rev)*: a `*domain.Details` is frozen after construction** — `prev` projections
double as event-payload sources, so state updates replace pointers, never mutate maps.

Per-sub compiled filters/orders own their `anyenc.Arena` and `collate.Buffer` (KeyOrder
buffers are not safe for concurrent use; all evaluation for one sub happens on its space
worker or under its space mutex).

## 4. Event pipeline

### 4.1 Feed intake

Feed callbacks run on writer goroutines and must not block or take the space mutex. The
callback appends `(id, details)` to a **coalescing queue** (map by id — a newer full-details
snapshot replaces a pending one, which is safe precisely because the feed carries full
state) and signals the worker.

**Bounded retention *(rev, F3)***: above a threshold of pending entries (~2k) the queue
degrades to **id-only**: details pointers are dropped and the worker re-fetches via one
batched `QueryByIds` per drain. Diff-based processing makes the re-fetch safe (same or newer
state); an id missing on re-fetch means hard-deleted → treated as a leave for every tracking
sub. This caps storm retention at ~threshold × details-size regardless of how many objects
an initial index touches. The map is replaced (not reused) after large batches so bucket
memory is returned.

### 4.2 Worker batch processing

One worker goroutine per space. Drain intake → lock the space mutex → apply the batch to
every sub → append resulting transitions to the space **outbox** → unlock → encode and
route. Per update `(id, newDetails)` and per sub:

1. `match := (scope ∋ id) && filter.FilterObject(newDetails)` — cheap; §9 makes
   "allocation-free on the miss path" a benchmark-enforced gate, with the two known
   offenders in `pkg/lib/database` (FilterLike's per-eval lowering, FilterOptionsEqual's
   `lo.Filter`) fixed upstream *(rev, F4)*.
2. Transition: enter / leave / stay (membership = `members` set). Ordered subs decide
   window relevance by comparing against the window boundary; window entries are located by
   binary search (`order.Compare` over ≤window-size entries; the id tiebreak makes it total).
3. For *visible* members that stay: a **single-pass `diffProject(prev, new, keys)`** *(rev,
   F8)* — iterate the requested keys once, compare against `prev`, allocate the diff and the
   new projection lazily on first difference. Empty diff ⇒ no event and no allocation; this
   is also what makes snapshot/feed overlap idempotent (§6.1).

After the batch each touched sub finalizes: ordered subs repair the window (boundary
bookkeeping; margin re-query on underflow) and emit the **window-diff script** (§5.2); any
sub whose total changed appends one `Counters`. Adapter `afterBatch` hooks run under the
mutex but only mutate adapter state or scopes (§7); heavier adapter work (groups recompute)
is deferred to a post-unlock dirty pass on the same worker.

### 4.3 Outbox: ordered, encoded off-mutex *(rev, F6)*

All emission — worker batches *and* subscribe-time AsyncInit snapshots — goes through a
per-space FIFO outbox: neutral transition records appended under the mutex (cheap, no pb),
encoded and delivered by the worker after release, grouped so one logical batch becomes one
`pb.Event` for broadcast and an in-order message run for queues. FIFO order guarantees
AsyncInit snapshot events always precede later feed events for the same sub, while pb
encoding (`ToProto` per record) never happens under the mutex.

### 4.4 Scope mutations

`setScope(ids)` (collections stream, dep-set recompute, ids-sub) runs through the same
worker path as a synthetic batch item: scope-diff produces enter/leave transitions; details
for entering objects come from `QueryByIds`. One code path for "membership changed", no
matter the cause.

## 5. Event semantics

### 5.1 Emission rules

Pinned by the client contract and by the crossspacesub/objectsubscription acceptance tests
(which assert exact sequences):

| Transition | Events, in order |
|---|---|
| object becomes visible | `ObjectDetailsSet{id, details(projected), subIds}` then `SubscriptionAdd{id, afterId, subId}` |
| object stops being visible | `SubscriptionRemove{id, subId}` |
| visible member changes details | `ObjectDetailsAmend{changed keys ∩ requested}` and/or `ObjectDetailsUnset{absent keys ∩ requested}` |
| visible member moves (ordered) | `SubscriptionPosition{id, afterId, subId}` |
| total changed | one `SubscriptionCounters{total, subId}` per batch — always last |

- `afterId`: `""` = head; otherwise the predecessor id *in the client's list at application
  time*. The client applies membership events in arrival order (its payload sort puts
  Add/Remove/Position first, details second, counters last), and the window-diff script
  guarantees every referenced `afterId` is already in place. An `afterId` absent from the
  client list corrupts it — hard invariant, unit-tested. Unordered subs always use `""`.
- **Amend minimality is a hard guarantee**: a key appears only if its value differs; no
  event if the projected diff is empty.
- Wire details pinned by the acceptance tests *(rev, S7)*: every `pb.EventMessage` carries
  the sub's `SpaceId`; `Counters` on internal queues carries the engine-generated per-space
  subId (crossspacesub keys its aggregation on it before patching); internal subs **do**
  receive live Counters on every membership change; the `SubscribeResponse` always has
  non-empty `SubId` and non-nil `Counters` (crossspacesub dereferences both).
- Dep subs emit **only** Set/Amend/Unset (`detailEventsOnly`) — membership events for
  `/dep` are dead surface (contract §4).
- One message carries one subId initially. Payload multiplexing across subs (`subIds[]`)
  is a router-level bandwidth optimization to add later if measured — the proto field stays
  `repeated`, the client already demultiplexes.
- Replace-on-resubscribe can let an in-flight batch for the old generation reach the client
  after the new response; the client's clear-and-replace semantics absorb it. Accepted,
  same-as-before behavior *(rev, N2)*.

### 5.2 Window maintenance and diff script (ordered subs)

Window = the ordered `winEntry` run; `limit 0` = unbounded (window is the whole set).
Updates interact with the window through boundary comparisons *(rev)*:

- entering member sorts before the window end (or window not full) → insert, possibly
  pushing the last entry out (its id stays in `members`);
- leaving/moving-out member → remove; **underflow repair**: re-query the store for the
  window range plus margin to pull successors in (the only store read on the hot path,
  bounded by window size; also used to resync after order-map changes, §7.1);
- member of the window with changed sort values → reposition by binary search;
- out-of-window member whose update doesn't cross the boundary → id-set bookkeeping only.

After the batch, compare `oldWindow`/`newWindow` id sequences and emit a script that
replays correctly on the client: simulate the client list starting from `oldWindow`; emit
`Remove` for old∖new; walk `newWindow` left→right keeping the previous id: missing →
`DetailsSet` + `Add{afterId: prev}`; present but out of relative order →
`Position{afterId: prev}`. O(W²) worst case on ≤500-element windows, O(diff) typical.
Slide-in/slide-out caused by *other* objects' changes falls out of the same diff
(contract §3.3, server-side window).

## 6. Lifecycle

### 6.1 Subscribe (`Search`)

1. **Normalize** (`normalize.go`): validate, generate subId if empty (bson id), ignore dead
   AfterId/BeforeId cursors, resolve `Source` — each entry tried as object-type **id or
   unique key**, else as relation id/key *(rev, S5: the JSON API sends `ot-…` unique keys)* —
   into `type In [...]` / OR-ed `relationKey NotEmpty` filters; decide ordered vs unordered:
   ordered iff the request has sorts or is a client (non-Internal) request; an Internal
   request with `Limit`/`Offset` (objectgraph) stays unordered and gets its *response*
   truncated by offset+limit only. Client requests without sorts get the bare id-tiebreak
   order with `sortVals = nil` — no per-member sort state *(rev, F9)*.
2. Resolve `idx := store.SpaceIndex(spaceId)` **before any engine lock** (§8).
3. Get/create the spaceState; creation wires the feed callback once.
4. Compile filters/orders (`database.NewFilters`); orchestrators prepare scopes
   (collection initial ids, dep keys).
5. **subId claim** *(rev, S3)*: under the registry write lock the subId slot is atomically
   claimed with a per-call placeholder, popping any previous owner; the old sub (any space)
   is then torn down silently — no Remove events; the response supersedes the client's
   list. On finalize, the new sub installs into the registry only if its own placeholder is
   still there; otherwise a concurrent Search won the slot and the loser tears down its
   freshly built sub (dep child and watchers included). `Unsubscribe` racing a claim cancels
   the placeholder, which the builder detects on finalize. The slot is the single
   serialization point — no cross-space double-locking.
6. Under the space mutex: install the sub, run the snapshot query (`QueryAndCount` for
   windowed, `Query` otherwise, `QueryByIds` for scoped), initialize members + window +
   visible `prev` projections, take total.
   Holding the mutex across the query is the no-data-loss invariant ("wire SubscribeForAll,
   then re-query", now guaranteed post-commit by the §2 store fix): the feed is wired before
   the query; concurrent writes either land in the query result or sit in the intake queue;
   diff-based processing (empty diff ⇒ no event, §4.2) makes the overlap idempotent — no
   lost events, no duplicates.
7. Respond: records projected to requested keys (window only), dependencies (dep adapter),
   `Counters{Total}`, and for `Internal: true` the queue (caller-provided `InternalQueue`
   or a fresh engine-owned `mb.MB`).
8. `AsyncInit: true` (crossspacesub promotion): no records in the response; the snapshot is
   appended to the outbox under the mutex (encoded off-mutex, §4.3) — per member
   `DetailsSet + Add("")`, one `Counters`. **An empty snapshot emits nothing** — no
   `Counters{0}` — matching the pinned promotion tests *(rev, S2)*.

### 6.2 Unsubscribe / teardown

`Unsubscribe(subIds...)` detaches subs and **cascades to their hidden attachments** (dep
child subs, collection watchers via `UnsubscribeFromCollection`, groups state); hidden subs
(`{subId}/dep`, groups internals) are not directly addressable and are excluded from
`SubscriptionIDs()` *(rev, N3)*. Unknown ids are ignored (consumers unsubscribe liberally
during shutdown). Engine-owned queues are closed; caller-provided ones never.
`UnsubscribeAndReturnIds(spaceId, subId)` additionally returns the member id set —
crossspacesub synthesizes Remove events from it when a space leaves. When a space's last
sub is removed, the spaceState unhooks the feed (`SubscribeForAll(nil)`), stops its worker,
and is dropped *(rev, N5/F11)*.

Space deletion (`DeleteSpaceIndex`) and indexer `DeleteDetails` give the engine no signal;
subs on a deleted space go quiet until their owners unsubscribe (crossspacesub handles this
via spaceview removal; client spaces disappear from the UI with the space). Documented
blind spot, same as before the rewrite *(rev, S6)*.

Dead surface dropped from the interface: `SubscribeIds(subId, ids)` (stub, zero callers)
and `UnsubscribeAll` (zero callers) are removed; `mock_subscription` is regenerated.
`SubscriptionIDs()` stays (registry key dump).

### 6.3 Component wiring

`Init` resolves `objectstore.ObjectStore` (by CName) and `event.Sender`, `kanban.Service`,
`CollectionService` by **non-panicking by-type scan** *(rev, S1)* — the acceptance fixtures
register mocks under arbitrary component names, and the objectsubscription fixture registers
no kanban/collection at all, so `app.MustComponent[T]` would panic at Init. Absence of a
soft dependency fails only the requests that need it (groups / collections). `Run` is a
no-op (spaceStates are lazy); `Close` unhooks feeds, stops workers, then closes engine-owned
queues.

## 7. Adapters

### 7.1 Dependencies (`deps.go`) — client subs with `NoDepSubscription: false`

Two separable concerns *(rev, B2)*:

**Render deps** (what the client needs for cells/covers): dep keys = requested keys whose
relation format is object/file (`GetRelationFormatByKey`); dep id set = values of dep keys
across the parent's *visible* members ∪ object ids in filter values of object-format
relations. The adapter owns a child core sub under `{subId}/dep`: derived scope, no filters,
parent's keys, `detailEventsOnly`, parent's delivery. Response `Dependencies` = the child's
snapshot. After each parent batch (`afterBatch` hook) the adapter recomputes the dep set
from changed members and calls `setScope` — new dep ids produce `DetailsSet` under the dep
subId via the normal scope-mutation path.

**Order deps** (what keeps the sort correct): orders over object/file/tag/status relations
compare via an id→text `OrderMap` (`database.KeyOrder`) — and the OrderMap already knows
exactly which target objects and relation options it depends on. So instead of a separate
reverse index, every feed item is simply offered to the sub's compiled order via
`Order.UpdateOrderMap([item])` — one map lookup for irrelevant items, a no-op for orders
without maps *(as built; simpler than the reviewed design)*. A reported change means the
comparator shifted under the window: rebuild via the §5.2 re-query, which re-sorts with the
updated map (the fresh store query also catches boundary crossings caused by out-of-window
members' dep changes) and emits the window diff. The contract's §3.5 case — a renamed
assignee visibly mis-sorting the list — is covered including boundary crossings.

The core knows nothing about relation formats.

### 7.2 Groups (`groups.go`) — `SubscribeGroups`

The adapter owns no core sub at all *(as built)*: it keeps a private relevance map
(matching object id → grouped-key value) fed by the batch loop, and marks itself dirty when
an object enters/leaves the match set, a member's grouped value changes, or a
`relationOption` of the relation changes (tag/status groups derive from option objects).
Dirty adapters recompute on the worker **after** the space mutex is released (the grouper
queries the store) with a freshly compiled `database.Filters` each time — the kanban
groupers mutate the filters they are given *(rev, S4)*. Recompute → `MakeDataViewGroups()`
→ diff group ids → broadcast `SubscriptionGroups{group, remove}` per delta. Response =
initial groups. `CollectionId` scoping is a **subscribe-time snapshot** folded in as an
id filter (the grouper computes from store queries, so live collection membership cannot
scope it; group sets refresh on re-subscribe — documented limitation). Groups subs are
rare (one per kanban view); recomputation is one store query.

### 7.3 Collections (`collections.go`) — `CollectionId`

`CollectionService.SubscribeForCollection(collectionId, subId)` returns initial ids + a
channel of full id-list updates. The adapter goroutine forwards channel updates as
`setScope` calls; effective membership = scope ∩ filter matches, maintained by the core's
single membership path.

### 7.4 Ids subscriptions (`SubscribeIdsReq`)

Pure normalization: fixed scope = requested ids, no filters (explicit ids are tracked even
when archived/deleted — soft-deletion reaches the consumer as detail changes), response
records in request order, missing ids tolerated and picked up later by the feed
(`DetailsSet + Add{afterId: nearest preceding requested id currently present}`). No
counters. Deps apply unless `NoDepSubscription` (the pb request carries the flag; response
has `Dependencies`) *(rev, N4)*.

## 8. Concurrency model

- **Locks**: service registry RWMutex (subId slots, spaceId→state) and one mutex per
  spaceState. Lock order: registry → spaceState, never the reverse; the subId slot
  state machine (§6.1.5) keeps replace/teardown single-space at a time. Encoding and
  delivery happen after unlocking (§4.3).
- **Deadlock rule** (pinned by `TestLazySubscribe_SearchFirstOpenerDoesNotDeadlock`):
  `store.SpaceIndex(spaceId)` may synchronously fire `OnSpaceIndexOpened`, which re-enters
  `Search` on the same goroutine (crossspacesub promotion). No engine lock may be held
  across `store.SpaceIndex()`; the engine resolves the index first, locks after. Adapters
  never call `store.SpaceIndex()` from worker context — they capture the index handle at
  subscribe time (§7.2).
- **Feed callbacks** never take the space mutex — they touch only the coalescing queue's
  own small mutex and signal the worker. Writers are never blocked by subscription work.
- **Worker** is the only mutator of sub state; Subscribe/Unsubscribe serialize with it via
  the space mutex. One worker per space keeps per-space event order strict while spaces
  proceed independently.
- **Queue overflow policy** *(rev, F7)*: engine-owned internal queues are killed (closed +
  logged) at a high watermark (~50k messages) — a consumer that far behind is broken, and
  transition events cannot be coalesced after encoding; consumers already treat a closed
  queue as terminal. Caller-provided queues (crossspacesub's shared queue) are never closed
  or policed by the engine.
- **Close()**: unhook feeds, stop workers, then close engine-owned queues.

## 9. Performance expectations → mechanisms

| Contract §3.10 expectation | Mechanism |
|---|---|
| dozens of subs per space, cheap create/teardown | sub = compiled filters + private maps; snapshot = one store query; no per-sub goroutines (exception: collection watcher) |
| cheap steady-state `limit 0` subs | members = id set; full details only for visible members the client renders |
| bounded per-update cost | per sub: one filter eval (benchmark-gated 0 allocs/op on the miss path) + id-set lookup; diff/projection only for visible members, single-pass and lazily allocating |
| storm tolerance | coalesce-by-id intake with id-only degradation above threshold (§4.1); batch processing; one Counters per sub per batch; encoding off-mutex |
| accurate total without full details | total = `members` count; window-only sort state (§3) |

Costs accepted knowingly: window diff O(W²) on ≤500 windows (sub-ms); margin re-query per
window underflow; `BuildOrderMap` at subscribe time for object-sorted views is two
full-space queries — measured by a dedicated benchmark, optimized only if it shows up
*(rev, N6)*.

Benchmarks (committed with the engine, `b.ReportAllocs()`): snapshot init over N objects;
change-batch processing across S subscriptions; **miss-path 0 allocs/op asserted**; limit-0
steady-state update cost; window shift on a 500-element window; object-sorted subscribe.

## 10. Module layout (package `core/subscription`)

| File | Contents |
|---|---|
| `service.go` | public API (kept), registries + subId slots, assembly |
| `normalize.go` | validation, subId, Source→filters, ordered/unordered decision, truncation |
| `spacestate.go` | per-space hub: feed wiring, coalescing intake, worker, outbox |
| `coresub.go` | the core primitive: members, scope, window, transitions, diff script |
| `events.go` | neutral ops → pb encoding (off-mutex), batch assembly, router |
| `deps.go` | dep orchestrator (render deps + order deps) |
| `groups.go` | groups orchestrator |
| `collections.go` | collection scope adapter |

`eventmatcher.go`, `fixture.go`, `objectsubscription/`, `crossspacesub/` are unchanged.
Plus the §2 store-side change in `pkg/lib/localstore/objectstore/spaceindex` (tx-buffered
notifications) and the two filter allocation fixes in `pkg/lib/database` *(rev)*.

## 11. Implementation phases

1. **Store prerequisite + core, unordered**: tx-buffered feed notifications; spacestate +
   intake + outbox + coresub (set semantics) + encoder/router + service skeleton, internal
   delivery, subId slots, replace/unsubscribe → crossspacesub + objectsubscription suites
   green (first acceptance gate).
2. **Ordered/windowed**: order compilation, window state + boundary bookkeeping +
   margin re-query, window-diff script, counters, RPC wiring + response shape; engine unit
   tests with event-level assertions.
3. **Scopes**: ids subs, collections, `setScope` transitions.
4. **Adapters**: deps (render + order deps, reverse index), groups.
5. **Hardening**: benchmarks (incl. miss-path alloc gate), storm/coalescing tests, filter
   allocation fixes, full consumer-suite pass (`core/syncstatus`, `core/files`,
   `core/pushnotification`, `core/acl`, `space`, `core/api`).

Each phase is a separate commit with all tests green.

## 12. Explicitly not built (contract §4)

afterId/beforeId request cursors; nextCount/prevCount (fields stay zero); nested
sub-subscriptions; fulltext in subscriptions; membership events for `/dep`;
`SubscribeIds(subId, ids)`; `UnsubscribeAll`; sorts/offset/limit live semantics for
internal subs beyond snapshot truncation; subIds payload multiplexing (deferred,
router-level optimization); live re-resolution of frozen filters (nested dot-keys,
quickOption dates — re-subscribe refreshes them).
