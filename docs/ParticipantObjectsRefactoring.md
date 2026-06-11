# Participant Objects Refactoring: Store-Only Participants

Status: implemented (all three phases) on this branch — see "Implementation notes" at the end
Related: startup profile findings 2026-06-10 (AccountSelect lavina), GO-7302 (lazy chats — same family of "do less on start" work)

## 1. Problem

On every app start, for every loaded space, `aclobjectmanager` reprocesses the full ACL
member list and materializes each member as a **participant smartblock**:

- `aclobjectmanager.processAcl()` iterates `aclState.CurrentAccounts()` and per member calls
  `UpdateParticipantFromAclState()` + `WatchParticipant()`
  (`space/internal/components/aclobjectmanager/aclobjectmanager.go:335-361`).
- Each `UpdateParticipantFromAclState` does `space.Do(participantId, ...)` → loads a smartblock
  through the object cache, applies details, runs the indexer
  (`space/internal/components/participantwatcher/participantwatcher.go:192-203`).
- `WatchParticipant` → `identityService.RegisterIdentity(...)`, which **synchronously invokes the
  observer callback with the cached profile** (`core/identity/identity.go:550+`), causing a *second*
  `space.Do` per member (`updateParticipantFromIdentity`).
- The participant editor pins itself in the object cache forever: `TryClose()` returns
  `false` (`core/block/editor/participant.go:102-104`).
- The skip check `lastIndexed == acl.Head().Id` uses an **in-memory** field
  (`aclobjectmanager.go:81,202-208`), so it never skips across restarts.

Measured on a real account (startup profile 2026-06-10, develop v0.50.8-83):

- **1,549 participant smartblocks loaded on start** (1,519 in a single space), ~2 loads+applies each.
- **2,202 participant smartblocks stayed pinned in the object cache until shutdown.**
- Each load runs the full smartblock pipeline: source → state build → template init → store query
  (`QueryByIds` in `participant.Init`) → Apply → indexer → sqlite/anystore writes — for objects whose
  details almost never change between starts.

## 2. Current architecture (verified)

The crucial fact: **participants are already virtual objects, and objectstore is already their only
persistence.**

| Aspect | Reality | Where |
|---|---|---|
| Source | `static` source — no tree, no storage; `PushChange` is a no-op, `ReadDoc` returns an empty state | `core/block/source/sourceimpl/service.go` (SmartBlockTypeParticipant case), `sourceimpl/static.go` |
| Editor | `participant` — thin wrapper; `Init` reads details back **from objectstore** (`QueryByIds`) and re-applies template | `core/block/editor/participant.go:44-72` |
| Persistence | spaceindex (anystore) record written by the indexer on every Apply | `core/indexer/spaceindexer.go:159-262` |
| Writes | 3 methods: `ModifyParticipantAclState` (ACL), `ModifyIdentityDetails` (identity profile), `ModifyProfileDetails` (own profile) — all just merge details and Apply | `participant.go:74-110` |
| Sync | participants are in `nonSyncableLayouts` — never synced, purely local derivations | `pkg/lib/localstore/objectstore/spaceindex/sync.go` |
| ID | `_participant_<spaceId>_<identity>`, type resolved by prefix, spaceId parseable from the id | `core/domain/id.go`, `space/spacecore/typeprovider/typeprovider.go:173` |

So the smartblock layer is used **only as a write funnel** into the store (plus restriction
enforcement and ObjectOpen rendering). The expensive part — load, state build, Apply, head-hash
bookkeeping — buys nothing: there is no tree, no sync, no history.

### Who reads participants (all store-only already)

Verified consumers and how they access participants:

- Member list / member get API: objectstore queries by `resolvedLayout == participant`
  (`core/api/service/member.go`).
- ACL removal subscription: cross-space subscription filtered by layout+status
  (`core/acl/participantsub.go`).
- aclupdater scheduling: store subscriptions only (`core/acl/aclupdater.go:90-155`).
- Chats: `spaceIndex.GetDetails(NewParticipantId(...))` for senders/mentions
  (`core/block/chats/service.go:535`, `chatsubscription/manager.go:273`).
- `creator` / `lastModifiedBy` relations: plain string ids resolved through store queries and
  dependency subscriptions.
