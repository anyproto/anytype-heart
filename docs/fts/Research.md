# Fulltext Search (tantivy) — research report

Scope: how anytype-heart uses tantivy via tantivy-go v1.0.6, focusing on indexing reliability,
write/query performance, memory, tokenizer/schema usage, boosting, and the
fulltext → objectSearch → sorting flow. Paths are repo-relative; tantivy-go refs point at the
module cache sources (rust side).

## Architecture map (current state)

- One global tantivy index for all spaces at `<repo>/fts_tantivy/16` (`ftsVer = "16"`,
  bump = full rebuild). Schema: 11 text fields — `Id` (SimpleIdTokenizer: simple+lowercase+ascii-fold+English stemmer,
  token len ≤1000, freqs+positions), `IdRaw`/`SpaceID`/`Author`/`OrderId`/`MessageId`/`Timestamp` (raw, basic, fast),
  `Title`/`TitleZh`/`Text`/`TextZh` (simple resp. jieba, freqs+positions, stored).
  `pkg/lib/localstore/ftsearch/ftsearch.go:249-426`.
- A document per relation value / text block / chat message; doc id = `objectId/r/<key>`,
  `objectId/b/<blockId>`, `objectId/m/<msgId>` (`core/domain/fulltext.go`).
- Write path: object change → spaceindexer heads-hash gate → `fulltextQueue` (anystore collection,
  `seq`-stamped with tantivy commit opstamp after indexing) → `ftLoopRoutine` every 10s (backoff to 60s) →
  `prepareSearchDocs` (loads smartblock through cache!) → AutoBatcher upsert (delete+add) →
  one tantivy commit per batch (`core/indexer/fulltext.go`, `pkg/lib/localstore/ftsearch/autobatcher.go`).
- Recovery: startup opstamp reconcile (`FtQueueReconcileWithSeq`), per-space ftQueueCounter for
  crash between space-DB and common-DB commits (`checkFTQueueConsistency`), once-per-session
  one-directional consistency check (`RunFTConsistencyCheck`), `tantivycheck` GC of extra segment files,
  `docCount==0` → enqueue-all rebuild.
- Read path: `ObjectSearch` RPC → `PrefixNameQuery=true` → `NamePrefixSearch` (name/snippet/pluralName
  docs only — block text is NOT searched here); `ObjectSearchWithMeta` → `Search` (detailed query);
  `ChatSearch` → raw `ftSearch.Search` + post-filter to one chat. All capped at 100 docs, highlights on.
  Results grouped per object, best doc picked, `ComputeFinalScore = ln(1+bm25) + recency + nameBoost`,
  default `_final_score` desc sort injected, store filters applied post-hoc per record.

---

## A. Headline bugs

### A1. `filterOutNotChangedDocuments` never filters anything — its optimization is dead code that adds cost
`core/indexer/fulltext.go:232-273`. During `Iterate`, docs that exist in the index and are
unchanged are deliberately *not* added to `changedDocs`. But the second loop re-adds every
`newDocs` entry not found in `changedDocs` ("doc is new as it doesn't exist in the index" — wrong:
it also re-adds every unchanged doc). Net: the function always returns ALL docs of the object.
Every object edit therefore:
1) runs a tantivy search (`Iterate`: query-parser search, TopDocs 10k, doc-store retrieval and
   decompression of the full stored `Text` of every doc — block texts up to 1MB);
2) then deletes+re-adds every doc of the object anyway.
Pure overhead, zero benefit; also invalidates the comment in `runFullTextIndexer` about empty
batches. Fix: collect seen ids in the Iterate callback, only add docs whose id was not seen.
No test covers this function. Also note `Iterate`'s `fields []string` parameter is ignored
(`autobatcher.go:28`).

