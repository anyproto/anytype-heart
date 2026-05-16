# Chat read-counter: bounded dominated-set resolution

- **Date:** 2026-05-16
- **Issue:** GO-7290 (follow-up performance work)
- **Status:** Design — pending review
- **Branch context:** `start-performance` (integration of go-6331 / go-7291 / go-7290)

## 1. Problem

An Android cold-start CPU profile (`anytype_profile_20260516_170652`, 60.3 s window,
92.36 s samples) shows the chat read-counter watermark path costing ~13 s / 14 %:

- `sourceimpl.(*watermark).advance` ≈ 13 s, of which:
  - `eachChange(...)` ≈ **9.67 s** — a full tree-change scan
  - `w.onRemove(read)` ≈ 3.28 s — chunked `id IN (...)` SQL updates

`advance` is invoked once per diff-manager (`messages`, `mentions`, `reactions`)
per chat store during `initDiffManagers`. On the captured account that is 108
chat stores × 3 counters ≈ **324 full change scans**, while `stat.json` shows
almost every chat fully read (`allChangesCount` mostly 1) — i.e. the scan is
overwhelmingly wasted work.

`eachChange` is `store.go:122` → `Tree().Storage().GetAfterOrder(ctx, "", …)`,
which streams **every** change of the tree (≈150 k rows on the captured space)
and tests `dominated()` per change, even though only currently-unread messages
can change state.

## 2. Goal / Non-goals

**Goal:** eliminate the per-counter full tree-change scan at cold start while
preserving the watermark's exact 2D `(OrderId, AddSeq)` read semantics — no
correctness relaxation.

**Non-goals:**

- No change to the dominance algorithm, multi-head frontier, pruning, or the
  persisted seen-heads KV value.
- No denormalization of `AddSeq` onto the chat message document (rejected:
  tree rebuild renumbers `AddSeq`, a frozen copy would desynchronise — see
  §8).
- No change to seen-head *derivation* (`MarkMessagesAsUnread` /
  `collectSeenHeads`); only dominated-set *enumeration* changes.

## 3. Established facts (code + real-DB evidence)

Verified against the source on `start-performance` and a real account DB at
`…/AASdKiEGfcyhxX3ufr4auHRviACUXxkF68uZwtSb2AnyRoMA2/`:

1. **The watermark engine is generic over a `(id, pair)` stream.**
   `watermark.advance(seenIds, resolve, eachChange)` (`readwatermark.go:46-87`):
   resolves seen heads via `resolve` → prunes the frontier → iterates the
   `eachChange` stream → emits `!marked && dominated(p, frontier)` ids to
   `onRemove`. `dominated` is per-head `∃H: x.OrderId ≤ H.OrderId ∧
   x.AddSeq ≤ H.AddSeq` (`readwatermark.go:14-21`).

2. **`resolvePair` and `eachChange` read from the same source.**
   `resolvePair(id)` = `Tree().Storage().Get(ctx, id)` point lookup
   (`store.go:112-118`); `eachChange` = `Tree().Storage().GetAfterOrder(ctx,
   "", …)` full stream (`store.go:122-129`). Both yield `readPair{OrderId,
   AddSeq}` from the **any-sync tree `changes` collection**.

3. **`changes` lives in a different database file than chat messages.**
   - Chat messages: `objectstore/{objId}.{spaceId}/crdt.db` →
     `{chatObjectId}chats` collection.
   - Tree changes: `spaceStoreNew/{spaceId}/store.db` → `changes` collection
     (any-sync `objecttree.CollName`, `storage.go:31`).
   These are separate SQLite files — **no single-query JOIN across them**. The
   only cross-store access is the existing `Tree().Storage().Get(id)` point
   lookup.

4. **OrderId is natively present per field on the chat document.**
   any-store stores a recursive CRDT order map `_o`. A real edited message:
   `_o.id = "!!bg"` (create), `_o.content = "!!cp"` (edit), `_o.reactions.👍🏼
   = "!!vR"` (reaction add). The chat repository also explicitly writes the
   create OrderId to `_o.id` (`repository.go:206`). Chat-collection indexes
   present: `_o.id` (plain), `pinned` (sparse), `rUnreadOrdId` (sparse,
   unique). `read` / `mentionRead` are **not** indexed.

