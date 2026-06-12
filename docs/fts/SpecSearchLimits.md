# Spec: search limits & chat-scoped search (A5)

## Problem

`performSearch` hard-codes `SetDocsLimit(100)` (docs, not objects — one object's blocks consume
multiple slots). Everything downstream operates on that truncated set:

- store-level filters (`QueryFromFulltext` → `FilterObject` per record) run AFTER → filtered
  searches lose recall arbitrarily;
- `offset`/`limit` are applied to the truncated, grouped set → pagination beyond ~100 objects
  silently returns nothing;
- chat search runs a space-wide query and post-filters to one chat → messages compete with all
  objects and other chats for the same 100 slots (`TestSearchReturnsAllMatches` documents the cap).

## Design (this pass)

### 1. Caller-controlled docs limit + page-filling escalation

```
Search(spaceId, query string, limit int)            // limit <= 0 → default
NamePrefixSearch(spaceId, query string, limit int)
```

The 100-doc cap was a deliberate noise gate: BM25 scores aren't comparable across queries, so
"top N most promising" is the only workable relevance cutoff, and it also bounded the pool the
additive recency/name boosts can re-rank plus the per-candidate anystore reads. Those properties
must be preserved.

At the same time clients universally treat a page shorter than the requested limit as the end of
the result set — and with post-FT filtering and doc→object grouping, a fixed cap makes short
pages lie (observed: desktop requests limit=100, gets 86 because filters consumed candidates,
stops paginating although more matches exist).

Resolution — **escalate-on-starvation** (`performFulltextQuery`):