- History, import id-mapping, sync status: string id construction/comparison only.
- Export/import: participants excluded by sbType check (`core/block/export/export.go`,
  `core/block/import/common/objectcreator`).

**No consumer outside `participantwatcher` requires a participant smartblock instance.**

### Client-facing surfaces that must keep working

1. `ObjectOpen`/`ObjectShow` on a participant id (member page). Today: static source + editor
   builds a read-only view; details come from the store in `Init`.
2. `ObjectSearch(Subscribe)` over participants — fired by store writes (see below).
3. Fulltext search over member names: participants fall into the `default` branch of
   `SmartBlockType.Indexable()` → **fulltext=true** (`pkg/lib/core/smartblock/smartblock.go:74-92`).
4. Edit protection: `objRestrictAll` for participants (`core/block/restriction/object.go:62`).

### Store write/event machinery already in place

- `spaceindex.ModifyObjectDetails(id, proc)` — atomic read-merge-write inside anystore `UpsertId`;
  diffs old vs new and **fires `sendUpdatesToSubscriptions` only on a real change**
  (`pkg/lib/localstore/objectstore/spaceindex/update.go:184+`). This is the exact same event path
  the indexer uses today, so internal/client subscriptions behave identically.
- `WriteTx` for batching, `UpdateObjectLinksDetailed` for the links collection,
  `AddToIndexQueueWithCounter` for fulltext, `BindSpaceId` for id→space binding — all public and
  already used by `spaceIndexer.indexBatch` (`core/indexer/spaceindexer.go:71-130`).
- Per-object persisted marker store: `GetLastIndexedHeadsHash` / `SaveLastIndexedHeadsHash`
  (`pkg/lib/localstore/objectstore/spaceindex/indexer.go`).

## 3. Goals / non-goals

Goals:

1. Zero participant smartblock loads during normal start (warm start, ACL unchanged).
2. Participant data written **directly** to spaceindex (anystore) from the ACL/identity layer;
   consumers only query the store.
3. Preserve behavior: subscriptions, member APIs, fulltext, restrictions, ObjectOpen.
4. Skip per-member reprocessing entirely when the ACL head hasn't changed since the last run.

Non-goals:

- Changing the participant data model, id scheme, or layout.
- Making participants syncable or tree-backed.
- Client (MW protocol) changes — nothing in the RPC surface changes.

## 4. Design

Three phases; each lands value independently and is independently revertable.

### Phase 1 — write directly to the store (removes ~2×N smartblock loads)

Introduce a small per-space component (suggested: `space/internal/components/participantstore`,
or evolve `participantwatcher` in place) that owns all participant writes:

```go
type ParticipantStore interface {
    // merge ACL-derived details (permissions, status, isHiddenDiscovery, ...)
    UpdateFromAclState(ctx context.Context, accState spaceinfo.ParticipantAclInfo) error
    // merge identity-profile-derived details (name, description, icon, globalName)
    UpdateFromIdentity(ctx context.Context, identity string, profile *model.IdentityProfile) error
}
```

Implementation of both methods:

1. `details := buildDetails(...)` — move `buildParticipantDetails` out of the editor
   (`core/block/editor/participant.go:112-131`) plus the static details the editor `Init` used to
   inject (see §5 "Details parity").
2. `spaceIndex.ModifyObjectDetails(id, merge)` — atomic merge; no event fired when nothing changed
   (this also makes the per-start rewrite cheap even before Phase 3: unchanged members produce a
   read+compare, no write, no subscription churn).
3. When the record is new or an FT-relevant key changes (old vs new `name`/`description` — the
   only two participant keys that produce FT docs, see §4.1): `objectStore.AddToIndexQueue(ctx,
   fullId)` — committed BEFORE the merge, so that a partial failure leaves the record unchanged
   and the caller's replay re-runs the whole write (a no-op replayed merge would otherwise never
   reach the enqueue again). See §4.1 for the full fulltext funnel including the consumer side.
4. On creation: `objectStore.BindSpaceId(id, spaceId)` (also before the merge, same reasoning —
   the binding feeds `idresolver.ResolveSpaceID`, which has no participant-prefix fallback).

Changes:

- `participantwatcher.UpdateParticipantFromAclState` / `updateParticipantFromIdentity` /
  `UpdateAccountParticipantFromProfile`: replace `space.Do(id, …Modify…)` with the calls above.
  (`UpdateAccountParticipantFromProfile` has **no callers** — delete it.)
- `core/block/editor/participant.go`: delete `ModifyProfileDetails`, `ModifyIdentityDetails`,
  `ModifyParticipantAclState`, `modifyDetails`, the `DetailsUpdatable` embed, and the
  `TryClose() (false, nil)` cache pin. The editor shrinks to `Init` only (read-only view for
  ObjectOpen). Participants opened by a user now evict from the cache by normal TTL.
- Delete the `participant` interface in `participantwatcher` (no more type assertion on smartblocks).

Concurrency: today per-participant writes are serialized by the smartblock lock; after the change,
`ModifyObjectDetails` runs the merge inside anystore's `UpsertId` modify-callback, which is atomic
per document — concurrent ACL/identity merges cannot lose updates.

Cold-start consideration (fresh login, 1.5k members, empty store): N individual `ModifyObjectDetails`
calls each open an implicit write. If profiling shows this matters, add a batched variant that
wraps the loop in one `spaceIndex.WriteTx` (same pattern as `spaceIndexer.indexBatch`).

#### 4.1 Fulltext funnel (mandatory part of Phase 1)

Enqueueing alone is **not** enough. The FT consumer loads smartblocks: `prepareSearchDocs`
(`core/indexer/fulltext.go:298`) runs `cache.DoContextFullID(i.picker, …)` to read relation values
and text blocks, then `TryRemoveFromCache`. Without a consumer-side change, every queued
participant would be materialized as a smartblock inside `ftLoop` — the loads would just move from
`processAcl` to the FT worker. Worse, `EnqueueAllForFulltextIndexing`
(`pkg/lib/localstore/objectstore/service.go:417`) enqueues **every store object** when the FT db
is wiped/rebuilt, so each FT rebuild would load all participants regardless of how careful the
writer side is.

**What a participant's FT docs actually are.** Verified against bundled relation flags
(`pkg/lib/bundle/relations.json`) and the doc-building loop (`fulltext.go:307-353`):

- `name` (shorttext, hidden — but force-included by `isName`) → doc `<id>/r/name` with `Title`.
- `description` (longtext, not readonly/hidden) → doc `<id>/r/description` with `Text`.
- `identity` and `globalName` are readonly+hidden → **already excluded from FT today** (member
  search by global name goes through store queries, not FT).
- Block docs: none — the participant template's title/description blocks carry
  `DetailsKeyFieldName` references and are explicitly skipped (`fulltext.go:364`).

So a participant's FT docs are a pure function of its store details. The codebase already
endorses store-side FT decisions: `RunFTConsistencyCheck` is documented as a check "that doesn't
load objects into cache" and uses `isFtIndexable(id, value)` built on the **static**
`typeprovider.SmartblockTypeFromID(id)` prefix resolution (`service.go:460-487`).

**Funnel design:**

*Producer* (`core/participants.ModifyDetails`): pre-read the record, compare old vs new
`name`/`description`; if the record is new or either changed, call
`objectStore.AddToIndexQueue(ctx, fullId)` BEFORE committing the merge (an enqueue that loses a
race is harmless — the FT consumer diffs against the index; the reverse order would lose the
enqueue forever, because a replayed merge is a no-op and never reaches it). The queue is a
persistent anystore collection (`fulltext_queue` in commonDb) with upsert-by-id dedup, so
repeated enqueues of the same participant collapse.

*Consumer* (`core/indexer/fulltext.go`): add a details-only branch to `prepareSearchDocs`,
selected by sbType **before** touching the cache:

```go
sbType, _ := typeprovider.SmartblockTypeFromID(object.ObjectId) // static, prefix-based
if sbType.FulltextDetailsOnly() {
    return i.prepareDetailsOnlySearchDocs(object, details), false, nil
}
// existing cache.DoContextFullID path for tree-backed objects
```

