# Space layer contract for the multi-tenant engine

Status: draft for discussion, 2026-09-16. Implementation-agnostic: this document states
what a server-side multi-tenant engine needs from heart's space layer, in terms a fresh
implementation or a fold into the `spacev2` rewrite can both be checked against. A
companion document, `2026-09-16-spacev2-multitenant-requirements.md`, maps these needs
onto the `spacev2` branch as it stands. The research behind both lives in
`docs/superpowers/research/2026-09-16-hosted-api/`.

## 1. Context and scope

The engine is a heart process that serves the hosted API for business organisations.
Decisions already taken:

- One process hosts a shard of orgs: thousands of orgs, tens of thousands of spaces,
  the vast majority cold at any moment.
- Each space is loaded under its **org's identity**. The org identity is the ACL owner
  of every org space by construction in ee-cloud. Keys come from an external registry,
  never from the process wallet.
- Writes are signed **per operation** with the key of the member who owns the API key.
  any-sync validates write permission against the signing key and takes the read key
  from the tree's account key, so a member key signs while the org key decrypts.
- The process has **one peer identity** for all sync traffic. Nodes authorise reads by
  chash responsibility only, not by peer identity. A small lazy pool of per-org
  connections serves the few identity-gated coordinator and filenode operations.
- Freshness contract: "everything synced so far", plus a reported per-space lag.
- Full-text search is required.
- Server profile has **no tech space, no personal space, no marketplace**. The space
  list and each space's status come from ee-cloud.
- Cold spaces are woken by a periodic head-hash poll against the nodes, by inbound sync
  messages, or by an API request.

This document covers the space layer only: everything between "a space id plus an org"
and "a loaded client space with its object cache, tree syncer and sync status". It does
not specify the tree-only sync path, the materialisation queue, the index unload hooks
in objectstore, subscriptions and the indexer, or API authentication. Section 7 names
the seams those adjacent pieces need from the space layer.

## 2. Vocabulary and model

**SpaceRef.** `{SpaceId, OrgId}`. Every request into the space layer names a space;
the org is a property of the space, not of the process. Nothing in the layer may
consult a process-wide "the account".

**Org identity.** The account keys (sign key and derived identity) and account
metadata of an org. Provided per space at load time by an `IdentityProvider`. The
process wallet is not an identity provider for org spaces.

**SpaceSource and StatusSource.** Where the set of spaces and each space's account
status (active, removing, deleted, joining) come from. In the client profile these are
the tech space's space-view objects. In the server profile they are table-backed and
fed by ee-cloud. The space layer depends only on the interfaces.

**Demand.** A request to have a space loaded, with a priority and a lease. Priorities,
lowest first: `Background` (reconcile, backfill), `Wake` (a remote change was
detected), `Interactive` (an API call is waiting). A lease is a soft hold that delays
idle eviction; it is not a pin.

**Hot set.** The loaded spaces. It has a cap. Admission and eviction are policies,
not hard-coded behaviour.

**Lifecycle.** A known space is in exactly one state:

```
              request / wake                     idle timeout / eviction
   Idle ───────────────────────► Loading ──► Loaded ──────────────────────► Unloading
    ▲                              │                                            │
    │            failure           ▼                                            │
    └──────── retry-after ──── Failed                                           │
    ▲                                                                           │
    └───────────────────────────────────────────────────────────────────────────┘
                                keep-data unload, re-armable

   any state ── status = deleted / removed ──► Removing ──► Gone (data deleted)
```

`Idle` means "on disk, not loaded". `Unloading` closes every per-space resource and
keeps storage on disk. A space that returns to `Idle` can be loaded again by the same
registry entry. `Removing` is the only path that deletes data.

**Wake.** A hint that a cold space has changed: an inbound sync message for a space
that is not loaded, a head-hash mismatch from the poller, or a status change from the
source. A wake becomes a `Wake`-priority demand, deduplicated and rate-limited.

**Per-space observability.** State, last state change, last successful sync time,
local heads hash and its age, last error, load count. This is what the API reports as
lag.

## 3. Requirements

Each requirement has acceptance criteria a reviewer can check without reading the
engine.

### S1. Identity is a per-space input

The space app is built with the org's account service. Every signing site inside the
space uses it: ACL operations, new object roots, settings and key-value writes,
participant and metadata derivation. The process wallet is never consulted for an org
space.

Acceptance:
- A test loads two spaces from two orgs in one process and asserts that the ACL
  identity, the signature on a newly created object's root, and the settings tree
  signature use the respective org keys.
- No component registered in a per-space child app resolves the account service from
  the root app. Today three sites still do on `develop`: the object cache is
  constructed with the deps account service but carries a TODO admitting it still gets
  the root account (`space/clientspace/space.go:147-161`); the tech space wrapper has
  no override (`space/clientspace/techspace.go`); the credential provider is root-only
  and is used when pushing a new space to a node (`any-sync
  commonspace/headsync/diffsyncer.go:259`). All three must be closed or bypassed in
  the server profile.

