# Chat Read-Counter "Computed Query" Redesign — Theory and Proof

Status: theory note grounding the computed-query redesign. Self-contained.
Branch: `go-7290-chat-read-counter-computed`.
Scope: the two-axis `(OrderId, AddSeq)` dominance read model and its
two-query enumeration in `GetUnreadMessagesComputed`.

> Conventions. "Verified" = read directly in the cited source.
> "Inferred" = derived from verified facts by argument. "Assumed" = relied on
> but not proven here (every such item is listed in §7). Citations are
> `path:line` at the head HEAD of this branch.

---

## §0. What is actually being asked, and what a correct answer must satisfy

The redesign replaces the materialized `read`/`mentionRead` boolean on each chat
message row with a *computed* predicate: a message is read iff its stored
coordinate `(OrderId, firstAddSeq)` is dominated by the resolved seen frontier.
`GetUnreadMessagesComputed`
(`core/block/chats/chatrepository/repository.go:452`) enumerates the unread set
by running two single-field index queries, unioning them, and filtering by the
in-memory dominance predicate `chatmodel.Dominated`
(`core/block/chats/chatmodel/readcoord.go:23`).

A correct theory must establish two *logically distinct* claims, which the task
(and the code comments) are careful to separate:

- **(A) Algorithmic coverage.** The two-query union + filter computes *exactly*
  the set `{peer X : ¬Dominated(X, F)}` for the given frontier `F`. This is a
  statement about the SQL/anystore enumeration faithfully realizing the math. It
  is independent of how `OrderId`/`AddSeq` were assigned.

- **(B) Semantic / order-of-adding invariance.** Given a *normalized* tree
  (fixed, convergent `OrderId` for every change) and a *fixed frontier as a set
  of change ids*, is the computed read/unread set invariant under the local
  apply order — i.e. under the different `AddSeq` assignments that are all valid
  topological extensions of the same DAG? This is a statement about the *model*,
  not the enumeration.

The honest result, proven below: **(A) holds unconditionally.** **(B) does NOT
hold unconditionally**; it fails exactly on one configuration — a message `X`
concurrent with a seen head `H∈F` such that `OrderId(X) ≤ OrderId(H)` and no
causally-related head also covers `X` — and that configuration is precisely the
already-accepted concurrent-merge divergence class recorded in the sign-off gate
(`S-concurrent-merge-divergence`, `S-cat12`, `S-cat19`). We characterize the
exact condition under which (B) holds and prove it there.

---

## §1. How Anytype uses trees

### 1.1 The object as a CRDT change DAG

An Anytype object is a Merkle-DAG of *changes*. Each change is a signed node
carrying `PreviousIds` (its parents / causal predecessors), a payload, an author
identity, and an ACL head reference. The set of changes with no successor are the
*heads*. A chat is such an object; **a chat message is a change**, and the
message *row* in the materialized store is the projection of that change's
payload.

There are two distinct stores:

- **The changes DB (`crdt.db`)** — the raw object-tree: `objecttree.Storage`
  holds each `StorageChange` with its `Id`, `PrevIds`, `OrderId`, `AddSeq`,
  snapshot info, and raw bytes
  (`commonspace/object/tree/objecttree/storage.go:38-49`).
- **The materialized any-store docs (`store.db`)** — the chat message rows,
  collection `{chatObjectId}chats`
  (`core/block/chats/chatrepository/repository.go:129`). A row carries the
  decoded message *plus* the read-model coordinates: `_o.id` (= OrderId) and
  `firstAddSeq` (`core/block/chats/chatmodel/chatmodel.go:50,58`).

### 1.2 How a change is created and applied

There are two code paths that turn a change into a materialized row. They differ
in one field that the entire read model rests on — `AddSeq` — so we name them
precisely:

- **LIVE-PUSH** (`store.PushStoreChange`,
  `core/block/source/sourceimpl/store.go:303`). A change authored *locally now*.
  `AddContentWithValidator` builds the change, assigns its `OrderId`
  (`objecttree.go:273`), and calls a *validator* callback *before* the change is
  committed to storage. Inside that callback the handler runs with
  **`AddSeq = 0`** — the comment at `store.go:333-337` states this explicitly:
  "AddSeq is 0 here: it is assigned only after AddAll". After the callback,
  `AddAll` assigns the real `AddSeq` (§2), and `store.go:351` records it as
  `MaxLastAddSeq` via `tx.UpdateMaxAddSeq`.

- **REPLAY** (`storeApply.Apply`,
  `core/block/source/sourceimpl/store_apply.go:22`). Changes received from sync /
  another device, or loaded from disk, applied in a batch via
  `IterateAfterAddSeq` (`store_apply.go:31`). Here each change already has its
  real `AddSeq` read straight from storage
  (`objecttree.go:732: ch.AddSeq = sc.AddSeq`), and it is threaded through
  `storestate.ChangeSet.AddSeq` (`store_apply.go:65`) into the handler.

In both paths the chat handler `BeforeCreate`
(`core/block/editor/chatobject/chathandler.go:73`) stamps the row:

```
msg.OrderId    = ch.Change.Order   // chathandler.go:106
msg.FirstAddSeq = ch.Change.AddSeq // chathandler.go:107
```

So `firstAddSeq` on a row is the change's `AddSeq` *as observed by the path that
materialized it*: 0 for a row created on the live-push path, the real per-batch
value for a row created on the replay path. (§2 and §6 show why this asymmetry is
benign: live-push rows are always own messages, which are excluded from the
counter.)

### 1.3 `AddRawChanges → AddAll`, reduce/iterate, snapshot/root

A batch of received raw changes enters via `ObjectTree.AddRawChanges`, which
*reduces* them into the tree: unattached changes wait until their parents attach
(`tree.go:312` wait-list), each attached change is appended to each parent's
`Next` list **kept sorted by change Id** (`tree.go:295-308`), and the tree is
re-iterated to (re)assign `OrderId` to any change lacking one
(`updateHeads`, `tree.go:431-479`). The root (first DAG entry, the snapshot)
always has an `OrderId` (`tree.go:217-220`) and is skipped by the read model
(`store_apply.go:52-55` treats the root `TreeChangeInfo` as a no-op).

---

## §2. How changes come from offline — when `AddSeq` is 0 vs real

The design rests on this distinction, so we state it as precisely as the code
allows.