- `FulltextDetailsOnly()` is a new method on `coresb.SmartBlockType` next to `Indexable()`
  (`pkg/lib/core/smartblock/smartblock.go`), returning true for `SmartBlockTypeParticipant` —
  one decision surface for "FT docs derive from details, no object load". Other derived types can
  opt in later.
- `prepareDetailsOnlySearchDocs` mirrors the existing relation loop — iterate detail keys, keep
  short/longtext formats via `formatFetcher.GetRelationFormatByKey`, skip readonly/hidden bundled
  relations except `isName`, set `Title` for name on non-file layouts (layout read from
  `resolvedLayout` in the details) — but reads from the store details that `prepareSearchDocs`
  **already fetched at its top** (`fulltext.go:278`, currently used only for the isDeleted
  shortcut). No block iteration (nothing stored), no `cache.DoContextFullID`, no
  `TryRemoveFromCache`.
- The existing `filterOutNotChangedDocuments` diffing and `AutoBatcher` commit path are reused
  unchanged, so tantivy writes stay batched and idempotent.

This branch automatically fixes all FT entry points at once, because they all converge on
`prepareSearchDocs`: normal queue processing, `EnqueueAllForFulltextIndexing` (FT db rebuild),
`FtQueueReconcileWithSeq` (crash reconciliation) and `RunFTConsistencyCheck` re-enqueues.

**Crash consistency.** Today's indexer pairs FT enqueues with per-object heads-hash rows
(`SaveLastIndexedHeadsHashWithFtQueueCtr`) to detect lost enqueues. Participants don't need that
machinery; instead the invariant is: *advance the high-level progress marker only after store
writes + FT enqueues commit* —

- ACL path (Phase 3): the per-space ACL-head marker is saved at the end of `processAcl`; a crash
  mid-loop replays the whole loop next start (merges are idempotent, enqueue dedups).
- Identity path (Phase 2): persist the identity profile cache entry *after* fanning out the
  participant merges + enqueues; a crash replays the fan-out for that profile.
- Backstop: `RunFTConsistencyCheck` catches participants missing from FT entirely.

A possible follow-up (out of scope): the same details-only builder could serve the *relation*
docs of all object types, leaving the smartblock load only for block text — but that changes FT
behavior for every type and is not needed for this refactoring.

### Phase 2 — decouple identity fan-out from per-space observers

Today identity→participant propagation is observer-based and rebuilt at every start:
`WatchParticipant` registers an `(identity, spaceId)` observer whose callback writes to that
space's participant. This forces O(N-members) registration (including per-member ACL metadata
decryption in `processAcl`) on every start, even when nothing changed — otherwise profile updates
would stop flowing.

Replace the fan-out direction:

1. **Persist per-identity profile encryption keys** in the identity service's commonDb cache
   (alongside the existing `/identity_profile/{identity}` encrypted-profile cache). The key is
   identical across spaces for a given identity (enforced today: `RegisterIdentity` errors on key
   mismatch, `identityEncryptionKeys` is keyed by identity alone). Security delta is nil in
   practice: the decrypted profile already lands in objectstore on the same disk.