### S2. Demand-driven load, keep-data unload, re-arm

Spaces load only in response to a demand. Unload releases every per-space resource,
keeps storage, and leaves the registry entry ready to load again.

Acceptance:
- Load, unload, load, repeated N times on one space: goroutine count, open file
  descriptors and heap return to the pre-load baseline within tolerance; storage is
  intact; the reloaded space serves the same heads.
- "Complete unload" is an enumerated checklist, not a hope. On `develop` the per-space
  resources are: the any-sync space app (about 17 components), two heart child apps,
  the object cache with its GC goroutine, tree syncer pools per peer, the per-space
  index database (never closed today, `pkg/lib/localstore/objectstore/service.go`),
  the full-text active-space flag, subscription registrations, sync-status registries,
  the type-provider cache, peer-store entries, backlink state. The space layer owns the
  first four and must expose an `OnUnload` hook so the rest can be closed by their
  owners.
- The only existing unload path on `develop` deletes storage
  (`space/internal/components/spaceoffloader`). A keep-data unload must exist and be
  distinct from removal.

### S3. Nothing loads without demand

Knowing about a space costs a registry entry, nothing else. No boot-time enumeration
creates controllers, goroutines, object opens or subscriptions per known space.

Acceptance:
- With 50 000 known spaces and zero demand, the process holds O(50 000) small structs
  and O(1) goroutines attributable to the space layer, and opens no smartblock and no
  per-space database.
- There is no deferred drain that eventually loads everything. On `develop`, lazy mode
  only defers: the backlog drains after ten seconds at concurrency two
  (`space/service.go`, `preloadRemainingSpacesTimeout`). That behaviour is a client
  policy and must be expressible as one, not baked into the layer.

### S4. External space list and status

The space list and per-space status come through interfaces. The server profile
provides table-backed implementations fed by ee-cloud. The client profile provides
space-view-backed implementations.

Acceptance:
- The space layer compiles and runs with no tech space, no personal space and no
  marketplace registered.
- Nothing on the load path requires a space-view object. On `develop` the hard tie is
  `spacestatus.Init`, which calls `GetSpaceView` and fails without one
  (`space/internal/components/spacestatus/status.go:81-93`), and it sits in the status
  child app built before the state machine in every controller.
- Status is a value type with a small set of getters. What the client loses without
  space views (name, icon, participants, notifications, push modes, the client
  space-list RPCs) is documented and is not on the sync or indexing path.

### S5. Load requests with priority, admission and readiness

The registry exposes a request API with priority and lease, a readiness future, a
non-blocking "if loaded" lookup, and a touch to extend a lease. Load concurrency is
bounded globally and fairly across orgs.

Acceptance:
- `Request` returns a future; N concurrent requests for the same space share one load.
- A wake storm of 10 000 spaces does not start 10 000 loads: admission bounds
  concurrent loads and queues the rest by priority, then by age.
- Per-org fairness: one org with 5 000 changed spaces cannot starve an `Interactive`
  request from another org.
- No process-wide serialisation on per-org operations. On `develop` the coordinator
  client serialises `SpaceSign` one-wide for the whole process
  (`space/coordinatorclient/coordinatorclient.go:33`); that must become per-org or
  bounded by a pool.

### S6. Wake-on-change is a first-class input

Inbound sync messages for a space that is not loaded produce a wake, not a silent drop.
A poller can submit a wake with the remote heads hash it observed.

Acceptance:
- On `develop`, `HandleMessage` resolves the space with `Pick` and drops the message
  if the space is not loaded (`space/spacecore/streamopener.go:87-91`). The layer must
  route that case to `Registry.Wake`.
- Wakes are deduplicated per space and rate-limited per org; a wake for a space whose
  local heads hash already matches the hint is a no-op.
- The head-hash probe is cheap on the node side: a single-range `HeadSync` with no
  elements is about 90 bytes each way and the node answers it from its in-memory head
  table without loading the space. The client-side limiter is the node's per-peer rate
  limit, which needs a batched variant or an exemption for the engine's peer. That is
  node work, named here so the space layer's poller is designed around it.

### S7. One injectable per-space build site

The assembly of a client space from its deps (object cache, tree manager, tree syncer,
sync status, key-value indexer, update listener) happens at one site that the engine
can replace. This is where the tree-only sync path and the materialisation listener
are installed.

Acceptance:
- A server-mode builder can be injected without forking the registry or the
  controller.
- The builder receives the `SpaceRef`, the org account service, and a deps struct. It
  does not reach into the root app for either.

### S8. Failure containment

One failing space or org does not block others. Failed loads back off per space with a
retry-after; repeated failures per org open a per-org breaker that sheds `Background`
demand first.

