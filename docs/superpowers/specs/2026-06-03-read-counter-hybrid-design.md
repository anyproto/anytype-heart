# Option B — Hybrid Convergent Read Counter: Design & Bounded-Walk Proof

Status: design proposal. Companion to
`2026-06-03-chat-read-counter-computed-theory.md` (theory, Theorems A/B, §5.3
cross-device defect) and `2026-06-03-read-counter-formal-problem.md` (abstract
problem). This document specifies the recommended fix and proves the one subtle
part — the bounded fallback walk — including a correction to an earlier,
incorrect band-detection rule.

---

## 1. Problem recap (one paragraph)

The shipped read counter decides read/unread by **dominance** on two stored
per-message scalars: `read_d(X) ⟺ ∃H∈F: g(X) ≤ g(H) ∧ a(X) ≤ a(H)`, where
`g` = `OrderId` (a replica-invariant linear extension of the causal order `≺`) and
`a` = `AddSeq` (a **device-local** linear extension). It is cheap (two indexed
range scans) but, because `a` is device-local, it **permanently diverges across a
user's devices** on the "ambiguous band" — concurrent messages that sort before a
read head (theory §5.3; the canonical `a1∥b1` case). The pre-regression algorithm
computed the **convergent** ground truth `read*(X) ⟺ ∃H∈F: X ⪯ H` (causal
down-set of the frontier) by walking the whole change tree — correct but O(tree)
on every cold start (~18.6s Android). **Option B keeps the cheap dominance fast
path and repairs only the band, with a bounded causal walk — recovering exact,
convergent `read*` without the whole-tree cost.**

Notation: `g(·)`,`a(·)` as above; `F` = frontier (seen heads, synced as IDs,
resolved locally); `Stable(F) = {X : ∃H∈F, X ⪯ H}` (causal down-set = the truly
read set, `read*`); `maxF = max_{H∈F} g(H)`; `minF = min_{H∈F} a(H)`. "Counted" =
peer (`creator ≠ me`) and counter-matching (messages: all; mentions: `hasMention`).
Both `g` and `a` extend `≺` (so `X ⪯ Y ⟹ g(X) ≤ g(Y) ∧ a(X) ≤ a(Y)`); `g` is a
total order and injective on distinct events.

---

## 2. The semantic decision (must be made explicitly)