2. **Store-side fan-out**: when the identity service decrypts a new/updated profile, instead of
   invoking per-space callbacks, query the store cross-space:
   `layout == participant AND identity == X` → `ModifyObjectDetails` each hit (same merge as
   Phase 1's `UpdateFromIdentity`). One write per space the identity is actually a member of.
3. `RegisterIdentity(spaceId, identity, key, callback)` shrinks to
   `EnsureIdentityTracked(identity, key)` — idempotent, cheap, no callback. The one-to-one-space
   key discovery (`getOneToOneKey`, techspace spaceview metadata key) stays as the key *source*;
   it just feeds `EnsureIdentityTracked` now.
4. On start, the set of identities to track = persisted keys ∪ identities discovered during ACL
   processing. No per-space observer maps to rebuild (`identityObservers` collapses).

After this phase, ACL processing is the only remaining per-start O(N) work — and it no longer
needs `RequestMetadata` decryption for already-known identities.

### Phase 3 — skip ACL reprocessing when the head is unchanged

Persist the "last processed ACL head" per space and early-out:

- Marker: `spaceIndex.SaveLastIndexedHeadsHash(ctx, aclObjectId, acl.Head().Id)` — reuses the
  existing per-object heads-hash collection (used today by the indexer for ordinary objects),
  keyed by the ACL object id. Written at the end of a successful `processAcl`.
- `processAcl` start: `if persistedMarker == acl.Head().Id` → skip `decryptAll`, owner/status
  writes, and `processStates` entirely. Keep the existing deferred
  `notificationService.AddRecords(...)` call — it already runs on the in-memory skip path today
  and has its own progress tracking. Keep `SetAclInfo` derivation only when the marker advances
  (push keys / joined date are functions of the ACL contents).
- **Do not reuse `RelationKeyLatestAclHeadId`** on the spaceview for this: that detail tracks the
  latest *remotely known* head (written by join/create flows and `spacewatcher` from coordinator
  updates, consumed by `spaceloader`/`joiner` as a wait target). It moves ahead of local
  processing; conflating the two would skip work that was never done.
- Invalidation: `RemoveAclIndexes` (`core/indexer/indexer.go:153`) must also delete the marker —
  it is the wipe used by both the `ForceReindexParticipantsCounter` reindex path
  (`core/indexer/reindex.go`) and `aclindexcleaner` on space offload. Bump
  `ForceReindexParticipantsCounter` once when shipping this refactor so every account rebuilds
  participant records through the new write path exactly once (optional but makes details parity
  uniform; see §5).

My-status edge case: `processStates` also calls `status.SetMyParticipantStatus` and the
account-deleted transition (`aclobjectmanager.go:342-350`). These are idempotent spaceview writes
derived from the same ACL head — skipping them when the head is unchanged is safe (they were
applied when the marker was written).

## 5. Details parity (new-record construction)

Today a participant's store record is the editor state's `CombinedDetails()` — template +
detail-injection + the three Modify methods. The direct write path must produce an equivalent
record for **newly created** participants (existing records are already in the store and merges
preserve them):

| Key | Value | Today set by |
|---|---|---|
| `id`, `spaceId`, `identity` | from `ParticipantAclInfo` | `buildParticipantDetails` |
| `participantPermissions`, `participantStatus` | from ACL state | `buildParticipantDetails` |
| `isHiddenDiscovery` | `status != Active` | `buildParticipantDetails` |
| `identityProfileLink` | account object id (self) / profile id (others) | `buildParticipantDetails` / `ModifyIdentityDetails` |
| `lastModifiedBy` | participant id | `buildParticipantDetails` |
| `name`, `description`, `iconImage`, `globalName` | identity profile | `ModifyIdentityDetails` |
| `isReadonly=true`, `isArchived=false`, `isHidden=false`, `layoutAlign=center` | constants | editor `Init` |
| `resolvedLayout = participant` | constant | detail injection on Apply |
| `type` = space's participant type id | `space.GetTypeIdByKey(bundle.TypeKeyParticipant)` | template/Init |
| `backlinks` | maintained by backlinks updater (store-side) | unchanged |

Blocks (title/description/featured-relations) are **not** stored — they are rebuilt by the editor
template at ObjectOpen time, which is unchanged.

## 6. ObjectOpen behavior

Kept as-is: static source + slimmed editor whose `Init` queries the store. Opening a member page
works exactly like today, restrictions (`objRestrictAll`) still come from the sbType.

One real behavior change: an **already-open** participant page no longer receives live detail
updates through smartblock Apply events (writes bypass the smartblock). Reopening always shows
fresh data, and any client view backed by a subscription still updates live. If live updates on
the open page matter, two contained options (recommended as a follow-up, not a blocker):

- a) After a store write, update the in-memory object **only if it is currently cached**
  (`ocache.Pick`-style accessor on objectcache — does not trigger a load), sending the normal
  amend event.
- b) Have the participant editor `Init` register a per-id store subscription
  (`database.Subscription` on spaceindex) and translate updates into detail events; closed on
  object close.

## 7. What gets deleted

- `participant` interface + 3 `space.Do` write paths in `participantwatcher`.
- `ModifyProfileDetails` / `ModifyIdentityDetails` / `ModifyParticipantAclState` /
  `modifyDetails` / `DetailsUpdatable` / `TryClose` pin in the participant editor.