5. **`AddSeq` is only in `changes`, keyed by change id, and is not indexed.**
   `changes` documents carry `o` (OrderId), `q` (AddSeq), `t` (treeId)
   (`storage.go:22-49`). The only index is `(t, o)` unique
   (`spacestorage.go:80-84`); `q` is unindexed. `AddSeq` is batch-granular
   (one value per `AddAll` batch, `storage.go:297/331`) and local to the peer;
   `message.id == its create change id`.

6. **Cold-start ordering: chat docs written before `advance`.** In
   `ReadStoreDoc`, `storeApply.Apply` + `tx.Commit()` (which writes chat docs
   via the storestate tx) completes **before** `initDiffManagers → advance`
   (`store.go:235-243`; `store_apply.go:22-47`). This holds for the cold-start
   path only; the live `SubscribeForKey` path has no such guarantee — handled
   by the cross-invocation invariant in §5, not by this fact.

7. **`advance` is shared by all three counters and three call sites.**
   `RegisterDiffManager` wires `messages`/`mentions` → `markReadMessages`,
   `reactions` → `markReadReactions` (`chatobject.go:229-246`). `advance` is
   called from `InitDiffManager` (`store.go:177`), the `SubscribeForKey` live
   callback (`store.go:188`), and `MarkSeenHeads` (`store.go:351`) — all
   passing `s.eachChange(ctx)`.

## 4. Core principle

The watermark's **dominance / dedup / emit logic** — `resolve` of seen heads,
`prune`, the `dominated` test, the `marked` set, and the `onRemove` delta
emission (`readwatermark.go:46-87`) — keeps its algorithm **unchanged**.
Multi-head union and the in-past-insert invariant are therefore preserved.

This is **not a zero-touch refactor**, and the spec must not pretend otherwise
(review S2/S3). Concretely:

- **`advance` signature changes.** Its third parameter changes from
  `func(yield func(string, readPair))` to one that also receives
  `maxFrontierOrderId` (defined in §5), because the provider needs that bound
  and it is only known *after* `resolve`+`prune` runs *inside* `advance`. The
  lines that compute the frontier, test `dominated`, update `marked`, and call
  `onRemove` are untouched; only the parameter type and the one call line
  change.
- **Per-counter provider routing.** Today all three counters share one
  `s.eachChange(ctx)` factory at three call sites (`store.go:177/188/351`,
  fact §3.7). To let `messages`/`mentions` use the bounded provider while
  `reactions` keeps the legacy full-stream in Phase 1, the stream factory
  must be **stored per `diffManager`** (selected at `RegisterDiffManager`)
  and the three call sites pass `manager.provider` instead of
  `s.eachChange(ctx)`. The legacy provider is the existing
  `GetAfterOrder(ctx, "", …)` wrapped to ignore `maxFrontierOrderId`.
- **Callers/tests update.** Every `readwatermark_test.go` `advance` call, the
  `readwatermark_export_test_shim.go` `Advance` wrapper, and the chatobject
  scenario sign-off harness pass the third arg with the old shape and must be
  re-shimmed (§9).

We change only the **source of the `(id, pair)` stream**: from "every change
in the tree" to "the bounded set of currently-unread candidates, each
`AddSeq`-resolved via the same `Storage().Get` point lookup `resolvePair`
already uses".

## 5. Correctness argument (completeness)

Define `maxFrontierOrderId` = the maximum `OrderId` over the pruned frontier
pairs (`max_{H ∈ frontier} H.OrderId`). Since `dominated(x, frontier)` requires
some head `H` with `x.OrderId ≤ H.OrderId`, any dominated change has
`OrderId ≤ maxFrontierOrderId`.

The replacement is correct iff the bounded candidate stream is a **superset of
every id the full scan would emit**. The old scan emits `id` iff
`!marked[id] && dominated(p, frontier)`. For each such id:

- `dominated(p, frontier) ⇒ ∃H: p.OrderId ≤ H.OrderId` ⇒ every emittable id
  has `OrderId ≤ maxFrontierOrderId`. The candidate query bounds by
  `_o.id ≤ maxFrontierOrderId`, so no dominated id is excluded on the OrderId
  axis.
- For the `messages`/`mentions` counters, an emittable id that actually flips
  a counter is an unread message row (a message-create change with
  `read==false` / `mentionRead==false`). An emittable id whose row is already
  `read==true` is a no-op (`SetReadFlag` is idempotent and `marked` already
  deduped it). Restricting the candidate set to unread message rows therefore
  drops only no-op emissions.
