# Spec: Happy-Path Space Load Optimization

Status: phase 1 in review (GO-7312, PR #3170), phase 2 in progress
Scope: per-space init/load pipeline (`mandatoryObjectsLoad`, bundled-object install check, per-space migrations, reindex maintenance passes)
Related: GO-7302 (lazy space loading), participant store-only refactoring (separate spec), tech-space spaceview snapshotting (separate work)

## 1. Summary

On every space load we pay a fixed tax that is only needed when something changed: we build full
object trees for all mandatory objects, derive ~100 object IDs cryptographically, run an
install/verify pass over all bundled system types and relations, run two migrations that scan the
object store, run several full-collection scans, and issue one objectstore query *per object* to
find outdated heads. On an account with 78 spaces this produces ~390 redundant tree builds,
~8,000 crypto derivations, ~80k+ SQLite statements, and several full scans of a 378 MB
objectstore — all to conclude "nothing changed".

The fix: make every maintenance pass **prove cheaply that it has nothing to do**, using data we
already persist (objectstore details, headstorage, per-space checksums) plus a small set of new
per-space markers. The happy path becomes a handful of point reads per space; the full machinery
still runs whenever bundles change, migrations are pending, trees are missing, or the store was
wiped. No pass is removed — each is gated or made O(1)/O(changed) instead of O(all objects).

## 2. Measured baseline (startup profile, 2026-06-10/11, 78-space account)

- 542 trees built in the first 10s (25.4s cumulative build time, 50,360 changes replayed).
  ~390 of these are mandatory objects (78 spaces × ~5), ~149 are tech-space space views.
- 989 objects fully loaded then dropped with "heads not changed, skipping indexing" — pure waste.
- ~98k SQLite statements in 10s (one `SetInterrupt` watcher goroutine each); 44% of CPU samples
  inside sqlite `step`, 32% in physical `pread` (cold page cache).
- `WorkspaceOpen` takes 2.75s, gated by this storm; `AccountSelect` returns in 0.95s.

## 3. Current flow (verified, with file references)

Space load gating chain:

```
spaceloader.Run → startLoad                          space/internal/components/spaceloader/spaceloader.go:100
  loadingSpace.load                                  space/internal/components/spaceloader/loadingspace.go:121
    builder.BuildSpace → clientspace.BuildSpace      space/clientspace/space.go:131
      DeriveObjectIDs (blocking)                     space/clientspace/space.go:161
      go mandatoryObjectsLoad                        space/clientspace/space.go:189
    sp.WaitMandatoryObjects (blocking)               loadingspace.go:128 → space.go:284
  → LocalStatusOk; spaceLoader.WaitLoad unblocks aclobjectmanager, migration runner, etc.
```

`mandatoryObjectsLoad` (space/clientspace/space.go:216) runs **on every load**:

1. `indexer.ReindexSpace(s)` — synchronous part of the call (core/indexer/reindex.go:159).
2. `LoadObjects(derivedIDs.IDs())` — full smartblock opens (tree build + state apply) for
   Workspace, Home, Archive, Widgets (+ Profile in the personal space)
   (space/internal/objectprovider/objectprovider.go:121; ID list pkg/lib/threads/derived.go:37).
3. `go tryLoadBundledAndInstallIfMissing` (space.go:194) — every load: derives installable IDs for
   all `bundle.SystemTypes` + `bundle.SystemRelations` (~96 sources), queries installed objects,
   diffs against `StoredIds()`, attempts a **30s-deadline remote fetch** for any missing tree, then
   calls `InstallBundledObjects` (core/block/object/objectcreator/installer.go:65) which always
   runs `queryDeletedObjects` + `listInstalledObjects` even for an empty source list.
4. `go migrationService.RunMigrationsWhenIdle` (core/migration/service.go:93) — spawns a goroutine
   per space polling every 10s (forever while offline) before checking the already-persisted
   `MigrationObjectContextVersion` gate (core/migration/objectcontext.go:297).
5. `migrationProfileObject` (space.go:396) — personal space only; already cheap (HasTree gate).
6. `TreeSyncer().StartSync()`.

In parallel, the loader app runs the migration Runner
(space/internal/components/migration/runner.go:82) after `WaitLoad`, **on every load, ungated**:

- `systemobjectreviser` (systemobjectreviser/systemobjectreviser.go:63): queries **all** types and
  relations in the space (indexed on resolvedLayout), then for each bundled one **constructs the
  full bundled details object** (`BundledTypeDetails()` / `ToDetails()`, line 161-182) before
  comparing `Revision` — ~140 detail constructions per space to conclude no-op.
- `readonlyfixer` (readonlyfixer/relationsfixer.go:76): query on `relationFormat` +
  `relationReadonlyValue` — **no index covers these keys → full collection scan** per space.

`ReindexSpace` happy path (all checksums match, core/indexer/reindex.go:159):

- `buildFlags` — one point read of per-space checksums. Cheap, keep.
- async `reindexOutdatedObjects` (reindex.go:721): iterates **all** headstorage entries and issues
  **one `GetLastIndexedHeadsHash` query per object** (the `todo: make it more effective` at
  reindex.go:743). This is the dominant source of the per-statement storm: O(total objects)
  queries per start.
- async `addSyncDetails` (reindex.go:489): `ListIdsWithoutSyncDetails`
  (spaceindex/sync.go:40) uses `Not Exists` filters — **full collection scan** per space, steady
  state returns nothing.
- `saveLatestChecksums` (reindex.go:282): **unconditional write transaction** per space per load.

`DeriveObjectIDs` (objectprovider.go:55): ~5 smartblock types + ~7 system types + ~89 system
relations + space chat ≈ **~102 derivations per space**, each building a root change via
any-sync `CreateTree`/`DeriveTree` (marshal + hash + CID; signature on the personal-space path)
— objectcache/tree.go:125. Memoized only for the lifetime of the in-memory space object.
Additionally, `GetTypeIdByKey`/`GetRelationIdByKey` (space.go:307-321) re-derive on **every
call** with no memoization — these are hot in install/reviser paths
(`relationutils.FillRecommendedRelations`).

### What is already efficient (keep as-is)

- Per-space checksums short-circuit full reindex (`buildFlags`).
- Heads-hash skip inside the indexer (`GetLastIndexedHeadsHash` compare on open).
- `StoredIds()` is in-memory (headsync diff `ExternalIds()`), not a DB scan.
- Object-context migration has a persisted version gate on the workspace object.
- `spaceloader` optimistic-Ok fast path (no transient Loading for known-good spaces).
- objectstore per-space DB warm-up with bounded concurrency.

## 4. Happy-path waste inventory (per space, per load)

| # | Work | Cost today | Needed when | Happy-path target |
|---|------|------------|-------------|-------------------|
| W1 | `LoadObjects` mandatory trees | 4–6 full tree builds, blocks space Ok | tree missing locally (new device/join, corruption) | ≤6 headstorage point reads |
| W2 | `DeriveObjectIDs` system maps | ~102 root-change builds | first load ever; system lists changed | 1 kv read |
| W3 | Bundled install/verify pass | 3+ queries, marketplace opens, 30s remote fetch for missing | bundle lists/content changed; space became writable | 1 kv read, skip |
| W4 | `systemobjectreviser` | full types+relations query + ~140 bundled-details constructions | bundle revision changed | skip (same kv read as W3) |
| W5 | `readonlyfixer` | full collection scan | never (one-shot historical fix GO-2331) | skip (same kv read) |
| W6 | `reindexOutdatedObjects` | O(N) point queries | always (crash recovery) — but shape is wrong | 1 bulk read + merge |
| W7 | `addSyncDetails` | full collection scan | once per space (backfill), version bumps | 1 kv read, skip |
| W8 | `saveLatestChecksums` | 1 write tx always | checksums actually changed | compare, skip write |
| W9 | `RunMigrationsWhenIdle` poller | goroutine ×10s ticks per space | migration not yet done | 1 indexed read, no goroutine |

## 5. Design

### 5.1 Per-space maintenance markers

Add a small per-space, per-concern marker store in the **common objectstore DB** (same DB as
`indexerChecksums`, so a store wipe wipes the markers — exactly the invalidation we want).
Implementation: `util/keyvaluestore.NewJson` collection `spaceMaintenance`, key
`{concern}/{spaceId}`, exposed via objectstore API next to `GetChecksums/SaveChecksums`
(pkg/lib/localstore/objectstore/indexer_store.go:444):

```go
type MaintenanceMarker struct {
    Hash     string `json:"hash"`     // content hash the pass was completed against
    CanWrite bool   `json:"canWrite"` // ACL write permission at completion time (W3/W4 only)
}

GetMaintenanceMarker(ctx, concern, spaceId) (MaintenanceMarker, error)
SetMaintenanceMarker(ctx, concern, spaceId, m MaintenanceMarker) error
```

One key per concern (`systemObjects`, `syncDetails`, `derivedIds`) — separate keys avoid
read-modify-write races between the independent async writers (install goroutine vs migration
runner vs indexer).

**`systemObjectsHash`** — the single content hash gating W3+W4+W5:

```
sha256(
  bundle.TypeChecksum,            // content of all bundled types (generated)
  bundle.RelationChecksum,        // content of all bundled relations (generated)
  join(bundle.SystemTypes),       // the system *lists* (checksums cover content of all bundled
  join(bundle.SystemRelations),   //   objects; lists can change membership independently)
  SystemObjectsMaintenanceVersion // new manual escape-hatch const, like ForceObjectsReindexCounter
)
```

Rationale for one hash: the installer only has new work when the system lists grow; the reviser
only has new work when a bundled `Revision` is bumped (which changes the content checksum);
the readonly fixer is a one-shot keyed on the version const. All three are driven exclusively by
build-time bundle content, so one hash gates all three. Re-running all three on any bundle change
is cheap and rare (once per app release that touches bundles).

### 5.2 W1 — Skip mandatory-object tree loads (probe, don't build)

In `mandatoryObjectsLoad` (space/clientspace/space.go:226), replace the unconditional
`LoadObjects(s.derivedIDs.IDs())` with:

```go
missing := ids where headstorage entry is absent or marked deleted   // point reads (same data
                                                                     // HasTree uses, space.go:414)
if len(missing) > 0 {
    err = s.LoadObjects(loadCtx, missing)            // unchanged: remote fetch allowed,
}                                                    // profile-deleted workaround preserved
```

- New device / just-joined space: trees absent → exact current behavior (remote fetch).
- Space created on this device: `CreateMandatoryObjects` already ran in `BuildSpace`; probes hit.
- Profile-deleted legacy accounts: entry marked deleted → load path → existing tolerance in
  `loadObjectsAsync` (objectprovider.go:166-173) still applies.
- Stale index after crash: not this mechanism's job — `reindexOutdatedObjects` (W6) detects
  heads-hash mismatch and reopens exactly the affected objects.

Why it is safe to not open these objects at startup: nothing retains them — today they are
TTL-evicted from the object cache ~60s after start (measured: 291 trees built then evicted).
Steady-state consumers (subscriptions, identity, detail service) read from the objectstore
index; incoming sync changes still open the smartblocks through the cache, so on-change
indexing and hooks keep firing; opening with unchanged heads writes nothing to the store (the
indexer skips on heads-hash match); and the store-wipe path still opens everything via the full
reindex.

Behavior changes (accepted, reviewed 2026-06-12):

1. A space whose mandatory tree is *present but corrupt* now reports Ok at load and surfaces
   the failure on first open, instead of failing the load. (`stopIfMandatoryFail` turned out to
   be a dead flag — stored but never read; the old outcome for retryable corruption was a space
   stuck Loading forever, so this is an improvement.) Deleted-marked trees keep the old
   outcome: the probe treats them as missing, so the load path surfaces the same error (or
   profile workaround) as before.
2. Editor state migrations on mandatory objects defer from startup to first open, and their
   outputs CAN be externally observable via indexed details (e.g. the workspace v2 migration
   sets `spaceUxType`, which is copied to the spaceview and consumed by the vault UI). Accepted
   because shipped migrations have long since applied everywhere, defaults cover missing
   values, and any open or synced change applies them. **Future migrations on mandatory-object
   editors must not assume they run at startup** — if startup application is required, add an
   explicit trigger (e.g. probe a persisted version detail and force one open).
3. ~~Open-time self-heals stop running~~ — mitigated by the **link-derived details reconcile
   marker**: `Archive`/`Dashboard` reconciles (`isArchived`/`isFavorite` vs the tree links) are
   now awaitable, and on full success persist `HashLinksList(treeLinkIds)` (order/duplicate
   insensitive, xxhash) into the object's `headsState` doc (`SaveLastReconciledLinksHash`).
   On every space load `indexer.reconcileLinkDerivedDetails` compares that marker against the
   hash of the *indexed* outbound links (valid stand-in for the tree under the happy-path
   heads==lastIndexed invariant; verified order-preserving end to end) — two point reads per
   space, no tree builds, no detail scans. A mismatch (crash/error between tree apply+index and
   the detail writes) logs a WARN and opens the object once, which re-runs the authoritative
   tree-based editor reconcile and refreshes the marker; an absent marker (first run after
   upgrade, store wipe) logs INFO and does the same. The store links are deliberately only a
   *trigger*, never a write source: outbound links may be a superset of link blocks (mentions,
   file/dataview targets in legacy objects), and only the editors' `GetIds()` is authoritative.
   The remaining un-healed crash window is the `Workspaces.Init` name/icon push to the
   spaceview — accepted (self-heals on next workspace open or change; cheap follow-up:
   compare workspace vs spaceview store details).

### 5.3 W3/W4/W5 — Gate install pass and migration runner on `systemObjectsHash`

**Install pass** (space.go:230): before spawning `tryLoadBundledAndInstallIfMissing`:

```go
m, _ := store.GetMaintenanceMarker(ctx, "systemObjects", spaceId)
if m.Hash == currentSystemObjectsHash() && m.CanWrite == !s.IsReadOnly() {
    skip
}
```

The goroutine itself is unchanged; on completion with **zero missing sources and no error**, and
after the migration runner has also completed (see below), the marker is written.

**Migration runner** (runner.go:82): same check before running `systemobjectreviser` +
`readonlyfixer`. Marker is written only when `err == nil && migrated == toMigrate` for both.

Writer coordination: the install goroutine (clientspace) and the runner (loader app) both
participate in one logical pass. Two options; the spec recommends (a):

- (a) Move the install/verify call into the migration Runner as a third migration step
  (`bundledinstaller`), so a single component owns the gate, ordering (install → revise → fix),
  and the marker write. `clientspace` keeps only the *space-created* install (space.go:179) and
  drops `tryLoadBundledAndInstallIfMissing` entirely. This also removes the today-implicit race
  where the reviser can run concurrently with installation.
- (b) Keep both, use two sub-keys (`systemObjects.install`, `systemObjects.migrations`).

`CanWrite` in the marker handles the read-only edge: `InstallBundledObjects` returns early for
read-only spaces (installer.go:70) and the reviser's writes would fail, so today those spaces
re-attempt every load. With the marker, a read-only space records `CanWrite=false`; when ACL
grants write (observable via `aclobjectmanager` state change or simply the next load's
`IsReadOnly()` check), the marker mismatches and the pass re-runs once with write access.

Also fix inside the pass (done in phase 1): in `reviseObject`
(systemobjectreviser.go:106), look up the bundled object's `Revision` directly from
`bundle.GetType/GetRelation` and compare **before** constructing full details via
`BundledTypeDetails()/ToDetails()`.

### 5.4 W6 — `reindexOutdatedObjects`: bulk merge instead of N point queries (phase 1, done)

Replace the per-entry `GetLastIndexedHeadsHash` loop (reindex.go:742-756) with a single scan of
the `headsState` collection (documents are tiny: `{id, h}`) merged against the headstorage
iteration:

```go
indexed := store.ListLastIndexedHeadsHashes(ctx)  // one Find(nil) iter → map[id]hash
IterateEntries(...) { if indexed[id] != headsHash(entry.Heads) → reindex }
```

O(N) point queries → 1 query + O(N) in-memory compares. Both sources iterate in id order, so a
streaming merge-join is possible if the map's transient memory (~50–80 B/object) matters; the map
is simpler and fine up to ~1M objects. This also collapses the `SetInterrupt` watcher-goroutine
count for this pass from N to 1.

### 5.5 W7 — `addSyncDetails`: one-shot marker

Gate the pass (reindex.go:489) on marker `syncDetails` with `Hash = strconv(SyncDetailsVersion)`
(new const, bump when `helper.InjectsSyncDetails`'s relation set changes). Write after the first
fully successful pass. Network-mode switches don't matter: the pass only adds *missing* relations
(presence check, not value — sync.go:40-51), so it is a no-op for objects that already have them
regardless of mode. Store wipe wipes the marker → backfill re-runs, correct.

### 5.6 W8 — `saveLatestChecksums`: write only on change (phase 1, done)

`buildFlags` already loads the stored checksums; skip `SaveChecksums` when the stored record
equals the latest (reindex.go:282, 855). Removes one write tx per space per start. (Note:
`SaveChecksums` already dedupes the physical disk write internally; the win is skipping the
write-connection acquisition during the startup storm.)

### 5.7 W2 — Derived IDs: persist + memoize

Phase 4 (independent, lower priority):

- Persist `threads.DerivedSmartblockIds` as JSON under marker `derivedIds` with
  `Hash = sha256(join(SystemTypes), join(SystemRelations))`. On hit: 1 read replaces ~102
  derivations. On miss (first load, list change): derive only the missing entries, save.
  Valid because derivation is a pure function of (spaceId, uniqueKey, signing identity) — all
  fixed per space.
- Memoize `DeriveObjectID` per space in `objectcache` (map[uniqueKey]id + RWMutex,
  objectcache/tree.go:125) so `GetTypeIdByKey`/`GetRelationIdByKey` and
  `FillRecommendedRelations` stop re-deriving on every call. This helps the cold path too.

Note: `derivedIDs.SystemTypes/SystemRelations` maps are consumed in exactly one place outside
provisioning (`core/block/detailservice/set_details.go`), so an alternative is lazy
derivation on first access — rejected for now because the persisted map is simpler than making
the shared struct lazily mutable, and it also speeds up the *first* access.

### 5.8 W9 — `RunMigrationsWhenIdle`: gate before polling (phase 1, done)

Call `isObjectContextMigrationDone` (one indexed `QueryByIds` on the workspace id,
objectcontext.go:297) **first**; only start the 10s polling goroutine when the migration is
actually pending. Removes 78 long-lived pollers per session in the steady state. The migration
also persists its marker on the zero-files path so the gate can close for file-less spaces.

### 5.9 Resulting happy-path per space

```
buildFlags                 1 point read
saveLatestChecksums        0 (unchanged → skipped)              [phase 1]
derived IDs                1 point read                          [phase 4]
mandatory objects          ≤6 headstorage point reads, 0 tree builds   [phase 2]
systemObjects gate         1 point read → skip install + reviser + readonlyfixer   [phase 3]
syncDetails gate           1 point read → skip full scan         [phase 3]
objectContext gate         1 indexed read → no poller            [phase 1]
reindexOutdatedObjects     1 bulk headsState read + in-memory merge (async)   [phase 1]
checkFTQueueConsistency    1 point read (+1 sparse-indexed query) — unchanged
```

≈ a dozen point reads and one bounded bulk read per space; zero tree builds, zero full scans,
zero crypto derivations, zero writes.

## 6. Invalidation matrix

| Event | Effect |
|---|---|
| App release changes bundled types/relations content or lists | `systemObjectsHash` mismatch → install + reviser + fixer run once per space, marker rewritten |
| `SystemObjectsMaintenanceVersion` / `SyncDetailsVersion` bump | corresponding pass re-runs once per space |
| Existing `Force*ReindexCounter` bumps | unchanged behavior (checksums mechanism untouched) |
| objectstore DB deleted/corrupt-reinit | markers live in the same DB → all passes re-run; `flags.enableAll()` reindex as today |
| space store (trees) deleted, objectstore kept | mandatory-object probes fail → re-fetched/created |
| space becomes writable (ACL change) | `CanWrite` mismatch → system-objects pass re-runs |
| user deletes/archives an installed system object | no change vs today: load-time pass never reinstalled deleted-but-present trees; reinstall happens via explicit install API |
| crash mid-pass | marker only written on full success → pass re-runs next load (all passes are idempotent today) |
| cross-device skew (older app on another device) | unaffected: reviser/install are local-bundle-driven; their object changes sync as ordinary tree changes |

## 7. Failure modes & mitigations

- **Marker says done but state regressed** (e.g. partial sync ate an installed relation's
  details): the marker only asserts "this pass completed against this bundle content". Object
  *content* regressions are sync-layer concerns, same exposure as today between two loads.
  Escape hatch: bump `SystemObjectsMaintenanceVersion`.
- **Corrupt-but-present mandatory tree** (5.2): surfaced on first open instead of at load.
  Accepted; deleted-marked trees keep the old load-path outcome.
- **Bulk headsState read memory** (5.4): ~80 B/object transient; switch to streaming merge-join
  if profiling shows pressure on 1M-object spaces.
- **Two writers, one marker** (5.3): resolved structurally by option (a) single owner.

## 8. Phasing and expected impact

| Phase | Items | Risk | Expected win (78-space profile) |
|---|---|---|---|
| 1 (done, PR #3170) | W6 bulk merge, W8 skip-write, W9 gate-first, reviser revision-first compare | low | tens of thousands of statements + per-statement goroutines eliminated; 78 write txs; 78 pollers |
| 2 | W1 probe-don't-build | medium (behavior sign-off in 5.2) | ~390 tree builds and most of the 25.4s cumulative build time off the critical `WaitMandatoryObjects` path → directly cuts `WorkspaceOpen` latency |
| 3 | W3/W4/W5 systemObjects marker (+ runner restructure 5.3a), W7 syncDetails marker | medium | removes per-space full scans, marketplace opens, ~140 detail constructions, 30s remote-fetch attempts in shared spaces |
| 4 | W2 derived-ID persistence + memoization | low | ~8k derivations → 78 reads; faster type/relation id resolution everywhere |

Each phase is independently shippable and independently revertible (markers are additive; reading
code falls back to "run the pass" when a marker is absent).

## 9. Test plan

- **Unit**: marker get/set round-trip; hash composition stability; gate logic for each pass
  (hit/miss/canWrite-flip); reviser revision-first compare; bulk-merge equivalence with the
  per-id implementation on synthetic headsState/headstorage fixtures (including hash-mismatch,
  missing-entry, ftQueueCtr cases); mandatory-object probe (present / absent / deleted / error).
- **Integration** (fixture pattern per CLAUDE.md):
  - load existing space twice → second load: zero `GetObject` calls for mandatory ids, zero
    install queries, no marker rewrites (assert via store spies/mock expectations);
  - delete one mandatory tree from headstorage → load → object re-fetched/created;
  - bump a bundled relation revision (test bundle) → reviser runs once, marker updated, second
    load skips;
  - read-only space gains write permission → install pass runs once;
  - wipe objectstore → all passes run, indexes rebuilt, markers repopulated;
  - kill the process between install and marker write → next load re-runs pass.
- **Perf regression harness**: count SQLite statements, tree builds, and goroutines created
  during AccountSelect on a multi-space fixture; assert happy-path budgets (e.g. ≤20 statements
  per space, 0 tree builds for warm spaces).
- **Manual**: cold start on the 78-space staging account; verify `WorkspaceOpen` latency,
  "heads not changed, skipping indexing" count ≈ 0, and that a release with a bundle change
  performs exactly one maintenance pass per space.

## 10. Observability

- Per-space load summary log: which passes ran vs skipped and why
  (`marker-hit | marker-miss:<reason> | forced`), duration of each.
- Existing reindex metrics unchanged; add counters for `mandatory_objects_probe_miss`,
  `system_objects_pass_runs`, `outdated_scan_duration_ms`.

## 11. Out of scope (related work)

- Tech-space space-view trees replay full history every start (snapshot_counter 1–2; 149 trees,
  9.5s I/O) — needs tree snapshotting/compaction, separate effort.
- Participant smartblock loads per ACL member (`aclobjectmanager.processStates`) — covered by the
  participant store-only refactoring spec.
- Lazy/deferred space loading on AccountSelect (GO-7302) — complementary: it reduces *how many*
  spaces load eagerly; this spec reduces *what each load costs*.
- headsync `FillDiff` double-diff allocation churn (any-sync).