**Verified — `AddSeq` is a per-space, per-`AddAll`-batch counter.**
`storage.AddAll` does `seq := s.addSeq.Add(1)` *once*, then assigns that single
`seq` to **every** change in the batch
(`commonspace/object/tree/objecttree/storage.go:297-299`; identical in
`AddAllNoError`, `:331-333`). `addSeq` is a shared `*atomic.Uint64` for the
space (`storage.go:87`, settable via `SetAddSeq`, `:368`). The head entry's
`LastAddSeq` is updated to `seq` (`storage.go:312,346`). On reload the counter is
restored from that persisted maximum (the replay path reads
`tx.GetMaxAddSeq()` at `store_apply.go:29` and advances it per change at `:32`).

Two consequences, both load-bearing:

1. **`AddSeq` is monotone in apply (batch) order, and equal within a batch.** All
   changes added in the same `AddAll` share one `AddSeq`; a later batch gets a
   strictly larger one.
2. **`AddSeq` is a linear extension of the causal DAG on each device.** A change
   is attached (and thus stored) only after all its parents
   (`tree.go` wait-list, `:312-320`), and a parent is in the same or an earlier
   batch. Therefore *ancestor ⟹ AddSeq ≤ descendant's AddSeq*, with strict `<`
   when they are in different batches and `=` when in the same batch. (Inferred
   from the attach-after-parents invariant + the per-batch counter.)

**When `AddSeq` is 0 vs real (verified):**

- 0 — only transiently, inside the live-push validator callback, before `AddAll`
  runs (`store.go:333-337`). A row materialized at that instant stores
  `firstAddSeq = 0`.
- real — on the replay path always (`store_apply.go:65`), and after `AddAll` for
  the live-pushed change (`store.go:351`).

**A peer's message always arrives via REPLAY**, hence is stamped with a real
`AddSeq` at row creation. (Inferred: a peer message is not authored locally;
it enters only through sync/disk, which is the replay path.) An own message can
be materialized on the live-push path with `firstAddSeq = 0` — but own messages
are excluded from the counter (§4, §6), so the 0 never reaches the dominance
test for a counted row.

**The same change can get DIFFERENT `AddSeq` on different devices.** Device P may
receive `{m1, x1}` in one batch (both get `seq=k`), while device Q receives `x1`
in batch `k'` and `m1` later in batch `k'+3` (so `AddSeq_Q(x1) < AddSeq_Q(m1)`).
Nothing constrains the *relative* `AddSeq` of two concurrent changes across
devices. This is the asymmetry the proof turns on. (Inferred from the per-batch
counter being driven by *local* receive order, which sync does not fix for
concurrent changes.)

---

## §3. The two primitives — formal definitions

Fix an object with change set `C` and causal (DAG) partial order `≺` ("`a ≺ b`"
= `a` is a proper ancestor of `b`). Write `a ∥ b` when `a ⊀ b ∧ b ⊀ a ∧ a ≠ b`
(concurrent). For a *replica* (device) `r`, let `C_r ⊆ C` be the changes present
on `r`.

### 3.1 `OrderId` (lexid)

`OrderId : C → Σ*` maps each change to a lexid string
(`var lexId = lexid.Must(...)`, `tree.go:23`; mirrored at
`storestate/state.go:29`). It is assigned by the tree's deterministic traversal:
on the live-push path `OrderId = lexId.Next(OrderId(lastIteratedHead))`
(`objecttree.go:273`); on the reduce path `updateHeads` fills gaps with
`lexId.NextBefore` and the tail with `lexId.Next`
(`tree.go:441-472`), where the iteration order is the reverse-postorder DFS
`topSort` (`treeiterator.go:46-79`) over `Next` lists **sorted by change Id**
(`tree.go:295-308`).

We use two properties.

> **Property O1 (topological / linear extension).** For all `a, b ∈ C`,
> `a ≺ b ⟹ OrderId(a) < OrderId(b)` (string `<`).

*Verified by construction + test.* `updateHeads` assigns OrderIds strictly
increasing along the traversal, and the traversal is topological (a change is
emitted only after all its `Next` are finished, `treeiterator.go:54-74`).
`TestBuildScenarioTree_BranchMergeDeterministic`
(`core/block/editor/chatobject/scenario_harness_test.go:121-123`) asserts
`orderOf[prev] < orderOf[child]` for both a linear and a merge parent. We take
O1 as established for the materialized fields; the deepest internal lexid
arithmetic (`NextBefore` always yielding a strictly-in-between string) is an
any-sync invariant (§7, A7).

> **Property O2 (replica-invariant comparison / convergence).** For all
> `a, b ∈ C` present on two replicas `r, s`,
> `sign(OrderId_r(a) ⟂ OrderId_r(b)) = sign(OrderId_s(a) ⟂ OrderId_s(b))`. The
> string *values* are local, but the *comparison sign* converges.

*Inferred from the construction.* The traversal that induces the order depends
only on (i) the DAG shape and (ii) the sibling tiebreak, which is the change Id
(`tree.go:295-308`). Change Ids are content/signature-derived and identical for
the same change on every replica. Hence two replicas holding the same sub-DAG
visit the changes in the same relative order, so the induced linear order — and
thus every pairwise comparison sign — agrees. (The absolute strings differ
because gap-filling depends on which neighbors were present when a change was
inserted; only the *order* is claimed to converge. This is the standard lexid
contract; see §7, A7. `TestBuildScenarioTree_BranchMergeDeterministic:119`
checks the weaker fact that two *identical* builds produce identical strings.)

`OrderId` is stored on the message row as `_o.id`
(`chathandler.go:106` → `chatmodel.OrderKey`/`"_o"`,
`chatmodel.go:50`; read back at `repository.go:479`).

### 3.2 `AddSeq`

`AddSeq_r : C_r → ℕ` is the per-space, per-batch local apply counter of §2
(`storage.go:297`). Its invariants:

> **Property S1 (linear extension of `≺`, per device).** For all `a, b ∈ C_r`,
> `a ≺ b ⟹ AddSeq_r(a) ≤ AddSeq_r(b)`, strict when `a, b` are in different
> apply batches, equal when in the same batch.

*Verified / inferred*, §2 consequence 2 (attach-after-parents +
per-batch counter).

> **Property S2 (device-local; concurrent pairs unconstrained).** For `a ∥ b`,
> the relation between `AddSeq_r(a)` and `AddSeq_r(b)` is determined by `r`'s
> *receive batching* and may differ across devices: any of `<`, `=` (same
> batch), `>` is possible, and a different device may realize a different one.

