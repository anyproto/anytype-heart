# The Space Subsystem — Architecture & Contract

> Reference for a clean-room reimplementation of the `space/` package (a.k.a. "space
> controller" / "space app"). It documents *what the subsystem is responsible for*,
> *the technology it sits on* (any-sync, tech space, space views, diffsync), *how the
> rest of anytype-heart consumes it* (contract, API, patterns), and *the full
> lifecycle of a space*. A reimplementation may break the current interface contract
> for a cleaner API — so this doc emphasizes **what callers actually expect**, not
> just the literal signatures. File references are `path:line` against the current
> tree.

---

## 1. What it is (mental model)

anytype-heart is a local-first, multi-device, end-to-end-encrypted object database. A
**space** is one isolated, independently-synced database of objects (pages, types,
relations, chats). Everything the user sees lives in some space. The space subsystem
is the layer that **turns a space id into a live, loaded, synced, indexed
`clientspace.Space` object** and manages that object's whole life — from "does this
space exist?" through loading, syncing, joining, deletion and offloading.

It is built as a **stack of wrappers**, each adding a concern:

```
            RPC handlers (core/space.go, core/workspace.go, core/account.go)
                 │        via acl.AclService / block.Service
                 ▼
 space.Service ("client.space")            ── registry of controllers, account bootstrap,
   │                                          create/join/delete, lazy loading, watcher
   ├── spacefactory.SpaceFactory           ── builds the right controller per space type
   │
   ▼
 spacecontroller.SpaceController (1 per space)  ── per-space state machine (mode)
   │   internal/{personal,shareable,streamable,marketplace}space
   │   drives mode.StateMachine: Initial→Loading→Offloading→Joining
   ▼
 mode.Process (loader | offloader | joiner | initial)  ── one child app.App per mode
   │   internal/spaceprocess/*
   │   loader registers the load pipeline (builder, spaceloader, aclobjectmanager,
   │   participantwatcher, migration, …)
   ▼
 clientspace.Space               ── anytype "domain" space: object cache, derived ids,
   │   space/clientspace          mandatory-object bootstrap, indexing, key-value service
   ▼
 spacecore.AnySpace              ── thin adapter over any-sync (embeds commonspace.Space
   │   space/spacecore            + KeyValueObserver); cached in an ocache
   ▼
 commonspace.Space (any-sync)    ── the real sync engine: object trees (CRDT), ACL,
                                    headsync/diffsync, key-value, storage, peer manager
```

Orthogonally, the **tech space** (a special per-account any-sync space) stores one
**SpaceView** object per real space; the SpaceView is the persistent, cross-device
source of truth for "which spaces exist and what is their status." A local objectstore
subscription (`spaceWatcher`) turns SpaceView changes into controller lifecycle
events. This is the reactive spine of the whole subsystem.

---

## 2. Responsibilities

The subsystem owns, end to end:

1. **Account/space bootstrap.** On login (`Run`): init the marketplace space, create or
   load the tech space, and create (new account) or discover (existing account) the
   personal space. `space/service.go:242` (`Run` → `createAccount` / `initAccount`).
2. **Space existence & identity.** Deterministic id derivation for personal/tech spaces
   from the account key; deriving/looking up SpaceView ids. `spacefactory.go:95,128`.
3. **Registry of live spaces.** `spaceControllers map[string]SpaceController` +
   `waiting map[string]controllerWaiter`, guarded by `s.mu`. `service.go:150-151`.
4. **Per-space lifecycle.** Loading, joining, offloading (local delete), and mode
   transitions in response to status changes — one `SpaceController` state machine per
   space.
5. **Status model.** Maintaining three status axes (AccountStatus / LocalStatus /
   RemoteStatus) on each SpaceView and reacting to them.
6. **Space CRUD API** to the rest of the app: `Create`, `CreateOneToOne`, `Join`,
   `InviteJoin`, `CancelLeave`, `Delete`, `Get`, `Wait`, `AddStreamable`.
7. **Lazy multi-space loading.** Defer non-preferred spaces at startup, promote on
   demand / on RPC / on timer (spec 2026-05-17). `service.go:497-684`.
8. **On-demand head sync** across all loaded spaces on app foreground (GO-7302).
   `service.go:807` (`SyncAllSpaceHeads`).
9. **Delegating deletion to the network** via the deletion controller + coordinator.
10. **Wiring any-sync per space** (peer manager, storage, coordinator client, tree
    syncer) through `spacecore`.

It explicitly does **not** own: object editing (that's `core/block`), indexing logic
(`core/indexer`, injected as `SpaceIndexer`), ACL record semantics (any-sync +
`core/acl`), or the sync-status UI icon (`core/syncstatus`, a separate subsystem — do
not conflate with `RemoteStatus`).

---

## 3. The technology underneath

### 3.1 any-sync primitives

any-sync (`github.com/anyproto/any-sync@v0.12.13`) is the p2p sync engine. The
concepts a reimplementation must understand:

- **`commonspace.Space`** — the sync container for a *single* space. Aggregates that
  space's object trees, ACL, headsync, key-value store, storage and peer manager.
  Built via `NewSpace(ctx, id, Deps{TreeSyncer, SyncStatus, Indexer})`. Key surface:
  `SyncHeads`, `TreeBuilder`, `Acl`, `Storage`, `KeyValue`, `HandleMessage`.
- **ObjectTree / CRDT** — every object (page, type, relation, chat) is a Merkle-DAG of
  signed changes with a set of *heads*. Change type is `"anytype.object"`
  (`spacedomain/type.go:67`). `TreeBuilder()` loads/creates them.
- **ACL** (access-control list) — a signed record chain (root built at space
  derivation from the account signing+master keys) defining members, permissions and
  shared read keys. `AclState().Permissions(identity).CanWrite()` drives
  `IsReadOnly`; `AclState().IsOneToOne()` drives `IsOneToOne`.
- **headsync / diffsync** — periodic Merkle/range reconciliation that compares the set
  of per-object head hashes with responsible peers to find what diverged, then pulls
  only the differences (cost O(differences), not O(all objects)). See §7.
- **key-value store** — a space-scoped, CRDT-replicated K/V store (used by chats and
  store-backed objects); synced separately from trees. A per-space
  `keyvalueobserver.Observer` fires on changes.
- **coordinator** — the network's central **directory + permission authority** (not a
  data replica): registers/signs spaces, reports per-space status & limits, authorizes
  deletion, brokers ACL records & invites, serves network config.
- **peer / node** — a *peer* is a live connection (another device over LAN, or a
  server); a *node* is a server from `nodeconf` responsible for a space. The per-space
  `clientPeerManager` maintains the set of responsible peers used for sync traffic.

### 3.2 `spacecore` — the any-sync adapter

`space/spacecore` wraps commonspace and owns an `ocache` of live spaces.

`SpaceCoreService` (`space/spacecore/service.go:70-81`):

```go
type SpaceCoreService interface {
    Create(ctx, spaceType, replicationKey, metadataPayload) (*AnySpace, error) // fresh, non-deterministic id
    Derive(ctx, spaceType) (*AnySpace, error)      // deterministic id + create local storage (idempotent)
    DeriveID(ctx, spaceType) (id string, error)    // compute id only, no side effects
    CreateOneToOneSpace(ctx, bPk crypto.PubKey) (*AnySpace, error) // id deterministic in the key-pair
    Delete(ctx, spaceId) error                     // network op: signed delete to coordinator
    Get(ctx, id) (*AnySpace, error)                // cached or load
    Pick(ctx, id) (*AnySpace, error)               // only if already loaded (hot path)
    CloseSpace(ctx, id) error                       // evict + close
    StorageExistsLocally(ctx, spaceId) (bool, error)
    app.ComponentRunnable
}
```

- **Deterministic ids.** `spaceId = NewSpaceId(cid(signedHeader), repKey)` where the
  header = `{Identity=accountPubKey, SpaceType, ReplicationKey=fnv64(accountPubKey)}`,
  signed with the account signing key (`spacepayloads.StoragePayloadForSpaceDerive`).
  So the id is a pure function of **(account signing key, space type)** — this is *why*
  each account has exactly one personal (`SpaceTypeRegular`) and one tech
  (`SpaceTypeTech`) space. `Create`/`CreateOneToOneSpace` use random read keys → new
  ids each time (shared/DM spaces).
- **`AnySpace`** (`space/spacecore/space.go:16`) embeds `commonspace.Space` and adds
  `KeyValueObserver()`. `TryClose` always returns false — lifecycle is owned by the
  ocache.
- **`spacedomain.SpaceType`** (`space/spacedomain/type.go:11`): `"anytype.space"`
  (Regular), `"anytype.techspace"` (Tech), `"anytype.chatspace"` (Chat),
  `"anytype.onetoone"` (OneToOne). At the any-sync level the type is just the signed
  header string; tech vs regular differ only *semantically*.

### 3.3 Local storage

`ClientStorage` (`space/spacecore/storage/storage.go:20`) provides per-space storage;
new accounts use **anystore (SQLite via any-store)** — one `rootPath/<spaceId>/store.db`
per space. `ClientSpaceStorage` (`anystorage/clientstorage.go:17`) adds `HasTree`,
`TreeRoot`, and the **space-created marker**: `MarkSpaceCreated` / `IsSpaceCreated` /
`UnmarkSpaceCreated`. That marker is a device-local one-shot flag meaning "*I* created
this space" — set right after Derive/Create; on the next `clientspace.BuildSpace` it
triggers `CreateMandatoryObjects` + `InstallBundledObjects`, then is cleared. A
*joined* space never has the marker, so it receives mandatory objects via sync instead
of recreating them. `StorageExistsLocally` is a non-destructive existence probe.

### 3.4 Tech space & space views (the source of truth)

The **tech space** is a single per-account `commonspace.Space` of type
`SpaceTypeTech`, derived deterministically. Because it's a normal any-sync space, its
contents sync across the user's devices. It stores:

- one **SpaceView** smartblock per real space (id derived from the real space id),
- one **AccountObject** (account-wide settings/profile),
- one **PersonalFavoritesStore**,
- a per-space key-value store.

`techspace.TechSpace` (`space/techspace/techspace.go:81-101`) is the metadata API:
`Run`, `StartSync`, `DoSpaceView`, `DoAccountObject`, `SpaceViewCreate`,
`GetSpaceView`, `SpaceViewExists`, `SetLocalInfo`, `SetPersistentInfo`, `SpaceViewId`,
`AccountObjectId`, … The `Do*` methods resolve the derived id → `objectCache.GetObject`
→ type-assert → **`Lock()`/`Unlock()`** → run the closure (the safe mutation pattern).

`clientspace.TechSpace` (`space/clientspace/techspace.go:20`) embeds **both** a full
`*space` (so the tech space is itself a usable `Space` with an object cache) **and**
the `techspace.TechSpace` metadata interface, sharing one object cache.

The **SpaceView** is a real object/smartblock (`core/block/editor/spaceview.go`,
layout `ObjectType_spaceView`) whose "fields" are object relations/details:

- `GetPersistentInfo()` reads `CombinedDetails()` → **synced** relations.
- `GetLocalInfo()` reads `LocalDetails()` → **local-only** relations (not synced).
- `SetSpacePersistentInfo` / `SetSpaceLocalInfo` write via `UpdateDetails` + `Apply`,
  with an `Equal()` early-out to skip no-op applies. This early-out is a **correctness
  invariant, not just an optimization**: an unconditional `Apply` re-enters the watcher
  (§6.1) and self-sustains a status-write storm / unbounded tree growth (the ~1300
  cold-start no-op applies, GO-7289; fixed by `50fcf258d`).

Beyond status, the SpaceView carries the full per-space metadata mutators — `SetOwner`,
`SetMyParticipantStatus`, `SetAclInfo` (also writes the derived push keys + the
`IsAclShared` gate), `SetSharedSpacesLimit`, `SetAccessType`,
`SetOneToOneIdentity`/`SetOneToOneInboxInviteStatus`, and the push mutators
`SetPushNotificationMode` / `SetPushNotificationForceModeIds` / `ResetPushNotificationIds`.
The three push mutators take a **`session.Context`** and apply via `NewStateCtx(ctx)` so
the resulting event routes back to the *originating* client session (every other mutator
uses plain `NewState` → broadcast to all sessions). Space-list **ordering** is a *local*
SpaceView relation `SpaceOrder` (lexid), written by `SpaceSetOrder`/`SpaceUnsetOrder`.

The user-facing **name / icon / homepage / uxType** of a space are **not** written to
the SpaceView by this subsystem directly. The `Workspaces` object inside each real space
mirrors a fixed relation set into its SpaceView (in the tech space) on every apply, via
`service.OnWorkspaceChanged` → `techSpace.SpaceViewSetData` (keys in `workspaceKeysToCopy`:
`Name, IconImage, IconOption, SpaceUxType, Homepage, SpaceType, CreatedDate, ChatId,
Description, AnalyticsSpaceId`, `core/block/editor/spaceview.go`). This is how the
space-switcher UI — which reads SpaceViews cross-space (§5.3) — shows each space.

The **AccountObject** (`core/block/editor/accountobject/`) holds account-scope data:
shared-spaces limit (synced relation, read by paywall/membership UI), profile
(name/description/icon), plus a CRDT K/V store for analytics id & inbox offset.

### 3.5 The status model (`space/spaceinfo`)

Three orthogonal axes stored on each SpaceView (all backing `model.SpaceStatus_*`):

| Struct | Persistence | Fields | Meaning |
|---|---|---|---|
| **SpacePersistentInfo** (`spacepersistentinfo.go:10`) | **synced** (CombinedDetails) | `SpaceID, AccountStatus*, AclHeadId, EncodedKey(guestKey), Name, OneToOne{Identity,RequestMetadataKey,InboxSentStatus}` | my account's relationship to the space |
| **SpaceLocalInfo** (`spacelocalinfo.go:12`) | **local-only** (LocalDetails) | `SpaceId, localStatus*, remoteStatus*, shareableStatus*, writeLimit*, readLimit*` | per-device runtime state |
| **ParticipantInfo** (`participantinfo.go`) | ACL-derived | `ParticipantAclInfo{Id,SpaceId,Identity,Permissions,Status}`, `OneToOneParticipantData` | participant descriptors |

- **AccountStatus** (`spaceinfo.go:63`): `Unknown / Active / Joining / Removing /
  Deleted` — **drives the controller mode**. `Removing` is reserved and never written
  today (always defensively mapped to Offloading).
- **LocalStatus** (`spaceinfo.go:10`): `Unknown / Loading / Ok / Missing` — set by the
  loader (`Missing` = offloaded).
- **RemoteStatus** (`spaceinfo.go:33`): `Unknown / Ok / WaitingDeletion / Deleted /
  Error` — the coordinator's view, set by the deletion-controller poll. (`Error` is
  never emitted by the poll; it exists for the separate UI mapping.)

