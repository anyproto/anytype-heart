# Derived-object delete: one store shape, not two

Status: plan, not implemented. Evaluates the proposal to stop wiping the index
row when a derived object (type / relation / relation option / template) is
uninstalled on the device that performed the uninstall, so its local store
matches what every other device already has.

Everything below marked **executed** was proven by running code (probes against
the real store fixture on `develop` @ `715a9f976`, or existing tests); marked
**traced** was established by reading the code paths end to end. Nothing here
required a live account, except the items listed in §9.

## 1. The two shapes, confirmed

"Uninstall a type" is not a deletion: `deleteDerivedObject`
(`core/block/delete.go`) sets `isUninstalled=true` and applies — it never calls
`DeleteTree`, and the space settings object would refuse a derived id anyway
(`ErrCantDeleteDerivedObject`). The tree survives on every device. Only the
local index diverges:

- **Deleting device.** The Apply indexes the full row (`injectDerivedDetails`
  stamps `isDeleted=true` whenever `isUninstalled` is present —
  `core/block/editor/smartblock/detailsinject.go`). `spaceIndexer.Index` blocks
  on its batcher, so that write commits first. Then `BeforeDelete` →
  `spaceindex.DeleteObject` **replaces** the row (`UpdateObjectDetails` is
  wholesale, not a merge) with `{id, spaceId, isDeleted}` — on `develop`
  (GO-7433, PR #3237) plus a nested `deletedSnapshot` of
  creator/createdDate/type/resolvedLayout/… Either way `name`, `isUninstalled`,
  `relationKey`, `uniqueKey`, `sourceObject`, `resolvedLayout` are gone as
  top-level keys. `DeleteObject` also deletes the headsState row, erases the
  object's outbound links and enqueues fulltext removal.
- **Every other device, and any cold sync.** The change arrives via
  `StateAppend`; `injectDerivedDetails` runs; the indexer writes full details
  (no `isDeleted` special-casing in `spaceIndexer.index`), current links, and
  the heads hash. `BeforeDelete` never runs. Full row + both flags, immediately.

The tombstone is transient even on the deleting device: `DeleteObject` removed
the headsState row, so at the next space load `reindexOutdatedObjects`
(unconditional, async behind the reindex limiter — `core/indexer/reindex.go`)
sees `"" != headsHash(entry.Heads)` and re-opens + re-indexes the surviving
tree. An existing test proves the trigger (an id with no headsState entry is
reindexed — `reindex_test.go`, `objNeverIndexed`). So **the steady state on
every device is the full row with both flags**; the wipe buys a window of
absence on one device that lasts, in practice, the rest of the app session.

Executed (probe, deletionaudit package + spaceindex fixture on `develop`):
after `DeleteObject` on a full corpse row, `isUninstalled` and `name` are gone,
`deletedSnapshot` is present, the headsState row is absent.

## 2. Why the wipe exists — decomposed by side effect

`spaceindex.DeleteObject` bundles four effects. They must be judged separately
for a *real deletion* (tree destroyed) versus a *derived uninstall* (tree
survives):

| Effect | Real deletion | Derived uninstall |
|---|---|---|
| Detail wipe | Correct: the tree is gone; whatever the row said can no longer be resolved against anything. GO-7433 deliberately snapshots the audit-relevant subset. | Wrong fit: the tree still holds everything; the wipe only manufactures a shape no other device has, and the row returns in full at the next load anyway. |
| headsState delete | Correct: no tree, no heads. | Counterproductive: it is what forces the (wasteful) full re-open + re-index of the object at every next load. The Apply already saved the current hash. |
| `eraseLinksForObject` | Correct: a destroyed object has no outbound links. | Divergent: receiving devices keep the links (the indexer rewrites them on every index). Erasing them makes backlink sets differ per device until the next load re-adds them. |
| Fulltext removal enqueue | Correct. | Redundant: the Apply's own index enqueues the id (heads changed), and `prepareSearchDocs` returns zero docs for any row with `isDeleted=true`, which makes `filterOutNotChangedDocuments` remove all existing docs. FT converges identically with or without the wipe (traced through `core/indexer/fulltext.go`). |

The famous comment — "do not completely remove object details, so we can
distinguish links to deleted and not-yet-loaded objects" — justifies keeping *a
row at all* (row with `isDeleted` = known-deleted; no row = not yet synced). It
does not justify stripping the details: the distinction holds exactly as well
with full details present, because it is carried by the `isDeleted` flag, which
both shapes have. The wipe predates the injected `isDeleted != true` default
filter (commit `28e800f24` "filter-out deleted objects by default"); hiding by
absence was once the only hiding there was. It no longer is.

## 3. What actually hides a corpse today

Every plain `Query` injects `isDeleted != true`
(`pkg/lib/database/database.go`, `addDefaultFilters`), and `NewFilters` is also
what the subscription service compiles for both snapshot queries and live
matching (`core/subscription/service.go` `compileFilters` — v1 cross-space
subscriptions go through the same per-space compile; the `QueryIterateRaw`
calls in `spacestate.go` reuse the compiled `FilterObj`, "raw" there means raw
sort order, not raw filters). Fulltext results are re-filtered through the same
compiled filters (`resolveFulltextResults`), and FT docs for `isDeleted` rows
are removed at index time. Export skips `isDeleted` explicitly
(`core/block/export/export.go`). The graph builds its node set from a
subscription search and only draws edges between surviving nodes
(`core/block/object/objectgraph/graph.go`). On top of that, this branch and
`develop` have been steadily adding *explicit* `isUninstalled` filters where
the injected default is not enough: `ListRelationOptions`
(`spaceindex/relations.go`), the API-key namespaces
(`objectcreator/apikey.go`), the v1 API type/property cache
(`core/api/service/cache.go`), the v2 `livePropertyFilters` (§8.41).

Flag-based hiding is the enforced policy already. The wipe is a second,
redundant mechanism on exactly one device — and §4 shows it is not merely
redundant but harmful.

## 4. What the wipe breaks today — two live bugs, both executed

Both follow from the same mechanism: **no key-filtered query can see a
tombstone**, because the tombstone lost every key the query filters on.

1. **Deletion audit (GO-7433, merged to `develop`).**
   `materializeUninstalled` discovers uninstalls via `uninstalledFilters()` —
   `isUninstalled == true` — but the tombstone carries no `isUninstalled` (it
   is in neither `SnapshotOnDelete` nor `preservedOnDelete`). Executed: after
   `DeleteObject`, both the discovery query and the output query
   (`auditFilters()`: `isDeleted && deletedDate exists`) return zero rows. The
   uninstalled type/relation is **entirely absent** from the audit on the
   deleting device — the one device whose user just did the uninstalling —
   until the next space load. The package's own doc comment ("The object also
   keeps its name, which no destroyed object can") and its own tests (which
   seed only the full-corpse shape) show the feature was designed for the
   receiving-device shape; the tombstone window is a hole its authors did not
   model.

