# Spec: chat message search scopes — cross-chat (space) and cross-space (vault)

Status: proposed (resolves the open point in `SpecSearchLimits.md` §2: "an empty ChatId can
legitimately mean 'all chats in the space' … decide with product").
Issue: GO-7449.

## Product ask

`ChatSearch` today searches messages of exactly one chat. Clients need:

1. **cross-chat within a space** (primary): "search messages in this space" — space-level chat UI;
2. **cross-space / vault-wide** (secondary): same, across all spaces.

The fulltext index already supports both for free: it is a single tantivy index for all spaces,
with the space as an ordinary indexed field.

## Current state (facts)

- RPC `ChatSearch` (`pb/protos/commands.proto`, `Rpc.Chat.Search`): request
  `{spaceId, chatId, sorts, fullText, offset, limit}`; response `repeated model.Search.Message.Result`.
- Handler → `chats.service.Search` (`core/block/chats/service.go:843`):
  1. `ftSearch.SearchChat(spaceId, chatId, fullText, offset+limit /*min 100*/)`;
  2. drops every hit with `path.MessageId == "" || path.ObjectId != req.ChatId` (service.go:874) —
     an empty `chatId` therefore returns nothing useful today (all message hits filtered out,
     then `chatObjectDo(ctx, "")` errors); no client depends on that behavior;
  3. hydrates via `chatObjectDo` → `sb.GetMessagesByIds` (opens the chat smartblock; the rest of
     the service already moved to tree-free reads via `chatrepository` in GO-7340);
  4. sorts with `getComparator(req.Sorts, scores)` (service.go:923), then applies offset/limit.
- FT scoping (`pkg/lib/localstore/ftsearch/ftsearch.go:574`): `Must` PhraseQuery on the tokenized
  `Id` field with `chatId + "/m"` (tokens `[chatid, m]`), boost 0; when `chatId == ""` the clause
  is skipped entirely → the query degenerates to a space-wide **object** search (messages compete
  with all docs for the candidate budget) — a latent trap, never exercised.
- The index is cross-space: `performSearch` adds `Must fieldSpace == spaceId` only
  `if len(spaceId) != 0` (`ftsearch.go:598`). `fieldSpace` is raw-indexed and stored.
- Message doc ids are `chatId/m/msgId` (`core/domain/fulltext.go`); the `Id` field uses
  `SimpleIdTokenizer` = SimpleTokenizer + RemoveLong + LowerCaser + AsciiFolding + Stemmer
  (tantivy-go `tokenizer.rs:30`) — **no stopword filter**, so token `m` survives and is present in
  every message doc (and essentially nowhere else: object ids/CIDs, relation keys and block ids
  are single alphanumeric tokens). tantivy-go has no exists-query (`searchquerybuilder.go`:
  Bool/Phrase/PhrasePrefix/TermPrefix/Term/EveryTerm/OneOfTerm/All), so "docs where MessageId is
  set" is not expressible — the `m` token is the expressible equivalent, with a cheap post-filter
  for pathological false positives.
- The chats service maintains a live registry `allChatObjectIds: map[chatObjectId]spaceId` for
  **all spaces** via a cross-space subscription on `resolvedLayout == chatDerived`
  (`core/block/chats/service.go:264-296`, guarded by `s.lock`), populated asynchronously in
  `Run` and kept current by add/remove events.
- Tree-free hydration exists: `chatrepository.Repository(spaceId, chatObjectId)` reads the
  persisted materialized view from the space's CRDT anystore, no smartblock/tree build
  (`core/block/chats/chatrepository/repository.go:100`). Caveats: (a) documented cold-start
  staleness window (service.go:954-965) — the same window the preview subscription already
  accepts; (b) `getOrInitRepository` **creates** the collection when missing — a write
  side-effect that must not be triggered for stale FT ids pointing at deleted chats.
- `model.Search.Message.Result` (`pkg/lib/pb/model/protos/models.proto`) already carries `chatId`
  per hit; it lacks `spaceId` (and `model.ChatMessage` has no space field either — the
  single-chat RPC never needed one). Precedent for per-item space attribution:
  `Chat.SubscribeToMessagePreviews.Response.ChatPreview{spaceId, chatObjectId, …}`. Object
  search sidesteps this structurally: its results are detail structs where `spaceId` is an
  ordinary detail key.
- The only existing cross-space search RPC is `ObjectCrossSpaceSearchSubscribe`/`Unsubscribe` —
  a subscription, not a one-shot query. Its request has **no spaceId field at all** (scope is
  implicitly all spaces via `crossspacesub` + `NoOpPredicate`, `core/object.go:250-259`);
  narrowing happens through detail filters. There is no one-shot cross-space `ObjectSearch`:
  `ObjectSearch`/`ObjectSearchWithMeta` require a spaceId (`SpaceIndex("")` returns an invalid
  store); `QueryCrossSpace` exists only as an internal objectstore API. The all-spaces-implicit
  shape cannot express "all chats within this space" — the primary scope here — so it is not a
  transferable precedent; an optional `spaceId` is.

## API shape — decision

Three scopes must be expressible: one chat / all chats in a space / all chats in all spaces.

### Option A (recommended): generalize the existing `ChatSearch` request

Scope is whatever the caller narrows to; no new endpoint, no request proto change:

| `spaceId` | `chatId` | scope                                   |
|-----------|----------|-----------------------------------------|
| set       | set      | one chat (behavior unchanged)           |
| set       | empty    | **all chats in the space** (new)        |
| empty     | empty    | **all chats in all spaces** (new)       |
| empty     | set      | one chat, FT unscoped by space (works today via the chatId clause; kept as-is, discouraged in the comment) |

Response: add `spaceId` to `model.Search.Message.Result` so hits are attributable in the two new
scopes (and redundantly-but-conveniently in the old one).

Why this over a separate RPC:
- the request already contains both scope fields; a separate endpoint would duplicate the whole
  Request/Response/Error tree minus one field, plus handler + bindings in ts/kotlin/swift;
- the `ObjectCrossSpaceSearchSubscribe` precedent is a separate endpoint because subscription
  *lifecycle* differs structurally (per-space subs managed by `crossspacesub`); a one-shot FT
  query is literally the same code path with one clause dropped;
- empty `chatId` today is an error/empty result no client relies on (RPC shipped 2026-01,
  GO-4645) — the change is additive;
- the failure mode of accidental scope-widening (client bug passes empty chatId) is visible, not
  silent: results carry foreign `chatId`s/`spaceId`s.

### Option B: separate RPC `ChatSearchAllChats` (or `ChatCrossSpaceSearch`)

Identical internals; request without `chatId` (`{spaceId, sorts, fullText, offset, limit}`,
empty `spaceId` = all spaces), same response model. Choose this only if explicit intent at the
API surface is valued over the duplication. The rest of this spec applies unchanged — the two
options differ only in proto plumbing.

### Option C (rejected): `repeated chatIds` in the request

Expressible in FT (Should-nested per-chat phrase clauses, boost 0), but no client need for
subset-of-chats search exists; single-chat and all-chats cover the products. Revisit if a real
use case appears — the design below doesn't preclude it.

## Design

### 1. FT layer: message-doc scoping without a chat

`ftsearch.SearchChat` (`ftsearch.go:574`) — when `chatId == ""`, instead of skipping the clause,
restrict to message docs:

```go
if chatId != "" {
    qb.Query(tantivy.Must, fieldId, chatId+"/m", tantivy.PhraseQuery, 0.0)
} else {
    // all message docs: every "chatId/m/msgId" id contains the token "m";
    // boost 0 keeps the clause out of BM25. Object docs never tokenize to a
    // bare "m" (ids/CIDs/relation keys/block ids are single alphanumeric
    // tokens); the rare false positive is dropped by the caller's
    // path.HasMessage() filter.
    qb.Query(tantivy.Must, fieldId, "m", tantivy.TermQuery, 0.0)
}
```

Space scoping needs no change: `performSearch` already adds/skips the `fieldSpace` clause on
empty `spaceId`. Update the `FTSearch.SearchChat` interface comment: *"chatId == "" searches
message docs of all chats; spaceId == "" searches all spaces"*.

Verified expressibility: tantivy-go `TermQuery` runs the field's analyzer over the text and takes
the first term (`convert.rs:116`), same analysis path the existing phrase clause relies on.

Cost: the `m` posting list is "every message doc in the index"; tantivy intersects it with the
space term and the text query via skip lists — same order of work as the per-chat phrase clause
(which also walks the `m` postings for the phrase intersection).

### 2. Result attribution: `spaceId` on the result model

`pkg/lib/pb/model/protos/models.proto`, `Search.Message.Result` += `string spaceId = 7;`
(regen protos). `FulltextResult.MessageModel()` (`pkg/lib/database/database.go:332`) can't fill
it (the FT doc id has no space); the chats service sets it from the registry / request.

No request proto changes (Option A). Update field comments in `pb/protos/commands.proto`
(`Rpc.Chat.Search.Request`): `chatId` — "empty = all chats in spaceId"; `spaceId` — "empty = all
spaces (chatId must also be empty)".

### 3. Service layer: `chats.service.Search` (`core/block/chats/service.go:843`)

Single code path for all scopes:

1. **FT query** — unchanged call, now meaningful with empty ids:
   `s.ftSearch.SearchChat(req.SpaceId, req.ChatId, req.FullText, ftLimit)`; same
   `ftLimit = max(offset+limit, 100)` budget (0 → default page).
2. **Post-filter** — replace the equality filter with:
   - `!path.HasMessage()` → drop (guards `m`-token false positives);
   - `req.ChatId != "" && path.ObjectId != req.ChatId` → drop (unchanged single-chat guarantee);
   - multi-chat scopes only: `path.ObjectId` not in the registry snapshot → drop (stale FT docs
     for deleted chats; also prevents `getOrInitRepository`'s CreateCollection side-effect).
     Registry snapshot: copy `allChatObjectIds` (filtered by `req.SpaceId` when set) under
     `s.lock` once per request.
3. **Group** message ids by `path.ObjectId`, preserving FT (score) order within each group.
4. **Hydrate** per chat via the tree-free path: `s.chatRepoService.Repository(spaceId, chatId)` →
   `repo.GetMessagesByIds(ctx, ids)` (spaceId from the registry). Missing messages (FT lag after
   delete/edit) are skipped, as today. **Unify the single-chat scope onto the same path** —
   replaces `chatObjectDo`+smartblock open; consistent with the GO-7340 decision that shipped
   `ChatGetMessagesByIds`/pinned reads tree-free, and single-chat search is issued from an open
   (warm) chat anyway. The cold-start staleness caveat (service.go:954) applies equally to the
   FT index itself (messages are FT-indexed *from* the materialized view,
   `core/indexer/fulltext.go:378-384`), so hydration cannot be meaningfully more stale than the
   hit list.
5. **Attribute**: set `result.SpaceId` (registry value; `req.SpaceId` when set), keep `ChatId`
   from the path.
6. **Sort** with `getComparator`, two fixes:
   - empty `req.Sorts` → default to `[{SCORE, Desc}]` explicitly. Today the order survives only
     because hits arrive score-ordered and `slices.SortFunc` (unstable) happens not to shuffle
     all-equal input; with per-chat grouped hydration the implicit order would be
     chat-grouped — wrong;
   - append a final tiebreaker `(chatId, messageId)` so equal keys (common for equal scores)
     sort deterministically — offset pagination must not shuffle between requests.
7. **Offset/limit** — unchanged slicing.

Sort-key semantics across chats (document in the proto comment):
- `SCORE` — meaningful in every scope (single BM25 query);
- `CREATED_AT` / `MODIFIED_AT` — meaningful in every scope (wall-clock);
- `ORDER_ID` — lexicographic CRDT positions are only ordered *within* a chat; across chats the
  comparison is deterministic but semantically arbitrary. Not rejected (clients may sort
  per-chat groups), just documented.

### 4. Out of the box / unchanged

- **No total count / hasMore** in the response — FT counting is unsupported (same stance as
  `ObjectSearch`); a short page means "no more results" within the candidate budget.
- **Candidate budget caveat** (already documented at service.go:844) carries over and bites
  sooner: all chats in scope share one `offset+limit`-driven BM25-truncated pool, so deep
  pagination with non-score sorts can overlap/miss across pages. Exact deep pagination needs a
  cursor (orderId/createdAt watermark) — explicitly out of scope, same as the single-chat case.
- **No dependency hydration** (chat names, space names, creators): clients resolve locally
  (they already subscribe to chat objects/space views); API v2 does its own creator enrichment.
- **ACL**: none needed — everything in the local index/stores is readable by the account.

## Edge cases

- **Registry warm-up**: `allChatObjectIds` fills asynchronously in `Run` (after the cross-space
  subscription responds, i.e. after stores load). A vault-wide search issued in the first
  moments of startup may return fewer chats. Acceptable: search UIs appear later; the FT index
  itself lags similarly. Do not fall back to the resolver for unknown ids (it would defeat the
  deleted-chat filter and re-open the CreateCollection side-effect).
- **Deleted space / deleted chat**: FT docs are cleaned by object/space deletion paths; any
  residue is dropped by the registry filter.
- **techspace / marketplace**: no message docs, nothing matches; no special-casing.
- **Just-sent messages**: `ObjectSearch` flushes the FT queue via `indexer.ForceFTIndex()` when
  `fullText != ""` (`core/object.go:93`); `ChatSearch` never has. Optional consistency
  improvement for all scopes — include it, but as a separate commit so it can be reverted alone
  if queue-flush cost shows up on mobile.
- **Empty `fullText`**: `prepareQuery` returns empty → nil results (unchanged). "Browse all
  messages" is not a search feature; `ChatGetMessages` covers per-chat browsing.

## Rejected alternatives

- **Space-wide query + client/server post-filter to message docs** (no `m` clause): messages
  compete with every object doc for the candidate budget — the exact regression GO-7316/A5
  removed.
- **Registry-driven fan-out** (one `SearchChat` per chat, or Should-nested per-chat phrase
  clauses): precise but O(#chats) query size/latency; a vault has one chat per space today but
  the shape shouldn't degrade if per-object chats ship. The single `m`-clause query is O(1) in
  scope size.
- **Dedicated doc-type field in the tantivy schema** (clean exists-style scoping): requires
  `ftsVer` bump → full reindex for every account (known peak-heap pain on mobile). Not worth it
  for a filter the `m` token already provides; revisit only if `m`-token false positives are
  ever observed in practice (the post-filter makes them a non-issue for correctness).
- **Returning stored `fieldSpace` from tantivy for attribution** (extend `parseSearchResult`
  includeFields): works (field is stored), but the registry is needed anyway for the
  deleted-chat filter and already carries spaceId — one source of truth, zero FT changes.

## Tests

FT layer (`pkg/lib/localstore/ftsearch`, real index via `newFixture`, `ftsearch_test.go:22`):
- index object docs + messages in chat1/chat2 (space1) and chat3 (space2);
  - `SearchChat(space1, "", q)` → chat1+chat2 messages only (no object docs, no space2);
  - `SearchChat("", "", q)` → all three chats;
  - `SearchChat(space1, chat1, q)` → unchanged single-chat scoping;
- budget test: many matching object docs in the space + one matching message → the message is
  still returned with a small limit (object docs excluded from the candidate set).

Service layer (`core/block/chats`, fixture `service_test.go:130`; `chatRepoServiceDummy` needs a
per-chat repo map):
- multi-chat grouping + `spaceId` attribution from the registry;
- FT hit for a chatId absent from the registry is skipped (no repo call);
- default SCORE-desc ordering across chats when `sorts` is empty (hits hydrated grouped-by-chat
  must come back interleaved by score);
- `CREATED_AT` sort across chats; offset/limit slicing; deterministic tiebreak on equal scores
  (assert stable order across two calls);
- single-chat scope: existing tests (incl. `TestService_SearchScoreSorting`) keep passing on the
  repository-based hydration.

Comparator unit test: tiebreaker ordering.

## Implementation checklist

1. `pkg/lib/pb/model/protos/models.proto`: `Search.Message.Result` += `spaceId = 7`; regen.
2. `pb/protos/commands.proto`: scope-semantics comments on `Rpc.Chat.Search.Request`; regen.
3. `pkg/lib/localstore/ftsearch/ftsearch.go`: `SearchChat` empty-chatId `m`-clause + interface
   comment.
4. `core/block/chats/service.go`: registry snapshot helper; post-filter; per-chat repo
   hydration (single-chat unified); spaceId attribution; default sort + tiebreaker.
5. Optional, separate commit: `ForceFTIndex()` before FT query in the `ChatSearch` handler
   (`core/chats.go:340`), mirroring `core/object.go:93`.
6. Follow-ups (not this change): API v2 space-level endpoint
   (`core/api/service/chat.go:277` `SearchChatMessages` can pass empty `chatId` once clients
   need it); cursor-based deep pagination shared with single-chat search.

## Open questions (product)

1. Ship vault-wide scope (`spaceId == ""`) together with within-space, or gate it? Cost is nil
   (it's the same code path); suggestion: ship both, clients adopt when ready.
2. Option A (generalize `ChatSearch`) vs Option B (separate RPC) — this spec recommends A;
   flipping to B changes only proto plumbing.