Pointer fields carry "unset" semantics (getters return `*Unknown`/0 when nil), so a
partial update only touches the fields it sets.

---

## 4. The space controller contract

### 4.1 `SpaceController` (`space/internal/spacecontroller/spacecontroller.go`)

One per space; the unit the service tracks.

```go
type SpaceController interface {
    SpaceId() string
    Start(ctx) error
    Mode() mode.Mode
    Current() any                                   // the current Process (loader/offloader/…)
    Update() error                                  // re-derive mode from persistent status
    SetPersistentInfo(ctx, spaceinfo.SpacePersistentInfo) error
    SetLocalInfo(ctx, spaceinfo.SpaceLocalInfo) error
    Close(ctx) error
    GetStatus() spaceinfo.AccountStatus
    GetLocalStatus() spaceinfo.LocalStatus
}
```

The controller's job: hold a `spacestatus.SpaceStatus` (a thread-safe facade over the
SpaceView), and translate **AccountStatus → mode** via a `mode.StateMachine`. `Start`
picks the initial mode; `Update` re-drives it (guarded by `lastUpdatedStatus`, so it
only fires on a real change); `SetPersistentInfo` writes to the SpaceView then calls
`Update`. `Current()` returns the live `Process`; the service type-asserts it to
`loader.LoadWaiter` to get the loaded `clientspace.Space`.