- The old scan also yielded non-message changes (edits, root) and
  changes dropped by `storeApply.applyChange` as `storestate.ErrValidation`
  (`store_apply.go:67-71`) — these exist in the tree `changes` collection but
  have **no chat row**. For `markReadMessages` the old scan's `id IN (...)`
  matched no row (no-op); the bounded provider simply never enumerates them
  (no chat row ⇒ not a candidate). Both never flip a counter — correctness-
  neutral.
- Each candidate's `AddSeq` is resolved from the **same** `Tree().Storage()`
  that `resolvePair`/`eachChange` read today → same epoch, no frozen copy.
  The `dominated` test then runs on identical data, so per-id results are
  identical.

**Cross-invocation invariant (corrected per review S4).** The naive
precondition "every dominated message's chat row is committed before `advance`
runs" is *not* provable for the live `SubscribeForKey` path: device B's
seenHeads KV value can arrive on device A before A has applied the underlying
chat changes. This is **not a regression** — the legacy full-scan `advance`
had the identical structural gap (it, too, only ran at the same three sites).
The real, sufficient invariant is:

1. *Per-invocation equivalence:* for a single `advance` call, the bounded
   provider enumerates exactly the unread chat rows that exist at that moment;
   over those, its emission equals the full scan's (the bullets above).
2. *Re-evaluation on next advance:* a dominated message whose chat row is not
   yet applied is not emitted and **not** added to `marked` (the engine only
   inserts into `marked` on emission, `readwatermark.go:80`). When the row
   later lands and any subsequent `advance` runs (next KV callback, next
   `MarkSeenHeads`, or next cold-start `initDiffManagers`), the message is a
   fresh unread candidate, still dominated, not in `marked` → emitted exactly
   once. The set of sites that trigger `advance` is unchanged from today, so
   completeness across invocations is preserved.

Conclusion: per `advance` invocation the bounded provider emits exactly the
full scan's delta for the applied rows; across invocations no dominated
message is permanently lost (`marked` never suppresses a never-emitted id) —
no correctness loss, full 2D semantics retained.

## 6. Components

### 6.1 Chat-collection indexes (schema)

Add to the existing `anystorehelper.AddIndexes` list (`repository.go:141`):

- **`read`** — a plain single-field index (serves the `messages` counter).
- **`hasMention`** — a plain single-field index (serves the `mentions`
  counter; see below — *not* `mentionRead`).

**The candidate query must use positive equality, not the repository's
existing unread filter (review B1).** `repository`'s `getReadFilter(false)` is
`filterReadFalse = query.Not{read==true}` (`readhandler.go:11,37`, a
deliberate negation to also match docs missing the field). In any-store
`Not.IndexBounds` yields **no index bounds** (`query/filter.go:367`), so the
planner gives a `read` index zero weight for that filter and full-scans the
collection. The bounded provider must instead issue a **positive-equality**
predicate `query.Key{Path:[read], Filter: Comp(==, false)}`. This is complete
because every message **always** serializes `read`/`mentionRead`/`hasMention`
as explicit bools (`chatmodel.go:461-463`), so there are no missing-field
docs; and `Comp.IndexBounds` (`query/filter.go:107`) *does* use the index.

**Mentions selectivity (review S1).** A `mentionRead == false` predicate is
*not* selective: `mentionRead` is `false` for every message not authored by
the current user (`chathandler.go:80-91`), i.e. nearly the whole chat. The
selective predicate is `hasMention == true` (few messages mention the user);
the genuine unread-mention set is `hasMention==true AND mentionRead==false`,
and `readMentionsModifier` already no-ops `hasMention==false` docs
(`readhandler.go:122-124`). So the `mentions` candidate query is
`Key{hasMention==true} AND Key{mentionRead==false}` index-anchored on
`hasMention`, with `mentionRead==false` and `_o.id ≤ maxFrontierOrderId` as
cheap residuals over the small mention set. A plain `mentionRead` index is
therefore **rejected**; `hasMention` is the correct anchor.

Rationale for `messages`: in steady state the unread partition is near-empty
(most chats fully read), so the positive `read == false` equality seek is the
selective driver and `_o.id ≤ maxFrontierOrderId` is a cheap in-memory
residual. A compound `(read, _o.id)` index remains **rejected** as YAGNI for
`messages` (the §6.3 fallback covers the rare large-unread case). Sparse
indexes are not usable (`read`/`hasMention` always exist; sparse keys on
absence). Adding to `AddIndexes` is additive (existing `_o.id`, `pinned`,
`rUnreadOrdId` indexes are kept — `anystorehelper.AddIndexes` only drops
indexes absent from the list), but the first open of each of the ~108 chat
collections after upgrade pays a **one-time synchronous `CREATE INDEX`**
backfill over that chat's docs (`collection.go:608-635`) — small per chat,
not "online"/zero-cost; still negligible versus the eliminated 150 k-row tree
scan.