- round 1 budget: `clamp(2*(offset+limit), 100, 2000)` — the 2x headroom exists because grouping
  and filters almost always trim some candidates, so an unpadded budget would need an escalation
  round (re-resolving every candidate) for nearly every full page; when few matches exist tantivy
  returns the same docs regardless of budget, so the padding costs nothing there.
  `q.Limit <= 0` → a single conservative 100-doc round ("everything" + fulltext means "the
  relevant matches", not the whole index);
- if the post-filter record count < `offset+limit` AND tantivy returned a full budget of docs
  (more matches exist), double the budget and re-run, up to `ftCandidatesHardLimit = 2000`;
- stop as soon as the page is filled or tantivy returns fewer docs than the budget (index
  exhausted for this query).

This restores the client contract — a short page again means "no more results" — with no proto
or client changes. The common query is still a single 100-doc round; only demonstrably starved
queries pay for more. `ftCandidatesTruncated` counts pages that stayed underfilled at the hard
limit (the only case where a short page can still lie), feeding the pushdown decision below.

A `hasMore` response flag was considered and rejected for now: it requires proto + all-client
changes, and without server-side escalation it cannot even guarantee progress on the next
offset request — escalation is needed first either way.

### 1b. Two-tier ordering: pagination-stable re-ranking

Escalation introduces a second hazard: the final order is `_final_score` (BM25 + recency/name
boosts), but the candidate pool depends on the budget, which grows with the offset. Re-ranking
the WHOLE pool means a recency-boosted tail candidate can jump into an earlier page on the next
request, shifting everything below it — duplicates on page N+1, and some results never returned
at all. Offset pagination is only sound over a prefix-stable order: growing the window must
append, never reorder.

Resolution (`queryFromFulltextRecords`):

- the BM25 *object* order is prefix-stable under budget growth (a larger top-K only appends
  objects with lower best-doc scores; ties broken deterministically by object id);
- the final-score boosts re-rank ONLY the first `ftRerankPoolSize = 100` objects — a fixed pool
  independent of the requested page, which is exactly the pool the original hard cap gave them;
  the head sort is stable so equal scores keep their BM25 order;
- everything beyond the head stays in BM25 order; related-object injections are derived from
  head hits only;
- the recency decay clock is truncated to the hour so the head order cannot drift between two
  consecutive page requests.

Three subtleties found by adversarial review, all required for the invariant to actually hold:

- **Head materialization**: the head must be the same OBJECT set for every request, so escalation
  also continues until ≥ `ftRerankPoolSize` objects were collected (or the index is exhausted) —
  a doc-count budget can yield fewer objects than docs when objects have several matching docs
  (`TestFulltextPaginationConsistencyMultiDoc`).
- **Object score = best doc score**: `preferPluralNameRelation` may choose a lower-scoring
  pluralName doc as the object's representative; the object is still ORDERED by its best doc's
  BM25 score, otherwise a budget increase that pulls in the pluralName doc would move the object
  down the order.
- **Request-independent injections**: the related-object injection budget is a constant
  (`ftRerankPoolSize`), never derived from offset/limit, so the injected set (and thus the head
  order) is the same for every page.
- **Lazy tail resolution**: the head must always be resolved in full (re-ranking can move any of
  its objects into the page), but the tail is already in final order, so candidates beyond
  `offset+limit` surviving records are not read from the store at all. Early exit yields a
  prefix of the same sequence, so consistency is unaffected; the per-search store reads are
  ~`max(head, needed + drops)` instead of the whole candidate budget.

Result: the full sequence `rerank(top-100) ++ bm25-tail` is identical for every budget, so
pages from different requests never overlap (`TestFulltextPaginationConsistency` pins
pages == slices of one big query). Remaining caveats: docs with EXACTLY equal BM25 scores
straddling the budget boundary can still insert mid-order (tantivy truncates by internal doc
order before the Go id tiebreak sees them — rare, accepted), and, shared with all offset
pagination over a live database, an index commit between two page requests can shift results;
the complete cure would be a server-side search-session snapshot (cursor), kept as a possible
follow-up. Clients de-duplicating by id remain a cheap belt.

Also note: the prefix-name path (`NamePrefixSearch`) runs without highlight generation, so the
`minFulltextScore`-without-highlights drop is disabled for it — low-scoring prefix matches used
to survive via their highlight ranges and must not silently disappear.

### 2. Chat-scoped message search

Chat docs are `chatId/m/msgId`. Scope the tantivy query instead of post-filtering:

```
SearchChat(spaceId, chatId, query, limit)
```

The scope clause is a Must PhraseQuery on the tokenized `Id` field with text `chatId + "/m"`
(tokens `[chatid, m]`, consecutive positions), boost 0 so it doesn't contribute to BM25.
NOT a TermPrefixQuery on IdRaw: prefix queries expand to at most ~50 terms per segment
(deleted terms included), silently truncating the match set — see Research.md G.0.

`chats/service.Search` passes `limit = offset + limit` — the whole candidate budget is
messages of that chat.

Open point: `RpcChatSearchRequest` without `ChatId` currently returns nothing (post-filter
mismatch); with prefix scoping, an empty ChatId can legitimately mean "all chats in the space"
(`idPrefix` empty + Must on `MessageId` field presence is NOT expressible today — instead keep
requiring ChatId, or match `"/m/"`-containing ids client-side as today; decide with product).

### 3. Highlights cost (related, cheap win)

`SetWithHighlights(false)` for `NamePrefixSearch` (the Go side discards non-title fragments and
title fragments are cleared anyway — see Research.md), keep highlights only for the detailed
search. Raising the docs limit makes highlight cost per search worse, so this lands together.

## Out of scope (follow-up, needs index schema work)

Filter pushdown: indexing `ResolvedLayout`/`isArchived`/`isDeleted` as tantivy fields so the
top-K is computed over the filtered set. Requires index version bump + reindex; only justified
if the telemetry from (1) shows truncation in practice.

## Tests

- Repro `TestSearchReturnsAllMatches` green via explicit limit.
- performQuery passes offset+limit-derived budget (unit test on the clamp).
- Chat search: messages of the target chat are found even when 1000+ other docs in the space
  match the query better; offset/limit behavior covered by the A6 tests.
