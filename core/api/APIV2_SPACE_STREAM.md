# API v2 — the spaces stream

Status: design, 2026-11-21, revised 2026-11-23 (a loading space counts as
visible, §3.3), not implemented · heart `develop` @ `b6321aac4` · gin
v1.10.0. §10 records where the working assumptions were corrected.

`GET /v2/spaces` answers "which spaces can this key see" once. A client that
must notice a new space — a bot that sets up every space it is granted, a
space picker — polls it today. The spaces stream holds that answer open over
Server-Sent Events: the list's rows, then a change whenever the answer
changes — a space created, joined, renamed, deleted or left, or the key's
grant edited.

Naming: this is the **spaces** stream. `SKILL.md:338` already says "space
streams" for `GET …/chats/stream`; keep the two apart in prose and code.

---

## 1. Goals and non-goals

Goals:

- One SSE route serving exactly `GET /v2/spaces`'s row set, kept current.
- A grant edit reaches an open stream as removals and additions without a
  reconnect; revoking the key ends it; expiry ends it at the instant the
  auth middleware would start refusing the key.
- No second definition of "visible space": the stream and the list call one
  function.
- A space still loading on this device is visible — on the stream and on
  every route that shares the predicate (§3.3), with a bounded wait wherever
  a request needs the loaded space (§3.4).

Non-goals: replay or resume; pagination, filtering or extra fields; a
wrapper tool; changing /v1; fixing the open-time grant check of the chat and
search streams (§8).

## 2. Wire contract

### 2.1 Route

`GET /v2/spaces/stream` · operationId `stream_spaces` · tag `Spaces` ·
`text/event-stream` · no body.

| param | meaning |
|---|---|
| `heartbeat` | keepalive cadence, 1–60 s, default 30; an out-of-range value falls back to the default, as on every stream (`v2/handler/chat_stream.go:37-45`) |
| `ids` | absent or `full`: the stream serves full space ids only. `compact` is a 400 at `ids`; any other value is the group middleware's own 400 (`v2/idshape.go:31-42`) |

`offset`/`limit` pass the group's pagination middleware and change nothing:
the snapshot is the whole set.

Why `ids=compact` is refused rather than ignored: a short reference is unique
only against the visible set at one instant (`v2/service/spaceref.go:149-153`).
A space joined while the stream is open can retire a short id the stream
already sent, and no client tracks a re-spelling of an id it holds. An
explicitly requested shape the route cannot serve is refused, the way the
search stream refuses `query` instead of ignoring it
(`v2/service/search_stream.go:79-83`).

### 2.2 Events

Every event is `event: <type>` + one `data:` line, no `id:`.

| type | when | body members |
|---|---|---|
| `space_added` | snapshot, one per visible space; live, when a space becomes visible | `space` — the `GET /v2/spaces` row (`id`, `name`, `description`, `icon_image`), `id` always the full id |
| `snapshot_complete` | once, after the snapshot (also for an empty set) | `has_not_granted_spaces`, always present |
| `space_updated` | a visible space's served row changed | `space`, the whole row |
| `space_removed` | a space left the visible set | `space_id` (full), `reason` ∈ `access_removed`, `gone`, `unavailable` (§3.6) |
| `not_granted_updated` | `has_not_granted_spaces` flipped | `has_not_granted_spaces` |
| `stream_closed` | last event before the server ends the stream | `reason` ∈ `key_revoked`, `key_expired` |

```
event: space_added
data: {"type":"space_added","space":{"id":"bafyreiujzdegxdncf32epf3dhodzdocis2jhtlgmxgedn73u55xtplpft4.28y6mgnwgodt7","name":"Work","description":"Team notes"}}

event: snapshot_complete
data: {"type":"snapshot_complete","has_not_granted_spaces":true}

event: space_added
data: {"type":"space_added","space":{"id":"bafyreiv4seh2kvj72ceuvw75efr6edt4sywb5wkh7dnsipzz7fk4zri3ry.28y6mgnwgodt7","name":"Research"}}

event: space_removed
data: {"type":"space_removed","space_id":"bafyreiujzdegxdncf32epf3dhodzdocis2jhtlgmxgedn73u55xtplpft4.28y6mgnwgodt7","reason":"access_removed"}

event: not_granted_updated
data: {"type":"not_granted_updated","has_not_granted_spaces":false}

event: stream_closed
data: {"type":"stream_closed","reason":"key_revoked"}

: keepalive
```

Model: `SpaceStreamEvent{Type, Space *SpaceRow, SpaceId, Reason string,
HasNotGrantedSpaces *bool}` — the pointer keeps `false` on the wire for
`snapshot_complete` and `not_granted_updated` and absent elsewhere.
`snapshot_complete` reuses `SpaceChatEventSnapshotComplete`, as the search
stream does (`v2/model/search_stream_event.go:13`).

### 2.3 Order and reconnect

- The snapshot is in full-id order, the list's order
  (`v2/service/discovery.go:127`).
- Live events come in recomputations (§3.5); one recomputation's events are
  in full-id order, `not_granted_updated` last.
- `space_removed` can arrive a second time for a space last removed as
  `unavailable`, carrying its final reason (`gone` or `access_removed`). A
  client stores the reason per id; `space_added` clears it.
- Events carry no id and `Last-Event-ID` is ignored. A reconnect is a fresh
  snapshot; a space the client holds that is absent from it has left the set,
  and its reason is not replayed.

### 2.4 Close conditions

| cause | what the client sees |
|---|---|
| client hangs up, or a write fails | connection ends |
| key revoked | `stream_closed{key_revoked}`, then the end |
| key expired | `stream_closed{key_expired}`, then the end |
| the key cannot be re-read for any other reason | the end, no event: fail closed, the reconnect's own authentication decides |
| the API server is rebound or stopped | the end, no event |

