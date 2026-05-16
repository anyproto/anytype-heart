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

6. **Chat doc is written before `advance` runs.** In `ReadStoreDoc`,
   `storeApply.Apply` + `tx.Commit()` (which writes chat docs via the
   storestate tx) completes **before** `initDiffManagers → advance`
   (`store.go:235-243`; `store_apply.go:22-47`).

7. **`advance` is shared by all three counters and three call sites.**
   `RegisterDiffManager` wires `messages`/`mentions` → `markReadMessages`,
   `reactions` → `markReadReactions` (`chatobject.go:229-246`). `advance` is
   called from `InitDiffManager` (`store.go:177`), the `SubscribeForKey` live
   callback (`store.go:188`), and `MarkSeenHeads` (`store.go:351`) — all
   passing `s.eachChange(ctx)`.

## 4. Core principle

The watermark's **dominance / dedup / emit logic** — `resolve` of seen heads,
`prune`, the `dominated` test, the `marked` set, and the `onRemove` delta
emission (`readwatermark.go:46-87`) — is **unchanged**. Multi-head union and
the in-past-insert invariant are therefore preserved by construction.

Exactly one mechanical change to the engine: `advance`'s stream parameter
contract gains the resolved frontier's `maxFrontierOrderId` (defined in §5),
because the bounded provider needs that bound and it is only known *after*
`resolve`+`prune` runs *inside* `advance`. Concretely, the third parameter
changes from `func(yield func(string, readPair))` to a form that also receives
`maxFrontierOrderId` (e.g. `func(maxFrontierOrderId string, yield func(string,
readPair))`); the lines that compute the frontier, test `dominated`, update
`marked`, and call `onRemove` are untouched. We change only the **source of
the `(id, pair)` stream**: from "every change in the tree" to "the bounded set
of currently-unread candidates, each `AddSeq`-resolved via the same
`Storage().Get` point lookup `resolvePair` already uses".

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
- The old scan also yielded non-message changes (edits, root). For
  `markReadMessages` those matched no message row (`id IN` no-op). Excluding
  them is a correctness-neutral improvement.
- Each candidate's `AddSeq` is resolved from the **same** `Tree().Storage()`
  that `resolvePair`/`eachChange` read today → same epoch, no frozen copy.
  The `dominated` test then runs on identical data, so per-id results are
  identical.

Precondition (holds, fact §3.6): every applied message-create change has its
chat row committed before `advance` runs. The live `SubscribeForKey` and
`MarkSeenHeads` paths run after the corresponding chat write on their existing
code paths; this invariant is asserted by the property test in §7.

Conclusion: the bounded provider emits exactly the same delta to `onRemove` as
the full scan, for the same frontier — no correctness loss, full 2D semantics
retained.

## 6. Components

### 6.1 Chat-collection indexes (schema)

Add two **plain** single-field indexes to the chat collection via the existing
`anystorehelper.AddIndexes` list (`repository.go:141`):

- `read`
- `mentionRead`

Rationale: in steady state the unread partition is near-empty (most chats
fully read), so an equality seek `read == false` is the selective driver; the
`_o.id ≤ maxFrontierOrderId` bound is then a cheap in-memory residual over a
tiny set. A compound `(read, _o.id)` index was considered and **rejected** as
YAGNI: it only helps the non-dominant "partially read, large unread backlog"
case, which the §6.3 fallback already handles. Sparse indexes are **not**
usable here — `read`/`mentionRead` always exist (sparse keys on field
absence); the existing `rUnreadOrdId` sparse index does not apply to a boolean
value. `EnsureIndex` is idempotent and matches the established pattern, so this
is an additive, online schema change.

### 6.2 Bounded candidate provider (replaces `s.eachChange`)

A per-counter provider passed as `advance`'s third argument, invoked by
`advance` with `maxFrontierOrderId` (computed inside `advance` after
`resolve`+`prune`, per §4) and the `yield` closure. The provider is
constructed by the `store` (it closes over the chat collection and
`Tree().Storage()`); the engine only invokes it and consumes `yield`. Steps:

For `messages` / `mentions`:

1. Index-seek the chat collection for unread docs
   (`read == false` / `mentionRead == false`).
2. Residually filter `_o.id ≤ maxFrontierOrderId`.
3. For each candidate id, `s.treeSource.Tree().Storage().Get(ctx, id)` →
   `readPair{OrderId, AddSeq}` (the same call `resolvePair` makes).
4. `yield(id, pair)` into the unchanged `advance` closure.

No new cross-database plumbing: step 3 reuses the existing `Storage().Get`
point lookup.

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
- **Phase 2 — `reactions`:** same `advance`, different candidate strategy —
  candidate query over the existing sparse `rUnreadOrdId` index, id source
  `rUnreadChIds` (reaction change ids), `AddSeq` via `Storage().Get` on those
  change ids. Designed here but **not blocking** Phase 1; reactions are a
  distinct mechanism (`markReadReactions` / `ClearUnreadReactions(maxOrderId)`,
  OrderId-only today).
- **Unchanged:** `resolvePair`, `prune`, `dominated`, the `SubscribeForKey`
  live path and `MarkSeenHeads` (they call the same `advance` and benefit
  automatically), the persisted seen-heads KV value, and
  `MarkMessagesAsUnread` / `collectSeenHeads` (seen-head derivation — a
  separate concern, explicitly out of scope).

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

- **Engine invariants unchanged:** existing `readwatermark_test.go` (the
  `dominated` linear / in-past-insert / multi-head cases) must pass with no
  modification — the engine is untouched.
- **Completeness property test:** for randomized change sets and frontiers,
  assert `boundedProvider`-driven `advance` emission == full-scan-driven
  `advance` emission (same `onRemove` id multiset), including in-past-insert
  and multi-head frontiers.
- **Ordering precondition test:** assert that on the `ReadStoreDoc` and live
  `SubscribeForKey` paths the chat row exists before the candidate query reads
  it.
- **Cold-start benchmark:** against the captured real DB (4,794-message chat,
  ~150 k changes), assert no full scan occurs and the `Storage().Get` count
  ≈ number of unread messages; capture before/after CPU on the
  watermark path.
- **Fallback test:** force the §6.3 threshold and assert the bounded scan
  stops at `maxFrontierOrderId` and yields the same emission.

## 10. Risks & open questions

- **`advance` signature change.** The stream factory must learn the frontier's
  `maxFrontierOrderId`. The engine body must remain unchanged; only the
  `store`-side factory wiring and the `advance` parameter plumbing change.
  Plan must keep this refactor mechanical and covered by the completeness
  test.
- **Fallback threshold.** Choosing it is deferred to the plan; default should
  be conservative (favouring the bounded path) and benchmarked.
- **Phase-2 reactions.** Reaction `AddSeq` resolution by reaction-change-id is
  designed but unbenchmarked; Phase 1 must not regress reactions (reactions
  keep today's `eachChange` path until Phase 2 lands).
- **Live-path ordering.** The §5 precondition for `SubscribeForKey` /
  `MarkSeenHeads` relies on existing write-before-advance ordering; the
  ordering test (§9) must lock it.

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
