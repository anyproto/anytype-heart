# Subscriptions: the real contract

What anytype-heart's subscription system must actually do — derived from how the desktop
client (anytype-ts) and anytype-heart's own services consume it today, confirmed against the
code of both repos. Written as groundwork for a clean-slate reimplementation: everything the
current implementation does beyond this document is candidate legacy.

Evidence base: anytype-heart @ develop (5e888f1b9), anytype-ts @ main. `ts:` paths are in
the anytype-ts repo, everything else in anytype-heart.

## 1. What a subscription is for

A subscription is a **live query over object details**: the caller describes a set of objects
(filters over relation values, optionally scoped to a collection or sources) and the relation
keys it cares about, and receives

1. an **initial snapshot** (records, already sorted and windowed when requested), and
2. an **incremental event stream** that keeps the caller's copy in sync: objects entering and
   leaving the set, their position in the ordering, and per-key detail changes.

Its purpose is to let consumers render and react to query results **without re-querying**:
the client mirrors records into per-subscription stores and applies events; internal services
keep derived state (sync counters, download queues, push tokens, caches) up to date.

There are two distinct consumer profiles with very different needs:

| | Desktop client (RPC) | Internal services (Go) |
|---|---|---|
| Ordering | **server-ordered**, client never sorts | none — set membership only |
| Pagination/window | limit + counters.total, growing window | none (always full set) |
| Dependencies | needed (dataview cells) | never (always disabled or ignored) |
| Counters | total only | none |
| Delivery | event broadcast to all sessions | per-consumer internal queue |
| Typical set size | windowed (50–500) or full-space | small filtered sets |

## 2. Consumers inventory

### Client (anytype-ts) — all through `U.Subscription.subscribe/subscribeIds` (ts:src/ts/lib/util/subscription.ts:155-311)

- **Dataview** (sets/collections/inline/calendar/timeline): view filters + sorts, per-view
  keys, limit 50–500, growing window; kanban = one groups-subscription + one subscription per
  visible group; calendar = one subscription per visible day.
- **Per-space singletons** (created on space open): types, relations, options, participants,
  deleted ids, archived ids, chats, recently-edited — most with `limit 0` (= full space) and
  `noDeps`; deleted/archived with `keys: [id]` only.
- **Per-type existence checks**: `typeCheck-{space}-{type}` with `keys [id], limit 1`, reads
  only `counters.total` — one subscription per object type.
- **Widgets/sidebar**: sidebar key set, `noDeps`, small limits; tree widget `subscribeIds`
  over expanded nodes' links.
- **Cross-space**: global chats, pending members, space-create member picker (4 users).
- **Profile/space views** in the tech space.

A single open space easily holds **dozens of concurrent subscriptions** (singletons + one per
type + per-view/per-group/per-day). Cheap subscription creation and teardown is a first-class
requirement, not an optimization.

### Internal (anytype-heart)

All internal consumers go through `Internal: true` queues, almost all via the typed
`objectsubscription.ObjectSubscription` wrapper (core/subscription/objectsubscription):
crossspacesub (space-view monitor), syncsubscriptions (3 per space), formatfetcher,
filesync/spaceusage (2), pushnotification (2), acl (2), space.spaceWatcher, chats,
filedownloader, JSON-API caches (4). Plus two odd ones:

- **objectgraph** (core/block/object/objectgraph/graph.go:103-121) subscribes, reads
  `resp.Records`, immediately unsubscribes — it wants a query engine, not a subscription.
- **JSON API list endpoint** (core/api/service/list.go:184-205) does subscribe+unsubscribe
  just to obtain `Counters.Total` alongside a page of records — i.e. it wants
  query-with-total, not a subscription.

## 3. The contract a reimplementation MUST honor

### 3.1 Identity and lifecycle

- Subscriptions are identified by a **caller-chosen string subId**, global per account
  (client formats: `{rootId}-{blockId}`, `{name}-{spaceId}`, fixed names; no window scoping).