### A2. `RemoveAclIndexes` FT deletion is a silent no-op
`core/indexer/indexer.go:153-177` passes bare object ids to `BatchDeleteObjects`, which does
`delete_term(Term::from_field_text(IdRaw, id))` (tantivy-go rust `c_util/util.rs`). `IdRaw` is
raw-tokenized full doc paths (`objectId/r/name`…), so a bare object id term matches nothing —
participant docs survive ACL removal and remain searchable. Also `ids, _, err := QueryObjectIds(...)`
error is overwritten by the next call without check. Same trap exists for `ftSearch.DeleteObject`.
Deletion must go through full doc ids (e.g. `ListByIdPrefix(objectId + "/")`) or the queue.

### A3. Space deletion never cleans the FT index
`core/indexer/reindex.go:898-910` (`RemoveIndexes` — used by spaceoffloader) deletes the
objectstore space index, never tantivy docs. Orphans accumulate forever: disk is never reclaimed,
corpus statistics (IDF / avg field length → BM25) drift, and `RunFTConsistencyCheck` is
one-directional (finds store-objects missing from FT, never FT-docs without objects;
`pkg/lib/localstore/objectstore/service.go:851-905`). Only a full rebuild (ftsVer bump,
panic-wipe, docCount==0) clears them.

### A4. Debug `fmt.Printf` leaks full object details to stdout
`pkg/lib/localstore/objectstore/service.go:882,889-897`: `RunFTConsistencyCheck` prints
`pbtypes.Sprint(details)` for every "missing" object (full user content), plus a
`checked %d objects` printf. Runs once per session when the queue drains. Privacy + log noise;
should be structured debug logging without details (typo "missisng" included).

### A5. 100-doc cap before filtering breaks recall, pagination, and chat search
`ftsearch.go:517` (`SetDocsLimit(100)`, TopDocs only — no offset support in tantivy-go path).
- Store-level filters run AFTER (per record `FilterObject`, `spaceindex/queries.go:193,214`):
  a filtered search (e.g. by type) can return near-zero results although thousands match beyond
  the top-100 (which is consumed by docs of any kind — multiple docs of the same object each
  take a slot before grouping).
- `Offset` is applied to the post-processed ≤100-object slice → pagination beyond that is
  impossible and silently empty.
- Chat search (`core/block/chats/service.go:843-899`) calls space-wide `Search` then keeps only
  `path.ObjectId == req.ChatId` — messages compete with every object and every other chat for
  the same 100 slots; busy spaces starve message results. (Also: empty `req.ChatId` filters out
  everything.)

### A6. Float score comparators truncated to int
- `spaceindex/queries.go:482`: `return int(b.Score - a.Score)` — BM25 deltas < 1.0 become 0 →
  "equal" in a **non-stable** `slices.SortFunc` → the "best doc per object" pick (highlight,
  relation key, score used downstream) is arbitrary between close-scoring docs.
- `core/block/chats/service.go:908`: same pattern on chat scores — which are *already* truncated
  by `MessageModel(): Score: int64(r.Score)` (`pkg/lib/database/database.go:330`), so most BM25
  scores (typically 0–5 with boost 1.0 on Text) collapse to 0–4; score sorting of chat results is
  effectively broken.
- Bonus chat pagination bug: `service.go:890-892` applies offset only when
  `len(results) >= offset`; paginating past the end returns the FIRST page again instead of empty.

---

## B. Indexing reliability

### B1. Queue lost-update race during batch processing
`FtQueueMarkAsIndexed` (`indexer_store.go:368-410`) overwrites the whole queue doc with
`{id, spaceId, seq}`. Anything written concurrently while the (up to 1000-object, multi-second)
batch was processing is erased:
- chat `ord` (MsgOrderId) and `del` (DeletedMsgIds) fields → messages/deletions enqueued mid-batch
  are dropped until the once-per-session consistency check (which doesn't cover message-level diffs at all);
- object re-enqueues (seq reset to 0 by `AddToIndexQueueWithCounter`) → object marked indexed though
  `prepareSearchDocs` may have read pre-edit state.
Fix: ModifyFunc that only sets `seq` and only if the entry wasn't re-dirtied (e.g. compare a
generation/counter), preserving `ord`/`del`.