2. **Same-session reinstall of a bundled type/relation.**
   `InstallBundledObjects` → `queryDeletedObjects`
   (`objectcreator/installer.go`) filters on
   `resolvedLayout IN (type, relation) AND sourceObject IN (...) AND
   (isDeleted OR isArchived)`. The tombstone has neither `resolvedLayout` nor
   `sourceObject`. Executed (probe with the exact filter tree): the corpse
   shape is found, the tombstone is missed. Traced onward: the miss falls
   through `listInstalledObjects` (also blind: injected `isDeleted` filter)
   into `installObject` → `createObjectInSpace`, which hits the existing
   derived tree; `ErrTreeExists` is tolerated and nothing flips
   `isUninstalled` back. Net: on the deleting device, "remove a type, then
   re-add it from the library" silently no-ops for the rest of the session.
   Receiving devices, and the deleting device after a restart, reinstall fine.

Plus the divergences that are not (known) bugs but are per-device
inconsistencies for the whole session: id-keyed subscriptions and dependency
detail fetches lose `name` on one device only; `FetchRelationByKey` /
`GetRelationLink` / `GetRelationById` fail on one device only (executed:
`FetchRelationByKey` resolves the corpse shape, errors on the tombstone);
backlink sets differ (links erased on one device only); and every one of these
flips back at the next load.