*Inferred*, §2 ("same change, different AddSeq").

> **Property S3 (live-push = 0; restored from max on load).** A change
> materialized inside the live-push validator stores `AddSeq = 0`
> (`store.go:333-337`); otherwise the real value
> (`store_apply.go:65`, `store.go:351`); the counter is restored from the
> persisted `MaxLastAddSeq` on reload (`store_apply.go:29`).

*Verified.*

`AddSeq` is stored on the row as `firstAddSeq` (`chathandler.go:107`;
`chatmodel.go:478` marshals it as a number; read back at `repository.go:480`,
`chatmodel.go:585`).

### 3.3 Their relationship — the key asymmetry

Both `OrderId` and `AddSeq_r` are linear extensions of the *same* causal partial
order `≺` (O1, S1). The difference is:

- `OrderId` extends `≺` using a **replica-invariant** tiebreak on concurrent
  pairs (the change Id, O2). The order of concurrent changes is fixed across all
  replicas.
- `AddSeq_r` extends `≺` using a **replica-local** tiebreak on concurrent pairs
  (the receive-batch order, S2). The order of concurrent changes is *not* fixed.

This asymmetry is the entire content of §5(B): on a fixed normalized tree the
`OrderId` axis is invariant, but the `AddSeq` axis is not — so any read decision
that depends on the `AddSeq` comparison *between two concurrent changes* is
order-dependent.

---

## §4. The read model

### 4.1 Frontier and the dominance predicate

Reading is tracked by a **seen frontier** `F`: a set of change ids the user has
read up to, persisted (synced) as a `KeyValueService` value per counter
(`store.go:420-433`). At runtime the source resolves each id to its local
coordinate via `resolvePair` (`store.go:115-121`, using lock-free
`Storage().Get`), prunes the resolved set to its maximal pairs
(`watermark.prune`, `readwatermark.go:93-105`), and exposes the result as
`GetReadFrontier` (`store.go:442-452`). So at the point the computed query runs,
`F` is a set of resolved coordinates `(OrderId, AddSeq)`.

> **Definition (read).** For a change `X` with coordinate
> `x = (OrderId(X), AddSeq(X))`,
>
>     read(X)  ⟺  Dominated(x, F)  ⟺  ∃ H ∈ F :  x.OrderId ≤ H.OrderId  ∧  x.AddSeq ≤ H.AddSeq.

This is exactly `chatmodel.Dominated` (`readcoord.go:23-30`) and the source-side
`dominated` (`readwatermark.go:14-21`); the two are byte-identical predicates.

**Why both axes (verified intent, `readcoord.go:13-22`, `readwatermark.go:11-13`):**

- The **`OrderId` axis** is the convergent timeline cut: "`X` is at or before
  some head in the replicated order." Because `OrderId` comparisons converge
  (O2), this part of the decision is identical on all devices.
- The **`AddSeq` axis** guards *late in-past inserts*: a change with a *small*
  `OrderId` (so it falls inside the timeline already read) but which *arrived
  after* the user read past that point. Such a change has an `AddSeq` larger than
  the head's, so `x.AddSeq ≤ H.AddSeq` fails and it correctly stays unread. The
  `LargeAddSeqBoundary` test (`computed_edgecases_test.go:77-112`) pins this: a
  row at `(o01, MaxInt64)` with frontier `(o09, 3)` stays unread because its
  `AddSeq` exceeds the head's, even though its `OrderId` is the smallest.

> **Lemma 0 (frontier monotonicity).** `Dominated(x, F)` is monotone in `F`:
> `F ⊆ F' ⟹ (Dominated(x,F) ⟹ Dominated(x,F'))`. Pruning `F` to maximal pairs
> does not change `Dominated` for any `x`.

*Proof.* `Dominated` is an existential over `F`; adding heads can only make it
true. For pruning: a pair `p` is dropped only when another kept pair `q` has
`p.OrderId ≤ q.OrderId ∧ p.AddSeq ≤ q.AddSeq` (`readwatermark.go:99`). Any `x`
dominated by `p` (`x ≤ p` on both axes) is then dominated by `q` by transitivity
of `≤`. ∎ (Stated in code at `readwatermark.go:89-92`, `readcoord.go:19-22`.)

### 4.2 The two-query enumeration

`GetUnreadMessagesComputed` (`repository.go:452-523`), for a counter and a
resolved frontier `F`:

1. Compute `maxF = max_{H∈F} H.OrderId` and `minF = min_{H∈F} H.AddSeq`
   (`repository.go:494-502`).
2. **Query A (timeline tail):** rows with `_o.id > maxF`
   (`repository.go:505`).
3. **Query B (recent arrivals):** rows with `firstAddSeq > int64(minF)`
   (`repository.go:508`).
4. Union into a `map[id]candidate` (dedup by id, `repository.go:460,477`).
5. For each candidate: drop if `creator == myIdentity` (own = read,
   `repository.go:515`); drop if `Dominated(coord, F)`
   (`repository.go:518`); the rest are unread.
6. The counter filter `getMessagesFilter()` is `AND`-ed into both queries:
   `nil` for messages, `hasMention == true` for mentions
   (`repository.go:454,463`; `readhandler.go:47-49,68-70`).
7. Empty frontier ⟹ enumerate *all* rows (`repository.go:488-492`) and (since
   nothing dominates) return every peer counter-matching row.

---

## §5. The proof

### §5.1 — (A) Algorithmic coverage

Let `M` be the set of message rows in the collection, each with coordinate
`coord(m)` and `creator(m)`. Let `counter(m)` be true iff `m` matches the counter
filter (always true for messages; `hasMention` for mentions). Define the **target
unread set**

    U(F) = { m ∈ M : counter(m) ∧ creator(m) ≠ myIdentity ∧ ¬Dominated(coord(m), F) }.

> **Theorem A.** `GetUnreadMessagesComputed` returns exactly `U(F)` (as a set).

We prove it via three lemmas, splitting the non-empty-frontier case from the
empty-frontier case.

> **Lemma A1 (band decomposition / superset).** For any `x` and non-empty `F`,
>
>     ¬Dominated(x, F)  ⟹  x.OrderId > maxF  ∨  x.AddSeq > minF.