### B2. Poison entries retry forever
`processQueuedObjects` (`core/indexer/fulltext.go:138-215`): for a chat object whose space/tree is
gone, prepare returns a known error with `isChat=true`; the filter step is skipped, `err != nil` →
`continue` without adding to `succeedIds` → entry stays `seq=0` → reprocessed every tick
(10s→60s backoff) forever, each time loading the object via `cache.DoContextFullID`. There is no
retry budget / dead-letter; `BatchProcessFullTextQueue` only stops when *all* remaining entries fail.

### B3. Shutdown guard bypass
`appClosingInitiated` is checked by `Index`/`BatchDeleteObjects` but NOT by the batcher's
`Finish()` (`autobatcher.go:211`), and `Close()` takes no `f.mu` (`ftsearch.go:670-678`). Safety
currently rests on component close ordering (indexer waits `ftQueueFinished` before fts closes).
A `Finish` racing `index.Close()` is a CGO use-after-free.

### B4. Panic = index wipe
tantivy-go installs a panic hook that `remove_dir_all`s the FTS dir on ANY rust panic
(`rust/src/c_util/util.rs:154-168`), and `tryToBuildSchema` removes the whole root on any open
error (`ftsearch.go:428-438`). Self-healing, but the cost is a full reindex during which search is
empty; rebuild enqueue waits for ALL spaces to warm up and runs one giant write tx
(`EnqueueAllForFulltextIndexing`, `service.go:773-820`) enqueueing every object including
non-indexable ones (acknowledged TODO in `prepareSearchDocs`).

### B5. Small correctness papercuts
- `removedDocIds := make([]string, len(object.DeletedMsgIds))` then `append` → N empty-string
  delete terms sent to tantivy for chats (`fulltext.go:166-177`).
- `performFulltextSearch` aborts the WHOLE search if a single doc id fails `NewFromPath`
  (`queries.go:459-462`) — one malformed id in the index makes search return errors; should skip.
- `isFtIndexable` (consistency check) requires non-empty name/description/snippet — objects with
  only block text are invisible to the recheck (`service.go:838-848`).
- `GenerateFTQueueCounter` trusts wall clock monotonicity and sleeps up to 1s at >10k ops/s
  (`indexer_store.go:43-73`).

---

## C. Write path performance & memory

- **A1 dominates**: every edit = 1 tantivy search + full delete/re-add of all docs of the object.
- Batcher flush threshold is 10k DOCS, not bytes (`autobatcher.go:201`); block docs are up to 1MB
  (`ftBlockMaxSize`), each duplicated into Zh fields → worst-case multi-GB batches. Should be byte-budgeted.
- Everything is tokenized, indexed (positions) and **stored twice** (Title+TitleZh, Text+TextZh
  always get the same value: `convertDoc`/`UpsertDoc`) — jieba runs over all-Western text on every
  index AND at highlight time; doc store and postings are ~2x. Language routing
  (`containsChineseCharacters`) only ever queries one side. Consider conditional indexing
  (detect script per doc) or jieba-only-when-CJK.
- `prepareSearchDocs` loads the full smartblock into cache per queued object (1000/batch), then
  `TryRemoveFromCache` — heavy churn on big spaces; `GetMessagesForIndexing` for `FtAllOrderId`
  loads ALL messages of a chat at once (TODO GO-6758 in code).
- Maintenance spikes: `ListAllObjectIds` runs an AllQuery with `SetDocsLimit(1_000_000_000)` and
  materializes every doc id through one JSON buffer (`autobatcher.go:122-151`);
  `RunFTConsistencyCheck` holds the whole id set in a Go map.
- `FTSearch.Index()` commits per document (tantivy-go `add_and_consume_documents` → `commit`),
  one segment per call — currently no production callers (only the batcher is used); it's an API
  footgun that should be removed or documented.

## D. Query path performance