### 4.2 The mode state machine (`space/internal/spaceprocess/mode/statemachine.go`)

```go
Mode: ModeUnknown | ModeInitial | ModeLoading | ModeOffloading | ModeJoining
type Process interface { Start(ctx) error; Close(ctx) error; CanTransition(next Mode) bool }
type ProcessFactory interface { Process(mode Mode) Process }
```

- The **controller is the `ProcessFactory`** (`Process(md)` returns a fresh Process).
- A single goroutine `loop()` serializes transitions: `ChangeMode(next)` dedups on the
  current mode, rejects a *different* concurrent transition with
  `ErrTransitionInProcess`, then the loop **closes the old Process → builds & starts the
  new one**; on start failure it falls back to `ModeInitial` (which must always start).
- All Processes' `CanTransition` currently return `true`; the real gating lives in
  `ChangeMode`. (A reimplementation could push per-Process rules back into
  `CanTransition` — e.g. offloading is meant to be terminal — instead of the current
  vestigial `spaceOffloader.CanTransition==false`.)

### 4.3 The four Processes (`space/internal/spaceprocess/*`)

Each Process wraps a child `app.App`; `Start`→`app.Start`, `Close`→`app.Close`.

| Process | Mode | Registers | Effect |
|---|---|---|---|
| **initial** | Initial | — | sentinel; start/close no-op; start state & failure fallback |
| **loader** | Loading | the load pipeline (§4.5) | builds a usable `clientspace.Space`; exposes `WaitLoad(ctx)` |
| **offloader** | Offloading | `spaceoffloader` | deletes local storage + files + indexes; `WaitOffload(ctx)` |
| **joiner** | Joining | `statusChanger` + `aclnotifications` + any-sync `aclwaiter` | waits for local ACL to reach the join head; accept→Active, reject→Deleted |

### 4.4 The five controller types (`space/internal/*space`, built by `spacefactory`)

| Type | AccountStatus → mode mapping | Notes |
|---|---|---|
| **personalspace** | Deleted→Offloading; else Loading | derived from account; runs `personalmigration`; loader rebuilt fresh per Start via `personalLoader` |
| **shareablespace** | Deleted/Removing→Offloading; Joining→Joining; Active/Unknown→Loading | the full-featured type (invite/join/leave/share) |
| **streamablespace** | Deleted→Offloading; else Loading | guest/stream access via a private `GuestKey`; selected when `EncodedKey != ""` |
| **marketplacespace** | n/a (no state machine) | **deprecated** (GO-6259); a `VirtualSpace` for bundled types/relations; `Mode()` hardcoded `ModeLoading` |
| **onetoone** | (built as a shareablespace) | DM space; extra `OneToOneParticipantData` (identity + request-metadata key) |

`SpaceFactory` (`space/spacefactory/spacefactory.go:53`) has paired methods: `Create*`
(derives/creates storage, creates the SpaceView, then the controller) vs `New*` (loads
an existing space whose SpaceView already exists). Plus `CreateAndSetTechSpace` /
`LoadAndSetTechSpace`.

### 4.5 The load pipeline (what `loader.New` builds)

Registration order (`loader/loader.go:42-50`), all `app.ComponentRunnable`:

1. **builder** — `BuildSpace(ctx, disableRemoteLoad)`: `spaceCore.Get` → assemble
   `clientspace.SpaceDeps` → `clientspace.BuildSpace` → set access type.
2. **spaceloader** — async build + exponential-backoff retry (1s→×1.5→20s) + publishes
   `LocalStatus` (Loading→Ok/Missing); exposes `WaitLoad`. Has an *optimistic-Ok* fast
   path (skip transient Loading if the store is on disk and was Ok last session).
3. **aclnotifications** — ACL-event notification sender.
4. **aclobjectmanager** — registers as the ACL updater; on each ACL change:
   set owner, set my participant status (→ `AccountStatusDeleted` when I have no
   permissions), derive push keys, compute joined date, delegate per-member work to the
   participant watcher. Fires the `SpaceLoaderListener`.
5. **participantwatcher** — registers/uses identities; writes participant records
   store-only (no smartblock); persisted-ACL-head skip optimization.
6. **migration** — runs `systemobjectreviser` + `readonlyfixer` after load.
7. *(+ personalmigration for the personal space).*

`clientspace.BuildSpace` (`clientspace/space.go:133`) creates the object cache +
`ObjectProvider`, derives object ids, bootstraps mandatory + bundled objects **iff the
space-created marker is set**, creates the KV service, then spawns
`mandatoryObjectsLoad` (reindex → load missing mandatory objects → install bundled →
`RunMigrationsWhenIdle` → `TreeSyncer().StartSync()`). `WaitMandatoryObjects` is the
readiness gate that `spaceloader.WaitLoad` waits on.

**Shared readiness barrier:** `aclobjectmanager` and the migration `Runner` each `Run`
a goroutine that first blocks on `spaceLoader.WaitLoad` — the fan-out point after the
async build. (`participantwatcher.Run` is a no-op; it is driven *synchronously* by
`aclobjectmanager` once the ACL has loaded, so it does not wait on the barrier itself.)

`SpaceLoaderListener` (`OnSpaceLoad`/`OnSpaceUnload`) is implemented by the **fulltext
indexer** (`core/indexer/fulltext.go:115`) — so space load/unload drives FT index
lifecycle.

---

## 5. The outward API (how the app consumes it)

### 5.1 `space.Service` ("client.space", `space/service.go:95`)

```go
type Service interface {
    Create(ctx, *SpaceDescription) (clientspace.Space, error)
    CreateOneToOne(ctx, *SpaceDescription, *IdentityProfileWithKey) (clientspace.Space, error)
    Join(ctx, id, aclHeadId string) error
    InviteJoin(ctx, id, aclHeadId string) error
    CancelLeave(ctx, id string) error
    Get(ctx, id string) (clientspace.Space, error)     // fast: controller must exist
    Wait(ctx, spaceId string) (clientspace.Space, error) // blocks until loaded (polls 500ms)
    AddStreamable(ctx, id string, guestKey crypto.PrivKey) error
    Delete(ctx, id string) error
    TechSpaceId() string; PersonalSpaceId() string; FirstCreatedSpaceId() string
    TechSpace() *clientspace.TechSpace
    GetPersonalSpace(ctx) (clientspace.Space, error); GetTechSpace(ctx) (clientspace.Space, error)
    SpaceViewId(spaceId string) (string, error)
    AccountMetadataSymKey() crypto.SymKey; AccountMetadataPayload() []byte
    PreloadRemainingSpaces(ctx) error
    app.ComponentRunnable
}
```