- **Re-subscribing with the same subId replaces the subscription.** This is load-bearing:
  the client's dominant pagination pattern is "re-subscribe same subId with limit =
  previousWindow + page" (ts grid.tsx:387-400, list, gallery, board columns, archive), and
  view switches do the same. (Current impl: service.go:206 unsubscribes before subscribing.)
- Explicit unsubscribe takes a list of subIds. Unsubscribed ids may be reused immediately.
- Events are **broadcast to every client session**; each window applies events for all subIds
  (multi-window relies on this; two windows showing the same object share one subId, and the
  last re-subscribe wins).

### 3.2 Query semantics (client path)

- **Filters**: conditions actually exercised by the client: Equal, NotEqual, In, NotIn,
  ExactIn, Empty, NotEmpty, Like, Greater, GreaterOrEqual, LessOrEqual, None; `Exists` is used
  internally (pushnotification). Plus:
  - **nested dot-keys** (`type.uniqueKey`) — used for template filtering and recent-edits;
  - `quickOption` date resolution (ExactDate etc.) and `includeTime`/`format` on both filters
    and sorts;
  - nested filter trees (And/Or operators with NestedFilters) — client sends them
    (ts dataview.ts:307-336);
  - **implicit default filters** (`isArchived/isDeleted NotEqual true`, layout/type exclusion,
    pkg/lib/database/database.go:84) are injected unless the request already references those
    keys — the client deliberately sends a no-op `Condition: None` filter on those keys to
    suppress injection when it wants archived/deleted objects (ts subscription.ts:114,121).
    A reimplementation must keep an equivalent opt-out, ideally an explicit flag instead of
    this trick.
- **Sorts**: multiple hierarchical sorts; text collation; `emptyPlacement` (Start/End);
  **Custom sort with `customOrder`** — used for drag-ordered views (order = explicit id
  list, ts dataview.ts:184-190), type ordering, sync-status menu. No text/fulltext query in
  subscriptions (removed Jan 2022, commit 08d6c1d82).