Acceptance:
- A space whose storage is corrupt reaches `Failed` with a retry-after; other spaces
  in the same org keep loading.
- No shared lock is held across a load step that talks to the network.

### S9. Observability per space

The registry exposes a snapshot of every known space's state, timestamps, heads hash
age and last error, and counters for loads, unloads, wakes and evictions.

Acceptance:
- The API's lag field for a space is derivable from the snapshot alone.
- Metrics are labelled by org, never by the process-wide account.

### S10. Bounded overhead

Idle spaces cost a struct. Loaded spaces have a measured, documented cost. There are
no per-space tickers while idle.

Acceptance:
- A benchmark records heap and goroutines for 0, 1 000 and 10 000 known-but-idle
  spaces, and for 1, 10 and 100 loaded spaces of a fixed shape. No such numbers exist
  anywhere today; this benchmark is Phase 0 work.

### S11. Server bootstrap profile

The space service starts without deriving or creating a tech space or a personal space
and without a marketplace entry. Account-level derived ids are not required for org
spaces.

Acceptance:
- Boot with the server profile performs no network call and no storage open until the
  first demand.

### S12. Deterministic shutdown

Close unloads every loaded space within a bounded time, in any order, and does not
wait on pending loads longer than a configured deadline.

## 4. Interfaces

Sketches, not final code. Names are chosen to be read, not to be kept.

```go
type SpaceRef struct {
    SpaceId string
    OrgId   string
}

type Priority int

const (
    PriorityBackground Priority = iota
    PriorityWake
    PriorityInteractive
)

type Demand struct {
    Priority Priority
    Lease    time.Duration // soft hold against idle eviction; zero means default
    Reason   string        // for logs and metrics
}

// IdentityProvider resolves the org identity used to build a space.
type IdentityProvider interface {
    AccountService(ctx context.Context, orgId string) (accountservice.Service, error)
    AccountMetadata(ctx context.Context, orgId string) ([]byte, error)
}

// SpaceSource is the set of spaces the engine is responsible for.
type SpaceSource interface {
    List(ctx context.Context) ([]SpaceRef, error)
    Watch(ctx context.Context) (<-chan SpaceEvent, error) // added, removed
}

// StatusSource is the account-level status of a space.
type StatusSource interface {
    Status(ctx context.Context, ref SpaceRef) (Status, error)
    Watch(ctx context.Context) (<-chan StatusEvent, error)
}

type Status struct {
    Account  spaceinfo.AccountStatus // active, removing, deleted, joining
    Persist  spaceinfo.LocalStatus   // present on disk, missing
}

type State int // Idle, Loading, Loaded, Unloading, Failed, Removing, Gone

type Info struct {
    Ref          SpaceRef
    State        State
    Since        time.Time
    LastSyncAt   time.Time
    HeadsHash    string
    HeadsHashAt  time.Time
    LastError    error
    RetryAfter   time.Time
    Loads, Unloads, Wakes, Evictions uint64
}

type Registry interface {
    // Request asks for the space to be loaded. Concurrent requests share one load.
    Request(ctx context.Context, ref SpaceRef, d Demand) (Future[clientspace.Space], error)
    // IfLoaded returns the space without loading it.
    IfLoaded(ref SpaceRef) (clientspace.Space, bool)
    // Touch extends the lease of a loaded space.
    Touch(ref SpaceRef, lease time.Duration)
    // Wake records that the space may have changed remotely.
    Wake(ref SpaceRef, hint WakeHint)
    // Unload releases resources and keeps data. Removal is a separate, status-driven path.
    Unload(ctx context.Context, ref SpaceRef) error
    Info(ref SpaceRef) (Info, bool)
    Snapshot() []Info
    Close(ctx context.Context) error
}

type WakeHint struct {
    RemoteHeadsHash string // empty when unknown
    Source          string // "stream", "poll", "status", "api"
}

// HotSetPolicy decides admission and eviction. The client profile keeps today's
// behaviour; the server profile enforces a cap.
type HotSetPolicy interface {
    Admit(ref SpaceRef, d Demand, stats HotSetStats) AdmitDecision // admit, queue, reject
    Evict(candidates []Info, need int) []SpaceRef
    IdleTimeout(ref SpaceRef) time.Duration
}

// Admission bounds concurrent loads globally and per org.
type Admission interface {
    Acquire(ctx context.Context, orgId string, p Priority) (release func(), err error)
}

// Builder is the single injectable site that turns a ref plus identity into a space.
type Builder interface {
    Build(ctx context.Context, ref SpaceRef, identity accountservice.Service, deps BuildDeps) (clientspace.Space, error)
}

type BuildDeps struct {
    TreeManager    treemanager.TreeManager
    TreeSyncer     treesyncer.TreeSyncer
    SyncStatus     syncstatus.StatusUpdater
    KeyValueIndexer keyvaluestorage.Indexer
    UpdateListener updatelistener.UpdateListener // engine installs the materialisation listener here
    Storage        spacestorage.SpaceStorage
    OnUnload       []func(ctx context.Context) error // owners of index, subscriptions, fts register here
}
```