Option B computes **exact `read*` = causal-down-set semantics**. This is the
convergent, pre-regression behavior, and it **eliminates** the three currently
"accepted" single-device divergences (`S-concurrent-merge`, `S-cat12`, `S-cat19`)
by making them MATCH the DAG baseline again. It **reverts** the design choice in
`2026-05-16-chat-read-counter-design.md` §5 (dominance / "a concurrent message
already present and sorting before the read head counts as read"). That choice is
*intrinsically device-local* ("already present" depends on which device read), so
it cannot be made convergent — adopting `read*` is the only convergent option.
**Action required:** bless the semantic change and empty the sign-off gate's
`watermarkAcceptedDivergence` map (`scenario_signoff_test.go:21-23`), flipping the
three scenarios to MATCH.

---

## 3. Correctness foundation

All proofs use only: `g,a` extend `≺`; `g` total & injective; `Stable(F)` is the
causal down-set.

**Lemma 1 (read ⇒ in the g-prefix).** `Stable(F) ⊆ {X : g(X) ≤ maxF}`.
*Proof.* `X ∈ Stable(F) ⟹ X ⪯ H ⟹ g(X) ≤ g(H) ≤ maxF`. ∎

**Lemma 2 (the g-tail is cleanly unread).** `g(X) > maxF ⟹ X ∉ Stable(F)`.
*Proof.* If `X ∈ Stable(F)` then by Lemma 1 `g(X) ≤ maxF`, contradiction. ∎
→ Every counted `X` with `g(X) > maxF` is unread. This is **Query A**, exact, no walk.

**Theorem 1 (band identity).** Define `Band(F) = {X : g(X) ≤ maxF ∧ X ∉ Stable(F)}`.
Then `Band(F)` equals the theory's ambiguous band `Amb(F)`, and **every** member is
truly unread.
*Proof.* Unread is immediate (`X ∉ Stable(F) ⟹ ¬read*(X)`). For `X ∈ Band(F)`, let
`H*` achieve `g(H*) = maxF`. `X ∉ Stable(F) ⟹ X ⋠ H*`. If `H* ≺ X` then
`g(H*) < g(X) ≤ maxF = g(H*)`, contradiction; so `H* ⊀ X`. Hence `X ∥ H*`, and
`g(X) ≤ g(H*)`, i.e. `X ∈ Amb(F)`. Conversely `Amb(F) ⊆ Band(F)` (an `Amb` member
is `∉ Stable` and has `g(X) ≤ g(H) ≤ maxF`). ∎

**Corollary 1 (exact decomposition).**
`{counted X : ¬read*(X)} = {counted X : g(X) > maxF} ⊎ {counted X : X ∈ Band(F)}`
— Query A (cheap) disjoint-union the band. So computing the band exactly gives the
exact, convergent count.

**Lemma 3 (the fast path never over-reads unread).** `¬read_d(X) ⟹ ¬read*(X)`.
*Proof.* If `read*(X)`, then `X ⪯ H` for some `H`, so `g(X) ≤ g(H)` and
`a(X) ≤ a(H)` (both extend `≺`), hence `read_d(X)`. Contrapositive gives the claim. ∎
→ The fast path's unread set is a **subset** of the true unread set: it can only
*miss* unread items (the band members it marks read), never invent them. The
missing items are exactly `Band(F) ∩ {read_d = read}`.

**Lemma 4 (unread is descendant-closed).** If `X ∉ Stable(F)` and `X ⪯ Y` then
`Y ∉ Stable(F)`.
*Proof.* If `Y ∈ Stable(F)`, `Y ⪯ H`, then `X ⪯ Y ⪯ H ⟹ X ∈ Stable(F)`,
contradiction. ∎ → The unread set is an **up-set** (order filter); its minimal
elements ("unread roots") have **all parents read**.

---

## 4. Why the earlier `a`-based band rule is wrong (correction)

A natural-looking shortcut (proposed in the mapping review) is to find the band by
the index probe `{g(X) ≤ maxF ∧ a(X) > minF}` and resolve only those. **This is
incorrect**: it misses band members with `a(X) ≤ minF`.

**Counterexample — the canonical bug itself.** Device A: `G; a1 ∥ b1`,
`g(a1) < g(b1)`, applied in order `a1`(a=1) then `b1`(a=2); `F = {b1}` so
`maxF = g(b1)`, `minF = a(b1) = 2`. Here `a1 ∈ Band(F)` (concurrent with `b1`,
`g(a1) < g(b1)`, `a1 ∉ Stable`), and it is the very message that is wrongly shown
read on device A. But `a(a1) = 1 ≤ 2 = minF`, so `a1 ∉ {a > minF}` — the probe
**skips the one event that needs fixing.** (Root cause: a band member can have
*arrived early* locally; the `a`-axis carries no convergent signal here — that is
the whole reason the dominance scheme fails.)

**Consequence.** The band cannot be found by any `a`-axis index range. It must be
found by **causal structure** (`g` + parent pointers). Theorem 1 gives the correct
target: `Band(F) = {g ≤ maxF} \ Stable(F)`.

---

## 5. The bounded fallback walk

Computing `Band(F) = {g ≤ maxF} \ Stable(F)` naively (enumerate `Stable(F)` and
subtract) is O(|read history|) — the cost we must avoid. We bound it with one
structural property of these trees.

**Property M (merge-recency).** Every new change links **all** current heads as
parents (`objecttree.go:378`, `TreeHeadIds: ot.tree.Heads()`). So any concurrent
fork is merged by the next change created after both branches are visible;
multiple heads exist only transiently, among changes created before they saw each
other. Formally: if `P` is the common ancestor at a fork and a later change `M` is
created with all heads visible, then both fork branches `⪯ M`.

**Lemma 5 (band members are shallow — directly above the read boundary).** Every
minimal element `X` of `Band(F)` (an "unread root") has **all parents in
`Stable(F)`**.
*Proof.* By Lemma 4 (up-set), the minimal unread elements have all parents read. A
minimal `Band(F)` element is a minimal unread element with `g ≤ maxF`; its parents
are read, i.e. in `Stable(F)`. ∎ → Unread roots hang **directly off** the read
region (a read parent, an unread child). They are the fork points where the user's
read history diverges from branches `F` does not cover. Under Property M these are
located at the recent, not-yet-merged-into-`F` tips.

**Algorithm (band enumeration).** Inputs: `F` (resolved), the local change store
(point lookups `GetChange(id) → {PreviousIds, g, a}`), the local heads `Hloc`.

```
1. Seed a backward traversal from  S = Hloc ∪ F,  processed in DECREASING g order
   (a max-heap keyed by g). Maintain two marks per visited event: READ, UNREAD.
   - every H ∈ F (and every ancestor we reach via an F-marked node) is READ;
   - every local head not (yet) shown READ starts UNREAD.
2. Pop the event E with the largest g.
   - If E is marked READ: mark all of E's parents READ; do NOT enqueue them for the
     UNREAD logic (their whole past is read — Stable is a down-set). [Pruning P1]
   - If E is marked UNREAD:
       * if g(E) ≤ maxF and E is counted, EMIT E  (it is a band member);
       * for each parent Pi: enqueue Pi. Its mark is READ if Pi ⪯ some F-head
         (i.e. Pi is reached on an F-seeded branch), else UNREAD.
3. Termination: stop when the heap is empty OR when every remaining event is READ
   (all active branches have converged into the read region). [Pruning P2]
4. Output:  unread = QueryA( g > maxF, counted )  ∪  EMITTED band.
```

The READ/UNREAD determination in step 2 is the co-traversal: an event is READ iff
it is reachable backward from `F`. Seeding the *same* traversal from `F` marks the
read down-set lazily; a branch from `Hloc` becomes READ the moment it meets an
`F`-marked event. Both seeds advance in decreasing `g`, so they meet at the recent
convergence point.

**Theorem 2 (correctness).** The algorithm emits exactly `Band(F) ∩ counted`, hence
(with Query A) computes exactly `{counted X : ¬read*(X)}`.
*Proof.* (Soundness) An emitted `E` is UNREAD-marked, meaning no `F`-seeded branch
reached it, i.e. `E` is not backward-reachable from `F` ⟺ `E ∉ Stable(F)`; with the
emit guard `g(E) ≤ maxF`, `E ∈ Band(F)` (Theorem 1), and `E` is counted. (Complete-
ness) Take any `B ∈ Band(F) ∩ counted`. By Lemma 4, `B` lies on an up-set whose
top elements are local heads (`B ⪯` some head, and that head is unread by Lemma 4),
so `B` is reachable from `Hloc` along an all-UNREAD descending path; the traversal
from `Hloc` descends through UNREAD events (step 2, UNREAD branch enqueues parents)
and reaches `B` before any pruning, because pruning P1 only stops at READ events and
`B`'s path down from the head is all UNREAD (Lemma 4), and P2 only fires once all
remaining are READ. Since `g(B) ≤ maxF` and `B` counted, `B` is emitted. ∎

**Theorem 3 (bound).** The traversal visits only events in the **unmerged suffix**
`U = {E : E is not ⪯-below the deepest fully-merged read event}`. Equivalently it
visits `Band(F)` ∪ (their read-parent boundary) ∪ (the `g>maxF` unread tail
reachable from `Hloc`). It never visits an event all of whose `g` is below the read
convergence point. Under Property M, `|U| = O(W · D)` where `W` = peak concurrent
width and `D` = depth from `F`'s heads to the convergence — both small for chat-like
histories; in particular `U = ∅`-of-band when the history is linear at the frontier
(`Band(F) = ∅`, fast path already exact).
*Proof sketch.* Pruning P1 stops every branch at the first READ event, and `Stable`
is a down-set, so no proper ancestor of a READ event is ever expanded for the UNREAD
logic; the only READ events touched are the immediate parents of UNREAD events (the
boundary). Thus visited = UNREAD events reachable from `Hloc` (= `Band(F)` ∪ the
`g>maxF` unread tail, by Lemma 4) ∪ their parents. The UNREAD events are, by
Property M and Lemma 5, confined to the recent unmerged suffix above `F`. ∎

**Key contrast with the old cost.** The old `RemoveBefore(F)` + `BuildHistoryTree`
visited the **whole** down-set / whole tree (O(read history)). This walk visits
only the **unread up-set's recent root region** — it *stops at* the read boundary
instead of traversing into it. Deep read history is never loaded.

**Honest caveats.**
- The tight bound **depends on Property M**. A pathological history with sustained
  wide concurrency that never merges (many long-lived parallel branches) inflates
  `U`. Property M makes that transient in practice, but the worst case is real —
  hence the **measurement** in §8 before committing.
- "Local heads" and parent pointers must be reachable by **point lookups** without
  building the whole tree. `GetChange(id)` provides `PreviousIds` and the stored
  `g`/`a` per change (point read from storage), and the current heads are known to
  the open tree. The walk therefore costs `O(|U|)` point reads, not a tree rebuild.
- Reuse: the descending parent-pointer traversal is exactly `changediffer.dfsPrev`
  mechanics; this can reuse that code seeded from `Hloc ∪ F` and bounded by P1/P2,
  rather than feeding it the whole tree.

---

## 6. Architecture & integration

**Per-message stored state: unchanged.** Keep `g` (`OrderId`) and `a`
(`firstAddSeq`/`AddSeq`) and their indexes. No new immutable label, no migration
beyond the existing `firstAddSeq` backfill. No mutable read flag is introduced (the
bool, if kept transitionally, is a shadow oracle only).

**Read path = fast path ⊕ band repair.** Both the bool-watermark path
(`readwatermark.go` `dominated` / `onRemove`) and the computed query
(`repository.go` `GetUnreadMessagesComputed`) consume the same dominance predicate;
both get the same wrapper:

```
unread_exact = QueryA(g > maxF, counted)                       // Lemma 2, cheap
             ∪ bandWalk(F, Hloc)                               // §5, bounded
read_exact(X) = ¬(X ∈ unread_exact)   // for per-message status / events
```

- **Count / enumeration:** `|unread_exact|` and its id-set come straight from the
  decomposition (Corollary 1). The band walk runs only when its cheap precheck says
  the band might be non-empty (see below); otherwise the fast path is returned as-is.
- **Cheap "band possibly non-empty?" precheck (avoids the walk on linear
  histories):** the band is non-empty only if the local tree has a head `h ∈ Hloc`
  with `h ∉ Stable(F)` and `g(h) ≤ maxF`, OR more simply if `|Hloc| > |F-heads
  covering them|`. Concretely: if every local head is `⪯`-covered by `F` (single
  merged head that `F` already contains/covers), `Band(F) = ∅` and we skip the walk.
  This is an O(|Hloc|) check. (Linear, fully-read-to-tip chats hit this and pay
  nothing beyond the fast path.)
- **Transition-delta / events:** when `F` advances `F_old → F_new`, the newly-read
  set is `Stable(F_new) \ Stable(F_old)`; the band walk already classifies the
  affected (recent) region, so read/unread *events* are derived from the
  before/after band+QueryA membership of the touched ids — no per-message flag flip.

**All three counters.** Messages and mentions share `g/a` with a `hasMention`
filter, so the wrapper applies to both unchanged. Reactions (mid-redesign) use the
same dominance shape and inherit the same band issue; the same wrapper applies once
their coordinates are in place.

**Failure direction (preserved-safe).** A frontier head whose change is not yet
local is omitted from `F` (existing `pending` behavior, `readwatermark.go:54-59`),
shrinking `Stable(F)` ⟹ **over-counting unread**, never hiding an unread. The band
walk inherits this (an unresolved head contributes no READ marks).

---

## 7. Tests

1. **Cross-device convergence gate (currently inexpressible).** Extend the scenario
   harness so each device has its **own tree with its own `AddSeq` space** (today it
   builds one shared tree in one `AddRawChanges`, collapsing the dimension —
   `scenario_harness_test.go:174,357`). New test `TestHybrid_CrossDeviceConvergence`:
   build device A apply-order `[a1],[b1]` and device B `[b1],[a1]`, `F={b1}`; assert
   **equal** unread counts (today the dominance path gives 0 vs 1; hybrid gives 1 vs
   1).
2. **Band-rule regression guard.** Pin the §4 counterexample: assert the hybrid
   emits `a1` as unread on device A, and that the `{a > minF}` rule would not
   (documents why the corrected rule is needed).
3. **Equivalence to ground truth.** Property test over random DAGs + frontiers +
   apply orders: assert `hybrid == old changediffer.RemoveBefore` (exact `read*`)
   on every replica, and that both are apply-order-invariant. This is the convergence
   proof in code.
4. **Bound test.** Assert the band walk visits `O(|U|)` changes (instrument the
   point-lookup count) and `0` beyond the fast path when the history is linear to the
   tip (`Band(F)=∅` precheck short-circuits).
5. **Sign-off flip.** `S-concurrent-merge`/`S-cat12`/`S-cat19` move from
   `watermarkAcceptedDivergence` to MATCH.

---

## 8. Cost & the decisive measurement

- **Storage:** unchanged (reuse `g`,`a`). **Migration:** none beyond existing
  backfill. **Blast radius:** anytype-heart only (no any-sync change).
- **Query:** fast path (2 range scans) + band walk `O(|U|)` point reads, gated by
  the O(|Hloc|) precheck → `0` extra for linear histories.
- **Decisive measurement before committing** (de-risks Property M / Theorem 3):
  instrument real DBs for the **band/unmerged-suffix size per query** — proxy:
  `|Hloc|` distribution and `count(counted, g ≤ maxF, not ⪯ F)` per cold start /
  sync tick. If big group chats sustain large `|U|`, the walk degrades toward the
  old cost and the long-term answer becomes a **producing-device key + per-device
  seq** (true version vector, `W = #devices`, bounded) going forward, paired with
  this hybrid for legacy. If `|U|` is small (expected from Property M), the hybrid
  is sufficient on its own.

---

## 9. Open questions / risks

- **R1 — Property M strength.** The bound relies on forks merging promptly. Confirm
  no chat workflow produces long-lived unmerged parallel branches; the §8
  measurement settles it. (Correctness does NOT depend on M — only the cost does;
  Theorem 2 holds regardless.)
- **R2 — READ-membership co-traversal.** The cleanest implementation marks `Stable`
  lazily by co-seeding from `F`. Verify the meet-in-the-middle terminates at the
  convergence point and that `g`-ordered processing is the right discipline (it is:
  processing by decreasing `g` guarantees a node is seen after all its descendants
  on the active branches, so READ marks propagate down before we decide UNREAD).
- **R3 — Semantic sign-off.** §2 reverts a deliberate product decision; needs an
  explicit bless + the sign-off gate update. Not a silent change.
- **R4 — Reactions interplay.** The reactions redesign (single-axis AddSeq,
  `ClearUnreadReactions` over-clear) must adopt the same wrapper; coordinate so the
  fix lands once at the model layer.
- **R5 — Heads availability cheaply.** Confirm `Hloc` and per-change `PreviousIds`
  are retrievable by point lookups at the moment the count is computed (cold start,
  before/while the tree lazy-loads) without forcing a full tree build.

---

## Appendix — proof obligations summary (for review)

| Claim | Statement | Status |
|---|---|---|
| Lemma 1 | `Stable(F) ⊆ {g ≤ maxF}` | proved |
| Lemma 2 | `g > maxF ⟹ unread` (Query A exact) | proved |
| Theorem 1 | `Band(F) = {g≤maxF}\Stable = Amb(F)`, all unread | proved |
| Corollary 1 | exact unread = QueryA ⊎ Band | proved |
| Lemma 3 | fast path has no false-unreads | proved |
| Lemma 4 | unread is descendant-closed (up-set) | proved |
| §4 | `{a > minF}` band rule is wrong (counterexample = `a1`) | proved |
| Lemma 5 | band roots hang directly off the read region | proved |
| Theorem 2 | bounded walk emits exactly `Band(F)∩counted` | proved (review R2) |
| Theorem 3 | walk bounded by unmerged suffix `|U|` | proved under Property M |