## 5. The proposal, concretely

Scope: `core/block/delete.go` only. In `deleteDerivedObject`, stop calling
`BeforeDelete`. Inline the one side effect a derived uninstall still wants —
`ObjectCloseAllSessions()` inside the existing `spc.Do` — and let the Apply's
own index write stand as the final store state, exactly as on a receiving
device.

What each `BeforeDelete` ingredient becomes for the derived path:

- `ObjectCloseAllSessions` — keep (close open editors on this device).
- `SetIsFavorite(id, currentValue)` — drop. It re-asserts the current value
  (likely a latent oddity even on the real-delete path); receiving devices
  never run it.
- `b.SetIsDeleted()` (blocks further Apply/StateAppend on the cached instance)
  — drop. Receiving devices never mark uninstalled objects; reinstall applies
  `isUninstalled=false` through the same object, and the mark would only ever
  be shed by the cache eviction that `DeleteObjectByFullID` already does
  (`spc.Remove`). Dropping it makes the deleting device behave like every
  other.
- `spaceindex.DeleteObject` — drop for derived. Details, links, headsState
  stay as the Apply indexed them; fulltext removal still happens through the
  normal pipeline (§2, traced).

`spaceindex.DeleteObject` itself is untouched: real deletions (trees
destroyed), `reindexDeletedObjects`, and the marketplace bundled-template
cleanup keep their semantics, and GO-7433's snapshot logic stays exactly as
merged. `sendOnRemoveEvent` and `spc.Remove` in `DeleteObjectByFullID` stay.
`deleteRelationOptions` and `unsetDefaultTemplateId` run as today (each option
goes through the same fixed path). Re-uninstalling an already-uninstalled
object degrades to a no-op Apply.

## 6. What breaks if details stay — the honest inventory

The test is: what depends on the removed object's details being *absent*, as
opposed to flag-filtered? Checked surface by surface (all traced, filter
behavior executed where noted):

- **Search / queries / subscriptions (v1 incl. cross-space, v2)** — hidden by
  the injected `isDeleted` filter in both shapes; no change.
- **Fulltext** — docs removed on the flag; no change (§2).
- **Export** — explicit `isDeleted` checks; no change.
- **Graph** — node set is flag-filtered, edges need both endpoints; no change.
- **Raw scans** (`IterateAll` in the FT rebuild and consistency check) — both
  check the `isDeleted` flag per row, not row absence; no change.
- **Point lookups and key probes** (`GetDetails`, `QueryByIds`,
  `GetRelationById`, `FetchRelationByKey`, dependency details for id-keyed
  subscriptions) — these DO change on the deleting device during the window:
  they go back to serving the corpse (with its flags). That is not new
  exposure: it is precisely what every receiving device serves today, and what
  the deleting device itself serves after its next restart. A user who removed
  a property and expects its name gone from these channels never actually had
  that on any other device or beyond the current session; the durable hiding
  is, and remains, the flag filters. Callers that must not treat a corpse as
  live already filter (`GetObjectType` errors on the flag;
  `ListRelationOptions`, apikey namespaces, v1 cache, v2 livePropertyFilters
  exclude `isUninstalled`).
- **Storage** — corpse rows keep full details for the session instead of only
  after the next load; steady-state size is identical.

No consumer was found that requires the details to be absent rather than
flagged. The one place whose comment *sounds* like it needs absence — "links
to deleted vs not-yet-loaded" — needs only the row + flag (§2).