Notes on the sketches:

- `Request` returning a future and `IfLoaded` being non-blocking are the two calls the
  API handlers and the poller use. Everything else is policy.
- `Wake` is deliberately not `Request`: a wake may be coalesced, delayed or dropped by
  policy; a request may not.
- `Builder` takes the identity as a parameter so the fix for S1 lives in one place.
  The three known leaks on `develop` are then closed by passing that identity down,
  not by another root lookup.
- `OnUnload` is the seam for S2's checklist items owned outside the space layer.

## 5. Policies

**Load pipeline.** Resolve status; if not loadable, stop. Acquire admission. Resolve
identity. Build. Register unload hooks. Mark loaded. Emit metrics. Every step is
cancellable and the space returns to `Idle` or `Failed` on cancellation, never to a
half-built state.

**Idle eviction.** A loaded space with no lease and no activity for `IdleTimeout`
becomes an eviction candidate. When the hot set is at its cap and a new demand arrives,
`Evict` picks by lowest priority of the last demand, then longest idle. `Interactive`
demand always wins admission over eviction of a `Background`-loaded space; it may queue
behind another `Interactive` load.

**Wake handling.** Coalesce per space over a short window. If the space is loaded,
forward to sync. If idle, compare the hint's remote hash with the stored local hash;
on mismatch or unknown hash, submit a `Wake` demand. Rate-limit per org so one noisy
org cannot occupy the load queue.

**Retry.** Failed loads use exponential backoff with jitter per space, capped. A per-org
breaker opens after a threshold of consecutive failures and sheds `Background` first.

**Client profile.** The same registry, with a `HotSetPolicy` that admits everything,
never evicts, and a `SpaceSource` that lists space views. Today's "load everything at
boot, lazy mode defers ten seconds" is one such policy and is preserved for clients.

## 6. What the server profile removes

| Client profile | Server profile |
|---|---|
| Space list from tech-space space views | Space list from ee-cloud through `SpaceSource` |
| Status from space-view details | Status from ee-cloud through `StatusSource` |
| Identity from the process wallet | Identity per space from `IdentityProvider` |
| Personal space created at first run | None |
| Marketplace static entry | None |
| Load everything, lazy mode defers | Load on demand, capped hot set, idle eviction |
| Inbound message for unloaded space dropped | Wake |
| Space views drive name, icon, participants | Not needed on the sync or index path |

## 7. Seams for adjacent work

These are not specified here, but the space layer must not make them impossible.

- **Tree-only sync path.** The engine's tree manager returns a sync tree without a
  smartblock and installs an update listener that enqueues `(space, object, heads)` to
  a bounded materialisation queue. That listener enters through `BuildDeps`.
- **Index unload.** The per-space index database, the full-text active flag,
  subscriptions and sync-status registries close through `OnUnload`.
- **Coordinator and filenode per-org connections.** Space creation, make-shareable
  and deletion sign with the org identity over a per-org connection. The space layer
  calls these through an interface keyed by org, never through a root coordinator
  client with the process identity.
- **API session to member identity.** The API layer resolves the member's signing key
  and passes it down as a per-operation parameter to the change-push path. The space
  layer does not see member identity.
- **Lag reporting.** `Info.LastSyncAt` and `HeadsHashAt` are what the API reports.

## 8. Acceptance test plan

1. Two orgs, one process: identity assertions on ACL, object root and settings
   signatures.
2. Load, unload, reload cycle: no goroutine, descriptor or heap growth over 100 cycles.
3. 50 000 known spaces, zero demand: memory and goroutine ceiling.
4. Wake storm: 10 000 wakes, admission holds concurrency at the configured bound,
   `Interactive` request from another org completes within its budget.
5. Corrupt storage on one space: `Failed` with retry-after, org's other spaces load.
6. Shutdown with 100 loaded spaces and 50 pending loads completes within the deadline.

## 9. Open questions

1. Should `Lease` be a soft hold or should there be a hard pin for long API operations
   such as export? A hard pin reintroduces the "reaped mid-operation" class of bugs
   the sidecar has today, so the default here is soft.
2. Per-org breaker thresholds and the cap on the hot set are deployment tunables; who
   owns them, config or ee-cloud?
3. Does the client profile keep a separate controller-per-space model or adopt the
   same registry with a permissive policy? This document assumes one registry, two
   policies.
4. Whether nodes' lack of read authorisation by peer identity is a stable property of
   the network. If it changes, the single-peer-identity assumption changes with it and
   the space layer would need a per-org peer pool as a build dep.