- Dead `UpdateAccountParticipantFromProfile`.
- Phase 2: `identityObservers` per-space callback machinery; `RegisterIdentity`'s synchronous
  cached-profile callback (`core/identity/identity.go:550+`).
- Phase 3: in-memory `lastIndexed` field in `aclobjectmanager`.

## 8. Risks & edge cases

| Risk | Mitigation |
|---|---|
| Missing detail keys on fresh records (parity bugs) | §5 table; one-time `ForceReindexParticipantsCounter` bump; integration test comparing old vs new record for the same ACL fixture |
| Fulltext regression (member names unsearchable) | producer-side enqueue on create / name / description change (§4.1); FT search test |
| FT worker (or FT-db rebuild) loads participants as smartblocks | details-only branch in `prepareSearchDocs` keyed by `FulltextDetailsOnly()` — covers queue, rebuild, reconcile and consistency-check entry points (§4.1) |
| Details-only FT docs diverge from smartblock-built docs | parity unit test: same details → identical `SearchDoc` set from both builders (only `name`→Title and `description`→Text docs exist for participants) |
| Subscription events lost or duplicated | `ModifyObjectDetails` fires only on real diffs — same primitive the indexer uses; subscription-service integration test (add member → event; restart → no spurious events) |
| Open member page goes stale | accepted (reopen refreshes); follow-up options in §6 |
| Guest-key spaces / one-to-one spaces take a different metadata path | guest-state synthesis stays in `processAcl` (untouched); one-to-one key discovery feeds `EnsureIdentityTracked` |
| Skip marker goes stale vs. store wipe | marker cleared inside `RemoveAclIndexes` (covers reindex + offload); marker lives in the same spaceindex db as the records, so deleting the space db drops both together |
| Identity key cache poisoning / mismatch | keep today's "key must match existing" check at `EnsureIdentityTracked` |
| Cold start with huge ACL (first login) | unavoidable O(N) once; optional `WriteTx` batching |

## 9. Testing

1. Unit: `participantstore` merge semantics (ACL-only, identity-only, both, no-change no-event)
   with `objectstore.StoreFixture`.
2. Unit: `processAcl` skip path (marker matches → no `processStates`; marker stale → full run;
   marker cleared after `RemoveAclIndexes`).
3. Integration: member add → record appears in store + subscription event + FT hit; permission
   change → single update event; member removal → status flips to Removed (record retained).
3a. FT funnel: `prepareDetailsOnlySearchDocs` parity vs the smartblock path (golden docs);
   name change → enqueue → searchable, permission-only change → **no** enqueue; FT-db wipe +
   `EnqueueAllForFulltextIndexing` reindexes participants with zero object-cache loads.
4. Integration: restart simulation — second `processAcl` over unchanged ACL produces zero store
   writes and zero events (Phase 1: zero *modified* writes; Phase 3: zero work).
5. Behavior: ObjectOpen on a participant id returns the same ObjectView as before (golden test).
6. Perf check on the 1,519-member space: start-time participant work and object-cache population
   before/after (expect: 0 participant smartblocks loaded; cache no longer pinned).

## 10. Expected impact (from the 2026-06-10 profile)

- −1,549 smartblock load+apply cycles on every start (each was: cache miss, static source, state
  build, template init, store `QueryByIds`, Apply, indexer head-hash check, store write).
- −2,202 permanently pinned cache entries (memory + shutdown close cost).
- FT worker no longer materializes participants either (today each queued participant goes through
  `cache.DoContextFullID` in `ftLoop`); FT-db rebuilds (`EnqueueAllForFulltextIndexing`) stop
  being a participant-load storm.
- Removes a large slice of the `aclobjectmanager.processStates` contribution to the
  AccountSelect→usable window and of the sqlite/anystore write amplification at start.
- Phase 3 additionally removes per-member ACL metadata decryption (`decryptAll`) for warm starts.

## 11. Implementation notes (what was actually built)

All three phases are implemented on this branch. Deviations and decisions vs the plan above:

- Shared write logic lives in the new `core/participants` package: `ModifyDetails`
  (atomic merge, creation from base details, BindSpaceId, FT enqueue on
  name/description change) and `BuildIdentityDetails`. `AclHeadMarkerId` (the Phase 3
  marker key) also lives there.