### 6.2 Bounded candidate provider (replaces `s.eachChange`)

A per-`diffManager` provider (stored at `RegisterDiffManager`, per §4),
invoked by `advance` with `maxFrontierOrderId` (computed inside `advance`
after `resolve`+`prune`, per §5) and the `yield` closure. The provider is
constructed by the `store` (it closes over the chat collection and
`Tree().Storage()`); the engine only invokes it and consumes `yield`.

`messages` provider:

1. Index-seek the chat collection with **positive equality**
   `Key{Path:[read], Filter: Comp(==, false)}` (§6.1 — *not*
   `getReadFilter(false)`).
2. Residually filter `_o.id ≤ maxFrontierOrderId`.
3. For each candidate id, `s.treeSource.Tree().Storage().Get(ctx, id)` →
   `readPair{OrderId, AddSeq}` (the same call `resolvePair` makes).
4. `yield(id, pair)` into the unchanged `advance` closure.

`mentions` provider: identical, except step 1 is
`And(Key{hasMention==true}, Key{mentionRead==false})`, index-anchored on
`hasMention` (§6.1).

`reactions` provider (Phase 1): the **legacy** full stream —
`GetAfterOrder(ctx, "", …)` ignoring `maxFrontierOrderId` — unchanged
behaviour until Phase 2 (§7).

No new cross-database plumbing: step 3 reuses the existing `Storage().Get`
point lookup (a pooled-connection `BEGIN/COMMIT` per call,
`db.go:248-267`; far cheaper than streaming ~150 k rows for small unread
counts, with §6.3 capping the pathological case).

### 6.3 Defensive bounded-scan fallback

If the candidate count exceeds a configurable threshold (pathological
"frontier exists but most messages unread"), fall back to a streaming
`Storage().GetAfterOrder` scan that **stops as soon as `OrderId >
maxFrontierOrderId`** (the stream is sorted by OrderId). This is strictly
cheaper than today's `""` full scan and removes any worst-case regression from
issuing N point lookups. The threshold value is an implementation/plan
decision; the design only requires that such a cap exists.

## 7. Scope & phasing

- **Phase 1 — `messages` + `mentions`:** the bounded provider (§6.2) + indexes
  (§6.1) + fallback (§6.3). These drive `markReadMessages`, the profile's hot
  path.
  In Phase 1 the `reactions` `diffManager` keeps the legacy full-stream
  provider (§6.2), so reactions behaviour is byte-identical to today.
- **Phase 2 — `reactions`:** swap the reactions provider for a bounded one —
  candidate query over the existing sparse `rUnreadOrdId` index, id source
  `rUnreadChIds` (reaction change ids), `AddSeq` via `Storage().Get` on those
  change ids. **Not blocking** Phase 1; reactions are a distinct mechanism
  (`markReadReactions` / `ClearUnreadReactions(maxOrderId)`, OrderId-only
  today).
- **Unchanged:** `resolvePair`, `prune`, `dominated`, the persisted
  seen-heads KV value, and `MarkMessagesAsUnread` / `collectSeenHeads`
  (seen-head derivation — a separate concern, explicitly out of scope). The
  `SubscribeForKey` live path and `MarkSeenHeads` benefit automatically
  *because* they call `advance` with the same per-`diffManager` provider
  (they pass `manager.provider`, not a counter-agnostic `s.eachChange`).

## 8. Rejected alternatives

- **OrderId-only trim (drop `AddSeq`).** Simplest, deletes the engine, but
  loses the "in-past insert stays unread" guard (a late-synced/backfilled old
  message would be silently marked read). Rejected: correctness relaxation
  not accepted.
- **Denormalize `AddSeq` onto the chat doc at apply time.** Would make the 2D
  predicate one indexed chat query with no cross-store lookup, but tree
  rebuild/migration renumbers `AddSeq` (`treemigrator` resets the counter and
  re-`AddAll`s), so a frozen copy desynchronises from the live `changes`
  value; also needs a one-time backfill scan and a schema-version guard.
  Rejected: reintroduces an epoch-consistency hazard the live-resolve model
  avoids.