- **Sources** (`setOf`: type/relation ids) and **CollectionId** (live collection membership —
  the set must update when the collection's links change, not only when objects change).
- **Limit semantics**: `0 = unbounded` (heavily used); `1` for existence checks; windowed
  lists rely on limit + total. **Offset**: used by exactly one client component (ListObject
  pager, ts component/list/object.tsx:227-243) and the JSON API; keep, but it is cold.
- **Keys**: every subscription declares the relation keys it needs; `id` is always present.
  Response records and detail events must be **projected to requested keys** (the client
  stores whatever arrives but renders only requested keys; extra keys are wasted bytes).

### 3.3 Ordering and membership events (client path)

The client **never sorts records itself**. List order = initial `records` order + faithful
application of events (ts dispatcher.ts:1015-1036, store record.ts:300-341):

- `SubscriptionAdd{id, afterId}` — insert after `afterId`; `afterId == ""` means head.
  Idempotent (client skips if present).
- `SubscriptionPosition{id, afterId}` — move within the list, same afterId semantics.
- `SubscriptionRemove{id}` — drop from list.
- `SubscriptionCounters{total}` — **only `total` is consumed**; it drives pagers, load-more,
  badges, calendar dots, type existence checks. Must stay accurate for the full matching set,
  not the window.
- Events for objects **entering/leaving the window** due to other objects' changes must be
  emitted (the window is maintained server-side).
- Within one event payload the client re-sorts event application (membership → details →
  counters, ts dispatcher.ts:13-26), so strict intra-batch event ordering is not required,
  but all events for one logical change should arrive in one payload.

### 3.4 Detail events (both paths)

- `ObjectDetailsSet{id, details, subIds[]}` — full snapshot of requested keys; client
  replaces the object's detail map (clear=true). Sent when an object first becomes visible
  to a subscription (this also doubles as "add" for the typed internal wrapper).
- `ObjectDetailsAmend{id, details(keyValues), subIds[]}` — **only changed keys**. The client
  merges; correctness tolerates extra keys, but minimality is a de-facto performance contract
  (per-key MobX observables → minimal re-renders).
- `ObjectDetailsUnset{id, keys, subIds[]}` — per-key deletion.
- One physical event may carry multiple subIds; the client demultiplexes.

### 3.5 Dependencies (client path only)

- Default-on (`noDepSubscription=false`): the middleware must track objects referenced by
  object/file-format relation values among the requested keys, return them as
  `dependencies` in the response and keep them updated via detail events under
  `{subId}/dep`.
- The client folds `/dep` details into the base subscription's detail store and **ignores
  add/remove/position/counters for dep subs** (ts dispatcher.ts:1026,1042,1627). Only
  detail events matter for deps.
- What depends on it: object/file relation cells and cover objects in dataviews. Tag/status
  options, types, relations, participants do NOT — they are resolved via the client's own
  full-space singletons. Internal consumers never use deps (crossspacesub rejects them).
- Dep objects can live in the same space; dep tracking must include objects referenced by
  filter values (current `forceSubIds` behavior) — the client filters by relation object ids
  and renders their names.
- A reorder of the parent list when a dependent object changes a sort-relevant value (e.g.
  sorting by assignee name and the assignee is renamed) must re-emit position events
  (current dep-order behavior, core/subscription/dep.go).

### 3.6 Ids subscriptions

`ObjectSubscribeIds{ids, keys, noDeps}`: live tracking of an explicit id list (tree widget,
chat mention/attachment deps, pinned search results). Records returned in request order;
objects missing now may appear later (the subscription must catch them when indexed).
No ordering/counters semantics.

### 3.7 Groups subscription (kanban)

`ObjectGroupsSubscribe{relationKey, filters, source, collectionId}` returns the distinct
groups (tag/status values, checkbox states) for a query and emits
`SubscriptionGroups{group, remove}` when the group set changes. Used only by kanban
(ts dataview.ts:480); record streaming for each column is a separate normal subscription
per group. Counters/limit/keys do not apply.

### 3.8 Cross-space subscriptions

`ObjectCrossSpaceSearchSubscribe{filters, sorts(unused), keys, source, noDeps}`: same query
fanned out over all spaces matching a predicate (all spaces for the RPC), with per-space
subscriptions aggregated under one subId; spaces appearing/disappearing later must join/leave
the result set dynamically (synthesizing Remove events for ids of a removed space — current
`UnsubscribeAndReturnIds` + fabricated events, crossspacesub.go:207-241). Rejects: deps,
limit, cursors, collections, sorts. 4 client users + 5 internal.

### 3.9 Internal contract (Go services)

Internal consumers need exactly this (everything else is unused by them):

> **Unsorted, unpaginated, key-projected live set over one space (or predicate-fanned over
> spaces), delivered as: initial records snapshot + a private queue of
> {appeared(full details), changed(amended keys / unset keys), disappeared(id)}.**

Concretely: `Internal: true` with optional externally-owned queue; fixed small key sets
(1–13 keys); flat filters (Equal/NotEqual/In/NotIn/Exists/LessOrEqual); `NoDepSubscription`;
dynamic late-start (today via `AsyncInit`, used only by crossspacesub when a space appears).
The typed wrapper interprets only DetailsSet (≈add), Amend, Unset, Remove — it explicitly
ignores Add (redundant with DetailsSet), Position, Counters, Groups
(objectsubscription/objsubscription.go:250-322).

A clean design could give internal consumers a first-class Go API (snapshot + typed
callbacks) rather than routing pb events through queues.

### 3.10 Performance expectations (from observed usage)

- Many concurrent subscriptions per space (singletons + per-type + per-view/group/day);
  creation must be cheap and ideally not require a full decoded copy of every matching object.
- `limit 0` full-space subscriptions over types/relations/options/participants must be cheap
  in steady state — they exist in every space for the whole session.
- Every object update in a space is evaluated against every subscription of that space;
  the per-update cost must be bounded (today: ~all subs × all filters per batch).
- The same update storm during sync/initial indexing must not accumulate unbounded queues.
- `counters.total` accuracy must not require keeping full details of all matching objects.

## 4. Confirmed-dead surface (do NOT carry into a reimplementation)

| Surface | Evidence |
|---|---|
| `afterId`/`beforeId` request cursors | Never set by client (always `''`, ts subscription.ts:170-171); RPC passes them into `SubscribeRequest` (core/object.go:229-230) **but the engine never reads them** — cursor pagination has been dead since birth |
| `counters.nextCount/prevCount` | Client decodes but discards (ts mapper.ts:1756-1761); response fields have been **swapped** (`NextCount: prev`, service.go:551-552) since 2021 without anyone noticing |
| `Service.SubscribeIds(subId, ids)` (Go method) | Empty stub, zero callers (service.go:234-236) |
| `Service.UnsubscribeAll` | Zero callers |
| Nested-filter sub-subscriptions | Hard-disabled `&& false` since Oct 2023 (service.go: subscribeForQuery, GO-1883) |
| Fulltext query in subscriptions | Removed Jan 2022 (08d6c1d82) |
| `ignoreWorkspace` | Removed (GO-4147) |
| Add/Remove/Position/Counters events for `/dep` subs | Client explicitly drops them (ts dispatcher.ts:1026,1042,1627); only `/dep` detail events are consumed |
| Response `subId` field | Client ignores it (always supplies its own) |
| `meta.keys` client-side | Stored, never read (ts subscription.ts:47) |
| Sorts/Offset/Limit/Counters for internal subs | No internal consumer uses them (crossspacesub rejects most outright, crossspacesub/service.go:106-127) |
| `SubscriptionAdd` as a separate signal for internal consumers | Typed wrapper derives adds from DetailsSet; the two raw consumers use only the id, which DetailsSet also carries |

Ambiguous / product decision needed:

- **Offset paging**: one cold client path + JSON API. Could be replaced by query-only
  pagination if the JSON API and ListObject move to one-shot queries with totals.
- **objectgraph & JSON-API "subscribe-then-unsubscribe"**: should become a plain query API
  that returns `total`; then subscriptions never need to serve one-shot consumers.
- **Amend minimality**: keep as a hard guarantee (recommended — it is what bounds client
  re-renders) or downgrade to best-effort.
- **The `Condition: None` injection-suppressor trick**: replace with an explicit
  `skipDefaultFilters` flag and keep the trick working during migration.

## 5. Requirements checklist for a blank-slate design

MUST
- [ ] subId identity with replace-on-resubscribe and multi-session event broadcast
- [ ] filters incl. nested dot-keys, And/Or trees, quickOption dates, default-filter
      injection with opt-out
- [ ] hierarchical sorts incl. custom id-order and emptyPlacement; server-side ordering
      exposed via Add/Position(afterId)/Remove
- [ ] windowing: limit (0 = all), accurate `total` for the full set
- [ ] detail events: Set (full, on entry), Amend (changed keys only), Unset; multiplexed
      subIds; projected to requested keys
- [ ] dependency tracking for object/file relation values incl. filter-value deps, delivered
      as `/dep` detail events + response `dependencies`; dep-driven parent reorder
- [ ] ids subscriptions; collection-scoped subscriptions (live membership); sources
- [ ] groups subscription for kanban (groups list + group add/remove events)
- [ ] cross-space fan-out with dynamic space join/leave
- [ ] internal Go-facing API: snapshot + appeared/changed/disappeared with key projection,
      private delivery, no ordering/counters overhead

SHOULD
- [ ] one-shot query-with-total API so objectgraph/JSON API stop abusing subscriptions
- [ ] explicit flag replacing the `None`-filter suppressor
- [ ] bounded memory: store only requested/sort/filter keys per tracked object; coalesce
      update storms

WON'T (drop)
- [ ] afterId/beforeId cursors, nextCount/prevCount, nested sub-subscriptions, text query,
      dep membership events, `SubscribeIds(subId, ids)` stub, `UnsubscribeAll`