## 7. Migration

None needed. Existing tombstoned derived rows self-heal through the mechanism
that already runs today: `DeleteObject` removed their headsState entry, so the
first space load after upgrade (an upgrade implies a restart) re-indexes the
surviving tree and restores the full row — executed at the trigger level (the
probe confirms the headsState row is gone; the existing `reindex_test.go` case
proves ids without a stored hash are re-indexed) and traced through
`reindexDoc` → `space.Do` → `injectDerivedDetails` → `Index`. This is not new
behavior added for the migration: it is the very path that ends the window
today, which is also why *tombstone-shaped derived rows stop existing entirely*
once the fix ships — old ones heal at load, new ones are never created. The
heal is async (reindex limiter), so a very large account has the same
minutes-scale post-load window it has today; acceptable.

## 8. The audit feature, and the v2 corpse code

**Audit: fixed, not masked.** The proposal makes GO-7433's design assumption —
an uninstall leaves a full row carrying `isUninstalled` — an invariant on
every device, immediately. `materializeUninstalled` discovers the row on the
first `List` after the uninstall, stamps `deletedBy/deletedDate` from the
object's own tree (which always had the truth), and the entry appears with its
name. Executed: the corpse shape is discovered by `uninstalledFilters()` and
serves its name. No change to the audit package is required; its
"self-healing re-derive" already tolerates the indexer overwriting stamps.

**API v2 corpse machinery on this branch** (`relationObjectHoldingKey`'s
derived-id fallback, `seedTombstonedTypeProperties`, the `corpseShapes`
tombstone leg): none of it becomes *wrong* — under the proposal it stops
firing, because the store state it compensates for stops occurring for derived
objects. Recommendation: **keep it as defence-in-depth**, at least for one
release. It costs one point lookup on paths that have already missed, it keeps
v2 correct against any store written by an older binary mid-session or any
residual `DeleteObject` caller, and deleting it now would couple this branch's
correctness to a core change that lands independently. Follow-up (optional):
once the core fix has shipped and the §8.41 probes confirm no tombstone-shaped
derived rows in the wild, retire `seedTombstonedTypeProperties` and the
derived-id fallback together with the tombstone fixture leg. Either way the
provenance comments (corpse test header, `relationObjectHoldingKey` doc,
APIV2 §8.41/ADDRESSING §2.3-6) must be re-stated as historical/defensive
rather than "normally the rest of the app session" — the fixture rule stands:
`corpseShapes` models shapes, and the tombstone shape becomes "produced by
real deletions and by pre-fix sessions", not "produced by every UI delete".

## 9. Alternatives

| | (a) Keep details on derived uninstall (proposal) | (b) Wipe, but preserve `isUninstalled` + `name` | (c) Make receivers wipe too | (d) Document and leave |
|---|---|---|---|---|
| Consistency | One shape, everywhere, immediately | Still two shapes; a slightly less lossy tombstone | One shape only if the reindexer also learns to re-tombstone uninstalled derived rows on every load — otherwise the row resurrects at each restart | Two shapes forever |
| Audit bug | Fixed structurally (executed) | Discovery + display fixed; still an exception path | Broken on **all** devices | Stays |
| Reinstall bug | Fixed (executed at filter level) | **Not fixed** — needs `sourceObject` + `resolvedLayout` too | Broken on all devices | Stays |
| Leak risk | None new: matches the existing majority/steady state; hiding stays flag-based | Same as today | Lowest leak surface, at the cost of breaking shipped features | Same as today |
| v2 corpse code | Becomes defence-in-depth, retirable | Still required | Still required, now for every device | Permanent requirement |
| Cost / blast radius | Small, `delete.go`-local; store semantics untouched | Small at store level, but the preserved set grows key by key (next consumer: `relationKey`? `uniqueKey`? …) until it has rebuilt the corpse | Large: StateAppend hook on the shipped sync path + reindexer changes; conflicts with GO-7433 | Zero now, paid on every future index consumer (three shapes to model) |