## 9. Testing strategy

- **Engine algorithm unchanged, signatures re-shimmed:** the `dominated`
  linear / in-past-insert / multi-head assertions in `readwatermark_test.go`
  keep their *expected values*, but every `advance` call there, the
  `readwatermark_export_test_shim.go` `Advance` wrapper, and the chatobject
  scenario sign-off harness must be mechanically updated to the new third-arg
  shape — the legacy provider (ignores `maxFrontierOrderId`, streams `all`)
  reproduces today's behaviour so expected emissions are unchanged. §9's
  guarantee is *expected values unchanged*, **not** *call sites unchanged*.
- **Completeness property test (the real gate):** for randomized change sets
  and multi-head / in-past-insert frontiers, assert bounded-provider-driven
  `advance` emission == legacy-full-stream-driven `advance` emission (same
  `onRemove` id multiset).
- **Cross-invocation test (review S4):** apply a seenHeads value via the
  `SubscribeForKey` callback *before* the dominated chat row exists; then
  apply the row and trigger a subsequent `advance`; assert the id is emitted
  **exactly once** (validates the §5 re-evaluation invariant and that
  `marked` does not suppress a never-emitted id).
- **Positive-equality index test (review B1):** assert the `messages`
  candidate query uses `Key{read==false}` (not `Not{read==true}`) and that an
  `.explain()`/index-usage probe shows the `read` index is selected, not a
  full collection scan; analogously `hasMention` for `mentions`.
- **Cold-start benchmark:** against the captured real DB (4,794-message chat,
  ~150 k changes), assert no full tree scan for `messages`/`mentions` and
  `Storage().Get` count ≈ number of unread candidates; capture before/after
  CPU on the watermark path.
- **Fallback test:** force the §6.3 threshold and assert the bounded scan
  stops at `maxFrontierOrderId` and yields the same emission as the legacy
  full stream.

## 10. Risks & open questions

- **Refactor surface (not purely mechanical).** Per §4 the change touches
  `advance`'s signature, adds per-`diffManager` provider routing at three
  call sites, and re-shims all `advance` tests + export shim + scenario
  harness. The dominance *algorithm* is unchanged; the wiring is not trivial.
  The completeness property test (§9) is the gate that the algorithm's
  behaviour did not move.
- **Fallback threshold.** Deferred to the plan; default conservative
  (favouring the bounded path) and benchmarked. Note `mentions` may sit near
  the threshold for chats where the user follows many mentions — benchmark
  with a mention-heavy chat, not only `messages`.
- **Phase-2 reactions.** Reaction `AddSeq` resolution by reaction-change-id is
  designed but unbenchmarked; in Phase 1 the reactions `diffManager` keeps
  the legacy full-stream provider, so reactions are byte-identical to today
  and the per-counter full scan for reactions is **not** eliminated until
  Phase 2.
- **Live-path correctness model.** Equivalence is *per `advance` invocation*
  against currently-applied rows; cross-invocation completeness rests on
  `marked` never suppressing a never-emitted id (§5). The cross-invocation
  test (§9) locks this; the naive "row before advance" precondition was
  withdrawn (review S4) as unprovable and unnecessary.

## 11. Key references

Code (`start-performance`):
`core/block/source/sourceimpl/readwatermark.go:14-21,46-87,93-105`;
`core/block/source/sourceimpl/store.go:112-118,122-129,131-156,167-195,235-243,348-355`;
`core/block/source/sourceimpl/store_apply.go:22-47`;
`core/block/chats/chatrepository/repository.go:129-150,206,440-482,484-500,697-719`;
`core/block/chats/chatmodel/chatmodel.go:40-61`;
`core/block/editor/chatobject/chatobject.go:229-246`;
`core/block/editor/chatobject/reading.go:90-101,106,131,173,217-226`.

Dependency (any-sync v0.12.4):
`commonspace/object/tree/objecttree/storage.go:22-49,61-77,233-257,297-333,398-417`;
`commonspace/spacestorage/spacestorage.go:80-84,133-137`.

Evidence: profile `anytype_profile_20260516_170652`; real DB
`…/AASdKiEGfcyhxX3ufr4auHRviACUXxkF68uZwtSb2AnyRoMA2/` (chats in
`objectstore/…2lvnkexvcryd2/crdt.db`, changes in
`spaceStoreNew/…2lvnkexvcryd2/store.db`, 150,736 changes).