- Highlights: `SnippetGenerator::create` per stored field per hit (`rust/tantivy_util/highlights.rs`)
  — re-tokenizes stored text (Text up to 1MB, twice: simple + jieba fields) for up to 100 hits on
  EVERY keystroke (`SetWithHighlights(true)` unconditionally, including NamePrefixSearch where the
  Go side then mostly discards them: `parseSearchResult` clears all fragments whenever a Title
  field matched, `ftsearch.go:604-607`). Cheap wins: highlights off for prefix search; restrict
  snippet generation to Text/TextZh; skip the Zh duplicate.
- `QueryFromFulltext` re-derives a naive title highlight via `strings.Index(lower(title), lower(query))`
  (`queries.go:221-227`) — byte offsets of a lowercased string applied to the original (UTF-16
  range conversion) — wrong ranges for case-folding that changes byte length; no stem/prefix awareness.
- `convertToHighlightRanges` builds `byteToRuneIndex` with `UTF16RuneCountString(highlight[:i])`
  per byte — O(n²) per fragment (`database.go:362-368`); fine at 150 chars, needless.
- `reader()` reloads on every search (manual reload policy) — right-after-commit searches pay
  segment-open synchronously; acceptable.
- Every `ObjectSearch` with text also fires `ForceFTIndex()` (`core/object.go:90-92`) — min
  10s interval guard exists.

## E. Tokenizer / schema issues

- **SimpleIdTokenizer applies an English stemmer to id tokens** (`RegisterTextAnalyzerSimple(tokenizerId, 1000, English)`,
  `ftsearch.go:410`): doc-path tokens like `pluralName` index as `pluralnam`. It works only because
  TermQuery input passes through the same analyzer (`extract_terms` uses the field tokenizer).
  Fragile coupling — a raw or non-stemming tokenizer is the right primitive for ids.
- **RemoveLongFilter(40 bytes)** on Title/Text (`tokenizer.rs:38`): unsegmented scripts
  (Japanese kana runs, Thai, Khmer…) form single tokens ≥40 bytes → silently dropped → those texts
  are unsearchable via the simple fields. Jieba covers Chinese; queries are routed to Zh fields by
  `unicode.Han` detection (`ftsearch.go:626-633`), which also captures Japanese kanji and sends
  them to a Chinese segmenter. Korean mostly survives via spaces. `validateLanguage` has no CJK
  entry (silently falls back to English stemmer for the simple analyzer).
- The whole index pays positions on `Id` (`IndexRecordOptionWithFreqsAndPositions`) only so that
  the query-parser phrase fallback works in `Iterate`; `Id`'s real uses are term matching
  (`buildObjectQuery`) and the parser path below.
- `Iterate` builds its query by string concat through the tantivy QueryParser
  (`SetQuery(fmt.Sprintf("Id:%s", objectId))`, `autobatcher.go:30`) — ids with parser syntax
  (`-`, `:`…) can mis-parse; multi-token ids (`_participant_*`) only work because the parser
  emits a PhraseQuery; 10k doc cap silently truncates huge objects (stale docs never removed).
  Should be a JSON-built TermQuery/prefix query on IdRaw.

## F. Boosting & scoring pitfalls

- `buildDetailedQuery` (`ftsearch.go:563-589`): the four Should clauses per field overlap — an
  exact title phrase match scores PhrasePrefix(20) + Phrase(20) + EveryTerm(0.75) + OneOf(0.5)
  simultaneously (tantivy sums matching Should clauses) → effective title boost ≈ 40x+ vs Text 1x,
  on top of BM25's built-in short-field advantage. Any Text match is ranking noise once a title
  matches anywhere. If that's intended, fine — but it makes `minFulltextScore=0.02` and the
  `ln(1+bm25)` blend behave very differently per match type.
