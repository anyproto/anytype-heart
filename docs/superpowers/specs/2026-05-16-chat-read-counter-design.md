# Chat Read-Counter — Design (OrderId+AddSeq Dominance)

**Status:** proposed design, pre-plan
**Date:** 2026-05-16
**Supersedes:** the engine-redesign (`objecttree.DiffManager` rebuild / snapshot cache / "engine B" / closure / frontier) program — **deleted** as over-engineering. See §7.
**Gated by:** the committed multi-device scenario suite (`2026-05-16-chat-read-counter-multidevice-scenarios-design.md`) — repurposed here from a *behavior-preservation gate* to the **semantic sign-off artifact** (§5).

## 1. Problem

Chat read-counters are produced by rebuilding an any-sync `objecttree.DiffManager` over the **whole** chat change tree from cold SQLite on every start (`store.InitDiffManager` → `BuildHistoryTree` + `RemoveBefore`), the dominant Android cold-start cost (~18.6 s `pread`). The DiffManager computes exact **DAG-ancestor** read state. Chat does not need that precision.

## 2. The model

Per counter (messages / mentions / reactions), a message `X` is **read** iff it was within the user's read region *and had already arrived* when they read:

```
read(X)  ⟺  ∃ H ∈ seenHeads :  OrderId(X) ≤ OrderId(H)  ∧  AddSeq(X) ≤ AddSeq(H)
```