*Proof.* Contrapositive. Suppose `x.OrderId ≤ maxF` and `x.AddSeq ≤ minF`. Let
`H*` attain `maxF = H*.OrderId`. Then `x.OrderId ≤ H*.OrderId`. And
`x.AddSeq ≤ minF ≤ H*.AddSeq` (since `minF` is the *minimum* AddSeq over `F`, it
is `≤` every head's AddSeq, in particular `H*`'s). So `H*` dominates `x`, i.e.
`Dominated(x, F)`. ∎

Lemma A1 is the formal version of the comment "their union is a superset of the
unread set" (`repository.go:503-504`). It says: every non-dominated `x` is caught
by Query A or Query B. Note the choice of `maxF`/`minF` is exactly what makes a
*single* head `H*` witness the implication — using `max OrderId` with `max AddSeq`
would be unsound (the two extremes could come from different heads). The code
takes `min` AddSeq, which is correct here.

> **Lemma A2 (candidate set = all non-dominated counter-matching peer rows that
> the two queries can reach, with no false negatives).** Let `Cand` be the map
> built by the two `collect` calls. For every `m ∈ M` with `counter(m)` and
> `¬Dominated(coord(m), F)`: `m ∈ Cand`.

*Proof.* By A1, `coord(m).OrderId > maxF` or `coord(m).AddSeq > minF`.
- If `OrderId > maxF`: Query A's predicate `_o.id > maxF`
  (`CompOpGt`, `repository.go:505`) matches `m`; with the counter filter
  `AND`-ed (`repository.go:463`), and `counter(m)` true, `m` is collected.
- If `AddSeq > minF`: Query B's predicate is `firstAddSeq > int64(minF)`
  (`repository.go:508`). Since `minF ≥ 0` and `coord(m).AddSeq > minF ≥ 0`, the
  value `coord(m).AddSeq` fits in the non-negative range and the cast
  `int64(minF)` preserves the comparison (`minF ≤ MaxInt64` by S-bound, §7 A6;
  the `LargeAddSeqBoundary` test exercises `MaxInt64`,
  `computed_edgecases_test.go:78`). So `m` is matched and collected.
Either way `m ∈ Cand`. (No false negatives.) ∎

> **Lemma A3 (filter soundness + dedup).** The post-filter
> (`repository.go:514-521`) returns from `Cand` exactly those `m` with
> `creator(m) ≠ myIdentity ∧ ¬Dominated(coord(m), F)`, each once.

