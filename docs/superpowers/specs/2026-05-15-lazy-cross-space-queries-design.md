# Lazy cross-space objectstore queries

Date: 2026-05-15
Status: Draft (pending review)
Issue: GO-7288

## Problem

On app start the objectstore opens **every** per-space anystore DB in parallel, causing a
large disk-activity spike that scales with the number of spaces.

The eager loader is `dsObjectStore.preloadExistingObjectStores`
(`pkg/lib/localstore/objectstore/service.go:401`). It enumerates every space directory via
`anystoreProvider.ListSpaceIdsFromFilesystem()` and `Init()`s all per-space stores in parallel
under a `sync.Once`. It is reached from `listStores()` →
`collectCrossSpace` / `iterateSpacesForFulltext`, i.e. from any of:

- `QueryCrossSpace`, `QueryByIdCrossSpace`, `ListIdsCrossSpace`
- `IterateSpaceIndex`
- `EnqueueAllForFulltextIndexing`, `RunFTConsistencyCheck`

Several of these callers run during startup (file subsystem, fulltext indexing), so the
first cross-space query triggers the full parallel open — the spike.

The previously committed work (`801cbbc3e`, GO-7288) made only the *subscription* path
(`crossspacesub.Subscribe`) lazy and added `OnSpaceIndexOpened` instrumentation. It did **not**
change `collectCrossSpace`/`preloadExistingObjectStores`, so the spike remains.

### Secondary finding (pre-existing latent bug)

Because `preloadExistingObjectStores` takes a one-time `sync.Once` filesystem snapshot, a set
of "destructive" cross-space callers already make irreversible decisions from a possibly
incomplete local index (spaces that appear later, or are mid-reindex, are missed). This is a
pre-existing data-loss risk that the lazy change *widens*. It is addressed defensively here
(see WS-B) and tracked for a deeper follow-up.

## Goals

- Remove the startup disk spike: no synchronous parallel open of all spaces on the
  cross-space query hot path.
- Component `Run()` methods must not block on store loading; app start time unaffected.
- Cross-space queries that can tolerate a partial, self-healing result run lazily.
- Cross-space callers that require completeness for correctness get an explicit, opt-in
  wait that resolves against an **authoritative** space set (not an objectstore-derived one).
- No behavior change for the already-safe subscription consumers.

## Non-goals

- The `Synced`-only block-deletion defense-in-depth guard (call it "R2"): deferred to a
  follow-up. This spec accepts the residual data-loss window for not-`Synced` files until the
  later "wait for indexation" phase lands.
- Fulltext `EnqueueAllForFulltextIndexing` / `RunFTConsistencyCheck` redesign: out of scope.
  `RunFTConsistencyCheck` is currently dead code (`ForceFTRecheckCounter == 0`); the per-space
  FT-epoch recovery is a separate tracked item.
- A deeper "remote file node as source of truth" redesign for `DeleteFileData`/offload:
  tracked as follow-up; this spec only adds the wait gate + per-space scoping for reconciler.
- Waiting for per-space indexation/reindex to *finish* (only "store loaded" in this phase;
  the wait primitive is designed to be extended to this later).

## Background: caller classification

Investigated all cross-space call sites (14 focused research passes). Two mechanisms:

**Subscription consumers (`crossspacesub.Subscribe`)** — all event-driven, tolerate a
partial-then-growing result, **no change needed**: `api/service/cache.go` (×3),
`block/chats/service.go`, `filedownloader`, `core/object.go` CrossSpaceSearchSubscribe,
`acl/participantsub.go`, `pushnotification`.

**Direct one-shot queries** — split:

Bucket 1 — `accept-lazy` (self-heals or best-effort, on/near startup), **no change needed**;
they converge as stores open via the background warm-up:

| Site | Why safe |
|---|---|
| `core/files/queries.go:29,79` (dedup) | miss → re-upload only; graceful fallthrough |
| `core/files/fileobject/service.go:227` | reversible archive, re-runs each startup |
| `core/files/fileobject/service.go:262` | re-queued later; other paths backstop |
| `core/files/fileobject/fileindex.go:133` | 60s re-poll converges |
| `core/files/fileobject/indexmigration.go:59` | self-heals via per-object `File.Init` |
| `core/block/debug.go:75`, `core/debug/debug.go:45,103` | diagnostic only |
| `maybeRunFTConsistencyCheck` | per-space reindex-on-open self-heals |

