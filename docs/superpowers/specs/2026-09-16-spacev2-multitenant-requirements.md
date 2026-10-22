# What a multi-tenant server engine needs from the space layer (mapped onto spacev2)

Status: draft for discussion, 2026-09-16. This is the implementation-anchored companion
to `2026-09-16-engine-space-layer-contract.md`, which states the same needs without
reference to any implementation. Read the contract first for the model; read this for
what to change on `origin/go-7348-spacecontroller-refactor` and how much it costs.

Mapping between the two documents:

| This document | Contract |
|---|---|
| R1 per-space identity | S1 |
| R2 demand load, idle unload, re-arm | S2, S12 |
| R3 external status and space list | S4, S11 |
| R4 wake-on-change | S6 |
| R5 per-space build site | S7 |
| R6 bounded overhead for cold spaces | S3, S10 |
| §4.7 admission, §4.5 demand policy | S5, S8 |
| §4.11 observability | S9 |

Research reports cited below live in `docs/superpowers/research/2026-09-16-hosted-api/`.


Audience: the author of `space/spacev2` and the team.
Companion: `docs/superpowers/research/2026-09-16-hosted-api/spacev2-review.md` (the branch review this derives from).

`space/...` refs are into `origin/go-7348-spacecontroller-refactor` unless marked
`[develop]`. Claims are **VERIFIED** (code read) unless marked **INFERRED**.

Branch drift, relevant to §5: merge-base `61ff89220`, **228 commits behind
develop**. `space/clientspace` and `space/techspace` are identical between branch
and develop tip; `space/spacecore` is not (develop added `techspacekey.go`,
`reexchange.go`, `spaceexchange.go`, peermanager/provider changes, a
`PullObserver` in `commonspace.Deps` — ~3.7k lines under `space/`). Five files
develop has since modified were deleted by the branch (`space/service.go`,
`space/init.go`, `space/spacewatcher.go`,
`space/internal/spaceprocess/loader/loader.go`, `space/dedupqueue/`). VERIFIED.

---

## 1. Purpose and scope

**Engine context (decided).** One heart process per shard of orgs; thousands of
orgs, tens of thousands of spaces, mostly cold. Each space is loaded under its
**org's identity** — a per-space `accountservice.Service` shadowing the process
wallet, chosen from an external registry — and writes are signed per operation
with a member key. Sync uses **one process peer identity** (nodes do not
authorize reads by peer identity); a small **lazy pool of per-org connections**
serves identity-gated coordinator/filenode ops. Freshness contract is **"synced so
far" plus a reported per-space lag**. Full-text is required. **No tech space, no
personal space, no marketplace**; space list and status come from **ee-cloud**.
Cold spaces wake on a periodic head-hash poll or on inbound sync messages. The
engine is built on the spacev2 branch.

**Scope.** What the engine needs from the space orchestration layer, and how each
need maps onto a concrete change. Adjacent work is named, not designed:

| Adjacent area | Seam the space layer must provide |
|---|---|
| Tree-only sync (`treemanager.GetTree`, materializer) | per-space selection of objectcache / treemanager / treesyncer at build time |
| Keep-data unload outside `space/` (objectstore, anystoreprovider, subscription, crossspacesub, indexer, syncstatus) | an ordered "unloading / unloaded" event around `Backend.Unload` |
| Per-operation member signing | the per-space account service (R1) is the carrier |
| API auth / org routing | `(orgId, spaceId)` resolvable without loading the space |
| ee-cloud client | implements `SpaceListSource` / `StatusSource` (§4.3) |
| Per-org connection pool | consumes the per-space identity; the space layer must stop assuming one wallet |

---

## 2. The engine's model of a space

**Space ref** — `(orgId, spaceId)`. `orgId` is not derivable from `spaceId` and is
not stored in the space; every space-layer API taking a bare `spaceId` must
resolve the org without loading.
**Identity per space** — the account service the space is built with; determines
the ACL signing key, participant id and owner metadata.
**Demand** — "should this space be resident" (exists today as `wanted`).
**Priority** — ordering for when a requested space loads and which is evicted
first (does not exist).
**Status source** — authority for account status; SpaceView today, ee-cloud in
server mode.
**Hot set** — bounded resident set, capped by count and (INFERRED, to be measured)
aggregate object-cache size.
**Lifecycle states** — as on the branch: `Idle` = unloaded, data on disk, distinct
from `Offloaded` = data deleted.
**Wake source** — anything raising demand for a cold space.
**Per-space lag** — `now − lastSuccessfulHeadSync`, plus whether the last compared
head hash differed. Must be readable **without loading the space**.