- `seenHeads` — the synced, CRDT-merged set of stable change-ids (the user's read frontier). **Unchanged** from today; it stays in `KeyValueService` (the only thing that *can* be synced — OrderId/AddSeq are local).
- `OrderId(·)`, `AddSeq(·)` — resolved **locally** per id via `s.treeSource.Tree().GetChange(id)` (`*objecttree.Change` has `OrderId string`, `AddSeq uint64`). OrderId is a deterministic topological order (content-hash sibling tiebreak — verified `tree.go:294-308`); AddSeq is the per-peer monotonic insert sequence, never reassigned (verified `storage.go:44-49`). Both are local, deterministic functions of the local change-set; never transmitted.
- Chat message id == tree change id, message OrderId == tree order (established by the suite harness). So a message's `(OrderId, AddSeq)` is its change's.

There is **no change graph, no differ, no `{id,prevIds}` cache, no any-sync modification.** Read state is the existing per-message SQLite flag (`chatrepository.SetReadFlag`); this model only decides which flags to flip.

### Why AddSeq is half the predicate (not a cursor)

`updateHeads` rewrites OrderId *into the past* for late/merge changes (verified `tree.go:431-470`). A message that syncs in late but is slotted into an old OrderId position has `OrderId(X) ≤ OrderId(H)` yet the user never saw it. The `AddSeq(X) ≤ AddSeq(H)` conjunct makes it correctly **unread**: it was inserted after the seen frontier existed. OrderId alone is structurally incapable of this; AddSeq is the only reliable "had this arrived when the user read" signal. (This is exactly the failure that motivated much of the deleted engine-B complexity — here it is solved *by construction*.)

### Multi-seen-head: per-head dominance, NOT per-axis max

The predicate is `∃ H` (per-head dominance) then OR — **not** `max(OrderId)` ∧ `max(AddSeq)` independently. Counter-example: `seenHeads {H1=(o10,a5), H2=(o4,a20)}`, `X=(o7,a8)`. Per-axis max → `o7≤10 ∧ a8≤20` → "read" (wrong). Per-head: no single H dominates X → correctly **unread**. In near-linear chat reading `seenHeads` is usually one head and this degenerates to two scalar comparisons; the per-head form is the correct general statement, O(|seenHeads|) tiny.

## 3. Why this is sufficient and correct for chat

- The chat's own model is **already** OrderId-range: `MarkReadMessages([AfterOrderId,BeforeOrderId])`, `collectSeenHeads(afterOrderId)`, repo flags keyed by OrderId, `ClearUnreadReactions(maxOrderId)`. This model is the natural completion of that, not a foreign concept.
- Late/in-past inserts → correctly unread (AddSeq conjunct). Linear reading → trivially correct. Cross-device → consistent for the same change-set (OrderId/AddSeq deterministic); divergence only during sync lag (inherent to any model) or the residual case in §5.

## 4. Edge cases (all small, graph-free)

1. **Monotone application (F3-immunity).** The durable truth is the sticky per-message SQLite flag; the seen frontier only ever advances (reads grow; `seenHeads` only grows). Never recompute "read" from scratch and never un-set — only OR-in newly-read ids as the frontier advances. OrderId reassignment then cannot corrupt read state.
2. **Pending seen-id.** A `seenHead` synced from a device whose change isn't local yet → `GetChange` misses → defer that head's contribution; re-resolve when it arrives (via the `GetAfterAddSeq(lastSeenAddSeq)` cursor). Small pending set.
3. **`MarkMessagesAsUnread(afterOrderId)`.** Already an OrderId-range repo op; additionally roll that counter's frontier back. Compatible.
4. **Counters.** mentions filter by mention; reactions already keyed off `maxOrderId` — both fit natively (three independent frontiers).
5. **Cost (measured, corrected).** Profiling the first cut showed a *regression*, not a win: `advance` ran a full `GetAfterOrder` change scan **and** re-emitted the entire dominated set to `onRemove` **on every call**, and `initDiffManagers` called it once per persisted device value × 3 counters → repeated full scans + a huge re-run `SetReadFlag` `id IN` query (the cold-start regression). **Fixed:** (a) `initDiffManagers` merges all device values and applies them in **one** `InitDiffManager`/advance per counter; (b) the engine tracks already-emitted ids (`marked`) so `onRemove` carries only the **newly-dominated delta**, bounding `SetReadFlag` to O(delta). This restores it to ≤ the old `DiffManager` (one lightweight id/OrderId/AddSeq scan per counter, no payload/graph). **Follow-up (not yet done):** drive candidates from the chat repo's *unread* set (O(unread), no tree scan at all) — requires a `RegisterDiffManager` contract change and, for an indexed `GetAfterAddSeq` tail, a `(TreeKey, AddSeqKey)` index in any-sync (cross-repo; only `(TreeKey, OrderKey)` exists today).

## 5. Residual semantic vs legacy DAG — the sign-off

This **is** a behavior change vs today's `DiffManager` (DAG-ancestor). They differ in exactly one case: a message *genuinely concurrent* to the seen head that **was already present** when the user read (so `AddSeq(X) ≤ AddSeq(H)`) and sorts before it (`OrderId(X) ≤ OrderId(H)`) — this model marks it **read**; the DAG marks it unread. For a linear chat timeline (both messages displayed, user scrolled past both) "read" is the intuitive answer; the DAG precision is general-object-tree correctness chat never wanted. The late-insert case (the scary one) does **not** diverge — both say unread — so the divergence is narrow and chat-favourable.

Because it is a (small, already-pervasive) semantic change, it requires **explicit owner sign-off**. The committed scenario suite produces exactly that: running the catalog with this engine flips the two crux tripwires (`S-concurrent-merge-divergence`, `S-cat12-dag-ancestor-vs-orderid-prefix`) to DIVERGENCE. That is not a failure — it is the **recorded, accepted decision**. All other scenarios must stay MATCH (any other divergence is a real bug).

**Open product decision (the only one):** accept that a genuinely-concurrent, already-present message sorting before the seen head counts as *read* in chat. Default recommendation: **yes** (matches chat UX and the codebase's existing OrderId-range model). The suite report is the sign-off record.

## 6. Implementation surface (small)

Replace the per-counter `*objecttree.DiffManager` with a tiny per-counter watermark engine while **keeping the chat-facing contract** so nothing downstream changes:

- `RegisterDiffManager(name, onRemove)` — unchanged signature; store `onRemove` + a per-counter frontier.
- `InitDiffManager` / `initDiffManagers` — resolve synced `seenHeads` → `(OrderId,AddSeq)` pairs via `Tree().GetChange`; compute the newly-read unread message-ids (dominance predicate over the repo's unread set) and fire `onRemove(those)` → existing `markReadMessages` → repo flags. (No `BuildHistoryTree`, no graph.)
- `MarkSeenHeads` — extend the frontier, compute delta, `onRemove`, persist synced `seenHeads` (unchanged) + local `lastSeenAddSeq`.
- `addToDiffManagers` / `updateInDiffManagers` — **no-ops** (documented): a newly pushed/synced change has the newest AddSeq, so it is never dominated by an existing frontier ⇒ unread by construction; no graph to maintain.
- `StoreSeenHeads` / `seenHeadsKey` / the `SubscribeForKey` sync — unchanged (synced anchor).
- `ProvideStat` — report the per-counter frontier pairs (debug).

Downstream (`chatobject.markReadMessages` → `chatrepository`) is **untouched**; the scenario harness still observes `onRemove → repo unread counts`, so it remains a faithful sign-off measurement.

## 7. Rejected (and why)

- **DAG-exact preservation / engine "B" / streaming closure / `{id,prevIds}` snapshot / frontier-state** — three independent reviews + this analysis: it defends a precision chat does not want, at the cost of a graph cache and/or a hand-rolled reachability reimplementation whose only safety net (the catalog) provably cannot cover its divergent paths (no `stepUpdate`; `IsNew` unavailable to a stream). Deleted.
- **Persisted change-graph cache (the snapshot-cache branch)** — unnecessary once read state is `(OrderId,AddSeq)` dominance: cold start needs only a few `GetChange` point lookups + a repo range op, not a tree rebuild. Branch retained in git history only.
- **Any any-sync change** — none required.

## 8. Next step

One small implementation plan (`../plans/2026-05-16-chat-read-counter-watermark-plan.md`): the dominance predicate, the per-counter watermark engine, the `store.go` swap (keeping the chat contract), pending-seen-id, markUnread, and the catalog run as the sign-off artifact (non-crux MATCH; crux DIVERGENCE recorded as the accepted semantic). No any-sync changes; no new package required.
