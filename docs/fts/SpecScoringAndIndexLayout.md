# Spec: scoring cleanup & index layout (design-only, Phase C)

Items here change ranking behavior or the on-disk schema (ftsVer bump → full reindex), so they
need product sign-off and a coordinated rollout. Nothing in this file is implemented in the
current pass. Background: Research.md sections E–G.

## 1. Boost structure (buildDetailedQuery)

Today the four Should clauses per field overlap; an exact title phrase scores
PhrasePrefix(20) + Phrase(20) + EveryTerm(0.75) + OneOf(0.5) *summed* (tantivy BooleanQuery sums
matching Should clauses). Effective title:text ratio ≈ 40:1 on top of BM25's short-field bias —
any body match is noise once any title matches.

Direction:
- Preferred: dis-max semantics per field (take the max-scoring clause, not the sum). tantivy has
  `DisjunctionMaxQuery`; tantivy-go's JSON query builder does not expose it → binding addition
  (new GoQuery variant + convert.rs case).
- Interim without binding work: drop the redundant `PhraseQuery` clause (PhrasePrefix matches all
  full-phrase matches already) and re-tune: Title PhrasePrefix 10, EveryTerm 2, OneOf 0.5;
  Text PhrasePrefix 3, EveryTerm 1, OneOf 0.25. Validate against the tantivy-go `testquality`
  corpus + manual queries before shipping.
- Must clauses contribute BM25 score in tantivy (no Lucene-style FILTER occur): the SpaceID term
  adds a per-space constant (breaks absolute thresholds like `minFulltextScore`), and
  `buildObjectQuery`'s relation-key clause adds per-key IDF (rarer `pluralName` outscores `name`
  for identical titles). Options: boost 0.0 on those Must clauses (kills their score
  contribution) — cheap and binding-compatible; verify tantivy honors zero boosts.

## 2. Final-score consistency

- `nameMatch` boost (+1.0) is granted only when the matched relation is `name`, but
  `preferPluralNameRelation` deliberately promotes `pluralName` docs — give pluralName (and
  arguably snippet-as-title for notes) the same boost.
- `preferPluralNameRelation` overrides the score-based best-doc choice unconditionally — prefer
  pluralName only among (near-)equal scores.
- `injectDefaultOrder` prepends `_final_score` to ANY text query, demoting explicit client sorts
  to tie-breakers. Decide: explicit sorts win (score becomes the tie-breaker), or keep current
  behavior documented. Also switch the final `sort.Slice` to `slices.SortStableFunc`.

## 3. Index layout (requires ftsVer bump + full reindex)

- **Conditional language fields**: index/store Title/Text into the Zh (jieba) fields only when
  the value contains Han characters. Saves ~2x postings + doc store for non-CJK users, halves
  tokenization and highlight cost. Query routing already works this way.
- **Id tokenizer**: drop the English stemmer (and positions) from the id field; ids should be
  raw-ish tokens. Today `pluralName` indexes as `pluralnam` and only matches because the query
  side is analyzed identically.
- **Unsegmented scripts**: RemoveLongFilter(40 bytes) silently drops long tokens — Japanese kana
  runs / Thai become unsearchable. Options: per-script segmentation (e.g. tantivy's
  `tokenizer-lindera` family — binding work), or at minimum an ngram fallback field for
  unsegmented runs.
- **Filter pushdown fields**: `ResolvedLayout`, `isDeleted`/`isArchived` as fast fields so the
  top-K respects filters (see SpecSearchLimits.md).

## 4. Misc primitives

- `Iterate` should build a JSON term query on the Id field instead of going through the string
  QueryParser with raw ids (`Id:%s` — parser metacharacters, phrase-fallback dependence).
- `ListAllObjectIds`' billion-doc listing goes away with per-space listing
  (SpecDeletionLifecycle.md).
- `FTSearch.Index()` (single doc = one tantivy commit/segment) has no production callers —
  remove from the interface or document as test-only.
- Batcher flush threshold should be byte-based (docs can be 1MB each), not 10k docs.
