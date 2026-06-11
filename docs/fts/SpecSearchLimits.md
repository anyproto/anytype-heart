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

### 1. Caller-controlled docs limit

```
Search(spaceId, query string, limit int)            // limit <= 0 → default
NamePrefixSearch(spaceId, query string, limit int)
```

- `performQuery` derives the limit from the request:
  `ftLimit = clamp(q.Offset + q.Limit, defaultFtLimit, maxFtLimit)` with
  `defaultFtLimit = 100`, `maxFtLimit = 1000`; `q.Limit <= 0` (no limit) → `maxFtLimit`.
  The grouping/filtering pipeline stays unchanged but operates on an adequate candidate set.
- Multiply the budget when filters are present? No — keep it simple and observable: a counter
  (`ftMisses`-style) records when the tantivy result count == ftLimit AND the post-filter output
  came up short of `q.Limit`; that telemetry informs whether pushdown (below) is needed.

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
