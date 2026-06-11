# Fulltext search fixes — plan

Companion docs: [Research.md](Research.md) (full findings),
[SpecDeletionLifecycle.md](SpecDeletionLifecycle.md), [SpecQueueGenerations.md](SpecQueueGenerations.md),
[SpecSearchLimits.md](SpecSearchLimits.md), [SpecScoringAndIndexLayout.md](SpecScoringAndIndexLayout.md).

## Confirmed bugs (each has a failing repro test on this branch)

| # | Bug | Repro test |
|---|-----|------------|
| A1 | `filterOutNotChangedDocuments` never filters: every object update re-indexes all its docs; the Iterate search is pure overhead | `core/indexer/fulltext_filter_repro_test.go` `TestFilterOutNotChangedDocuments` |
| A2 | `BatchDeleteObjects` called with bare object ids deletes nothing (IdRaw stores full doc paths, delete is exact-term) — ACL-removed participants stay searchable | `pkg/lib/localstore/ftsearch/ftsearch_repro_test.go` `TestBatchDeleteObjectsDeletesAllDocsOfObject` |
| A5 | tantivy search hard-capped at 100 docs before store filters/pagination; chat search shares the cap with all space docs | `.../ftsearch_repro_test.go` `TestSearchReturnsAllMatches` |
| B1 | `FtQueueMarkAsIndexed` overwrites queue entries, losing `ord`/`del`/pending state written while a batch was processing | `pkg/lib/localstore/objectstore/indexer_store_repro_test.go` `TestFtQueueMarkAsIndexedPreservesConcurrentUpdates` |
| A6 | chat search: float BM25 scores truncated to int64; SCORE comparator truncates diffs <1.0 to "equal"; offset past the end returns the first page again | `core/block/chats/service_search_repro_test.go` `TestService_SearchScoreSorting`, `TestService_SearchOffsetBeyondEnd` |

Confirmed by inspection (no test needed):

- A3: space offload (`indexer.RemoveIndexes`) never removes the space's tantivy docs; the
  consistency check is one-directional, so orphans accumulate until a full index rebuild.
- A4: `RunFTConsistencyCheck` prints full object details (user content) to stdout via leftover
  debug `fmt.Printf` (`pkg/lib/localstore/objectstore/service.go:882-897`).
- B5 papercuts: empty-string delete terms for chats (`core/indexer/fulltext.go:166`); one
  malformed doc id aborts the whole search (`spaceindex/queries.go:459`); per-object best-doc
  comparator truncates float deltas (`spaceindex/queries.go:482`).

## Implementation order (one commit per item) — ALL IMPLEMENTED on this branch

### Phase A — direct fixes, no design change

1. **A1** ✅ `filterOutNotChangedDocuments`: track ids seen during `Iterate`; treat a new doc as
   "new" only if its id was not seen. Turns the dedup on for the first time.
2. **A4** ✅ Replaced the `fmt.Printf` diagnostics with structured debug logging without
   user content.
3. **A6** ✅ Chat search: float scores kept alongside results and sorted on (`cmp.Compare`);
   proto `Score` rounded instead of truncated; offset past the end yields an empty page.
4. **B5** ✅ Papercuts: `removedDocIds` slice construction; malformed ids skipped in
   `performFulltextSearch` instead of failing the search; `cmp.Compare` on float scores in
   the per-object sort.

### Phase B — spec'd designs

5. **A2 + A3** ✅ Deletion lifecycle (SpecDeletionLifecycle.md): object deletion expands to doc
   ids via a phrase query on the tokenized Id field (NOT a prefix query — see the
   `TermPrefixQuery` truncation finding, Research.md G.0, discovered by
   `TestBatchDeleteObjectsManyDocs`); FT + queue cleanup on space offload
   (`removeFullTextIndexes`); per-space orphan GC in `RunFTConsistencyCheck`.
6. **B1** ✅ Queue generations (SpecQueueGenerations.md): `gen` stamped on every enqueue;
   `FtQueueMarkAsIndexed` is a conditional ModifyFunc that only sets `seq` when the listed
   generation is unchanged, preserving `ord`/`del`/pending state otherwise.
7. **A5** ✅ Search limits (SpecSearchLimits.md): `Search`/`NamePrefixSearch` take a caller
   limit (performQuery derives it from offset+limit, clamped to [100, 1000], with truncation
   telemetry); new `SearchChat` scopes the query to the chat's message docs; highlights
   disabled for the prefix-name search.

### Phase C — design-only for now (SpecScoringAndIndexLayout.md)

Boost/scoring cleanup (overlapping Should clauses, nameBoost for pluralName, sort-injection
semantics), conditional Zh indexing, id tokenizer without stemmer, RemoveLongFilter vs
unsegmented scripts, structured Iterate query. These change scoring behavior or require an
index version bump; they need product validation and a coordinated reindex, so they are
specified but not implemented in this pass.

## Conventions

- Each fix lands with its repro test turning green; repro tests stay as regression tests.
- Commit messages need GO-{issue} prefixes (issue numbers TBD).