*Proof.* The loop iterates the map (keys are ids → no duplicates: a row matching
both queries is written under its id twice but stored once, `repository.go:477`;
`TestGetUnreadMessagesComputed_CandidateDedup` confirms,
`computed_edgecases_test.go:189-206`). It skips `creator == myIdentity`
(`:515`) and skips `Dominated(coord, F)` (`:518`). `coord` is read back from the
stored fields `_o.id` and `firstAddSeq` (`:479-480`), which equal the values
written at `chathandler.go:106-107`. Counter membership was enforced at collect
time (A2's filter), so every surviving `m` satisfies `counter(m)`. ∎

*Proof of Theorem A (non-empty `F`).*
- (`⊆`) Every returned `m` satisfies `counter(m)` (collect filter),
  `creator(m) ≠ myIdentity` and `¬Dominated` (A3). So returned `⊆ U(F)`.
- (`⊇`) Take `m ∈ U(F)`. Then `counter(m)`, `creator ≠ myIdentity`,
  `¬Dominated`. By A2, `m ∈ Cand`. In the filter, `creator ≠ myIdentity` and
  `¬Dominated` both hold, so `m` is returned (A3). So `U(F) ⊆ returned`.
Hence returned `= U(F)`. ∎

*Empty-frontier case.* When `F = ∅`, the code collects all rows
(`Exists{}` on `id`, `repository.go:490`) with the counter filter, then filters:
`Dominated(x, ∅)` is false for every `x` (empty existential), so the result is
`{ m : counter(m) ∧ creator(m) ≠ myIdentity }`, which equals `U(∅)`. Confirmed by
`TestGetUnreadMessagesComputed_AddSeqZeroRows/empty frontier`
(`computed_edgecases_test.go:175-182`). ∎

**Coverage of the enumerated boundary conditions** (each is a sub-case of A1–A3,
plus a pinning test):

- `firstAddSeq = 0` / absent rows. A row with `AddSeq = 0` can never satisfy
  Query B (`firstAddSeq > minF`, `minF ≥ 0` ⇒ needs `0 > minF`, impossible). It
  is reachable only via Query A, i.e. only if `OrderId > maxF`. If its OrderId is
  inside the read prefix it is (by A1, since `0 ≤ minF`) dominated and correctly
  *not* returned. This is exactly the intent at `repository.go:445-447` and is
  pinned by `TestGetUnreadMessagesComputed_AddSeqZeroRows`
  (`computed_edgecases_test.go:155-183`). Absent `firstAddSeq` decodes as 0
  (`GetInt`, `repository.go:480`; `chatmodel.go:585`) — same treatment — and is
  the case `BackfillFirstAddSeq` exists to repair (§6, `repository.go:530`).
- Equal coordinates. `Dominated` uses `≤` on both axes, so a row exactly equal to
  a head is dominated (read). Query A is strict `>` so it does not surface a row
  *at* `maxF`; such an in-past row with a higher AddSeq is surfaced by Query B
  instead. `TestGetUnreadMessagesComputed_QueryABoundaryGt`
  (`computed_edgecases_test.go:215-232`) pins both directions and proves Query B
  is load-bearing at the boundary.
- Duplicate / non-maximal heads in `F`. The `max`/`min` reduction
  (`repository.go:494-502`) is invariant to duplicates; and Lemma 0 says
  `Dominated` is invariant to non-maximal heads.
  `TestGetUnreadMessagesComputed_DuplicateFrontierCoords`
  (`computed_edgecases_test.go:118-147`).
- `int64(minF)` encoding. Lemma A2 handles the cast; the near-ceiling test
  exercises `MaxInt64`.
- Own-exclusion and mention sub-filter. A3 + the counter filter; pinned by
  `TestGetUnreadMessagesComputed_OwnMentionExcluded`
  (`computed_edgecases_test.go:238-264`) and
  `..._MentionFrontierIndependentOfMessages` (`:272-294`).

**Empirical corroboration.** The differential gate
`TestGetUnreadMessagesComputed_DifferentialVsBool` and the edge-case suite assert
`computed == bool path == oracle` over fixed message sets and frontier shapes
(`computed_edgecases_test.go:61-71`). A scratch fuzz harness over 20k randomized
message sets × independent message/mention frontiers
(`zz_shortcut_fuzz_scratch_test.go:93-182` — present in the working tree but
*untracked* / not committed; its header marks it transient) asserts the
*contract* reproduction
(`SAFE == REAL` always, `:163-167`) — i.e. given the *real* per-row AddSeq, the
two-query enumeration matches the dominance oracle on every trial. (This same
fuzz also demonstrates the §5(B) phenomenon; see below.) Theorem A is the closed
form of what these tests sample. ∎

### §5.2 — (B) Semantic / order-of-adding invariance

Now fix a **normalized tree**: the DAG and every `OrderId` are fixed (a
deterministic function of the DAG, O1/O2). Fix a frontier **as a set of change
ids** `F₀ ⊆ C`. Vary the local apply order: this varies the `AddSeq` assignment
over the set `T` of all valid topological extensions of `≺` consistent with the
per-batch rule (S1). For each `A ∈ T`, resolve `F₀` to coordinates using `A`,
giving frontier `F(A)`, and let

    Unread(A) = { peer counter-matching X : ¬Dominated( (OrderId(X), A(X)), F(A) ) }.

The question: is `Unread(A)` the same for all `A ∈ T`?

#### The reduction

Because `OrderId` is fixed, the only thing that varies is `A`. For a fixed
candidate `X` and head `H ∈ F₀`, the head's contribution to dominating `X` is the
conjunction

    OrderId(X) ≤ OrderId(H)   ∧   A(X) ≤ A(H).

The first conjunct is fixed across `A`. So **the dominance of `X` by `H` can
change with `A` iff the second conjunct, `A(X) ≤ A(H)`, can change with `A`,**
and this matters only when the first conjunct holds.

Classify the pair `(X, H)` by `≺`:

> **Lemma B1 (causal pairs are order-stable).**
> - If `X ≺ H`: by S1, `A(X) ≤ A(H)` for *every* `A`. Combined with O1
>   (`OrderId(X) < OrderId(H)`), `H` dominates `X` under every `A`.
> - If `H ≺ X`: by S1, `A(H) ≤ A(X)`; and by O1 `OrderId(H) < OrderId(X)`, so
>   `H` does not dominate `X` under any `A` (the OrderId conjunct already fails,
>   strictly).
> - If `X = H`: trivially dominated under every `A`.

*Proof.* Direct from O1 and S1. ∎

> **Lemma B2 (concurrent pairs can flip).** If `X ∥ H`, then by S2 there exist
> `A, A' ∈ T` with `A(X) ≤ A(H)` and `A'(X) > A'(H)`. Hence the conjunct
> `A(X) ≤ A(H)` is *not* invariant. If additionally `OrderId(X) ≤ OrderId(H)`,
> then `H` dominates `X` under `A` but not under `A'` (assuming no *other* head
> dominates `X`).

*Proof.* S2 gives an apply order placing `X`'s batch before `H`'s (so
`A(X) < A(H)`, or `=` if co-batched) and another placing `H`'s strictly before
`X`'s (`A'(H) < A'(X)`). Both are valid topological extensions because `X ∥ H`
imposes no causal constraint between them. The dominance conclusion is then a
direct evaluation of `Dominated`. ∎

#### The verdict

> **Theorem B (exact invariance condition).** `Unread(A)` is invariant over all
> `A ∈ T` **iff** for every candidate `X` and every head `H ∈ F₀` with
> `X ∥ H` and `OrderId(X) ≤ OrderId(H)`, `X` is *causally covered*: there exists
> a head `H' ∈ F₀` with `X ≺ H'` or `X = H'`.
>
> Equivalently — define the **order-stable read set**
>
>     Stable(F₀) = { X : ∃ H' ∈ F₀, X ⪯ H' }   (the causal down-set of F₀, ⪯ = ≺ or =)
>
> Then `Unread(A) = { peer counter-matching X : X ∉ Stable(F₀) }` for *all* `A`
> iff no candidate `X` lies in the "ambiguous band"
>
>     Amb(F₀) = { X ∉ Stable(F₀) : ∃ H ∈ F₀, X ∥ H ∧ OrderId(X) ≤ OrderId(H) }.

*Proof.*

(⇐, sufficiency) Suppose `Amb(F₀) = ∅` for the candidate set. Take any candidate
`X`. Two cases.
- `X ∈ Stable(F₀)`: some `H' ⪯`-covers `X`, i.e. `X ⪯ H'`. By Lemma B1, `H'`
  dominates `X` under *every* `A`. So `X` is read under every `A` — invariant
  (and excluded from `Unread`).
- `X ∉ Stable(F₀)`: for every head `H`, `X ⋠ H`, so either `H ≺ X` (Lemma B1:
  `H` never dominates `X`) or `X ∥ H`. For the concurrent heads, since
  `X ∉ Amb(F₀)`, we must have `OrderId(X) > OrderId(H)` for *all* of them — so
  the OrderId conjunct fails for every head, and `X` is *not* dominated under any
  `A` (the AddSeq axis is irrelevant when the OrderId axis already fails). So `X`
  is unread under every `A` — invariant.
In both cases the membership of `X` in `Unread(A)` does not depend on `A`. Hence
`Unread(A)` is constant, and equals `{peer counter-matching X : X ∉ Stable(F₀)}`
(the `Stable` members are read, the rest unread). ∎

(⇒, necessity) Suppose some candidate `X ∈ Amb(F₀)`: `X ∉ Stable(F₀)`, and there
is a head `H` with `X ∥ H` and `OrderId(X) ≤ OrderId(H)`. By Lemma B2 there are
`A, A'` with `H` dominating `X` under `A` but not `A'`. Could another head rescue
invariance? No: since `X ∉ Stable(F₀)`, *no* head causally covers `X` (Lemma B1's
"always dominates" case is unavailable), and any *other* concurrent head `H₂`
either also flips (same argument) or never dominates `X` (if
`OrderId(X) > OrderId(H₂)`). We can choose `A'` to additionally place `X` after
*all* concurrent heads' batches, so that under `A'` no head dominates `X` (each
concurrent head fails the AddSeq conjunct, each `H ≺ X`-or-`OrderId` head already
fails). Then `X ∈ Unread(A')` but `X ∉ Unread(A)`. So `Unread` is not invariant.
∎

#### Plain-language reading, and the concrete configuration

Theorem B says: **the computed read/unread result is invariant under apply order
exactly for changes that are either (a) causal ancestors of a seen head — always
read — or (b) `OrderId`-after every seen head they are concurrent with — always
unread. The single order-dependent configuration is a message `X` that is
*concurrent* with a seen head `H`, sorts at-or-before it in the convergent order
(`OrderId(X) ≤ OrderId(H)`), and is not causally covered by any other seen
head.** For such an `X`, whether it counts as read depends on the device-local
`AddSeq` relationship between `X` and `H`, which depends on the order in which
that device received them.

This is *exactly* the accepted concurrent-merge divergence:

- `S-concurrent-merge-divergence` (`scenario_catalog_test.go:31-47`): `m1 ∥ x1`,
  both children of `G`; device reads `x1` so `F₀ = {x1}`. `m1 ∉ Stable({x1})`
  (m1 is not an ancestor of x1). If `OrderId(m1) ≤ OrderId(x1)` then
  `m1 ∈ Amb({x1})` and its read-state is order-dependent. The sign-off records
  the watermark result as `m1` read (count 0) vs the DAG-ancestor baseline's
  `m1` unread (count 1) — `watermark_signoff.md:11-13` — and marks it an
  **accepted** divergence (`scenario_signoff_test.go:20-23`).
- `S-cat12-dag-ancestor-vs-orderid-prefix` (`scenario_catalog_test.go:296-317`):
  `early ∥ seen` (both children of `e1`), reading `seen`; `early` is the
  concurrent message that "sorts before the seen head" (the `Intent` text,
  `:316`). Divergence at step 3 (`watermark_signoff.md:98`).
- `S-cat19-msg-mention-concurrent` (`:476-507`): the per-counter analogue
  (divergences at steps 2 and 4, `watermark_signoff.md:151-153`).

These three — and *only* these three — are the catalog's accepted divergences
(`scenario_signoff_test.go:20-23`; `TestComputed_EqualsWatermark`
`scenario_computed_test.go:169-196` proves the band-decomposition computed engine
reproduces the watermark exactly on all of them, so the computed query inherits
the same three).

#### Two important qualifications, proven

1. **The batch-granularity of `AddSeq` shrinks `Amb` but does not empty it.** If a
   device receives `X` and `H` in the *same* `AddAll` batch, then `A(X) = A(H)`
   (S1 equal case), so `A(X) ≤ A(H)` holds and — when `OrderId(X) ≤ OrderId(H)` —
   `H` dominates `X` *on that device*. This is why the scenario harness, which
   adds the whole DAG in one `AddRawChanges` call (`scenario_harness_test.go:174`,
   one `AddAll` ⇒ one `seq`, `storage.go:331`), sees `m1` *read* in
   `S-concurrent-merge-divergence`: co-batching forces `AddSeq(m1) = AddSeq(x1)`,
   collapsing the dominance decision onto the OrderId axis alone. But a *different*
   device that received `x1` in an earlier batch than `m1` (a genuine late in-past
   insert of a concurrent change) has `AddSeq(m1) > AddSeq(x1)` and sees `m1`
   *unread*. So the divergence is real across devices, exactly as Theorem B's
   necessity proof constructs. (Verified: `storage.go:331-333` per-batch; harness
   single batch at `scenario_harness_test.go:174`.)

2. **Empirical confirmation that the AddSeq *value* matters.** The scratch fuzz
   (`zz_shortcut_fuzz_scratch_test.go`, untracked working-tree harness) compares,
   on identical message sets and
   frontiers, two `firstAddSeq` policies that agree on the *read bool* but differ
   on the stored AddSeq: `REAL` (the true AddSeq) vs `SHORTCUT` (zero the AddSeq
   on rows the bool marks read). It *reports* (does not fail on) every divergence
   (`:168-174,177`). The SHORTCUT diverges from REAL on a non-trivial fraction of
   trials — i.e. changing the AddSeq coordinate of a row, holding OrderId and the
   frontier fixed, *changes* the computed unread set. That is the direct
   observable shadow of Theorem B / Lemma B2: dominance is genuinely
   AddSeq-sensitive, hence apply-order-sensitive, in the ambiguous band. (The
   `SAFE` policy, which only zeroes rows read in *every* counter, equals REAL
   always — `:163-167` — which is the §6 statement that zero-AddSeq is safe
   precisely for own / fully-read rows.)

#### Why this is acceptable (the design contract)

The model deliberately chooses **(OrderId, AddSeq) dominance** over **strict
DAG-ancestor** semantics (`scenario_catalog_test.go:9-13` documents that the
*intended* contract is dominance, and the DAG baseline is the foil). The product
meaning of dominance is: "a message is read if it is at-or-before the read point
in the convergent timeline *and* was already present when the user read that
point." For a concurrent message that sorts before the seen head and was *already
present* on the reading device when the head was read (co-batched or earlier),
treating it as read is the desired behavior — the user saw it. For one that
arrived *later* (the late in-past insert), keeping it unread is also desired.
Theorem B shows these are the *only* two outcomes, and that they are stable except
in the genuinely-ambiguous "concurrent and earlier-or-equal-order and not
otherwise covered" band — where the device-local arrival order *is* the product's
ground truth for "was it already there." The non-invariance is faithful encoding of per-device observation **only when the
read and the dominance computation happen on the same device** — a qualification
§5.3 shows fails for a single user's *synced* devices, where this acceptability
argument is materially weaker (arguably wrong) and the divergence becomes a
*permanent* cross-device disagreement on the unread count.

#### §5.3 — Re-examining acceptability for the cross-device (single-user) case

The §5.2 steelman treats each device's answer as a legitimate per-device
observation. That holds when the read and the dominance check happen on the
*same* device. It is materially weaker for a single user whose devices *sync read
state* — the entire purpose of the seen-heads sync.

**Concrete repro** (real any-sync trees, real `AddSeq`; empirically confirmed).
`G; a1 ∥ b1`, both children of `G`, `OrderId(a1) < OrderId(b1)`.
- Device B applies `b1` (AddSeq 1); user reads up to `b1` (synced frontier id-set
  `{b1}`); `a1` syncs in *later* (AddSeq 2). On B: `a1`=(Ord_a1, **2**),
  `b1`=(Ord_b1, **1**).
- Device A applies `a1` (AddSeq 1) then `b1` (AddSeq 2); receives only head-id
  `{b1}`. On A: `a1`=(Ord_a1, **1**), `b1`=(Ord_b1, **2**).
- Both hold `{G,a1,b1}` and frontier `{b1}`:
  - **Device A: `a1` READ** (`1 ≤ 2`) → unread count **0**.
  - **Device B: `a1` UNREAD** (`2 ≤ 1` false) → unread count **1**.
  Permanent: nothing further syncs, the watermark `marked` set never un-sets
  (`readwatermark.go:76`), the device-local AddSeq is stable. No convergence.

**Why this is not benign.** The read happened on device B, where `a1` was
*absent* — the user never saw `a1`. Device A nonetheless reports `a1` read,
derived purely from the order *A* happened to receive `a1` vs `b1` over the
network, which has no bearing on what the user saw on B. The AddSeq axis is meant
to encode "was X already present when the user read the head"; across a *synced*
read it instead encodes "was X already present in *this* device's arrival order,"
and those coincide only when the read happened locally. So the under-counting
device fabricates a read the user never made — the dangerous direction (a real
unread silently shown read).

**Status of the "accepted" label.** The three sign-off divergences are recorded
as watermark-vs-DAG-baseline *on one device* (`watermark_signoff.md:11-13`). The
cross-device manifestation — two of one user's devices disagreeing on the
*count* — is the same root mechanism but a configuration the sign-off gate
**cannot express**: the harness builds one shared tree and loads the whole DAG in
one `AddRawChanges` (one `AddAll` ⇒ one shared AddSeq,
`scenario_harness_test.go:174`), deterministically co-batching concurrent changes
and collapsing the decision onto OrderId. The bug lives entirely in the
per-device-AddSeq dimension the harness collapses — so it is currently
**untested**, and it **contradicts** the design's written claim that cross-device
is "consistent for the same change-set ... divergence only during sync lag"
(`2026-05-16-chat-read-counter-design.md:37`): the divergence is *permanent*, not
sync lag.

**Therefore** this should be treated as an *open product decision* made with the
cross-device framing explicit — not a pre-settled benign divergence. The only
replica-invariant cut is `OrderId`; a convergent fix would demote `AddSeq` to a
guard consulted only for *causally-related* pairs, or handle the
concurrent-late-arrival case with an OrderId-only rule (both larger changes,
flagged not designed). Whatever is decided applies *identically* to the bool
watermark and the computed redesign (`TestComputed_EqualsWatermark`), so it is a
model decision, not a redesign blocker.

---

## §6. Edge-case coverage table

For each case: the mechanism, and the precise condition under which it is
covered. "Covered" = the computed result equals the intended dominance semantics.

| Case | Covered? | Mechanism / precise condition | Evidence |
|---|---|---|---|
| Own messages | Yes, unconditionally | `creator == myIdentity` skipped before the dominance test (`repository.go:515`); own rows are also marked read on insert (`chathandler.go:84-90`). Their `firstAddSeq` may be 0 (live-push, S3) but never reaches the counted set. | `computed_edgecases_test.go:238-264` |
| Pre-feature `firstAddSeq = 0` / absent | Yes, with the §5.1 caveat | Absent decodes to 0 (`repository.go:480`); a 0-AddSeq peer row is reachable only via Query A and is dominated iff its OrderId is in the read prefix. `BackfillFirstAddSeq` (`repository.go:530-570`) stamps real values idempotently so future rows use the real AddSeq axis. Condition: backfill has resolved the change, OR the row's OrderId already decides it. | `computed_edgecases_test.go:155-183`; `repository.go:525-585` |
| Mention vs message counters | Yes, unconditionally | Independent frontiers (separate diff managers, `chatobject.go:286,292`), shared `firstAddSeq` field, distinct counter filter (`hasMention`) `AND`-ed into both queries (`repository.go:463`; `readhandler.go:68-70`). | `computed_edgecases_test.go:272-294`; `..._OwnMentionExcluded` |
| markunread (frontier regression) | Yes | `MarkMessagesAsUnread` re-derives a *smaller* seen set; `InitDiffManager` builds a *fresh* watermark per init (`store.go:218-223`) so the frontier shrinks rather than accumulating; the computed query reads the (smaller) `GetReadFrontier`. Dominance monotonicity (Lemma 0) ⇒ fewer reads. | `store.go:213-241`; catalog `S-cat17`, `S-cat8` (MATCH, `watermark_signoff.md:133-138,63-68`) |
| Offline batch with reordering | Conditional — see §5(B) | Invariant *except* for candidates in `Amb(F₀)` (concurrent with a seen head, `OrderId ≤` it, not causally covered). Those are the accepted divergences. | Theorem B; `scenario_signoff_test.go:20-23` |
| In-past insert (small OrderId, late arrival) | Yes, unconditionally | Caught by Query B (`firstAddSeq > minF`, `repository.go:508`); stays unread because its AddSeq exceeds the head's. This is the *raison d'être* of the AddSeq axis. | `computed_edgecases_test.go:77-112,215-232` |
| Unresolvable / not-yet-local seen head | Yes (eventually) | An id in `F₀` not present locally is deferred to `pending` (`readwatermark.go:54-59`) and *omitted* from the resolved frontier the computed query sees, so it cannot cause false reads; when the change later lands, `updateInDiffManagers` re-advances and re-resolves it (`store.go:405-409`). Condition: the head's change eventually syncs in. | `store.go:374-409`; catalog `S-cat7`, `S-cat16` (MATCH) |
| Snapshot / compaction | Yes (within the model) | The root/snapshot change is skipped by the read model (`store_apply.go:52-55`); message rows persist across snapshots with their `(OrderId, firstAddSeq)`. OrderId of the root is always set (`tree.go:217-220`). Compaction does not re-key existing rows. (Assumption A8: compaction preserves the stored `firstAddSeq` and the OrderId comparison order of surviving rows.) | `store_apply.go:49-57`; `tree.go:274-277` |

---

## §7. Assumptions and boundaries

Numbered; each marked **always** (with the establishing reason) or **conditional**
(with the condition).

1. **A1 — Heads frontier resolves to the read coordinates the rows were stamped
   with.** *Always.* `chathandler.go:106-107` stamps `_o.id`/`firstAddSeq` from
   `ch.Change.Order`/`ch.Change.AddSeq`; `resolvePair` reads the same
   `(OrderId, AddSeq)` from tree storage (`store.go:115-121`). Same source ⇒ one
   coordinate space. (The scenario harness reproduces this by stamping
   `firstAddSeq` from the same tree the frontier resolves against,
   `scenario_harness_test.go:390-396`.)

2. **A2 — `OrderId` is a linear extension of `≺` (Property O1).** *Always.*
   Construction in `updateHeads`/traversal (`tree.go:441-472`,
   `treeiterator.go`), checked by `scenario_harness_test.go:121-123`.

3. **A3 — `OrderId` comparisons converge across replicas (Property O2).**
   *Always, conditional on A7.* Follows from the sibling tiebreak being the
   replica-invariant change Id (`tree.go:295-308`) and the lexid order being
   determined by the traversal. The *value*-level convergence of `NextBefore`
   gap-fills is an any-sync lexid contract (A7).

4. **A4 — `AddSeq` is a per-device linear extension of `≺` (Property S1),
   constant within a batch, strictly increasing across batches.** *Always.*
   Per-batch counter `storage.go:297-299,331-333`; attach-after-parents
   `tree.go:312-320`.

5. **A5 — A peer (counted) message always carries a *real* `AddSeq` on its row.**
   *Always.* Peer messages enter only via replay (`store_apply.go:65`), never via
   the live-push validator. The only `firstAddSeq = 0` peer rows are *pre-feature*
   rows, handled by A6/backfill (§6) — and even then the OrderId axis covers them
   when they are in the read prefix.

6. **A6 — `minF` and every counted row's `AddSeq` fit in `int64`** (so
   `int64(minF)` in Query B and the `GetInt` decode are exact). *Conditional but
   effectively always.* `AddSeq` is a `uint64` apply counter; reaching `2^63`
   batches is not physically attainable. The boundary is exercised at `MaxInt64`
   (`computed_edgecases_test.go:78`). A pre-feature `firstAddSeq = 0` is the only
   value below the real range and is handled separately.

7. **A7 — lexid `Next`/`NextBefore` produce strictly-ordered strings consistent
   across replicas given the same neighbor order.** *Assumed (any-sync layer).*
   This is the foundation of O1/O2 at the *string-value* level. It lives in
   `github.com/anyproto/lexid` and `objecttree` (`lexId.Next`, `tree.go:469`;
   `lexId.NextBefore`, `tree.go:450`). Not proven here; would be verified in the
   `lexid` package and `objecttree` order tests. If A7 failed, O1/O2 (and hence
   all of §5) would not hold.

8. **A8 — Snapshot/compaction preserves surviving rows' `firstAddSeq` and the
   relative `OrderId` order.** *Conditional.* The read model assumes a message
   row's `(OrderId, firstAddSeq)` is stable once written and that compaction does
   not reorder survivors. Verified only indirectly (catalog restart cases
   `S-cat11`, `S-cat23` MATCH); a dedicated compaction test is not in the cited
   suite.

9. **A9 — The frontier passed to `GetUnreadMessagesComputed` is the *resolved,
   pruned* seen frontier** (`GetReadFrontier`, `store.go:442-452`). *Always, by
   wiring.* Lemma 0 makes the *pruned-ness* immaterial to correctness; it only
   bounds size.

10. **A10 — Production status: the computed query runs in *shadow* mode, not yet
    authoritative.** *Boundary, not an assumption used in the proof.*
    `shadowComputedReadCountEnabled = false` by default
    (`chatobject.go:228`); when enabled, `shadowComputedReadCount`
    (`chatobject.go:234-262`) runs the computed enumeration alongside the bool
    path and logs count divergences. The bool `read`/`mentionRead` remains the
    live source of truth until rollout. So Theorem A's *exactness* and Theorem
    B's *order-(non)invariance* are properties of the predicate the shadow
    computes and the watermark engine already ships
    (`readwatermark.go`); they characterize the behavior the system will adopt
    when the flag flips, and they match the shipped watermark counter (proven
    equal by `TestComputed_EqualsWatermark`, `scenario_computed_test.go:169`).

11. **A11 — `≺` is the same on all replicas (a global causal order exists).**
    *Always.* The DAG is content-addressed and append-only; `PreviousIds` are
    immutable parts of a signed change. Two replicas holding a change hold the
    same parents. (Foundational CRDT property; relied on throughout §5(B).)

---

## §8. Summary of the verdict

- **(A) Algorithmic coverage:** `GetUnreadMessagesComputed` computes *exactly*
  `{peer, counter-matching X : ¬Dominated(X, F)}` — Theorem A — for any frontier,
  including the empty frontier, `AddSeq = 0`/absent rows, equal coordinates,
  duplicate heads, the `int64` boundary, own-exclusion, and the mention
  sub-filter. This holds unconditionally (given A1, A6) and is corroborated by the
  differential/edge/fuzz tests.

- **(B) Order-of-adding invariance:** *Not* unconditional. On a fixed normalized
  tree with a fixed frontier-as-id-set `F₀`, the computed unread set is invariant
  over all valid local apply orders **iff** no candidate lies in the ambiguous
  band `Amb(F₀)` = {concurrent with some head `H∈F₀`, `OrderId ≤ OrderId(H)`, and
  not causally covered by any head} (Theorem B). That band is exactly the
  concurrent-message-sorting-before-a-seen-head configuration, i.e. the three
  catalog cases `S-concurrent-merge-divergence`, `S-cat12`, `S-cat19` accepted by
  the sign-off gate. Everything *outside* the band — causal ancestors of a head
  (always read) and OrderId-after every concurrent head (always unread) — is
  fully order-invariant. The math is bounded to that single configuration; whether
  it is a *defect* is an open product decision, not a settled fact. §5.3 shows the
  cross-device manifestation — two of one user's synced devices reporting different
  unread *counts*, the under-counting device fabricating a read from its own
  irrelevant local arrival order — is materially more serious than the
  single-device "accepted divergence" framing, is undocumented and untested (the
  sign-off harness cannot express per-device AddSeq), and contradicts design §3
  l.37. Accept-and-document vs. fix-toward-OrderId-convergence is unresolved. It
  applies identically to the bool watermark and the computed query, so it is a
  model decision, not a redesign blocker.