(b) deserves one more sentence: every consumer found in §4 needs a *different*
key the tombstone lost (`isUninstalled`, `name`, `sourceObject`,
`resolvedLayout`, `relationKey`, `uniqueKey`, format). Preserving them all is
(a) with more code and a second code path to maintain. (c) fights the
reindexer and breaks two shipped features everywhere. (d) leaves two proven
bugs in place.

**Recommendation: (a).**

## 10. Blast radius, testing, open items

This is core delete semantics on a shipped path, so the change must land with
its own evidence:

- **Existing tests that encode the wipe.** Store-level
  `TestDeleteObject` (`spaceindex/objects_test.go`) and `develop`'s
  `delete_test.go` (snapshot preservation) test `spaceindex.DeleteObject`
  itself — untouched, keep passing. No unit test asserts the tombstone at the
  `deleteDerivedObject` level (checked: no `_test.go` exercises that path's
  store outcome). The v2 `corpse_addressability_test.go` tombstone legs build
  the shape via fixtures and keep passing mechanically; only comments change
  (§8). `develop`'s deletionaudit tests already seed the corpse shape — the
  proposal makes them representative instead of optimistic.
- **New tests to add with the change.** (1) `deleteDerivedObject` leaves the
  full row: `name`, `isUninstalled`, `isDeleted`, `relationKey`/`uniqueKey`
  intact, headsState intact; (2) audit `List` includes a same-session
  uninstall (promote the §4 probe into a real regression test); (3)
  `queryDeletedObjects` finds a same-session uninstalled bundled object
  (reinstall regression test); (4) idempotence: double uninstall.
- **Regression watch.** Clients (ts/ios/android) receive the same
  `ObjectRemove` broadcast and flag-filtered subscription updates as today;
  the only observable client delta is that id-keyed reads keep serving details
  during the window (as they already do on other devices). Verify in QA:
  uninstall type/relation → search, sidebar, graph, dataview relation list on
  the *same* device; reinstall in the same session; audit list in the same
  session.
- **Could not settle without a live account.** End-to-end reinstall UX during
  the window (the filter miss is executed; the `ErrTreeExists` swallow is
  traced); whether any client reads the wiped row shape directly (judged
  unlikely — clients consume flag-filtered subscriptions); multi-device
  convergence timing.

## 11. Where this ships

The fix is core delete semantics, not API v2: it should land as its own change
against `develop` (post-GO-7433), under its own issue — GO-7433 is the natural
epic to attach it to, since it completes that feature's contract on the
deleting device; the reinstall bug (§4.2) may deserve its own issue number as
independently reportable. This document lives on the GO-7383 branch because
the tombstone window was isolated during the API v2 corpse-policy work
(§8.40–8.41), whose fallback code is the main in-repo consumer of the answer.

### Appendix: probe inventory (executed on `develop` @ `715a9f976`)

Throwaway probes, run in a scratch worktree, not committed anywhere:

1. Full corpse relation row → `DeleteObject` → `GetDetails` shows
   `isUninstalled`/`name` stripped, `deletedSnapshot` present;
   `uninstalledFilters()` and `auditFilters()` both return zero rows;
   `ListLastIndexedHeadsHashes` no longer contains the id. PASS.
2. Same row, no `DeleteObject` (proposed shape): `uninstalledFilters()` finds
   it, name intact. PASS.
3. `FetchRelationByKey`: resolves the corpse shape (today's receiving-device
   behavior), `ErrObjectNotFound` on the tombstone. PASS.
4. `queryDeletedObjects`' exact filter tree: finds the corpse, misses the
   tombstone. PASS.

Existing tests relied on: `reindex_test.go` ("objNeverIndexed" is re-indexed
when no heads hash is stored) — the heal/migration trigger.
