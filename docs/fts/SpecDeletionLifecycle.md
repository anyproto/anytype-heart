# Spec: FT deletion lifecycle (A2 + A3 + orphan GC)

## Problem

Tantivy deletion is exact-term on `IdRaw`, which stores full doc paths
(`objectId/r/relationKey`, `objectId/b/blockId`, `objectId/m/messageId`). Three holes:

1. `indexer.RemoveAclIndexes` passes bare object ids to `BatchDeleteObjects` → no doc matches →
   participants from removed ACLs stay searchable forever.
2. Space offload (`indexer.RemoveIndexes`) deletes the objectstore space index but never touches
   tantivy, and does not clear the space's FT queue entries.
3. `RunFTConsistencyCheck` only finds store-objects missing from FT; FT docs whose object (or
   whole space) is gone are never garbage-collected. Orphans cost disk, skew BM25 corpus stats,
   and only disappear on a full index rebuild.

## Design

### 1. Object-level deletion primitive

Make `BatchDeleteObjects(objectIds []string)` do what its name says:

```
for each objectId:
    docIds, pageFull = listDocIdsForObject(objectId)   // phrase query on tokenized Id + prefix filter
delete all collected docIds (exact IdRaw terms); re-list only pageFull objects, loop (capped)
```

- IMPLEMENTATION NOTE (changed from the original spec): the listing deliberately does NOT use
  `ListByIdPrefix`/`TermPrefixQuery` — prefix queries expand to at most ~50 terms per segment and
  deleted terms keep consuming the budget (see Research.md G.0, found by
  `TestBatchDeleteObjectsManyDocs`). Instead `listDocIdsForObject` runs a PhraseQuery on the
  tokenized `Id` field and filters the resulting IdRaw values by the exact `objectId + "/"`
  prefix (bare `objectId` docs included).
- `listDocIdsForObject` caps at 10k per call → objects whose page was full are re-listed after
  the delete until done (huge chats can exceed 10k message docs); the loop is iteration-capped.
- Doc-path-level deletion (used by the batcher) keeps its own method; rename internals if needed
  so the two granularities are impossible to confuse:
  - `BatchDeleteObjects([]objectId)` — object granularity (prefix).
  - `AutoBatcher.DeleteDoc(docId)` — doc-path granularity (exact).
- `RemoveAclIndexes`: stop ignoring the `QueryObjectIds` error.

### 2. Space offload cleanup

New ftsearch method:

```
ListIdsBySpace(spaceId string) ([]string, error)   // Must TermQuery(SpaceID) + fast-field IdRaw
```

`indexer.RemoveIndexes(spaceId)`:
1. `ClearFullTextQueue([spaceId])` — drop pending/tombstone queue entries of the space.
2. Loop: `ids = ListIdsBySpace(spaceId)`; batch-delete ids (doc-path granularity, they come from
   the index itself); repeat until empty.
3. Existing objectstore cleanup (unchanged).

Failure mode: FT-removal errors propagate out of `RemoveIndexes`, so the space offloader's retry
loop (every 20s, and re-offload after restart — `LocalStatusMissing` is only persisted on
success) re-runs the idempotent removal until it completes. A quit mid-removal therefore
self-heals; the orphan GC can NOT serve as backstop here, since it never iterates a space whose
store was deleted. No ordering hazard: by this point the space is not searchable through the
store anyway (results without details get dropped per record).

### 3. Orphan GC in the consistency check

Extend `RunFTConsistencyCheck` (runs once per session when the queue drains), per iterated space
only — never touch spaces that weren't iterated (lazy loading must not cause mass deletion):

```
for each iterated space:
    storeIds  = ids seen in store.IterateAll          (already iterated today)
    ftIds     = ListIdsBySpace(spaceId) → group doc ids by objectId
    missing   = storeIds where isFtIndexable && objectId not in ftIds   (exists today)
    orphans   = ftIds whose objectId not in store
    enqueue missing; BatchDeleteObjects(orphans)
```

Memory: per-space id sets instead of today's global `ListAllObjectIds` map; drop
`ListAllObjectIds` once both callers are migrated (its 1e9-docs listing is one of the memory
spikes). Deletion happens object-by-object through the same primitive as (1).

Safety valves (hardened after adversarial review):

- the check only runs once the objectstore warm-up finished (probed non-blockingly from the FT
  loop; retried at a later queue drain otherwise) — running against a partial space registry
  would permanently skip not-yet-opened spaces;
- orphan GC is skipped for a space whose store is EMPTY while its FT docs exist (far more likely
  a wiped/not-yet-rebuilt store than 100% genuine orphans);
- soft-deleted objects keep store stubs (`isDeleted=true`); those are treated as absent so their
  leftover FT docs — the main historical leak — stay collectable;
- a single GC pass is capped (50k objects) and deletions are chunked (1000 objects per
  ftsearch-lock acquisition); remaining orphans get collected next session.

Activation: `ForceFTRecheckCounter` bumped 0 → 1 (GO-7316) — without the bump the whole
consistency check, backfill and orphan GC are unreachable for existing users.

## Compatibility

No schema/index version change. Works on existing indexes. The repro test
`TestBatchDeleteObjectsDeletesAllDocsOfObject` turns green with (1).

## Tests

- (1) repro test + multi-page deletion (>10k docs object) unit test against real tantivy.
- (2) indexer test: RemoveIndexes deletes space docs + queue entries, keeps other spaces intact.
- (3) store test: orphaned FT docs get deleted, missing docs get enqueued, non-iterated spaces
  untouched.