- Must clauses score in tantivy (like Lucene MUST, there is no FILTER occur in this query builder):
  the SpaceID term adds a per-space constant (skews absolute thresholds like `minFulltextScore`),
  and in `buildObjectQuery` the relation-key term (`Id:name|snippet|pluralName`) adds per-key IDF —
  rarer `pluralName` docs systematically outscore `name` docs for identical titles.
- `preferPluralNameRelation` (`queries.go:647-655`) picks the pluralName doc for an object
  regardless of score; then `ComputeFinalScore(..., nameMatch: RelationKey == "name")`
  (`queries.go:191,211`) grants the +1.0 name boost ONLY to `name` — the preferred pluralName hit
  loses it. Inconsistent on both ends.
- `injectDefaultOrder` (`database.go:164-183`) PREPENDS `_final_score` whenever TextQuery is set —
  an explicit client sort (e.g. lastModifiedDate) is demoted to tie-breaker; if score-override is
  intended, explicit sorts should probably win instead.
- Final `sort.Slice` is non-stable (`queries.go:256`) — equal final scores reorder between calls.
- `recencyDecay` uses lastOpened/lastModified — local-only signal (lastOpenedDate), so ranking
  differs across devices; fine if intended, worth knowing.

## G. Tantivy primitives used incorrectly / risky

0. **`TermPrefixQuery` silently truncates** (discovered while fixing A2, confirmed by
   `TestBatchDeleteObjectsManyDocs`): tantivy-go compiles it to a single-term
   `PhrasePrefixQuery`, whose prefix expansion is capped (~50 terms per segment), and *deleted*
   terms keep consuming the expansion budget until segments merge. So `ListByIdPrefix` returns
   at most ~50 ids per segment, and right after a partial delete it can return 0 while matching
   docs still exist. Complete prefix listings must use a term/phrase query (no expansion cap) +
   client-side prefix filter — see `listDocIdsForObject` in ftsearch.go. The fallback
   diagnostics (`checkDocExistsInTantivy`) still use the truncating variant — acceptable for
   sampling, not for enumeration.
1. Delete-by-term with un-prefixed ids on a raw-tokenized field (A2) — exact-term semantics ignored.
2. TopDocs::with_limit as the only collector — no offset/count support; pagination emulated in Go
   over a truncated set (A5). tantivy-go exposes nothing else, so fixing means binding work
   (e.g. `TopDocs::with_limit(limit+offset)` or COUNT collector).
3. Stemming analyzer on an identifier field (E) — works by accident of symmetric analysis.
4. String QueryParser fed raw ids (E/Iterate) instead of structured term queries.
5. Overlapping Should clauses as "boost tiers" (F) — double/triple counting instead of dis_max-like
   semantics (tantivy has `DisjunctionMaxQuery`; not exposed by tantivy-go builder).
6. Positions indexed on fields that only need term/prefix matching (Id, and arguably TitleZh/TextZh
   duplicates for non-CJK users).
7. `SetWithHighlights(true)` + snippet generation over every stored field instead of the two
   fields actually used.
8. Single-doc `Index()` API = one commit/segment per call (currently unused — keep it that way).

---

## Suggested priorities

1. **A1** — fix the unchanged-doc filter (large, constant write-path win; trivial diff).
2. **A2 + A3** — make FT deletion correct (doc-path prefix delete) and wire it into space
   offboarding; add orphan GC to the consistency check (it already lists all FT object ids).
3. **A4** — remove the stdout detail dump.
4. **A6** — fix both float comparators (use `cmp.Compare`) + chat offset bug + stop truncating
   chat scores to int64 (proto change or scale).
5. **A5** — raise/parametrize the docs limit (limit+offset aware), or push at least space+type
   filters into the tantivy query; for chat search, query with a `MessageId`-exists/chat filter.
6. **B1/B2** — ModifyFunc-based MarkAsIndexed preserving `ord`/`del`; retry budget for poison entries.
7. **E/D** — conditional Zh indexing & highlight scoping (index size −~2x, faster queries);
   reconsider RemoveLongFilter for unsegmented scripts.