Errors callers branch on (`service.go:82`, `load.go:113`): `ErrSpaceNotExists`,
`ErrSpaceDeleted`, `ErrSpaceIsClosing`, `ErrSpaceStorageMissig`, `ErrFailedToLoad`.
`convertSpaceError` maps any-sync errors (`ErrSpaceIsDeleted`→`ErrSpaceDeleted`, etc.).

**`Get` vs `Wait`:** `Get` returns immediately once the controller exists and the
loader has finished (`ErrSpaceNotExists` if no controller); `Wait` first checks
`SpaceViewExists`, promotes a deferred/absent space, then polls every 500ms until the
controller appears. Use `Wait` when the space may not be loaded yet (object open,
account bootstrap); `Get` when you expect it loaded. `Get(techSpaceId)`
short-circuits to a `techSpaceReady`-gated getter.

### 5.2 `clientspace.Space` — what "a loaded space" gives you (`space/clientspace/space.go:39`)

This is the **most important outward type** — the object callers hold after
`Get`/`Wait`. It embeds `objectcache.Cache` (create/derive/get objects) and
`objectprovider.ObjectProvider` (derive/load object id sets), plus:

```go
Id() string
DerivedIDs() threads.DerivedSmartblockIds     // Workspace/Home/Archive/Widgets/Profile/SpaceView/SpaceChat — most-used
Do(objectId, func(sb smartblock.SmartBlock) error) error   // core primitive: GetObject+Lock+apply
DoCtx(ctx, objectId, apply) error
CommonSpace() commonspace.Space               // reach any-sync: .Acl(), .AclClient(), .SyncHeads()
TreeBuilder() objecttreebuilder.TreeBuilder   // raw CRDT trees / history
Storage() anystorage.ClientSpaceStorage
GetTypeIdByKey(ctx, TypeKey) (id, error); GetRelationIdByKey(ctx, RelationKey) (id, error)
IsReadOnly() bool; IsPersonal() bool; IsOneToOne() bool; SpaceType() spacedomain.SpaceType
GetAclIdentity() crypto.PubKey
KeyValueService() keyvalueservice.Service; RefreshObjects(ids) error
WaitMandatoryObjects(ctx) error
DeleteTree(ctx,id) error; StoredIds() []string; DebugAllHeads() []headsync.TreeHeads
Close(ctx) error
```

### 5.3 Consumer patterns

The dominant idiom across the app:

```go
spaceID, _ := resolver.ResolveSpaceID(objectId)   // core/block/object/idresolver
spc, _     := spaceService.Get(ctx, spaceID)      // or Wait(...)
spc.Do(objectId, func(sb smartblock.SmartBlock) error { ... })
```

`Get` is used ~73×, `Wait` ~5×. Method-frequency on the returned space: `DerivedIDs()`
~32, `CommonSpace()` ~23 (almost all ACL), `Do`/`DoCtx` ~17, `TreeBuilder()` ~15.

Top consumers:

| Subsystem | Uses it for |
|---|---|
| **core/block/service.go** (the object hub) | resolves the space for every object open/create/delete; re-exposes `Do`/`GetObject`/`DoFullId` app-wide |
| **core/acl/aclservice.go** | sharing/membership → `CommonSpace().AclClient()`, `Join/InviteJoin/Delete/CancelLeave`, `TechSpace().DoSpaceView` |
| **core/block/object/objectcreator** | resolve target/marketplace space to create objects, install bundled types |
| **core/files/fileobject** | file objects; `PersonalSpaceId`; migration (Wait) |
| **core/block/import**, **export** | derive Widgets/Workspace ids, create/export objects |
| **core/anytype/account** | bootstrap: `Wait` personal → `DerivedIDs`; `TechSpace().AccountObjectId` |
| **core/history** | `TreeBuilder().BuildHistoryTree` |
| **core/inviteservice**, **core/inbox** | invite/join flows via `TechSpace().DoSpaceView` |

RPC surface (handlers in `core/space.go`, `core/workspace.go`, `core/account.go`),
routed through `acl.AclService` / `block.Service` (every space-touching proto RPC has a
handler): `WorkspaceCreate`→`Create`; `WorkspaceOpen`→`Wait` (+ `UpdateLastOpenedDate`
on the SpaceView); `SpaceDelete`→`Delete`; `SpaceJoin`/`SpaceJoinCancel`→
`AclService.Join`/`CancelJoin`; the six `SpaceInvite*`; `SpaceMakeShareable`/
`SpaceStopSharing`; `SpaceRequestApprove`/`SpaceRequestDecline`; `SpaceParticipantRemove`;
`SpaceParticipantPermissionsChange`; `SpaceParticipantsAddList` (direct-add + inbox
invites); `SpaceLeaveApprove`; `SpaceChangeOwnership`; `SpaceSetOrder`/`SpaceUnsetOrder`;
`AccountPreloadRemainingSpaces`→`PreloadRemainingSpaces`. Name/icon/homepage/uxType are
written via `WorkspaceSetInfo`/`WorkspaceSetHomepage` on the Workspace object (then
mirrored to the SpaceView, §3.4); push mode via `PushNotificationSet*`. Account bootstrap
(`AccountCreate`/`Select`/`Recover`/`Stop`/`Delete`/`Migrate`) drives `Service.Run`/`Close`
indirectly.

**Downstream readers of SpaceView relations (indirect but load-bearing contract):**
- `core/subscription/crossspacesub` enumerates spaces by subscribing to SpaceView objects
  (layout `spaceView`, `AccountStatus NOT IN {Deleted,Removing}`) keyed on `TechSpaceId()`,
  gated on the object-store `OnSpaceIndexOpened` signal. It feeds chats ("all chats"),
  ACL/participants, push, file-download and API caches. If `TechSpaceId()` is empty early,
  a SpaceView loses `TargetSpaceId`/`SpaceAccountStatus` semantics, or a space opens
  without emitting `OnSpaceIndexOpened`, all of these silently return empty (no error).
- `core/syncstatus/spacesyncstatus` derives the UI sync icon + `EventSpaceSyncStatusUpdate`
  from the `RemoteStatus`/limits the subsystem writes onto the SpaceView.
- `core/pushnotification` subscribes to SpaceViews filtered `SpacePushNotificationKey
  Exists AND IsAclShared` — so if `SetAclInfo` never writes the derived push keys, push
  is silently disabled for every space.

The subsystem broadcasts **no** `model.Event` itself; its entire client-facing event
surface is (a) SpaceView detail changes (observed via object-store subscriptions) and
(b) **notifications**: `aclnotifications` converts ACL records into five `model.Notification`
types (join-request → owner; request-approved / removed / request-declined /
permissions-changed → the affected member only), gated on `NotificationService.LoadFinish()`
+ a last-id replay cursor to avoid re-notifying on every restart; separately
`service.sendNotification` emits a `ParticipantRemove` (deterministic id `spaceId_identity`)
when the coordinator reports the space deleted.