```
                    demand raised (API / wake / policy)
                                  │
        ┌────────── Idle ◄────────┴──────────┐
        │  (data on disk,                    │ Unload  (keep data;
        │   nothing resident)                │  OnSpaceUnload; ocache evicted)
        │           │ Load (admission-gated) │
        │           ▼                        │
        │       Loading ──error──► Idle      │
        │           │ ok                     │
        │           ▼                        │
        │        Loaded ─────────────────────┘
        │           │   demand dropped (idle evictor / cap)
        │           └── lag tracked here; head-sync runs here only
        │
        ├── status=Deleted ──► Offloading ──► Offloaded (data deleted)
        │                                          │
        └───────────── status back to Active ──────┘

Closed: terminal, service shutdown only.
```

Two properties the engine depends on: `Loaded → Idle → Loaded` is a normal
repeatable cycle, and `Idle` costs ~nothing (no working goroutine, no open
smartblock, no timer, no subscription).

---

## 3. Needs R1–R6

### R1 — Per-space identity injection at load time
**Statement.** A space must load with an `accountservice.Service` chosen per
space, covering the ACL identity, the key signing new object roots, the
participant id, the owner metadata, and the credential for node-side registration.
**Why.** Changes in an org's space must be attributable to that org's member; a
registered space must carry that org's receipt. One process wallet makes all orgs
indistinguishable on the wire and cryptographically wrong.
**Acceptance.** Two spaces in one process sign with different keys
(`AclState().Key()`); a newly created object's root is signed by the org key
(today `objectcache/tree.go:36` uses the process key); `MyParticipantId(spaceId)`
differs; no load-path code reads the process wallet for a non-tech space.

### R2 — Load on demand, idle unload keeping data, re-arm
**Statement.** Unload while keeping storage and indexes on disk, reload later,
arbitrarily often; no boot-time load of every space and no mechanism that
eventually demands all of them.
**Why.** Tens of thousands of spaces, a hot set of hundreds; the resident set must
be bounded independently of how many spaces exist.
**Acceptance.** N unload/reload cycles leave no growing per-space state
(goroutines, fds, map entries); boot with 10 000 on-disk spaces loads only the
hot set and no timer or drain raises demand for the rest; after unload the
space's `store.db`/`objects.db`/`crdt.db` handles are closed (adjacent work, but
the space layer must signal it); reload yields a working space including
full-text.