- Phase 1: `participantwatcher` builds ACL/base details (it owns the space-derived
  inputs: participant type id via `space.GetTypeIdByKey`, account object id via
  `techSpace.AccountObjectId()`) and delegates the write to `participants.ModifyDetails`.
  The editor (`core/block/editor/participant.go`) kept only `Init`; the `TryClose`
  cache pin, `DetailsUpdatable` and all `Modify*` methods were deleted. New-record
  parity details: `creator = addr.AnytypeProfileId` (what the static-source creation
  info produced), `type`, `resolvedLayout`, `isReadonly/isArchived/isHidden`,
  `layoutAlign`. Outgoing links are NOT written: `identityProfileLink` is hidden, so
  the old pipeline never put it into the links detail either.
- Phase 1 FT funnel: `SmartBlockType.FulltextDetailsOnly()` + a details-only branch in
  `prepareSearchDocs` (`core/indexer/fulltext.go`) using the details already fetched
  for the isDeleted shortcut; static `typeprovider.SmartblockTypeFromID` resolves the
  type without any load.
- Phase 2: `RegisterIdentity(spaceId, identity, key)` — callback parameter removed
  repo-wide (interface in `core/identity` + `space/internal/components/dependencies`).
  Keys persist in commonDb collection `identity_encryption_keys`
  (`crypto.SymKey.Marshall` / `UnmarshallAESKeyProto`); `nil` key means "use the
  persisted one". Fan-out: `identity.service.updateParticipants` writes
  profile-derived details into the participant records of the registered spaces
  (update-only — creation stays in the ACL path); registration itself does a targeted
  write for the registering space using the freshest cached profile (in-memory cache
  preferred over the persisted one). `GetMetadataKey` also falls back to the
  persisted key store. The per-(identity, space) observer map degraded to a plain
  set used only to scope identity-repo polling.
- Phase 3: `participantwatcher` owns the marker
  (`Get/SetProcessedAclHeadId` over `spaceindex` heads-hash collection, key
  `participants.AclHeadMarkerId`) and `WatchPersistedParticipants` (queries the
  space's participant records, registers each identity with its persisted key; any
  failure makes `processAcl` fall back to full processing). `processAcl` early-outs
  on marker match; the deferred `notificationService.AddRecords` still runs and
  `upToDate` is still computed (cheap). `RemoveAclIndexes` clears the marker, which
  covers both the `ForceReindexParticipantsCounter` reindex and space offload
  (`aclindexcleaner`).
- `ForceReindexParticipantsCounter` was NOT bumped: existing records already contain
  every key the new path writes, so no uniform rebuild is needed. Bump it later only
  if a parity gap surfaces.

Post-review fixes (general-purpose agent review of the diff):
- `ModifyDetails` commits BindSpaceId + FT enqueue BEFORE the merge (see §4.1) — the
  original after-merge order could lose them permanently on partial failure, since a
  replayed merge is a no-op.
- The own identity's encryption key is derivable on demand: `resolveEncryptionKey`
  falls back to `DeriveAccountMetadata` for `myIdentity` (and persists the result),
  so `RegisterIdentity(me, nil)` on the Phase-3 skip path never fails even when the
  key was never explicitly registered (e.g. accounts whose 1:1 spaces derived it via
  `GetMetadataKey` only, which previously skipped persistence).
- `updateParticipants` re-reads the freshest cached profile under the identity lock
  before writing — moving writes outside the lock had opened a narrow
  registration-vs-broadcast window where a stale snapshot could land last.

## 12. Open questions

1. Component placement: extend `participantwatcher` in place vs. new `participantstore`
   component (spec assumes a new interface either way; naming is cosmetic).
2. Should `lastModifiedDate` be bumped on participant detail changes for parity? (Today Apply
   sets it implicitly; consumers don't appear to read it for participants.)
3. Phase 2 key persistence: store the raw sym key in commonDb (like the encrypted profile cache)
   or behind the account's local encryption — needs a quick decision with whoever owns
   `core/identity`.
4. Do we want the §6(a) `Pick`-if-cached live-update path in the first iteration, or ship without
   and watch for client reports? (Recommendation: ship without.)