**Expectations a reimplementation must preserve:** (a) objectId→spaceId resolution is
eventual-consistent — callers use `ResolveSpaceIdWithRetry` (which retries *forever*,
bounded only by the caller's ctx); (b) tech-space metadata is mutated via
`TechSpace().DoSpaceView` / `DoAccountObject`, not the generic `Do`; (c) lazy loading is
transparent — `Get`/`Wait` must promote a deferred space; (d) `Do` locks the object for
the duration of the closure; (e) the objectId→spaceId `bindId` binding read by
`ResolveSpaceID` is written **as a side effect** of building any object through the
space's object cache (`source.Service.NewSource` → `BindSpaceId`) — a loader that
bypasses that path leaves resolution permanently not-found; (f) `block.Service` re-exports
both `GetObjectByFullID` (errors now if the space isn't loadable) and
`WaitAndGetObjectByFullID` (blocks until `Service.Wait` promotes the space) — both have
callers, so both semantics must exist.

---

## 6. Lifecycle of a space

### 6.1 The reactive spine

The controllers are driven **reactively** from SpaceView changes, not directly by
callers. Writing a SpaceView status is the primitive; the watcher does the rest:

```
write SpaceView detail (any device via any-sync, or local)
        │  Apply → objectstore index update
        ▼
spaceWatcher subscription (tech space, filter ResolvedLayout==spaceView)   space/spacesub.go
        │  dedupqueue keyed by spaceId (coalesce bursts → latest)          space/spacewatcher.go
        ▼
service.onSpaceStatusUpdated(spaceViewStatus)                              space/service.go:427
        │  per-view serialized (status.mx)
        ├─ remoteStatus==Deleted & account!=Deleted → notify + write AccountStatusDeleted
        ├─ maybeReleaseOnPreferredBroken (lazy fallback)
        └─ decideAndApplySpaceStatus → (defer | applySpaceStatus)
                 │  startStatus: build/pick controller (load.go:52)
                 ▼
           ctrl.Update() → sm.ChangeMode(mode from AccountStatus)
```

`startStatus` picks the controller type from persistent info: `SpaceID ==
personalSpaceId` → personal; `EncodedKey == ""` → shareable; else → streamable
(`load.go:80-86`). A `waiting` map + per-space channel dedupes concurrent builds.

### 6.2 Account bootstrap (`service.Run`)

- **New account** (`createAccount`, `service.go:339`): init marketplace → `createTechSpace`
  → `create(ctx,nil)` (personal space) → `watcher.Run()` → `techSpace.StartSync()`.
- **Existing account** (`initAccount`, `service.go:273`): init marketplace →
  `loadTechSpace` (15s deadline). Fallbacks handle offline old-account restore:
  DeadlineExceeded + personal exists locally → `createTechSpaceForOldAccounts`;
  `ErrSpaceMissing` on nodes → same. Then `computeLazyMode` → `watcher.Run()` (which
  replays existing SpaceViews via `afterRun`) → optional lazy drain goroutine →
  `techSpace.StartSync()`.

Tech-space sync is deliberately started **after** the watcher is running.

### 6.3 Status × mode × effect

| AccountStatus | Controller mode | Process | Effect |
|---|---|---|---|
| `Unknown` / `Active` / default | `ModeLoading` | loader | build space; LocalStatus Loading→Ok |
| `Joining` (shareable only) | `ModeJoining` | joiner | ACL waiter; accept→`Active`, reject→`Deleted` |
| `Removing` (legacy, unwritten) | `ModeOffloading` | offloader | local offload |
| `Deleted` | `ModeOffloading` | offloader | delete local storage/files/indexes → LocalStatus `Missing` |

**LocalStatus during load** (`spaceloader`): `Loading` (or optimistic `Ok`) → on
success `Ok`; `ErrSpaceDeletionPending`→`Missing`+`WaitingDeletion`;
`ErrSpaceIsDeleted`→`Missing`+`Deleted`; ctx-cancel→untouched; else `Missing`.

### 6.4 Scenario walk-throughs

- **Create** (`create.go:106`): `spaceCore.Create` (random id) →
  `factory.CreateShareableSpace` (calls `MarkSpaceCreated`, then creates the SpaceView
  `AccountStatusUnknown`) → loader → `WaitLoad` returns the `clientspace.Space` →
  registered → `UpdateCoordinatorStatus`.
- **Load existing**: watcher replay → `onSpaceStatusUpdated` → `startStatus` builds the
  controller → mode from persisted AccountStatus → loader (optimistic-Ok fast path).
- **Join** (`join.go:10`): `factory.CreateInvitingSpace` → SpaceView `Joining` → joiner
  → `aclwaiter`; accept → `Active`(+aclHead) → next `Update` → Loading; reject →
  `Deleted` → Offloading. `InviteJoin`/`CreateActiveSpace` go straight to `Active`.
- **Delete (owner)** (`service.Delete`, `service.go:180`): write `AccountStatusDeleted`
  on the SpaceView → watcher → controller → `ModeOffloading`. `spaceOffloader.Run`:
  (a) `delController.AddSpaceToDelete(id)` (schedules the network delete), (b) local
  offload retry loop → `DeleteSpaceStorage` + `FileSpaceOffload` + `RemoveIndexes` →
  `LocalStatus Missing`. The deletion controller polls the coordinator and calls
  `spaceCore.Delete` (signed delete) for owner-owned spaces.
- **Leave / removed-by-owner**: `aclobjectmanager.processStates` detects my own
  `NoPermissions()` and writes `AccountStatusDeleted`; rest identical to delete.
- **Remote-delete detected**: deletion-controller poll sets `RemoteStatusDeleted` →
  watcher flips `AccountStatusDeleted` (+ participant-remove notification if the space
  was `Ok`) → Offloading.
- **Cancel leave / restore** (`join.go:96`): write `AccountStatusActive` → `Update`
  transitions Offloading→Loading.

### 6.5 Lazy multi-space loading (spec 2026-05-17)

When `config.PreferredSpaceId` is set and valid, `computeLazyMode` enables lazy mode:
the watcher **defers** non-preferred, not-yet-started spaces into `deferredStatuses`
instead of building them. The backlog is drained (bounded concurrency `preloadConcurrency=2`)
by any of: the `AccountPreloadRemainingSpaces` RPC, a 10s safety timer, or a
preferred-space-broken fallback (`maybeReleaseOnPreferredBroken`). `Get`/`Wait`/
workspace-open promote a single deferred space on demand via `ensureSpaceStarted`
(which reads the real persistent info so a guest/stream space isn't mis-built as
shareable). The decision (defer vs build), the `releasing` flag, and the backlog
snapshot/clear are one critical section under `s.mu`, atomic w.r.t. `drainDeferred`.

### 6.6 Shutdown (`service.Close`)

Cancel ctx, set `isClosing`, close all controllers concurrently, then the tech space,
then the watcher. Controller `Close` = `sm.Close()` (closes the current Process) then
`app.Close` (closes spacestatus). In-flight/late drains and builds re-check `isClosing`.

---

## 7. Sync (diffsync / headsync) & remote status

### 7.1 diffsync

Each loaded space owns an any-sync `headSync` component: an `ldiff.DiffContainer`
(range-based diff structures), a `DiffManager` holding one `Element{Id, Head=hash(heads)}`
per object (trees + ACL + KV store, kept live by a head-storage observer), a
`DiffSyncer`, and a `periodicsync` with period `SyncPeriod`.

A **diff round** (`diffSyncer.Sync`): get responsible peers → for each,
`DiffManager.TryDiff` negotiates the diff version and runs `ldiff.Diff` → returns
`newIds` / `changedIds` / `removedIds`. `applyDiff` routes them: the ACL id →
`syncAcl.SyncWithPeer`; the KV-store id → `keyValue.SyncWithPeer`; everything else
(object trees) → `treeSyncer.SyncAll`. If the peer lacks the whole space
(`ErrSpaceMissing`) it `SpacePush`es header+ACL+settings and re-diffs.

Triggered: **periodically** (`SyncPeriod`), **on head change** (observer keeps ldiff
current), **on demand** (`space.SyncHeads` runs one round now + resets the timer), and
**on foreground** — `service.SyncAllSpaceHeads` (`service.go:807`, GO-7302) fans out
`SyncHeads` over all `AllLoadedSpaceIds()` (only `ModeLoading` spaces), bounded to 10
parallel, fired by `networkState` on resume after >20s background.

### 7.2 Remote status pipeline

`coordinatorStatusUpdater` == the **deletion controller** (the only implementer of
`UpdateCoordinatorStatus`). Its `updateLoop` (immediate, then every 180s, plus
on-demand `notify()`) calls `coordinatorClient.StatusCheckMany(ids)`; `convStatus`
maps coordinator `SpaceStatus` (`Created→Ok`, `PendingDeletion→WaitingDeletion`,
else→`Deleted`) and pulls shared-space + per-space read/write limits. It writes a
`SpaceLocalInfo{RemoteStatus, ShareableStatus, WriteLimit, ReadLimit}` back via
`service.UpdateRemoteStatus` → `techSpace.DoSpaceView` → `SetSpaceLocalInfo`. That
SpaceView change re-enters the watcher, closing the loop (and possibly flipping
AccountStatus / sending a notification). Immediate polls are kicked from `service.Run`,
the **shareable** controller's `Start` (deferred; personal/streamable controllers hold
no updater), `create`/`CreateOneToOne`, `AddSpaceToDelete`, and after every coordinator
`SpaceSign` — itself serialized concurrency-1 in the `coordinatorclient` wrapper. It is
*also* called cross-subsystem by `core/payments` to refresh limits.

The deletion controller also (a) deletes owner-owned queued spaces
(`AddSpaceToDelete` ∩ coordinator-reports-owner → `spaceCore.Delete`), and (b) syncs
the shared-spaces limit.

> Note: `core/syncstatus/*` computes the **UI sync icon** (Synced/Syncing/Error/
> Offline) and is a *separate* subsystem — it does not write the SpaceView
> `RemoteStatus`.

---

## 8. App wiring & lifecycle idioms

A reimplementation must respect these any-sync `app.App` conventions:

- **Nested `ChildApp` hierarchy.** Controller app (registers `spacestatus`) → per-Process
  `ChildApp()` (registers the loader/offloader/joiner pipeline). `Start`/`Close`
  cascade in registration / reverse order. Components resolve deps across the parent
  chain via `app.MustComponent[T](a)` in `Init` (never at construction; `New()` only
  captures config). A closed child app can't be restarted — hence personal/streamable
  wrap the loader (`personalLoader`) to rebuild a fresh `loader.New` each `Start`.
- **`Run` = spawn goroutine that first blocks on `spaceLoader.WaitLoad`** — the shared
  readiness barrier for the pipeline's consumer components.
- **Retry-goroutine + close-channel pattern** (loadingSpace, offloadingSpace): a
  background retry loop, a `loadCh`/`loadErr`, a `WaitX` selecting on `ctx.Done()` vs
  the channel, and `Close` cancels ctx then waits.
- **Dependency interfaces** (`space/internal/components/dependencies/`) decouple the
  pipeline from the app: `SpaceIndexer`, `BundledObjectsInstaller`, `FileOffloader`,
  `IdentityService`, `MigrationService`, `BuiltinTemplateService`. Keep these as the
  injection seams. Note `IdentityService.RegisterIdentity(spaceId, identity, encKey)`
  with a **nil key = reuse the persisted key** (the participant-reload fast path).
- **Service-level couplings** injected in `service.Init` (`service.go:192-207`), beyond
  the pipeline seams: `subscription.Service` (the watcher subscription), `notificationService`
  + `objectstore.SpaceNameGetter` (participant-remove notifications), `inboxSender`
  (one-to-one invite resend), `identityService`, `aclJoiner`, and the
  `coordinatorStatusUpdater` (= the deletion controller). A clean-room build must provide
  all of these.
- **`config.Config` inputs** read by the service: `IsNewAccount()` (create-vs-init
  bootstrap branch), `PreferredSpaceId` (lazy mode), `AutoJoinStream` (stream auto-join
  with exponential backoff), and `PersistAccountNetworkId()` / `ResetStoredNetworkId()` —
  the network-id mismatch guard; the id is persisted **only after** a successful space
  init, and reset on failure. Persisting it unconditionally corrupts the guard.
- **Account metadata.** `service.Init` derives `accountMetadataSymKey` + `Payload` via
  `domain.DeriveAccountMetadata` (SLIP-0021 path `m/SLIP-0021/anytype/account/metadata`).
  The payload is injected into every `spacecore.Create`/join as this member's ACL metadata
  (the wrapped `ProfileSymKey`); other members read it from the ACL to decrypt this
  account's profile, and `AccountMetadataSymKey` encrypts the identity-repo profile push.
  The derivation is re-done independently in `core/identity` and `core/anytype/account`
  and **must stay byte-identical**, or cross-device profile decryption fails silently.

Component registration lives in `core/anytype/bootstrap.go` (order: `storage`,
`peerstore`, `coordinatorclient`, `streamOpener`, `spacecore`, `peermanager`, …,
`spacefactory`, `space`, `deletioncontroller`; plus a separate `BootstrapMigration`
storage-migration app, §10).

---

## 9. Concurrency & lifecycle invariants (must-preserve)

The doc above describes the happy path; these are the non-obvious invariants a naive
rewrite deadlocks, races, or corrupts on. Ranked by blast radius.

**Whole-subsystem:**

1. **Status writes must be `Equal()`-guarded (correctness, not perf).** Every
   `SetSpaceLocalInfo`/`SetSpacePersistentInfo` `Apply` re-enters the watcher → another
   status write. The `Equal` early-out breaks that feedback loop; "always apply" is a
   live-lock, not just extra I/O. (Also §3.4.)
2. **The watcher does not truly order per-space updates.** `onSpaceStatusUpdated` spawns
   a goroutine per event; two updates for one space are mutually excluded only by the
   shared `*status.mx`, with **no ordering guarantee**. Correctness survives *only*
   because `ctrl.Update()` re-reads the **live** SpaceView status (not the captured
   snapshot). Do not (a) run the callback synchronously "for ordering" — one slow build
   head-of-line-blocks status for every space; nor (b) trust the snapshot for the mode
   decision. (The `remoteStatus==Deleted` and `maybeReleaseOnPreferredBroken` branches do
   use the snapshot — a residual stale-notification edge.)
3. **Pipeline components must publish lifecycle changes *through* the SpaceView, never by
   driving their own controller.** `aclobjectmanager` writing `AccountStatusDeleted` flips
   the mode on the *watcher* goroutine, which closes the loader app the component runs
   inside — safe only because it's a different goroutine. A component that calls
   `ChangeMode`/`Update` directly deadlocks (it would join the goroutine closing its own
   app).

**Per-space:**

4. **"Close old Process (drain its goroutines) before building the new one" is the only
   thing serializing offload vs load on storage** — no lock guards `store.db`. The state
   machine runs `cur.Close(ctx)` then `Start(next)` strictly sequentially; making any
   `Close` async races `DeleteSpaceStorage` against `BuildSpace` on an offload↔restore
   flip.
5. **`Process(md)` must mint a *fresh* instance every call.** Closed any-sync child apps
   cannot restart and there is no double-Start guard; restart-safety is purely structural
   (`personalLoader` rebuilds `loader.New` each `Start`). Caching/reusing a Process spawns
   a second build goroutine against a stale, closed space.
6. **StateMachine transition contract:** same mode → no-op; a *different* in-flight target
   → `ErrTransitionInProcess` (rejected, not queued); same-target callers piggyback; the
   buffer-1 `notify` provably never drops a wakeup (only the loop resets `next→Unknown`,
   and it is the sole consumer). Start-failure falls back to `ModeInitial` (must always
   start) and sends `nil`/`ErrFailedToStart` to waiters.
7. **`lastUpdatedStatus` is advanced *before* the blocking `ChangeMode` and not rolled
   back on failure** — a latent "stuck controller": if `ChangeMode` fails, a later event
   with the same AccountStatus no-ops and the space never retries until the status
   actually changes.
8. **The `waiting` map is poisoned on a failed build (never deleted)** — every later
   `getCtrl`/`Join` for that id returns the cached error for the session. `Join`/`InviteJoin`
   additionally read the *pre-wait* waiter snapshot, so a `Join` racing an in-flight build
   that then fails can nil-deref (`ctrl.Mode()` on nil). Real hazards a unidirectional
   redesign (§11) would eliminate.
9. **Dual-path dedup requires registering the `waiting` entry *before* the SpaceView is
   created** (before the factory call fires the subscription); otherwise the watcher and
   the direct build race into two controllers for one space.

**Narrower traps:** `techspace.Do*` hold a bare **non-reentrant** mutex across the closure
(nesting `DoSpaceView`/`DoAccountObject`, or re-entering `Do*` for the same object,
self-deadlocks; there is no lock ordering); spacecore's ocache `TryClose` **must** stay
`false` (no refcount; a 60s-idle GC would otherwise close a live space held by an earlier
`Get`) and `CloseSpace` is *not* sticky (a later `Get` resurrects the space — "don't reopen
an offloaded space" is enforced only by the controller/mode layer); `techSpaceReady` is the
happens-before barrier for the `s.techSpace` pointer (close exactly once, only *after*
assignment; on total init failure leave it unclosed so `Get(techSpace)` blocks); `Close`
must `ctxCancel()` **before** `isClosing.Store(true)`, and the scattered `isClosing`
re-checks (`drainDeferred`, `applySpaceStatus`, `onSpaceStatusUpdated`) are what stop a
late timer-drain from building controllers `Close` already stopped tracking;
`MarkSpaceCreated` is cleared *before* `InstallBundledObjects` (a crash between them skips
bundled objects permanently); `spaceloader.onLoad` maps ctx-cancel → **leave status
untouched** (mapping all load errors → `Missing` would hide healthy spaces after any
mid-load shutdown).

Two dormant any-sync hazards worth respecting: **GO-7332** (ocache double-close on wake,
fixed in the pinned any-sync `entry.go`) and **GO-7333** (`TryRemove`-on-loading nil-deref,
dormant only because spacecore evicts via `Remove` under an `isActive`-gated GC — a
reimplementation that leans on `TryRemove`/GC eviction re-arms it).

---

## 10. Beyond the control plane: data/sync plane & platform integration

§1–§9 cover the *control plane*. A clean-room reimplementation also owns a **data/sync
plane and platform layer** the sections above under-cover — the highest-risk blind spots:

1. **Storage migration (data-loss risk).** §3.3 describes only the anystore backend, but
   existing users' data lives in **badger** or **sqlite** (`spacecore/oldstorage`,
   `badgerstorage`, `sqlitestorage`). A separate `BootstrapMigration` app
   (`spacecore/storage/migrator` + `migratorfinisher`), driven by the `AccountMigrate`
   RPC, migrates per-space trees, rebuilds the objectId→spaceId `bindId` index, and only
   then (gated on `SetMigrationDone`) destructively cleans up the old store. Ignoring this
   makes every pre-existing space read as empty/`ErrSpaceStorageMissing`.
2. **P2P transport + inbound sync RPC (not provided by any-sync).** `spacecore/localdiscovery`
   (mDNS/zeroconf LAN discovery, iOS local-network-permission probe, Android delegation),
   `spacecore/clientserver` (inbound QUIC listener with persisted port — discovery is
   inert if `!ServerStarted()`), `spacecore/rpchandler` (server side of
   `SpaceExchange`/`SpacePull`/`SpacePush`/`HeadSync`/`ObjectSyncStream`/`StoreDiff`/`StoreElements`),
   `spacecore/peer.go` + `spacecore/streamopener.go` (the subscribe-on-open, stream-tag
   multiplexing that carries many spaces over one peer stream), `spacecore/peerstore`
   (local peer↔space registry whose observers drive status). Node-only sync survives
   without discovery/clientserver, but the stream protocol + rpchandler are the wire
   contract.
3. **`credentialprovider`** overrides any-sync's default to produce the coordinator-signed
   **space receipt** (via `SpaceSign`); without it nodes reject space create/push.
4. **Crypto contracts that must be reproduced exactly** (else silent cross-device/-member
   failure): account-metadata derivation (§8); **push-notification key derivation**
   (`aclobjectmanager/pushnotificationkeys.go`: `pushDeriveSpaceKey(firstMetadataKey)`,
   `pushDeriveSymmetricKey(currentReadKey)`); the **key-value service** privacy encoding
   (`keyvalueservice`: salt = first ACL record's read key, derived key =
   `hex(SHA256(salt‖clientKey))`, value = `uint16 keylen‖clientKey‖value`) — this is how
   store/chat objects sync their per-object diff cursors cross-device.
5. **`spacecore/typeprovider`** — the authoritative `Type(spaceId, id) → SmartBlockType`
   resolver (prefix/CID-codec classification, tree-root fallback, persistent cache) that
   object loading depends on. (The sibling `space/typeprovider/` is a stale empty dir —
   ignore it.)
6. **One-to-one inbox flow** — `CreateOneToOneSendInbox` → `inboxSender.ResendFailedOneToOneInvites`,
   the `OneToOneInboxSentStatus` (`ToSend→Sent`) state machine driven by the inbox reading
   SpaceViews, and techspace `Set/GetInboxOffset`. DM creation is non-functional without it.
7. **Lower-risk but real:** `virtualspaceservice` (registers local-only space ids in the
   objectstore — tied to the deprecated marketplace path); `anystorage` corruption-backup
   (`<id>_backup_<ts>` + `ListCorruptedBackups`/`DeleteBackup`, surfaced by the
   `SpaceDeleteCorruptedBackup` RPC); the `coordinatorclient` `SpaceSign` rate-limiter.
   **Dead code to skip:** `internal/components/syncstopper` (registered nowhere; builds a
   `periodicsync` it never starts).

Test-encoded invariants worth promoting to contract points: `objectprovider` fail-fast
`LoadObjects` vs never-fail `LoadObjectsIgnoreErrs` (concurrency 10); `dedupqueue`
full-queue-drop + panic isolation + no-exec-after-Close; `keyvalueservice` uint16
key-length cap; the lazy-mode hook seams and the "preserve `EncodedKey` so a guest space
builds via `NewStreamableSpace`" rule (`space_lazy_test.go`).

---

## 11. Notes for a clean-room reimplementation

**Keep (hard contract other subsystems depend on):**

1. `clientspace.Space` — the object model consumers hold. Its `Do`/`DoCtx` locking
   semantics, `DerivedIDs()`, `CommonSpace()` escape hatch, `WaitMandatoryObjects`
   readiness gate, and `GetType/RelationIdByKey` are load-bearing.
2. `space.Service` verbs (`Get`/`Wait`/`Create`/`Join`/`Delete`/`TechSpace()`/
   `PersonalSpaceId`/`TechSpaceId`) and the documented error set.
3. The **SpaceView-as-source-of-truth + status axes** model, and that persistent info
   syncs while local info does not. Any new design still has to interoperate with
   existing synced SpaceView relations.
4. The `SpaceLoaderListener` (OnSpaceLoad/OnSpaceUnload) contract — fulltext indexing
   depends on it.
5. Deterministic derivation of personal/tech space ids (existing data can't be
   re-derived differently).
6. `SpaceIndexer` / `BundledObjectsInstaller` / `IdentityService` / `MigrationService`
   injection seams.
7. **SpaceView relations as a cross-subsystem contract.** `crossspacesub` (space
   enumeration), `spacesyncstatus` (sync icon), `pushnotification` (push keys +
   `IsAclShared`), and the Workspace→SpaceView mirror (`workspaceKeysToCopy`) all read
   specific SpaceView relations. Renaming/dropping them, or opening spaces without the
   `OnSpaceIndexOpened` signal, silently breaks these readers.
8. **The objectId→spaceId `bindId` binding** must be written when objects load
   (`source.Service.NewSource`), and the notification/`session.Context` side effects (§5.3,
   §3.4) preserved.
9. **On-disk storage migration** (badger/sqlite → anystore) and the crypto derivations
   (account metadata, push keys, KV privacy) — see §10; changing any of these loses data
   or breaks interop with existing installs.

**Candidates to redesign — and forward-looking goals:**

The reimplementation should not just clean up the current pain points but actively
enable a few strategic capabilities:

- **First-class lazy loading & pause/unload of spaces.** Today lazy mode only *defers*
  the initial build; there is no way to pause/evict an already-loaded space and reload it
  on demand. Make load ⇄ unload a supported, reversible lifecycle transition (a real
  "paused/unloaded" state distinct from offloaded/deleted), so idle spaces can be released
  and re-promoted transparently via `Get`/`Wait`. This subsumes today's `deferredStatuses`
  machinery into one uniform mechanism.
- **Memory efficiency as a design constraint.** Loaded spaces are the dominant driver of
  RSS (object caches, trees, diff managers, subscriptions). Design for bounded resident
  memory: cap the number of concurrently-loaded spaces, evict by LRU/idle, and make the
  per-space footprint releasable — pairing directly with the pause/unload capability above.
- **Prepare for spaces on different networks (future).** Today the account signing key +
  a single coordinator/`nodeconf` are assumed account-wide (network id is a global config
  guard, §8). To allow spaces that live on *different* any-sync networks later, keep the
  network identity **per-space** in the model rather than global: route
  coordinator/peer/credential resolution through a per-space network handle instead of the
  singleton. Full support needs any-sync changes, but the reimplementation can at least
  avoid baking the single-network assumption into the controller/spacecore contract.

1. **Lifecycle model: unidirectional vs dual-path (open decision).** `Join`/`InviteJoin`
   carry `// TODO: refactor using a unidirectional model where we change/create the
   space view and it asynchronously starts the controller` (`join.go:11,54`). Today
   create/join both *directly* build a controller *and* the watcher reacts to the
   SpaceView — two paths, careful dedup via `waiting`. The trade-off, to be decided
   before implementation:
   - *Unidirectional* — **all** lifecycle flows through "write SpaceView → watcher
     builds controller," making the reactive spine the only path. One code path, simpler
     invariants; but callers that expect a synchronous, error-returning `Create`/`Join`
     must instead await the space becoming ready (via `Wait`), so the error surface and
     RPC semantics change.
   - *Dual-path (today)* — keep direct build + reactive watcher with explicit dedup.
     Preserves synchronous `Create`/`Join` semantics and lower migration risk, at the
     cost of two paths that must stay consistent.
   Recommendation deferred; both are viable and this doc stays descriptive of the
   current dual-path behavior.
2. **`Current() any`** is untyped; callers type-assert to `loader.LoadWaiter`. A typed
   accessor (e.g. `WaitLoad(ctx) (clientspace.Space, error)` on the controller) would
   remove the assertion and the "wrong mode" failure.
3. **`CanTransition` is vestigial** (all Processes return `true`); mode gating lives
   entirely in `ChangeMode`, and the `spaceOffloader` component's `CanTransition==false`
   is dead code. Either drop it or make it authoritative — but note **offloading is *not*
   terminal**: `CancelLeave`/restore relies on `Offloading→Loading`, so any authoritative
   rule must still permit that. (See §9 for why fresh Process instances + drain-before-build
   are load-bearing regardless.)
4. **Marketplace space is deprecated** (GO-6259) — a reimplementation should be able to
   drop the `marketplacespace` controller and the `VirtualSpace` bundled-object install
   path entirely.
5. **`AccountStatusRemoving`** is dead (never written). Collapse the enum or make the
   Removing→Deleted transition explicit.
6. **Two overlapping status stores.** AccountStatus/AclHeadId live on the SpaceView
   *and* are cached in `spacestatus.SpaceStatus` *and* mirrored into the objectstore
   index for the subscription. The `Equal`-guard on writes is a **correctness invariant**
   (it breaks a re-entrant write→watcher→write loop, §9.1), not merely a churn optimization
   — a redesign that keeps the mirror must keep the guard. A single authoritative store
   with an explicit change feed would remove both the mirror and the feedback hazard
   (GO-7289 cold-start churn).
7. **Bootstrap fallbacks** (`initAccount` old-account/offline branches) are dense and
   hard to test — worth isolating behind an explicit "tech space resolution" step.

### Key file map

- Contract: `space/internal/spacecontroller/spacecontroller.go`,
  `space/internal/spaceprocess/mode/statemachine.go`
- Service & orchestration: `space/service.go`, `space/{load,create,join,waiter,spacewatcher,spacesub}.go`
- Controllers: `space/internal/{personal,shareable,streamable,marketplace}space/`
- Processes: `space/internal/spaceprocess/{loader,offloader,joiner,initial}/`
- Pipeline components: `space/internal/components/{builder,spaceloader,spacestatus,aclobjectmanager,participantwatcher,spaceoffloader,migration,personalmigration,dependencies}/`
- Domain space: `space/clientspace/{space,techspace,virtualspace}.go`, `space/internal/objectprovider/`
- any-sync adapter: `space/spacecore/`, `space/spacedomain/`, `space/coordinatorclient/`, `space/spacecore/credentialprovider/`, `space/spacecore/typeprovider/`
- Storage & migration: `space/spacecore/storage/{anystorage,migrator,migratorfinisher}/`, `space/spacecore/oldstorage/`, `space/spacecore/storage/{badgerstorage,sqlitestorage}/`
- P2P transport: `space/spacecore/{localdiscovery,clientserver,peerstore,peermanager}/`, `space/spacecore/{rpchandler,peer,streamopener}.go`
- Key-value: `space/clientspace/keyvalueservice/`, `space/spacecore/keyvalueobserver/`
- Tech space & views: `space/techspace/`, `core/block/editor/spaceview.go`, `core/block/editor/accountobject/`, `core/block/editor/workspaces.go`
- Status model: `space/spaceinfo/`
- Deletion/remote status: `space/deletioncontroller/`
- Factory: `space/spacefactory/spacefactory.go`
- Notifications & push: `space/internal/components/aclnotifications/`, `space/internal/components/aclobjectmanager/pushnotificationkeys.go`
- Consumers (outward): `core/block/service.go`, `core/block/object/idresolver/`, `core/acl/aclservice.go`, `core/subscription/crossspacesub/`, `core/space.go`, `core/workspace.go`, `core/account.go`
- Account metadata: `core/domain/account.go`
- One-to-one inbox: `core/inbox/inboxservice/`, `space/create.go`