`stream_closed` says what the next request would answer (the auth
middleware's two 401 messages, `server/middleware.go:38-39`), so a client
stops instead of reconnecting into a 401. Nothing else ends the stream.

### 2.5 Refusals before the first byte

| status | cause |
|---|---|
| 400 `validation_failed` | `ids=compact`; an unknown `ids` or out-of-range `limit` (group middleware) |
| 401 | the shared auth middleware; or the open-time re-read (§4.4 R1) found the key revoked or expired — answered with the auth middleware's own body and `WWW-Authenticate` |
| 403 | key scope (`ensureJsonApiScope`, a Limited key). Never `space_not_granted`: the route addresses no space |
| 429 `too_many_streams` | the stream cap (§5) |
| 500 | the subscription cannot be opened |

No 404: there is no `{space_id}`.

## 3. Visibility — one definition

### 3.1 What "visible" is today

`liveSpaceRowsWithGrantInfo` (`v2/service/discovery.go:80-129`): query the
tech space for `resolvedLayout == spaceView` — the store adds
`isArchived != true` and `isDeleted != true` (`pkg/lib/database/database.go:87-131`;
the layout filter suppresses only the type default, `:156`) — then, per
record: skip an empty `targetSpaceId` or one already taken; skip a view
failing `isLiveSpaceView` (`v2/service/space.go:53-60`); skip a space
`util.SpaceGrantRefusal` refuses, setting `has_not_granted_spaces` unless it
is the tech space (`:113-118`); keep the first survivor per target (`:119` —
the tie-break is store order); sort by id. Every other enumeration reads it:
`liveSpaceRows` (`:73`), the global-search fan-out (`search.go:1074`),
short-reference resolution (`spaceref.go:208`) and spelling
(`space.go:124`), whoami's names.

### 3.2 The refactor

- `spaceViewKeys` — the detail keys the definition reads: `id`, `targetSpaceId`,
  `spaceLocalStatus`, `spaceAccountStatus`, `spaceRemoteStatus`, `name`,
  `description`, `iconImage`.
- `visibleSpaces(views []*domain.Details, grant *util.ApiGrant, techSpaceId string)
  (rows []SpaceRow, hasNotGranted bool)` — pure; its body is today's loop,
  over views sorted by view id.
- `liveSpaceRowsWithGrantInfo(ctx)` = the same query, each record projected
  to `spaceViewKeys`, then `visibleSpaces(records, util.ApiGrantFromCtx(ctx), tech)`.
- The stream calls `visibleSpaces(its view map, its current grant, tech)`.

Two properties make this one definition rather than two that agree today:
the list projects to the key list the stream subscribes with, so the
function cannot start reading a key the stream does not carry; and the grant
is a parameter, never read from `ctx` — the stream's grant changes after
open, and a helper reading `ctx` would keep serving the open-time grant
without any test noticing.

Behaviour change to the list: between two live, granted views of one target,
the winner moves from store order to view-id order. A space view id is
derived from its space (`space/techspace/techspace.go:277-298, 371-383`), so
two views of one target exist only in damaged data; the change makes the
existing answer deterministic, which the stream needs because its view map
has no order.

Liveness gets a classifier: `spaceViewState(details) → live | unavailable | gone`,
and `isLiveSpaceView(d)` becomes `spaceViewState(d) == live`. Its `live`
branch is local status Unknown, Loading or Ok and account status Unknown or
SpaceActive — today's predicate plus Loading (§3.3); the other two states only
name *why* a view is not live (§3.6).

### 3.3 Loading counts as live

Decision (2026-11-23). It is made in `spaceViewState`, so it holds on every
surface `isLiveSpaceView` gates: the list, GET-one (`space.go:100`),
`PATCH /v2/spaces/{id}` (through GET-one, `space.go:241`), `ensureSpace` and
with it every per-space route (`service.go:128-146`), short-reference
resolution and spelling, whoami's names, and the global-search fan-out. A
stream that showed a loading space while GET-one answered 404 "not available"
would be the drift §3.2 exists to prevent.

Behaviour change of `GET /v2/spaces` and the per-space routes: a space whose
local status is Loading is listed and served instead of refused. The loader
publishes Loading when a space starts loading and was not on disk and Ok last
session (`spaceloader.go:127-137`): every space on a new device's first start,
a space created or joined during its first load, a space that failed to load
retried at the next start. A load that keeps failing with retryable errors
publishes nothing until it gives up — it retries with backoff up to 20 s until
the app stops (`loadingspace.go:74-104`), and the ACL-head check retries for
as long as that head is not local (`:139-147`) — so such a space now stays
listed for as long. /v1 keeps its own filter (`core/api/service/space.go:44-53`):
the two APIs disagree about a loading space.

No existing test pins Loading as not live. The statuses the v2 tests use are
Deleted + Missing (`service/discovery_test.go:78-79, 163-164`,
`service/space_test.go:81-82`), Joining (`handler/discovery_test.go:46`) and
Removing (`service/search_test.go:997`), all still not live, so none changes;
§7 adds the tests that pin the new rule.

No create or join flap remains, so nothing delays a removal:

| sequence | statuses | on the stream |
|---|---|---|
| create | Unknown → Loading → Ok (`spacefactory.go:261-276`, `spaceloader.go:131-136, 154-156`) | one `space_added` |
| join | SpaceJoining → SpaceActive (`joiner.go:43-51`) → Loading → Ok | one `space_added`, at SpaceActive |
| start, on disk and Ok last session | Ok while it builds (`spaceloader.go:127-137`) | live throughout |
| start, otherwise | Loading → Ok, or → Missing | live; `space_removed{unavailable}` only on failure |
| failed load retried | Missing → Loading → Ok or Missing | `space_added`; again `space_removed{unavailable}` if it fails |

A Missing space is retried only at a start or on an account-status change —
the controller changes mode on nothing else (`shareablespace/shareable.go:80-99,
122-142`) — so the last row's pair is an attempt and its failure, both true,
at most once per retry; no delay would make either event more accurate.

### 3.4 What a loading space costs a request

- **Store reads never wait.** The lists, search, types, properties, members
  and the spaces list read the index. For a space not yet on disk they answer
  from what is indexed so far: empty or partial, silently. (v1's hang on a
  guest space came from v1 opening every listed space; v2's list reads the
  store and cannot hang.)
- **Live reads and every write wait for the load.** Every write reaches
  `spaceService.Get` (`core/block/service.go:245-250`; adapters
  `objectreadadapter.go:55`, `objectmutateadapter.go:45`,
  `objectcreateadapter.go:62`), which waits for the loader
  (`space/service.go:453-461` → `space/load.go:102-111` →
  `spaceloader.go:189-218`) with no bound but the context. So does every read
  of live state: an object or a type document (`ReadObject`); a query's or a
  collection's objects and views, which read their list object live
  (`list_read.go` `readListTarget`); every chat read and the chat stream's
  open, whose chat lookup reads the deletion mark from the space's head
  storage (`chat.go` `ensureChat` → `objectreadadapter.go:88-104`); the widget
  list (`widgetadapter.go:73`); and a file download, for the same deletion
  mark (`file_download.go`).
- **The request context has no deadline.** The server sets only
  `ReadHeaderTimeout` (`core/api/service.go:258-261`); the context ends when
  the client disconnects. `/mcp` adds 60 s (`server/mcp.go:52, 427`).
- **Some writes drop even that.** `PATCH /v2/spaces/{id}` calls
  `WorkspaceSetInfo`, which ignores its context (`core/workspace.go:139-140`)
  and waits on the component context (`detailservice/set_details.go:29`); type
  and property updates call `ObjectSetDetails` (`v2/service/typeops.go:234`,
  `schema_write.go:1014, 1470`), which waits under `context.Background()`
  (`detailservice/service.go:143-149`, `block/cache/cache.go:40-41`). Those
  wait until the load ends or the app stops, past a client disconnect; on the
  in-process transport the call lease stays taken that long
  (`server/inproc.go:71-76`).
- **Not new in kind.** A space on disk and Ok last session publishes Ok while
  it builds (`spaceloader.go:127-137`); `ensureSpace` admits it today and the
  same waits apply, unbounded when the build keeps retrying. The decision
  widens the admitted set to spaces that load from the network — the ones most
  likely to retry for long — and turns today's immediate 404 for them into
  those waits.

Mitigation, needed for that reason, and the smallest that bounds every path:

- A narrow port `apicore.SpaceLoads{ WaitLoaded(ctx, spaceId) error }`,
  implemented in package `api` over `space.Service.Get`.
- `ensureSpaceLoaded(ctx, spaceId)` in the v2 service waits through it under
  `context.WithTimeout(ctx, spaceLoadWait)`, 10 s. On the deadline: 503
  `space_loading`, `Retry-After: 5`, message "space %q is still loading on this
  device". Any other error is the one the route's own `Get` would have hit,
  mapped as today; a cancelled request ends with its own error.
- Called by every write and by every live read, before the first thing
  that needs the loaded space. For a write that validates against the index
  the rule is:
  - **pure request validation** — body shape, field values, nothing read
    from the space — comes first and never waits, on a dry run and on a real
    write alike;
  - **a real write** waits before its first read of space state, the index
    included (`ensureSpaceLoadedForWrite`). The index of a loading space is
    partial, so "no such property", or "no duplicate key", read from it is
    not a fact; and a 404 reads as final where a 503 is retried;
  - **a dry run** does not wait and does not start a load: it sends nothing,
    and answers from the index as it is. When the space's served view says
    Loading, whatever it concluded from the index — a refusal ("no such
    property", "that key is taken") or a dry-run result — carries the
    store-read warning below, so that a dry-run 404 is not read as final
    (`dryRunLoadingNote`, and `LoadingSpaceWarnings` on the result). A refusal
    of the request itself carries none.

  Who gates where:
  - a write whose validation and dry run read live state — an object, a
    type document, a derived id — gates at entry, through
    `ensureSpaceWrite`, dry runs included: object create, patch, delete and
    restore, template and discussion create, type create and update, widget
    delete;
  - `UpdateSpace`, chat create and file upload read no space state before
    their RPC: `ensureSpaceWritable` at entry, `ensureSpaceLoaded` right
    before the RPC, after the dry-run return;
  - type delete and property create, update and delete validate against the
    index: `ensureSpaceWritable`, the body checks, then
    `ensureSpaceLoadedForWrite`, then the index;
  - query, collection and widget create and widget update gate after their
    body checks and before their first read of space state. Their dry runs
    read live state too (a derived id, the widget roots), so those take the
    wait there — except a collection's item check, which a dry run answers
    from the index first;
  - `ensureChatWrite`, through `ensureChat` (`chat.go`);
  - the live reads: `GetObject` (`object.go`, which the type-document read
    reaches), `ensureChat`, `readListTarget`, `ListWidgets`, and the
    deletion-mark branch of `GetFileContent`;
  - `ListTemplates`, a store read, on the one branch that is not: `?type=`
    naming a built-in type with no row in the space derives the type's id
    through `creator.TypeIdByKey`, which reaches `space.Service.Get`.

  Always after the grant, verb and existence checks, so a 403 or a 404 never
  waits, and before any RPC, which is what bounds the paths that drop the
  context.
- With no `SpaceLoads` wired (a service built without it, as most tests do)
  the gate is a no-op: nothing is bounded and nothing is refused.
- Called whatever the view's status says: in the start fast path the status is
  Ok while the space builds, and for a loaded space the call is a map lookup
  and a lock (`space/load.go:22-30`, `spaceloader.go:194-201`).
- Store reads stay ungated. For a space whose view says Loading they add one
  warning — still loading on this device, results can be incomplete — to the
  list envelope's `warnings` (`v2/model/model.go:216`): the object, type,
  property, option, member, template and chat lists, the space search, and
  the global search for each loading space in its fan-out. The warning is
  `Service.LoadingSpaceWarnings`, a second read of the space view made where
  the envelope is assembled (the handler, or the search methods, which
  already return warnings), not a value threaded out of `ensureSpace`:
  that would change the signature of every store-read method for one lookup.
- 10 s sits well under the `/mcp` call timeout, so an MCP caller gets the 503
  rather than its own timeout.

It ships with or before the predicate change. The spaces stream opens no
space and is unaffected.

### 3.5 The subscription

`apicore.ObjectSearchService.OpenObjectSearch` on the tech space id, with
the list's one filter (`resolvedLayout == spaceView`), `Keys: spaceViewKeys`,
no sorts. The adapter adds `id`, `name`, `type`, `discussionId`
(`objectsearchadapter.go:38-43`); a change to a key the row does not serve
produces an engine update and no event, because the stream emits only what
changes the served set.

It is filtered by neither liveness nor grant:

- A grant edit is a re-filter of the in-memory view map: no resubscribe, no
  window with neither subscription open.
- A status transition arrives as an update carrying details
  (`objectsearchadapter.go:173-206`), which is what lets the stream give a
  removal reason. An engine removal carries only the id
  (`objectsearchadapter.go:207-215`, `core/objectsearch.go:36-47`); under this
  filter that happens only when the view itself is deleted or archived, or
  stops being a space view — reason `gone`.
- "Every space view" means every view the list's query starts from: the
  subscription compiles the same filter through the same `database.NewFilters`
  (`core/subscription/service.go:390-399`), so the snapshot and the list read
  one record set, default filters included.

Every input — an engine batch or a grant re-read — triggers a recomputation: `visibleSpaces` over the current view map and grant, diffed
against what was sent.

The guard stays in the v2 service: the adapter accepts any space id
(`objectsearchadapter.go:63-66`); this path hands it the tech space with a
fixed filter and fixed keys, and no caller input reaches that query — the
standing of `ListSpaces`' own tech-space read (`discovery.go:82`). The
adapter is unchanged. One subscription per stream over at most a few hundred
views; a shared hub (the `spacechathub.go` shape) is not worth it under a cap
of 16.

### 3.6 Removal reasons

For a space id that left the visible set, the first match wins:

1. `access_removed` — `util.SpaceGrantRefusal(grant, id, tech) != ""`, the
   check `visibleSpaces` ran. First, so a space the key lost never reveals its
   later state.
2. `gone` — no view of the target is left in the map, or every one left is
   `gone`.
3. `unavailable` — otherwise.

| view state | written by | class |
|---|---|---|
| account `SpaceDeleted` (7) | the `SpaceDelete` RPC, leave and delete alike (`core/space.go:26-28` → `space/service.go:223-229`); removal from the ACL (`aclobjectmanager.go:386-388, 427-429`); an observed remote deletion (`space/service.go:491-503`); a rejected join request (`joiner.go:54-58`) | `gone` |
| account `SpaceRemoving` (10) | leaving in progress | `gone` |
| local `Missing` + remote `RemoteDeleted` (one write) | `spaceloader.go:160-162` | `gone` |
| local `Missing` after offloading | `spaceoffloader.go:112`, which runs only for account Deleted or Removing (`shareable.go:85-95, 135-142`) | `gone` (by the account row) |
| view deleted or archived (engine removal) | — | `gone` |
| local `Missing` + remote `RemoteWaitingDeletion` | deletion pending on the network (`spaceloader.go:157-159`) | `unavailable` |
| local `Missing` | a load that failed for good (`spaceloader.go:169-170`) | `unavailable` |
| account `SpaceJoining` (9) | `spacefactory.go:226`, `join.go:25` | `unavailable` |
| any other local value (`Error` and above) | never written as a local status (`spaceinfo.go:13-16`) | `unavailable` |

`Loading` is no longer a removal cause (§3.3). `unavailable` now means a
space the account still holds that did not load on this device — the load
failed, or the network deletion is pending — or one the account is joining;
it can return at the next start.

Why three reasons and no more: `SpaceDeleted` is written the same way for a
delete, a leave, a removal from the ACL and a declined join, so no status can
tell them apart, and a reason claiming to would be a guess. `gone` means "this
account no longer holds the space"; a re-join brings it back as
`space_added` (the view's statuses are reset, `spacefactory.go:349-357`).

### 3.7 The tech space

It inherits the list's rule through `visibleSpaces`: an all-spaces grant
never covers it (`util/grant.go:107-118`), that refusal never sets
`has_not_granted_spaces` (`discovery.go:114-116`), and an explicit grant
admits it exactly as the gate does. It has no space view of itself
(`discovery.go:104-112`), so it does not appear; the fixture in §7 gives it
one so that the rule is tested rather than assumed. Subscribing to the tech
space's views is not access to the tech space, for the reason in §3.5.

### 3.8 `has_not_granted_spaces`

Decision: on `snapshot_complete`, and re-sent as `not_granted_updated` when a
recomputation flips it. A snapshot-only flag is wrong after the first change
with no way for the client to tell — a grant widened to every space, or a new
space the key is not granted, flips it with no row event. It falls out of
`visibleSpaces` at no cost. What a flip discloses: that the set of hidden live
spaces became empty or non-empty, when it happened. A poller learns the same
bit later; no id, name or count, as on the list (`v2/model/model.go:271`).

## 4. Grant changes on an open stream

### 4.1 What the code provides

- Edit (`LinkLocalUpdateApp`): persists the grant (`core/application/sessions.go:444`),
  collects the key's session tokens under `appSessionsLock` (`:457-462`), calls
  `RevokeToken` per token (`:463-465`). Sessions stay alive. A failed persist
  returns before the sweep (`:444-446`).
- Revoke (`LinkLocalRevokeApp`): deletes the link file (`:511`), closes and
  untracks every session under the lock (`:523-533`), calls `RevokeToken` per
  token (`:538-542`).
- `Server.RevokeToken(token)`: `evictGen++` and deletes every cached entry
  carrying that **session token** (`server/server.go:226-235`).
- A request carries the entry it authenticated with — `Token`, `Grant`,
  `ExpireAt`, `KeyId` (`server/middleware.go:172-180, 219`).

The signal does not say edit or revoke; the stream learns which by
re-reading.

### 4.2 Registration

`Server` gains `watchers map[sessionToken]set[*keyWatch]`, guarded by
`srv.mu` — the mutex of `KeyToToken` and `evictGen`. `RevokeToken`, inside
the critical section of its sweep, does a non-blocking send on each of that
token's watchers' capacity-1 `changed` channel. Signals coalesce; the stream
consumes the signal before it re-reads, so any number of sweeps yields at
least one re-read that sees the last state.

Keyed by token, and complete: the stream's token was minted by
`mintAppKeySession` and tracked under the key's hash (`sessions.go:141-153,
181-190`); only `WalletCloseSession(token)` (`:208-222`) or a revoke untracks
it, and the HTTP layer never hands its session token to a client. So every
later edit and the revoke sweep this token. Keying by `KeyId` would need the
hash at `RevokeToken`, which receives only the token, and the swept entry that
carries the hash may be gone already. A hash-keyed signal from
`core/application` is possible but buys nothing while the invariant above
holds.

### 4.3 Re-reading the key

`keyWatch.Current(ctx)` re-mints through the call the auth middleware makes,
`WalletCreateSession` with the bearer key (`middleware.go:158`), skipping the
cache read and caching the result under the same eviction-generation rule
(`:182-194`). That block becomes one `Server` helper both call.

| mint answer | stream outcome |
|---|---|
| `NULL` | new grant and `ExpireAt` |
| `APP_TOKEN_NOT_FOUND_IN_THE_CURRENT_ACCOUNT` | `ErrKeyRevoked` → `stream_closed{key_revoked}` |
| `APP_TOKEN_EXPIRED` | `ErrKeyExpired` → `stream_closed{key_expired}` |
| anything else (e.g. `UNKNOWN_ERROR` while the account stops, `core/wallet.go:57-62`) | `ErrKeyUnverified` → the end, no event |

Why the mint and not a new wallet port: it is the gate's own derivation of a
key's grant, so the stream and the next request cannot disagree; its
`ReadAppLink` runs under `appSessionsLock`, serialized with the revoke's sweep
(`sessions.go:135-153`); and a read by hash cannot open a legacy v0 link file
— `list` verifies only v1/v2 envelopes (`core/wallet/applink.go:584`) — so the
key string is needed either way. The watch already holds it: it is the bearer
the request carried. Cost: one tracked session per re-read, i.e. per edit per
open stream — what a cache miss costs.

The v2 layer never sees the key. It sees
`KeyWatch{State() KeyState; Changed() <-chan struct{}; Current(ctx) (KeyState, error); Closing() <-chan struct{}; Close()}`
with `KeyState{Grant *util.ApiGrant; ExpireAt int64}`, declared in
`v2service` (the consumer) and implemented in `server`, which keeps the
dependency direction `server → v2`.

### 4.4 Races

**R1 — an edit between the request's cache read and the registration.** The
middleware snapshots `evictGen` in the critical section of its cache read
(`middleware.go:152-155`) and today drops it once the mint is cached. It now
rides the gin context, and `WatchKey` compares it with `evictGen` under
`srv.mu` while registering:

- unequal → an eviction happened since the grant was read → `Current()`
  before the subscription opens; a revoked or expired key is the 401 of §2.5;
- equal → the grant is current. On a cache hit, an entry can sit in the cache
  with a stale grant only if an edit's sweep raced its mint, and the
  mint-generation rule refuses to cache exactly that (`middleware.go:182-194`);
  a sweep after the read bumps the generation. On a miss, the snapshot
  precedes the mint's read, and an edit persisted after that read sweeps after
  it (persist before sweep, `sessions.go:444, 463`): before registration it
  bumps the generation, after it signals the watcher.

The comparison is conservative: `evictGen` counts every eviction of every
key, the per-request expiry delete included (`middleware.go:203-206`), so a
mismatch can be another key's. That costs one redundant re-read, never a
missed edit. Doing the re-read before the snapshot also means the snapshot
never carries a space the key lost before the stream existed.

**R2 — an edit during a re-read.** Its sweep signals the registered watcher;
the channel holds the signal; the next iteration re-reads.

**R3 — edit against revoke.** The wallet's `appLinkMu` lets a racing edit
either finish before the delete or fail with `ErrAppLinkNotFound`
(`core/wallet/wallet.go:41-49`), and a failed edit returns before its sweep.
Every revoke ends with a sweep that follows its delete (`sessions.go:511` →
`:538-542`). Each re-read reports the link file as it is at read time, and
the last signal precedes the last re-read, so the stream ends on
`stream_closed{key_revoked}` whatever order the two sweeps arrive in. After
the end the watcher is unregistered; a late sweep finds nothing.

**R4 — the unscoped key.** A nil-grant key registers like any other. An edit
can scope a legacy key or clear a grant (`sessions.go:428-434`,
`applink.go:249-254`); the re-read's grant, nil included, replaces the
stream's, and `visibleSpaces` treats nil as unscoped (`discovery.go:113`).

**R5 — expiry.** `ExpireAt` comes from the entry and from every re-read. The
stream ends at the first instant the middleware refuses
(`now.Unix() > ExpireAt`, `middleware.go:199`); one predicate,
`util.KeyExpired(expireAt, now)`, serves both. The timer only bounds latency:
the worker re-checks the wall clock at every wake and at least every 60 s,
because a monotonic timer does not run during system sleep and a key can
expire across a closed laptop lid.

**R6 — rebind and stop.** `ReassignAddress` builds a new `Server`
(`core/api/service.go:342-355`, `bindLocked :233-249`) and `RevokeToken`
reaches only that one (`:357-365`). The old server's streams keep running:
`http.Server.Shutdown` does not interrupt active connections and its deadline
here is 1 ms (`service.go:37, 316-326`), so an old stream would never hear a
revoke. `Server` gains `CloseStreams()`, closing the channel every `keyWatch`
exposes as `Closing()`; `apiService.shutdownLocked` calls it before
`Shutdown`, under `s.lock` — the lock `RevokeToken` takes to find the server.
A revoke therefore reaches the old server's watchers or arrives after they
were told to end.

## 5. Routing, gates, middleware, wrapper

- **gin.** Probed against v1.10.0 (`go.mod:36`): `GET /v2/spaces/stream`
  reaches the stream with an empty `space_id`; `GET /v2/spaces/streamx`
  reaches GET-one; `GET /v2/spaces/stream/objects` and
  `…/stream/chats/stream` reach their `:space_id` routes (gin backtracks);
  `PATCH /v2/spaces/stream` reaches `update_space`. Only GET-one for the
  literal reference `stream` is shadowed. Two places can produce it:
  - a served short reference: tails are `[a-z2-7]` and the last character's
    eight values include `m` (`spaceref.go:43-49`), so `stream` is a possible
    tail, about 2⁻²⁸ per space. `shortSpaceRefs` keeps a space whose tail is a
    reserved static segment (`{"stream"}`) at its full id, so every served
    spelling routes;
  - the in-heart full wrapper: `get_space {"space_id":"stream"}` passes
    `onePathSegment` (`wrapper/full/executor.go:256-261`) and would open this
    stream over the buffered in-process transport, holding a stream slot
    until the caller's deadline. The executor refuses an assembled method and
    path equal to another operation's literal path.
- **`resolveSpaceRef`**: no `space_id`, passes through (`v2/spaceref.go:39-43`).
- **Authorization**: `{Read, GlobalServiceFiltered}` beside `GET /v2/spaces`
  (`v2/authz.go:114`). The gate's no-space branch admits a service-filtered
  route for every grant (`authz.go:236-243`) and the read verb passes
  read-only grants. The class is accurate: the service picks the space set.
- **`?ids=`**: validated by `ensureIdsShape`; `compact` refused by the
  handler (§2.1).
- **Analytics**: `V2StreamSpaces`. `ensureAnalyticsEvent` fires after
  `c.Next()` (`middleware.go:379-396`), i.e. once, when the stream ends, with
  200 — as for the other streams.
- **Rate limit**: none; a GET read takes no write limiter. The cap bounds it.
- **Cap**: `maxConcurrentSpacesStreams = 16` on its own slot counter, so this
  stream and the search and chat streams cannot starve one another
  (`search_stream.go:24-27`); the 429 is worded like `search_stream.go:95-103`.
- **Registration**: iff `ObjectSearch` is wired — the search stream's flag
  (`v2/router.go:231`).
- **In-process transport and wrappers**: HTTP only. The full tier needs
  `"stream_spaces": {Exclude: reasonStream}` (`wrapper/full/overlay.go:36,
  46-48`); without it the derivation turns the route into a tool, and the
  conformance test refuses an operation neither derived nor listed. The
  curated wrapper calls no stream (`wrapper/routes.go:16-31`); `/mcp` pushes
  nothing (`server/mcp.go:446-462`).

## 6. Component changes

| file | change |
|---|---|
| `v2/model/space_stream_event.go` (new) | event and reason constants, `SpaceStreamEvent` |
| `v2/model/ref.go` | `OpStreamSpaces = "stream_spaces"`, row `{"GET", "/v2/spaces/stream"}` |
| `v2/service/discovery.go` | `spaceViewKeys`, `visibleSpaces`; `liveSpaceRowsWithGrantInfo` = query + project + `visibleSpaces` |
| `v2/service/space.go` | `spaceViewState` with Loading in the live set; `isLiveSpaceView` through it; `spaceRemovalReason`; the comments at `:46-52` and `discovery.go:24-31` |
| `v2/service/spaceref.go` | reserved tails in `shortSpaceRefs` |
| `v2/service/spaces_stream.go` (new) | `KeyWatch`, `KeyState`, `ErrKeyRevoked/Expired/Unverified`; cap; `OpenSpacesStream(ctx, KeyWatch)`; `SpacesStream` with a worker goroutine merging engine changes, `Changed()`, `Closing()` and the expiry timer into one output queue; `Snapshot/Ready/Drain/Close` |
| `v2/service/service.go` | `spacesStreams` slot counter; an injectable clock for the expiry timer; `ensureSpaceLoaded` and `spaceLoadWait`; `ensureSpaceWrite` calls it; `ensureSpaceRead` for the live reads |
| `v2/service/space.go`, `search.go`, `v2/handler/discovery.go`, `object.go`, `chat.go` | `LoadingSpaceWarnings` and the one wording of the warning; the searches append it; `storeListResponse` adds it to the store-read lists |
| `v2/service/chat.go`, `space.go`, `object.go`, `list_read.go`, `widgets.go`, `file_download.go` | `ensureChat` (and through it `ensureChatWrite`), `UpdateSpace`, `GetObject` (which the type-document read reaches, `discovery.go:286`), `readListTarget`, `ListWidgets` and `GetFileContent` call `ensureSpaceLoaded`; the store-read lists and search add the loading warning |
| `core/api/core/core.go`, `core/api/service.go`, `server/server.go` | the `SpaceLoads` port, its package-`api` implementation over `space.Service.Get`, `V2Deps.SpaceLoads` into `v2service.NewService`; `.mockery.yaml` + `make test-deps` |
| `v2/model/model.go`, `v2/handler/error.go` | `CodeSpaceLoading` and its 503; `Retry-After` set where the error becomes bytes |
| `v2/handler/spaces_stream.go` (new) | handler + annotations; the `ids` refusal; the search stream's loop (`v2/handler/search_stream.go:35-79`), returning after a drained `stream_closed` |
| `v2/router.go` | `RouteDeps.WatchKey func(*gin.Context) (v2service.KeyWatch, bool)`; the route |
| `v2/authz.go` | the authorization row |
| `server/server.go` | `watchers`, the signal in `RevokeToken`, `CloseStreams`, `keyWatch`, the shared mint-and-cache helper |
| `server/middleware.go` | the helper; the observed generation on the gin context; `util.KeyExpired` |
| `server/router.go` | `WatchKey: srv.watchKey(mw)` in `RouteDeps` |
| `core/api/service.go` | `shutdownLocked` calls `s.srv.CloseStreams()` before `Shutdown` |
| `util/keystatus.go` | `KeyExpired` |
| `wrapper/full/overlay.go`, `executor.go` | the exclusion; the shadow guard |
| `scripts/fix_openapi_v2.py` | `stream_spaces` in `STREAM_OPERATIONS` (`:320`; the 429 follows, `:327`); a 503 policy for the operations that call `ensureSpaceLoaded` |
| `v2/openapi_responses_test.go` | 59 → 60 operations (`:109`); the response-pair golden (`:226`) grows by the stream's five (200, 400, 401, 403, 429) and one 503 per gated operation — take the number from the generated document; `stream_spaces` in `resourceLimited` (`:255`); the gated set pinned |
| `v2/handler/discovery.go:16`, `v2/handler/space.go:26` | descriptions: a loading space is listed and served |
| `v2/markdown/api.md` | a "Watch your spaces" section after "Watch a search" (`:339`) and a task-table row (`:57-65`); the space-reference section (`:106-118`) notes the stream serves full ids; the shared errors say a loading space can answer 503 `space_loading` |
| `v2/SKILL.md` | a row in "Which operation" (`:64`) and a bullet with the cap |
| `core/api/CLAUDE.md`, `APIV2.md` Phase 7, `docs/superpowers/specs/2026-09-18-full-wrapper-in-heart-design.md:177` | pointers; the exclusion table |

Then `make openapi`, never `go mod tidy` (it drops the pinned
`swaggo/swag/v2` the generator needs). The handler description stays under
400 characters and free of section marks and constraint numbers
(`core/api/openapiprose_test.go:34, 43-60`); the event list belongs in
`api.md`, which `TestV2DocumentProse` pins byte for byte (`:140-151`).

## 7. Test plan

A test whose fixture cannot express the failure it guards proves nothing
(the rule of the removed APIV2.md §8.38/§8.40, §10). For this feature that
means:

- **Shaped space ids** (`<59-char base32 cid>.<base36>`, `spaceref.go:74-91`).
  The existing grant fixtures use `spaceA`-style ids
  (`server/grant_gate_test.go:31-40`), which never shorten, so a stream
  serving short ids would pass every assertion on them. The fixture includes
  two ids sharing a tail and asserts that default `GET /v2/spaces` differs
  from `?ids=full` — a self-check that the spelling axis is live.
- **Statuses in the shapes their writers write**: `Missing` + `RemoteDeleted`
  in one write, deletion as `isDeleted` (an engine removal with no details),
  legacy views with no status at all.
- **Every divergence class present**: a `Loading` view; a deleted and an
  archived view (the default filters); two views of one target with the dead
  one sorting first; a live view whose `targetSpaceId` is the tech space id.
- **A control on each side of a rule**: wherever Loading is asserted live, an
  `Ok` view and a `Missing` view sit beside it, so a surface that admits
  everything or nothing cannot pass.

| test (file) | pins | how its fixture can fail |
|---|---|---|
| `TestSpaceViewState` (`service/liveness_test.go`) | liveness over every (local, account, remote) status triple; the reason table of §3.6 | The liveness oracle is the literal rule (`local ∈ {0,1,2} ∧ account ∈ {0,8}`) written in the test. Comparing `isLiveSpaceView` with `spaceViewState` would be tautological once one is defined through the other; the literal fails on any edit that moves a triple in or out of live, Loading included |
| `TestLoadingSpaceIsLive` (`service/liveness_test.go`, beside `TestSpaceViewState` and `TestVisibleSpaces`) | one fixture — a Loading, an `Ok` and a `Missing` view — through the list, GET-one, `ensureSpace` (a nested list), short-reference resolution and the global-search fan-out: Loading answers exactly as `Ok`, `Missing` as today | the two controls rule out a surface that admits everything or nothing; a surface that kept a private predicate disagrees on the Loading view. No existing test needs to change (§3.3) |
| `TestEnsureSpaceLoaded` (`service/service_test.go`, a fake `SpaceLoads` blocking on a channel, `spaceLoadWait` shortened) | loaded → passes at once; still loading at the bound → 503 `space_loading` with `Retry-After`; a load error → mapped as the route maps it today; a cancelled request → its own error, not 503 | the fake blocks until its context ends and the test has a deadline, so an unbounded wait fails the test instead of hanging |
| `TestWritesIntoALoadingSpaceAreBounded` (service) | `PATCH /v2/spaces` (`WorkspaceSetInfo`), a type and a property update (`ObjectSetDetails`), an object create, patch and delete, a chat write, and every gated read (object, type document, chat messages, query objects, widget list, file content) on a space whose load blocks: each answers 503; a dry-run space update, a store read and a refused key do not wait | the writes are the paths that drop the context (§3.4); the strict `mw`, creator, reader and widget mocks fail on any call, so an entry point without the gate fails here rather than in production |
| `TestDryRunsAndRefusalsDoNotWaitForALoad` (service) | on a space whose load blocks: a dry run of each index-validated write returns at once; a real write refused for its own content returns its 400 at once; a real write whose refusal would come from the index (an unknown type or property, a taken key, an unknown collection item) answers 503 after the bound, and the same request as a dry run answers its 404 or 400 at once | the no-wait cases run with the bound an hour away under the test's own 5 s deadline, so a gate placed before validation fails them; the index cases run both ways, so a gate missing before the index read, or one that also stops dry runs, fails one side |
| `TestDryRunsOfALoadingSpaceSayTheIndexMayBeIncomplete` (`service/liveness_test.go`) | a dry run's index-based refusal (unknown type or property, taken key or name, unknown collection item) and its dry-run result carry the loading warning as their last issue on the Loading space, and exactly one issue fewer on the `Ok` space; a bad-body refusal carries none | the `Ok` space holds the same index, so a warning attached unconditionally fails the control and one never attached fails the Loading side |
| `TestStoreReadsWarnWhileLoading` (`service/liveness_test.go` for the warning itself and both searches; `handler/discovery_test.go` for the list envelopes) | a list and a search on the Loading space carry the warning; on the `Ok` control there is none; a global search names only loading spaces in its fan-out | the control rules out an unconditional warning; a key granted the loaded space alone gets none, so a loading space it cannot see is not disclosed |
| `TestVisibleSpaces` (`service/liveness_test.go`) | rows and flag under nil, all-spaces read, all-spaces readwrite, a narrowed list naming dead and tech spaces, an explicit tech grant | each case runs over every input order of the duplicate-target pair, so a store-order or map-order tie-break fails; the tech-target view fails an `AllowsSpace`-only grant check; a narrowed case whose only non-granted views are dead fails a flag computed before the liveness check |
| `TestSpacesStreamAgreesWithListAndGate` (`core/api/spacesstream_integration_test.go`, package `api`) | the anti-drift test: snapshot rows = `GET /v2/spaces?ids=full` rows (ids, names, descriptions, icons, order); `snapshot_complete` flag = the list's flag; per fixture id, present in the stream ⇔ GET-one 200, absent and refused by the grant ⇔ 403 `space_not_granted`; after a live phase (Missing → Loading → Ok, Active → Deleted, a view deleted, a view created) and a drain, the stream's held set = the list again | It runs on one real store through `server.NewServer` with the real adapter over `subscription.NewInternalTestService` (package `api` can import `server`; package `server` cannot import the adapter, and a mocked `ObjectSearch` would be fed by the test itself). A subscription filter that opted out of a default filter shows the deleted view on one side; a grant applied with `AllowsSpace` shows the tech-target view under all spaces; a stream or list keeping the old predicate disagrees on the `Loading` view; short ids differ on shaped ids. Expectations are derived from the stream's output only, the discipline of `TestWhoamiAgreesWithTheGate` (`whoami_test.go:249-322`) |
| `TestSpacesStreamLive` (`service/spaces_stream_test.go`, `fakeObjectSearch` of `search_stream_test.go:26`, fake `KeyWatch`) | added, updated, removed per transition; a create's Unknown → Loading → Ok emits one `space_added` and nothing else; Loading → Missing emits `space_removed{unavailable}`; Missing → Loading emits `space_added`; `unavailable` then Deleted emits a second `space_removed{gone}`; `unavailable` then de-granted emits `access_removed` | the create sequence arrives as three separate engine changes, so a stream on the old predicate emits the removal and re-add the §3.3 table rules out; deletion arrives as a detail-less removal, so a classifier that waits for details misnames it; the Missing + RemoteDeleted write arrives as one change, so classifying on either field alone fails |
| `TestSpacesStreamGrantChange` (same file) | narrow → `access_removed` (and the flag flip); widen → `space_added`; nil → scoped; scoped → nil; permission-only change → nothing; `ErrKeyRevoked`, `ErrKeyExpired` → `stream_closed` as the last frame; any other error → the end with no frame | the views cover spaces in the old grant only, the new only, both, neither, and the tech target, so a re-read that kept the old grant or re-filtered from `ctx` leaves a visible difference |
| `TestSpacesStreamExpiry` (same file) + `TestKeyExpired` (`util`) + a middleware boundary case | open at E−2 s; at E still open; at E+1 s `stream_closed{key_expired}`; the middleware admits at E and refuses at E+1 s | both boundary instants are asserted on both sides, so an off-by-one between stream and middleware fails |
| `TestKeyWatchRegistration` (`server/server_test.go`) | own token's sweep signals; another token's does not; a generation older than `evictGen` is stale and re-reads before `WatchKey` returns; `Close` empties the registry; `CloseStreams` closes every `Closing()` | the strict `WalletCreateSession` mock fails a re-read the other-token sweep must not cause; the registry size is asserted, so a leak fails |
| `TestSpacesStreamEditDuringMintIsNotLost` (`server/grant_gate_test.go`) | R1 on the miss path, end to end over `httptest.Server`: the first mint's `Run` hook calls `RevokeToken` and returns `{A, B}`, the second returns `{A}`; the snapshot is A alone and no removal follows | without the generation comparison the snapshot carries B and nothing ever removes it — the sweep preceded the registration. Modelled on `TestGrantEditDuringMintIsNotLost` (`:609`) |
| `TestSpacesStreamEditAndRevoke` (same file) | an edit narrows the open stream and it stays open (a later engine change still arrives); a revoke ends it with `stream_closed{key_revoked}`; a sweep after the end mints nothing; the revoke's signal arriving before the edit's ends the stream all the same | the narrowed grant excludes a space in the snapshot, so a no-op re-read is visible; strict mock call counts catch both a missed re-read and one after the end |
| `TestSpacesStreamEndsOnServerStop` (`server`) | `CloseStreams` ends the response body | the read has a deadline; a stream selecting only on the request context keeps the body open past it |
| handler tests (`handler/spaces_stream_test.go`) | `ids=compact` → 400 at `ids`, `ids=full` → 200; 429 at cap + 1; frames are `event:`/`data:` with no `id:`; the keepalive comment | the 429 case holds the slots through the real slot counter, not a stub |
| shadow guards | `authz_test.go`: `GET /spaces/stream` beside `GET /spaces/:space_id` passes a narrowed read-only grant while `GET /spaces/other` is 403, proving the probe reached the global branch; `spaceref_test.go`: a shaped id with tail `stream` is served full; `executor_test.go`: `get_space {"space_id":"stream"}` is refused with nothing sent | each pairs the guarded case with an unguarded neighbour, so a guard that refuses or admits everything fails |
| goldens | `TestV2RouteAuthzConformance` (registration, credential-less 401), `TestOperationsMatchOpenAPI`, `openapi_responses_test.go` (the stream and the 503 set), `TestV2DocumentProse`, the full-wrapper overlay conformance | each fails on a missing row by construction |

## 8. Out of scope, follow-ups

- **The chat and search streams fix the grant at open** (`v2/service/chat_space_stream.go:56`,
  `chat_stream.go:117`, `search_stream.go:76`): a narrowed key keeps
  streaming a de-granted space. A pre-existing, separate bug. `KeyWatch` is the
  hook that can close them: register at open, re-read on `Changed()`, end the
  stream when its space is no longer granted.
- **Those streams also outlive a rebind or stop** (R6). They can select on
  `Closing()` too.
- **/v1 still excludes a loading space** (`core/api/service/space.go:44-53`);
  not changed here.
- **The context-dropping RPCs of §3.4** (`WorkspaceSetInfo`,
  `ObjectSetDetails`) still wait without bound for every other caller —
  desktop, /v1. The gate bounds v2 only.
- **`spaceUnavailableError` says "deleted, left, or still joining"**
  (`space.go:62-67`) for a space whose load failed too. Pre-existing;
  rewording it changes `service/discovery_test.go:170`.
- **The external `anytype-mcp` bridge** derives its tools from the OpenAPI
  document: it must exclude `stream_spaces` and has the `get_space "stream"`
  shadow over real HTTP.

## 9. Open questions

| question | recommendation |
|---|---|
| Does the external `anytype-mcp` bridge exclude SSE operations by content type, or by a list like `overlay.go`? Not answerable from this repository | check before release; if by list, add `stream_spaces` there with the `get_space` shadow guard |

## 10. Corrections to the working assumptions

No decision is contradicted by the code. One needed addition — closing
streams on rebind (R6). The first revision also held `unavailable` removals
for 5 s to hide a create-time flap; making Loading live (§3.3) removed the
flap, and the hold with it. Corrections:

1. APIV2.md §8.38–§8.40 no longer exist: they were removed by `47f41a69d`
   ("Compact the API v2 and Import v2 documentation"). The fixture rule this
   document applies is in `git show 47f41a69d^:core/api/APIV2.md`, lines
   5631-6272: a fixture whose values read the same in both vocabularies cannot
   fail; fixtures must take the production store shapes; every fix needs a
   test seen to fail on revert.
2. The anti-drift test is `TestWhoamiAgreesWithTheGate`
   (`server/whoami_test.go:249-322`). It pins whoami against the gate, not the
   list; under an all-spaces grant its claimed set comes from the flag, not
   from any enumeration; and its fixture (`registerGrantTestSpace`,
   `grant_gate_test.go:31-40`: unshaped ids, no statuses, no tech-space view)
   cannot express a liveness, spelling or tech-space drift. Nothing pins the
   list against the gate today; the stream joins through a new list-and-gate
   test (§7) rather than by extending that one.
3. "All space views" is every non-deleted, non-archived space view: the
   store's default filters apply to the list's query and to any subscription
   alike (§3.5).
4. An edit sweeps only the tokens tracked for the key, and only after a
   successful persist; a revoke that wins the wallet lock makes the racing edit
   fail before its sweep (§4.4 R3).
5. The list's de-duplication is "first live, granted record in store order",
   not a defined choice (§3.2).
6. No test pins today's exclusion of a loading space (§3.3), and a space
   admitted while it loads is not new: the start fast path already serves an
   `Ok` space that is still building, with the same unbounded waits (§3.4).
7. (Implementation.) The first revision of §3.4 named the object and
   type-document reads as the only live reads. The chat reads, the query and
   collection reads, the widget list and the file download reach
   `spaceService.Get` too; all are gated. So is the template list when its
   type filter names a built-in type the space has not installed (review
   round 1). Pure request validation never waits; a real write waits before
   its first read of space state, the index included; a dry run of a write
   that validates against the index does not wait (review rounds 1 and 2).