Bucket 2 — `needs-wait` (correctness / data-loss), all **rare, user/GC/RPC-triggered, never
on startup hot path**:

| Site | Failure if partial |
|---|---|
| `core/files/fileobject/service.go:704` `DeleteFileData` | deletes shared blob still referenced elsewhere |
| `core/files/fileoffloader/offloader.go:131` offload-all | deletes only local copy of un-synced files |
| `core/files/fileoffloader/offloader.go:198` `offloadFileSafe` | data-loss on space-deletion path |
| `core/files/reconciler/reconciler.go:188` | missed space's files seen as orphaned → remote deletion |
| `core/block/template/templateimpl/impl.go:401` `TemplateExportAll` | silently incomplete archive |
| `core/debug/service.go:284` `DumpLocalstore` | incomplete bug-report artifact |

The local blockstore is a single global content-addressed flatstore
(`filestorage/flatstore.go:83-85`, keyed by CID only) — deleting blocks "for one space"
deletes them for all spaces, hence the data-loss severity.

## Design

### 1. `preloadExistingObjectStores`: keep, bound, run async

Keep the function but change how and when it runs:

- **Bounded parallelism.** Replace the unbounded `wg`-fanned `index.Init()` loop with a
  worker pool / semaphore of `preloadConcurrency` (const, default small, e.g. 4; tunable).
- **Async from `Run()`.** The objectstore component's `Run()` launches the warm-up in a
  background goroutine (`go s.backgroundWarmUp(s.componentCtx)`) and returns immediately.
  `Run()` never blocks; app start time unaffected.
- The warm-up iterates the **authoritative space set** (see §3), `Init()`s each per-space
  store with bounded concurrency, calls `markSpaceIndexOpened` per store (existing path), and
  marks itself complete (closes a `loaded` channel) when the full set is done.

### 2. `collectCrossSpace` / `listStores`: lazy by default

`listStores()` no longer triggers a synchronous full preload. It returns only the
currently-open `s.spaceIndexes`. The `sync.Once`-guarded eager preload is removed from the
hot path. Result: Bucket-1 callers see only already-open spaces and self-heal as the
background warm-up (and normal space activity) opens more.

### 3. Authoritative space set (Refinement R1)

The set the warm-up iterates and the wait primitive gates on must NOT be derived from the
objectstore (`space.AllSpaceIds()` and the spaceview subscription are themselves
objectstore-derived and lazy). Authoritative source:

```
authoritativeSpaceIds = spaceStorage.AllSpaceIds()  ∪  objectstore.ListSpaceIdsFromFilesystem()
```

- `spaceStorage.AllSpaceIds()` — `space/spacecore/storage/storage.go:23`, impl
  `space/spacecore/storage/anystorage/storageservice.go:56` — raw spacecore CRDT storage dir
  scan, independent of the index.
- `∪ ListSpaceIdsFromFilesystem()` covers objectstore data with no matching raw spacecore
  dir (orphaned index, pre-build leftovers).

**Implementation consideration / risk:** the objectstore component must obtain
`spaceStorage` (spacecore `ClientStorage`, provided as `spacestorage.CName`). Wiring must
avoid an app-component init cycle. If a direct dependency is not acceptable, inject a small
provider interface (`AllSpaceIds() ([]string, error)`) resolved lazily at warm-up time
(warm-up runs after `Run`, so the provider is available by then). The plan must verify the
app component graph; precedent exists (`filestorage/rpchandler.go:97` uses
`spaceStorage.AllSpaceIds()`).

### 4. Wait primitive

Add to the `ObjectStore` interface:

```go
// WaitStoresLoaded blocks until the background warm-up has opened every store in the
// authoritative space set, or ctx is done. Safe to call from any non-Run goroutine.
WaitStoresLoaded(ctx context.Context) error
```

Backed by a channel closed when `backgroundWarmUp` finishes the full authoritative set.
Designed to be extended later (phase 2) to also await per-space indexation completion
without changing the signature or callers.

(Equivalent alternative: a `WaitLoaded bool` field on `database.Query` consumed by
`QueryCrossSpace`. Chosen: explicit `WaitStoresLoaded(ctx)` — clearer call sites, reusable
by `IterateSpaceIndex`/`ListIdsCrossSpace` callers, no query-struct plumbing.)

### 5. Bucket-2 callers: opt into the wait