### R3 — Status and space list from an external source
**Statement.** Responsibility set and per-space account status come from ee-cloud;
no tech space, personal space or marketplace.
**Why.** A tech space holding 50 000 SpaceView trees is a scalability wall on its
own — boot-time load on the critical path under a 15 s deadline, head-sync
diffing all elements, each SpaceView its own derived tree (mechanism VERIFIED,
magnitudes INFERRED — `docs/superpowers/research/2026-09-16-hosted-api/space-lifecycle.md` §3.4`).
**Acceptance.** Orchestration runs with a status source doing no objectstore or
smartblock access; no load-path component calls `techspace.GetSpaceView`; a space
add/remove in ee-cloud needs no local object write; a space learned about 100 ms
ago with no local record can be loaded.

### R4 — Wake-on-change
**Statement.** An inbound head update for a non-resident space, and a poll result
showing divergence, must both enqueue a prioritised load request without blocking
the sync or poll path.
**Why.** Cold spaces must become fresh without being resident; today such updates
are dropped.
**Acceptance.** `streamOpener.HandleMessage` for a non-resident space queues a
load rather than erroring (today `space/spacecore/streamopener.go:87-91` calls
`Pick`, which does not load — `space/spacecore/service.go:218-224`); enqueue is
non-blocking and bounded, so 10 000 wakes do not start 10 000 loads; a caller can
ask "is it resident / when will it be" without forcing a load.

### R5 — Per-space selection of the object/sync machinery
**Statement.** Which objectcache, treemanager and treesyncer a space is built with
must be selectable per space or per deployment profile at build time.
**Why.** The engine replaces `treemanager.GetTree` and adds a materialization
queue; client and server profiles must coexist in one binary during migration.
**Acceptance.** A server profile supplies a different `clientspace.SpaceDeps`
without touching orchestration; the choice is made in one place and is testable by
substitution.

### R6 — Bounded per-space overhead for cold spaces
**Statement.** A known-but-cold space costs O(100 bytes), no working goroutine, no
open smartblock, no ticker, no network subscription.
**Why.** 50 000 × (goroutine + open SpaceView smartblock) is the difference
between a process that boots and one that does not.
**Acceptance.** With 50 000 known spaces and a hot set of 200, goroutine count is
O(hot set); boot does no per-space I/O for cold spaces; enumerating all known
spaces touches no per-space locks.

---

## 4. Recommendations, in priority order

**small addition** = mechanical, contained in `space/spacev2`; **design change** =
alters an interface other code depends on. Days are for one engineer familiar with
the area, excluding review/QA.

### 4.1 Backend factory injection — P0, small addition, 0.5 d
**(a)** `newController` hardcodes the production backend
(`space/spacev2/service.go:311-316`). `Backend` is already exported and
well-specified (`space/spacev2/controller.go:15-41`).
**(b)**
```go
// BackendFactory builds the Backend for one space. Must be cheap and
// non-blocking: it runs under the registry lock.
type BackendFactory interface {
	NewBackend(spaceId string) (Backend, error)
}
```
**(c)** Small addition, 0.5 d.
**(d)** Unblocks every other load-path variation: server-mode backend (R3, R5),
tree-only backend, per-org identity (R1). Cheapest change with the highest
downstream leverage.

### 4.2 Per-space deps resolver, and the three identity gaps outside spacev2 — P0, 0.5 d inside + 5–8 d outside
**(a)** One shared `*BackendDeps` is built in `Run` and handed to every backend
(`space/spacev2/service.go:213-227`, `backends.go:91-100`). Identity reaches the
space at one site:
```go
// space/spacev2/backends.go:396-422 (buildClientSpace)
if guestKey != nil {
    ctx = context.WithValue(ctx, spacecore.OptsKey, spacecore.Opts{SignKey: guestKey})
}
sp, err := clientspace.BuildSpace(ctx, clientspace.SpaceDeps{
    AccountService:  b.deps.AccountService,  // backends.go:410 — the ROOT service
    PersonalSpaceId: b.deps.PersonalSpaceId, // backends.go:411 — root-derived
    …
})
```
The guest path is the working precedent: `spacecore.Opts{SignKey}`
(`space/spacecore/service.go:55-58`) is consumed at `:263-275`, wrapped in
`customAccountService` (`space/spacecore/account.go:9-23`) and registered into the
per-space child app, shadowing the root service for `syncacl`, `settings`,
`keyvalue`, `aclclient`. VERIFIED.
**(b)**
```go
// SpaceDepsResolver supplies the identity-bearing dependencies for one space.
// The default returns the process-wide values.
type SpaceDepsResolver interface {
	For(ctx context.Context, spaceId string) (SpaceDeps, error)
}

type SpaceDeps struct {
	AccountService         accountservice.Service // per-org; shadows the wallet
	AccountMetadataPayload []byte                 // derived from the org key
	CredentialProvider     credentialprovider.CredentialProvider
	PersonalSpaceId        string                 // "" in server mode
}
```
resolved in the `BackendFactory` and threaded into both `spacecore.Opts` and
`clientspace.SpaceDeps`.

**Three gaps this feeds live outside `spacev2`** — the branch does not touch
`clientspace` at all (VERIFIED: no diff vs merge-base *or* develop tip):
1. **Object cache signs new object roots with the root key.**
   `space/clientspace/space.go:161` — `objectcache.New(deps.AccountService, …)`,
   with the load-bearing TODO above it:
   `space/clientspace/space.go:147-149` — `sp.aclIdentity = res.SignKey.GetPublic()`
   / `// todo: fixme we pass the real account service in case of streamable space to the objectcache`.
   So `aclIdentity` is per-space but the cache is not. **Most load-bearing gap for R1.**
2. **Tech space has no override path.** `space/clientspace/techspace.go:54`;
   `space/spacev2/bootstrap.go:132-142` passes the root service. Moot in server
   mode, but blocks running both profiles in one binary.
3. **Credential provider is process-global and root-only.**
   `space/spacecore/credentialprovider/credentialprovider.go:18-41` holds one
   `coordinatorclient` and calls `SpaceSign`; it is resolved from the **root** app
   by `commonspace/headsync`, so the per-space `AccountService` does not shadow it.
   It must become per-space *and* route over an org-identity connection — the one
   identity consumer not solvable inside the space layer.

**(c)** spacev2 part: small addition, 0.5 d. Gaps 1–3: design change, 5–8 d,
path-independent.
**(d)** R1 in full; per-operation member signing; the connection pool's consumer side.

### 4.3 `StatusSource` and `SpaceListSource`, SpaceView-backed by default — P0, design change, 2–3 d
**(a)** Two hard ties. Status: `spaceBackend.AccountStatus`
(`space/spacev2/backends.go:114-123`) → `spaceView()` (`:102-112`) →
`TechSpace.GetSpaceView` — a full smartblock open, cached in `b.view` (`:79`) for
the session. Discovery: `newSpaceWatcher` is constructed directly in `Init`
(`space/spacev2/service.go:177`) over a hardcoded tech-space subscription
(`space/spacev2/watcher.go:39-62`). Also the post-load pipeline registers v1
`spacestatus` (`space/spacev2/backends.go:434`), whose `Init` does
`techSpace.GetSpaceView` (`space/internal/components/spacestatus/status.go:81-93`)
and which `aclobjectmanager` and `participantwatcher` resolve from the app.
Upside: the lifecycle's *only* status input is one method, so the reconciler,
registry, retry and freshness protocol are already source-agnostic.
**(b)**
```go
// StatusSource is the authority for a space's synced status and the target for
// the local-status publication. SpaceView-backed by default, ee-cloud-backed in
// server mode.
type StatusSource interface {
	AccountStatus(ctx context.Context, spaceId string) (spaceinfo.AccountStatus, error)
	PersistentInfo(ctx context.Context, spaceId string) (spaceinfo.SpacePersistentInfo, error)
	SetLocalInfo(ctx context.Context, info spaceinfo.SpaceLocalInfo) error
	SetPersistentInfo(ctx context.Context, info spaceinfo.SpacePersistentInfo) error
	Release(spaceId string) // drop any cached handle (§4.12)
}

// SpaceListSource enumerates the spaces this process is responsible for.
// The SpaceView watcher is one implementation; an ee-cloud poll/stream another.
type SpaceListSource interface {
	Run(ctx context.Context, onChange func(SpaceChange)) error // onChange must not block
	Close() error
}

type SpaceChange struct {
	SpaceId string
	OrgId   string                  // "" in client mode
	Status  spaceinfo.AccountStatus // hint; StatusSource stays authoritative
	Removed bool
}
```
Give `spacestatus.SpaceStatus` a second implementation rather than a second
caller: four getters are what the pipeline branches on and
`GetSpaceView() techspace.SpaceView` is the only method leaking the smartblock
(**INFERRED** from `docs/superpowers/research/2026-09-16-hosted-api/space-lifecycle.md` §3.2`; re-verify the method
inventory before committing).
**(c)** Design change, 2–3 d. Much cheaper now than after `backends.go` grows callers.
**(d)** R3 entirely; removes the per-space SpaceView open dominating R6.

### 4.4 Lazy controller creation; separate discovery from the registry — P0, small addition, 1–2 d
**(a)** A controller is created for **every** discovered SpaceView before the
demand policy is consulted:
```go
// space/spacev2/service.go:338-348
ctrl, err := s.registry.getOrCreate(ev.spaceId)   // always
…
if s.wantedOnDiscovery(ev.spaceId) { ctrl.SetWanted(true) }
ctrl.Poke()
```
`newController` starts the reconcile goroutine immediately
(`space/spacev2/controller.go:101`) and `run()`'s first act — before any converged
check — is reading status (`:245`), i.e. opening the SpaceView smartblock. Boot
therefore costs N goroutines + N smartblock opens **regardless of lazy mode**. In
v1 a deferred space got no controller at all (`[develop] space/service.go:569-574`
parks the status in `deferredStatuses`). **This is a regression versus develop for
our workload.** VERIFIED. Enumeration is registry-backed too
(`space/spacev2/api.go:410-417`), which is why every space needs a controller —
the deletion driver depends on it
(`space/deletioncontroller/deletioncontroller.go:49-54`).
**(b)** Split "known" from "resident-capable":
```go
// spaceIndex is the cheap, complete set of known spaces: one small struct per
// space, no goroutine, no I/O, no smartblock.
type spaceIndex struct {
	mu    sync.RWMutex
	known map[string]spaceRecord
}

type spaceRecord struct {
	OrgId      string
	LastStatus spaceinfo.AccountStatus // last hint from the SpaceListSource
	LastSeen   time.Time
	LastLag    time.Duration           // §4.11
}
```
The discovery handler always updates the index and calls `registry.getOrCreate`
only when the demand policy wants the space or the hinted status is terminal
(Deleted/Removing) or Joining. `AllSpaceIds()` reads the index. Second half, with
§4.3: let the loop skip the status read when `wanted == false` and the hint is
non-terminal — that needs `Poke(hint)` instead of `Poke()`
(`space/spacev2/controller.go:112-117`).
**(c)** Small addition, 1–2 d; the `Poke(hint)` half is a small protocol change, +0.5 d.
**(d)** R6; makes boot cost independent of the number of known spaces; prerequisite
for any meaningful hot-set cap.

### 4.5 `DemandPolicy` replacing `wantedOnDiscovery` and the lazy-to-eager collapse — P1, design change, 1 d
**(a)** Demand on discovery is a hardcoded two-mode policy
(`space/spacev2/service.go:383-388`) and the drain permanently flips to eager:
```go
// space/spacev2/service.go:393-409
func (s *service) drainDeferredLater() {
	select { case <-s.preloadCh: case <-time.After(preloadRemainingSpacesTimeout): … }
	s.lazyReleased.Store(true)
	for _, ctrl := range s.registry.all() { ctrl.SetWanted(true) }
}
```
`preloadRemainingSpacesTimeout` is 10 s (`:39`). The branch's last commit
(`1c4072ab1`) deliberately strengthened this so spaces discovered *after* the
drain are also demanded — correct for a client, fatal for us. A hot-set policy
cannot coexist with it; it must replace it.
**(b)**
```go
// DemandPolicy decides residency. The client's eager/lazy/preload behaviour is
// one implementation; the server's LRU cap is another.
type DemandPolicy interface {
	OnDiscovered(rec spaceRecord) bool                      // demand it?
	OnAccess(spaceId string, prio Priority) (evict []string) // stay within the cap
	Tick(now time.Time) (evict []string)                     // idle eviction
}
```
**(c)** Design change, 1 d — cheap only while `wantedOnDiscovery` /
`drainDeferredLater` / `lazyReleased` have no other callers.
**(d)** §4.6; makes "no drain that eventually loads everything" structural rather
than configurable.

### 4.6 Hot-set cap and idle eviction — P1, small addition on top of §4.5, 1–2 d
**(a)** Nothing ever calls `SetWanted(false)`: the only call sites are
`space/spacev2/service.go:346`, `:408`, `space/spacev2/api.go:104`, `:286`, `:330`,
all `true`. VERIFIED. No last-use tracking, no cap. The primitive itself is
complete and tested: `SetWanted(false)` → `decide` → `TargetIdle` → `step` sees
`StateLoaded` → `unloadStep` (`space/spacev2/controller.go:312-315`, `:340-354`) →
`Backend.Unload` (`space/spacev2/backends.go:323-341`, which keeps disk data and
must not write `LocalStatusMissing`). `TestPauseUnloadReload` covers the cycle.
**(b)** An LRU `DemandPolicy` plus a `Touch` on every access path; eviction is
`ctrl.SetWanted(false)` and nothing else changes.
```go
type Priority int
const (
	PriorityBackground  Priority = iota // poll-driven catch-up
	PriorityWake                        // inbound head update
	PriorityInteractive                 // API request
)
```
**(c)** Small addition, 1–2 d, given §4.4 and §4.5.
**(d)** R2's bounded hot set. Note: without the objectstore/anystoreprovider
close-only twins (adjacent), eviction frees memory and goroutines but not fds —
the space layer's job here is only to emit the event in the right order.

### 4.7 Admission control in the load step — P1, small design addition, 1 d
**(a)** Each controller loads on its own goroutine with no global bound
(`space/spacev2/controller.go:326-338`). v1's drain had `preloadConcurrency = 2`
(`[develop] space/service.go:81`); v2's drain just demands everything
(`space/spacev2/service.go:407-409`), relying on the client's small space count.
A wake storm (R4) would attempt N simultaneous loads.
**(b)**
```go
// Admission bounds concurrent loads and orders them by priority.
type Admission interface {
	Acquire(ctx context.Context, spaceId string, prio Priority) (release func(), err error)
}
```
called around `c.backend.Load` in `loadStep`; priority is the highest requested
since the last attempt, stored next to `wanted`.
**(c)** ~40 lines but it changes the controller contract: design addition, 1 d.
**Add the hook now even as a no-op** — retrofitting after backends exist means
touching all of them.
**(d)** R4 at scale; predictable I/O under wake storms; makes the freshness
contract expressible ("we are N loads behind").

### 4.8 Exported demand API — P1, small addition, 0.5 d
**(a)** The primitives exist but are unexported. `AddStreamable` already shows the
non-blocking request shape (`space/spacev2/api.go:326-331`: `getOrCreate` +
`SetWanted(true)`, no wait). Readiness futures exist as `WaitLoaded(ctx)`
(`space/spacev2/controller.go:173-201`) with a correct freshness rule — a caller
that just promoted a space cannot be failed by a pre-change decision (`:176`,
`:185-191`). The non-blocking probe is `SpaceIfLoaded` (`:159-166`).
**(b)**
```go
// RequestLoad raises demand without waiting. Safe to call at high rate.
func (s *service) RequestLoad(spaceId string, prio Priority) error
// Touch records a use for the hot-set policy. Non-blocking, never loads.
func (s *service) Touch(spaceId string)
// SpaceIfLoaded returns the resident space or nil. Never loads.
func (s *service) SpaceIfLoaded(spaceId string) clientspace.Space
// Ready closes the channel when the space is resident.
func (s *service) Ready(spaceId string) (<-chan struct{}, func())
```
`Ready` is the only new machinery; the controller already broadcasts on a
replaceable `changed` channel (`space/spacev2/controller.go:406-409`).
**(c)** Small addition, 0.5 d.
**(d)** R4's producer side; the API layer's "serve stale, report lag"; the poll
scheduler.

### 4.9 Wake-on-change landing zone — P1, 0.5 d in spacev2 + 3–4 d adjacent in spacecore
**(a)** Unchanged from develop — `space/spacecore` is byte-identical with the
merge-base. Inbound head updates for a non-resident space are dropped:
```go
// space/spacecore/streamopener.go:87-91
sp, err := s.spaceCore.Pick(peerCtx, syncMsg.SpaceId())
if err != nil { return }
return sp.HandleMessage(peerCtx, syncMsg)
```
`Pick` is cache-only (`space/spacecore/service.go:218-224`). The subscribe side
sends `getOpenedSpaceIds()` (`space/spacecore/service.go:290-296`, used at
`streamopener.go:41`), so a node never pushes updates for a non-resident space.
**(b)** The space layer only has to expose a landing zone; `spacecore` needs an
optional sink:
```go
// WakeSink receives spaces whose messages arrived while not resident.
// Implemented by the engine; nil in client mode.
type WakeSink interface {
	Wake(spaceId string, prio Priority)
}
```
wired into `HandleMessage` in place of the error return, with the subscribe set
changed from `getOpenedSpaceIds()` to a hot-set-aware provider. The engine's
implementation calls `RequestLoad(spaceId, PriorityWake)` and dedupes.
Caveat: node-side stream-tag membership is a linear scan over a slice, so
subscribing tens of thousands of tags on one stream is O(n²) node-side
(mechanism VERIFIED in any-sync `net/streampool`, magnitude INFERRED) — subscribe
the hot set, poll the rest.
**(c)** spacev2: small addition, 0.5 d. spacecore: 3–4 d, adjacent.
**(d)** R4.

### 4.10 Bootstrap profile without tech/personal/marketplace — P1, design change, 2 d
**(a)** `Run` is unconditional: `initMarketplace()`
(`space/spacev2/service.go:183`), `resolveTechSpace` (`:187-210`),
`parentApp.Register(ts)` (`:212`), `close(techSpaceReady)` (`:228`),
`computeLazyMode` calling `techSpace.SpaceViewExists` (`:237`, `:281-289`),
`techSpace.StartSync()` (`:268`), and for a new account a synchronous first-space
create (`:243-258`). `Init` derives `personalSpaceId`/`techSpaceId` from the
process wallet (`:153-160`) and `DeriveAccountMetadata` from its sign key (`:166`).
The API routes tech and marketplace ids (`space/spacev2/api.go:61-66`) and `Wait`
gates on `ts.SpaceViewExists` (`:89-95`).
**(b)** A profile selected at construction, not a pile of nil checks:
```go
type Profile struct {
	TechSpace   bool // resolve, register, StartSync
	Personal    bool // derive personalSpaceId, heal its view, personalmigration
	Marketplace bool // register the virtual space
	Discovery   SpaceListSource
	Status      StatusSource
	Backends    BackendFactory
	Demand      DemandPolicy
	Admission   Admission
}
```
with `ClientProfile()` reproducing today's behaviour verbatim. `Run` becomes a
sequence of conditional steps; `Get`/`Wait` lose their special cases when those
are off.
**(c)** Design change, 2 d. Mostly a consequence of §4.1–§4.5; doing it first
would be premature.
**(d)** R3's "no tech/personal/marketplace"; both profiles in one binary during
migration.

### 4.11 Per-space observability without loading — P2, small addition, 1 d
**(a)** `State()` is per-controller (`space/spacev2/controller.go:152-156`) and
`AllLoadedSpaceIds` (`space/spacev2/api.go:420-427`) takes every controller's
mutex per call — used by `SyncAllSpaceHeads` (`:435`) and, via `AllSpaceIds`, by
the 180 s deletion loop. There is no per-space lag anywhere: head-sync freshness
lives inside the resident any-sync space and disappears on unload.
**(b)** Fold observability into the §4.4 index so it survives unload:
```go
type SpaceStat struct {
	SpaceId        string
	OrgId          string
	State          State
	Wanted         bool
	LastLoadedAt   time.Time
	LastUnloadedAt time.Time
	LastSyncAt     time.Time     // last successful head-sync while resident
	LastHeadHash   string        // from sync or from the cold poll
	HeadHashAge    time.Duration
	LastErr        string
}

func (s *service) Stat(spaceId string) (SpaceStat, bool)
func (s *service) StatAll(filter func(SpaceStat) bool) []SpaceStat
```
The freshness contract is exactly `LastSyncAt` / `HeadHashAge`, and both must be
writable by the **cold poller**, not only by a resident space.
**(c)** Small addition, 1 d.
**(d)** The freshness contract; poll prioritisation; any operational view of a
50 000-space shard.

### 4.12 Cleanups — P2, 0.5–1 d total
- **Release the SpaceView on unload.** `spaceBackend.view`
  (`space/spacev2/backends.go:79`, set at `:102-112`) is cached for the session and
  never released, so a paused space still pins a smartblock. `Unload` (`:323-341`)
  should drop it (`StatusSource.Release`, §4.3).
- **Delete or mark-superseded the stale plan docs.**
  `docs/superpowers/plans/2026-07-02-spacev2-m2-foundation.md` (1193 lines) and
  `…-m3-controllers.md` (206 lines) describe the *stashed* pre-restart design
  (state machine, `ErrTransitionInProcess`, ready-futures, `builderFor`) that no
  longer exists; `docs/SpaceController.md` (+933 lines on the branch) documents the
  v1 the same branch deletes. A reader following them implements the wrong
  architecture.
- **Remove the dead `builder` / `spaceloader` production code.** After the cutover
  `space/internal/components/builder/` is reachable only from
  `space/internal/components/spaceloader/spaceloader.go:30`, which nothing starts;
  `spaceloader` survives only as the interface `space/spacev2/presetloader.go:23`
  satisfies and that `aclobjectmanager.go:44`, `migration/runner.go:18` and
  `personalmigration.go:18` resolve. Keep the interface, move it, delete the rest.

---

## 5. Two paths

### Path A — fold into the existing rewrite
**Reused:** the reconciler, the input/decision freshness protocol, the registry,
the production backend, 67 tests, the completed cutover, the tech-space resolution
decision tree.
**Changes:** §4.1–§4.12 as independently reviewable increments ≈ **15 d inside
`spacev2`**, plus the identity gaps outside it (5–8 d) and the path-independent
adjacent work.
**Risks.** (1) The branch is **runtime-unverified** (`space/spacev2/HANDOFF.md:151-156`):
no end-to-end run against accounts of any vintage, no cross-device profile or
push, no lazy mode against a real client — building on it inherits whatever the
first real run finds, though the risk concentrates in bootstrap, lazy mode and
deletion-driver ordering, which the server profile replaces anyway. (2) **228
commits of drift**, with `space/spacecore` substantially changed on develop and
five branch-deleted files modified there; the rebase is real work the branch owes
regardless. (3) Two P0 changes (§4.3, §4.5) alter interfaces the client path also
uses, so they need the author's agreement, not just a patch.

### Path B — fresh greenfield on the same design
**Reused:** the design (`space/spacev2/DESIGN.md` is normative and correct), the
untouched lower layers, and probably a direct copy of `state.go` + `controller.go`
— the parts carrying the subtle concurrency reasoning and its tests.
**Changes:** the interfaces of §4.1–§4.10 designed in from the start rather than
retrofitted; no client-profile code to preserve; no `wantedOnDiscovery` /
`drainDeferredLater` / marketplace / tech-space special cases to make conditional.
**Risks.** (1) The 67 tests do not transfer for free — `api_test.go` and
`backends_test.go` are written against the current service and backend shapes, and
losing them loses the strongest evidence the reconciler is correct. (2) Two
orchestration layers in the tree, with the client one still moving. (3) The
cutover work (24 consumers, the `space` facade, `bootstrap.go`, `.mockery.yaml`,
the deletion-driver seam) would be redone or forked.
**Effort. INFERRED:** 20–28 d to the same functional point, i.e. +8–12 d over
Path A, most of it re-deriving what the branch already proved.

### Recommendation
**Path A, with §4.1–§4.4 landed in the branch before engine work starts.** The
branch independently arrived at three of the five primitives the engine needs —
first-class keep-data unload with re-arm, a single-method status input behind an
exported interface, one injectable per-space build site — and deletes ~7400 lines
we would otherwise modify and re-reconcile. The layers where the hard problems
live are byte-identical, so no unload or tree-only work is duplicated by the choice.

Order of work:
1. Branch hygiene, by the author: rebase onto develop, run the pending runtime
   verification, delete the stale plan docs (§4.12).
2. §4.1 backend factory + §4.2 per-space deps resolver — half a day each, no
   behaviour change, unblock everything.
3. §4.4 lazy controller creation + space index — fixes the one regression the
   branch introduces for our workload; needed before any cap is meaningful.
4. §4.3 `StatusSource` / `SpaceListSource` — largest single win for R3, much
   cheaper now than later.
5. §4.5 `DemandPolicy`, then §4.6 hot-set cap, then §4.7 admission (hook first,
   policy later), §4.8 demand API.
6. §4.10 bootstrap profile, §4.11 observability.
7. In parallel, path-independent: the three identity gaps (§4.2), the `spacecore`
   wake sink (§4.9), the keep-data unload work outside `space/`.

If §4.1–§4.4 cannot land in the branch, the fallback is still "build on the branch
and do them ourselves" — not to start from develop, whose `spacefactory` /
`spaceprocess` layer is being deleted regardless.

---

## 6. Open questions for the spacev2 author

1. **Rebase plan.** 228 commits of drift with `space/spacecore` substantially
   changed on develop and five branch-deleted files modified there. Rebase now,
   merge develop in, or land the branch and rebase the engine work after?
2. **Runtime verification.** Plan and timeline for the vintage matrix in
   `HANDOFF.md:151-156`? Does it gate merge, or run in parallel?
3. **`Poke(hint)`.** Open to the poke carrying the last known status so a cold
   controller can decide without a status read (§4.4)? It slightly weakens the
   "events carry no payload" property in `DESIGN.md:173`; the hint would be
   advisory, with the `StatusSource` still authoritative.
4. **Where should `AllSpaceIds` live?** It is registry-backed today
   (`space/spacev2/api.go:410-417`) and the deletion driver depends on that.
   Moving it to a separate index is the crux of §4.4 — any reason the deletion
   driver needs controllers rather than ids?
5. **`spacestatus` substitution.** Is a second `spacestatus.SpaceStatus`
   implementation the right cut for the post-load pipeline
   (`backends.go:434`), or would you rather narrow what `aclobjectmanager` and
   `participantwatcher` depend on?
6. **`Backend` contract enforcement.** Would you accept a shared conformance suite
   (`spacev2.TestBackendContract(t, factory)`) checking the retry / `Fatal` /
   idempotent-`Offload` / sequential-invocation rules documented at
   `controller.go:15-41`?
7. **Client hot-set.** Does the client want the §4.6 cap too (desktop accounts
   with hundreds of spaces), or stay eager? That decides whether `DemandPolicy` is
   shared or server-only.
8. **Per-space `PersonalSpaceId`.** `clientspace.SpaceDeps.PersonalSpaceId`
   (`backends.go:411`) drives access-type and derivation decisions. In server mode
   there is no personal space — is empty-string safe everywhere it flows, or does
   it need an explicit "no personal space" value?