Each Bucket-2 site calls `s.objectStore.WaitStoresLoaded(ctx)` immediately before its
cross-space query and propagates any error. All are user/GC/RPC-triggered, so blocking on
the bounded warm-up is acceptable and never touches startup:

- `fileobject/service.go:704` `DeleteFileData`
- `fileoffloader/offloader.go:131` `offloadAllFiles`
- `fileoffloader/offloader.go:198` `offloadFileSafe`
- `templateimpl/impl.go:401` `TemplateExportAll`
- `core/debug/service.go:284` `DumpLocalstore`

### 6. Reconciler: per-space scoping (in addition to the wait)

`reconcileRemoteStorage` deletes *remote* files when a fileId is absent from the local
`haveIds`. Even with the wait, a space outside the authoritative set (or with a corrupt
index) would still be treated as orphaned. So additionally:

- Call `WaitStoresLoaded(ctx)` first.
- Build `haveIds` keyed by `(SpaceId, FileId)` and, in the `IterateFiles` callback, **skip
  any remote file whose `FullFileId.SpaceId` is not in `objectStore.OpenedSpaceIds()`** — a
  space that was not positively accounted for is never reconciled (never deleted).
  `IterateFiles` already exposes per-space `FullFileId.SpaceId`
  (`rpcstore/store.go:485-504`).

This makes "never delete a remote file for a space we cannot account for" a structural
guarantee, not a timing assumption.

## Data flow

```
app start
  └─ objectstore.Run()  ── returns immediately ──▶ app continues (no spike)
        └─ go backgroundWarmUp(ctx)
              authoritativeSpaceIds = spaceStorage.AllSpaceIds() ∪ ListSpaceIdsFromFilesystem()
              for each, bounded(preloadConcurrency): Init() + markSpaceIndexOpened()
              when all done: close(loadedCh)

Bucket-1 query  ──▶ collectCrossSpace ──▶ listStores() = currently-open stores (partial OK, self-heals)

Bucket-2 op (user/GC/RPC)
  └─ WaitStoresLoaded(ctx)  ── blocks on loadedCh ──▶ full authoritative set open
        └─ cross-space query  (reconciler also per-space scoped on OpenedSpaceIds)
```

## Error handling

- `backgroundWarmUp`: a per-space `Init()` failure is logged and that space skipped (as
  today); the warm-up still completes so `WaitStoresLoaded` cannot hang on one bad store.
  A skipped/failed space is excluded from `OpenedSpaceIds()`, so reconciler per-space
  scoping correctly refuses to reconcile it (no false orphan deletion).
- `WaitStoresLoaded`: returns `ctx.Err()` if the caller's context is cancelled before
  warm-up completion. Bucket-2 callers propagate the error and abort the destructive op
  (fail safe — do nothing rather than act on a partial view).
- Authoritative-set query failure (`spaceStorage.AllSpaceIds()` error): logged; warm-up
  falls back to `ListSpaceIdsFromFilesystem()` alone and records that coverage was
  degraded; `WaitStoresLoaded` still completes (callers degrade rather than hang).

## Testing

- `backgroundWarmUp` does not block `Run()`; `Run()` returns before stores are open
  (assert via timing / a blocking fake store `Init`).
- Bounded concurrency: with N spaces and limit K, at most K concurrent `Init()` calls
  (instrumented counter, max observed ≤ K).
- `WaitStoresLoaded` returns only after every authoritative-set store is opened; returns
  `ctx.Err()` on cancellation; completes even when one store `Init()` fails.
- Authoritative set = union of spacecore storage ids and objectstore fs ids (cover the
  divergence case: id present in one source only).
- Bucket-1 lazy query returns partial results before warm-up completes and converges after.
- Reconciler: a space absent from `OpenedSpaceIds()` never has its remote files enqueued
  for deletion, even when present in remote `IterateFiles` (regression test for the
  pre-existing data-loss bug).
- Existing crossspacesub and objectstore suites stay green under `-race`.

## Follow-ups (tracked, out of scope here)

1. R2: `Synced`-only local block-deletion guard in the offloader (defense-in-depth).
2. "Remote file node as source of truth" for `DeleteFileData` / offload reference checks.
3. Per-space FT epoch + re-enqueue on `OnSpaceIndexOpened`; revive/replace
   `RunFTConsistencyCheck`.
4. Phase 2: extend `WaitStoresLoaded` to also await per-space indexation completion.
