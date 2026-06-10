# Option D — Causal Ordinals: Convergent O(1) Read Counters Without Read Flags

Status: **implementation spec** (supersedes `2026-06-03-read-counter-hybrid-design.md`
as the product design; reuses its §5 walk machinery internally).
Companions: `2026-06-03-chat-read-counter-computed-theory.md` (Theorems A/B, §5.3
cross-device defect), `2026-06-03-read-counter-formal-problem.md` (abstract problem),
adversarial verification report (agent af41306, 2026-06-03 session).

**Base note (2026-06-10).** This spec is implemented on branch
`go-7290-chat-read-counter-causal-ordinals`, cut from `develop` @ `2f913ae15`.
develop still runs the **old DiffManager** read counter (the `(OrderId, AddSeq)`
watermark of PR #3154 was never merged; `readwatermark.go` is absent from
develop). Consequences for this spec:
- **D2 is a no-op user-visibly on this base**: develop already serves exact
  `read*` (DAG-ancestor) semantics, so Option D changes no user-facing behavior —
  it removes the O(tree) cold-start rebuild (~18.6 s Android) and the
  `SetReadFlag` write amplification while *keeping* the semantics. There is no
  accepted-divergence map to empty here; that paragraph applies only if the
  watermark branch ever lands first.
- §6's "Removed" list maps on this base to the **old-approach** surfaces: the
  per-message `read`/`mentionRead` bools + `SetReadFlag`, and the
  `objecttree.DiffManager` rebuild path (`NewDiffManager`/`BuildHistoryTree` at
  init). The watermark/computed surfaces named in §6 exist only on the
  prototype branches (`go-7290-chat-read-counter`,
  `go-7290-chat-read-counter-computed`), which remain as reference material —
  their scenario catalog, harness, and measurement tooling are carried over or
  ported as §11 prescribes.

Decisions this spec rests on:
- **D1 (product, 2026-06-10):** snapshots will NOT be used for CRDT chat trees.
  Verified current reality: chat store changes never set `IsSnapshot`
  (`store.go:319-324`; only the genesis root is a snapshot, `change.go:55-61`),
  any-sync v0.12.4 has no snapshot policy/compaction for object trees (only
  whole-tree delete, `storage.go:351-362`; `reduceTree` is in-memory only,
  `treereduce.go:31-32`), and a new device materializes **full history** locally
  (`objecttree.go:820-849`, `loaditerator.go:106-125`). §10 turns D1 into a
  guarded contract.
- **D2 (product):** adopt **exact causal semantics** `read*(X) ⟺ ∃H∈F: X ⪯ H` —
  the old convergent DiffManager semantics. This eliminates the three accepted
  single-device divergences (`S-concurrent-merge`, `S-cat12`, `S-cat19` flip to
  MATCH) and the permanent cross-device count divergence (theory §5.3). The
  sign-off gate's `watermarkAcceptedDivergence` map is emptied.
- **D3:** the mutable per-message `read`/`mentionRead` bools and `SetReadFlag`
  are removed; read status is computed. `AddSeq` remains only as the replay
  cursor; it is no longer a read-model axis.

Notation (as in the companion docs): `≺`/`⪯` causal order; `X ∥ Y` concurrent;
`g(X)` = OrderId (replica-invariant order); `F` = seen-head frontier (synced as a
change-id set, resolved locally; unresolved heads pending); `maxF = max_{H∈F} g(H)`
over resolved heads; `Stable(F) = {X : ∃H∈F, X ⪯ H}`; "counted" = peer
(`creator ≠ me`) message matching the counter (messages: all; mentions:
`hasMention`); `Hloc` = current local tree heads.

---

## 1. The design in one page

Two layers. The CORE is self-sufficient (exact + convergent on its own); LABELS
make it O(1) on the hot paths.

**CORE — band-set decomposition.** For each counter, maintain one small
device-local cached object per frontier:

```
bandSet(F) = { live counted messages m : g(m) ≤ maxF ∧ m ∉ Stable(F) }
```

— the (usually empty) set of unread messages *below* the frontier cut. Then:

```
unreadCount = COUNT(live counted, g > maxF)   [existing _o.id index]  +  |bandSet(F)|
unreadIds   = RANGE(live counted, g > maxF)                           ∪  bandSet(F)
status(X)   = g(X) > maxF ? UNREAD : (X ∈ bandSet(F) ? UNREAD : READ)
```

`bandSet` is recomputed by a bounded walk **only when F changes** (a user read /
markunread — rare), is maintained **O(1) per arriving change** in between
(Theorem 3 below), and is persisted, so queries — including cold start — never
walk and never open the tree.

**LABELS — causal ordinals.** Per change `X`, immutable, locally computed,
never synced:

```
pastMsg(X)     = |strict-past(X) ∩ counted message-creates|     (message ordinal)
pastMention(X) = |strict-past(X) ∩ my-mention creates|
pastSize(X)    = |strict-past(X)|
```

Any function of the causal past is replica-invariant and immutable by
construction (the past is content-addressed and append-only); per-account
functions (`creator ≠ me`, mentions-of-me) are valid because convergence (P2) is
only required across **one account's** devices, which compute the same function
of the same DAG. Labels serve three jobs: the **zero-band certificate** (skip
the walk entirely — the common case), the walk's **termination bound**, and
**O(1) single-head counts** in the no-delete case.

**Why this is exact and convergent.** `read*` and every quantity above is a
function of (DAG, F-as-id-set) and the account — no device-local term (`AddSeq`
appears nowhere). The count identity is Theorem 1; the O(1) maintenance is
Theorem 3. The canonical cross-device case (`a1 ∥ b1`, `F={b1}`): `a1` is in
`bandSet` on **every** device → unread = 1 everywhere (the watermark gave 0 vs 1
permanently).

---

## 2. Correctness foundation

All statements assume parent-first attachment (verified: a change attaches only
when its full causal past is attached — `tree.go:248-275`, waitList
`tree.go:239,258-263,313-325`) and OrderId stability of stored changes (the
existing system-wide assumption, theory §7 A8).

**Theorem 1 (count identity, delete-proof).** With the live message collection
as the universe `U` (hard deletes remove rows — `state.go:323-324`),

```
|{m ∈ U : ¬read*(m)}| = |{m ∈ U : g(m) > maxF}| + |bandSet(F)|.
```

*Proof.* Split `U` by `g ≤ maxF` vs `g > maxF`. For `g(m) > maxF`: if
`m ∈ Stable(F)` then `m ⪯ H` for some head, so `g(m) ≤ g(H) ≤ maxF` (g extends
≺) — contradiction; hence every such `m` is unread (hybrid doc Lemma 2). For
`g(m) ≤ maxF`: unread ⟺ `m ∉ Stable(F)` ⟺ `m ∈ bandSet(F)` by definition. The
two parts are disjoint and exhaust `U`. Deletes are handled by construction:
both terms range over the live collection / live set. ∎

**Theorem 2 (zero-band certificate, sound under deletes).** Let `H*` be the
resolved head with `g(H*) = maxF`. Define over the **create universe** (label
sidecar rows, which deletes never remove):

```
bandCreated(H*) = createdRank(maxF) − pastMsg(H*) − [H* is a counted create]
```

where `createdRank(c)` = number of counted creates with `g ≤ c` (indexed count
on the sidecar). Then `bandSet(F) ⊆ band_created(F) ⊆ band_created(H*)`, and
`|band_created(H*)| = bandCreated(H*)`. Hence **`bandCreated(H*) = 0 ⟹
bandSet(F) = ∅`** — and the walk is skipped.

*Proof.* `Stable(F) ⊇ Stable({H*})` ⟹ band(F) ⊆ band({H*}) (more heads only
cover more). Over creates, `band({H*}) = {creates : g ≤ maxF} ∖ (past(H*) ∪
{H*})`, whose size is `createdRank − pastMsg − [H*]` because
`past(H*) ∪ {H*} ⊆ {g ≤ maxF}` (Lemma 1) and `pastMsg` counts exactly the
counted creates in the strict past. Deletes only shrink the live band relative
to the created band. ∎

The certificate is an exact count when no below-cut creates were deleted, and a
sound upper bound always — which is all the fast path needs. It is also the
walk's stop condition: terminate after emitting `bandCreated(H*)` members or
when the boundary closes, whichever first.

**Theorem 3 (O(1) incremental maintenance).** A change `X` newly attached to
the local tree is never an ancestor of an already-resolved frontier head.
Therefore the `bandSet` update rule on attach is, without any ancestry check:

```
if X is a counted live message and g(X) ≤ maxF:  bandSet += X    (in-past insert)
if g(X) > maxF:                                  no action       (tail covers it)
```

*Proof.* Parent-first attachment: when head `H` attached, every ancestor of `H`
was already attached. `X` attaches after `H`, so `X ⋠ H`. This holds for every
resolved head; pending heads contribute nothing to `Stable` until they attach,
and their attachment triggers a frontier re-resolve (§5). ∎

This is also where the late in-past insert — the entire reason `AddSeq` existed —
is handled *by construction*: the late arrival joins `bandSet` (stays unread,
convergently), and **no existing label or cache entry changes**.

**Theorem 4 (fold well-definedness / apply-order invariance).** Labels computed
by the fold of §3 are identical on every replica of the account, regardless of
batch partitioning or arrival order. *Proof sketch.* The fold consumes only
`(parents' labels, parents' counted-flags)`; parent-first attachment guarantees
parents are labeled first; by induction every label equals the closed-form
`|strict-past ∩ ...|`, a pure function of the content-addressed DAG + account.
The replay stream is OrderId-sorted (`GetAfterAddSeq(...).Sort(OrderKey)`,
`storage.go:259-281`), which is parent-first since `g` extends `≺`. ∎ (Property
test in §11 pins this with randomized batch splits.)

---

## 3. The label fold

Computed at apply time for **every change** in the chat tree (messages, edits,
deletes, reactions, system) — required because persisted seen heads can be
arbitrary changes: `collectSeenHeads` walks all change types
(`chatobject.go:802-821`), `MarkMessagesAsUnread` re-seeds from them
(`reading.go:90-101`), and `readStoreTreeHook` marks `headsBeforeJoin`
(`reading.go:218-232`).

```
counted(P)  = 1 iff P is a peer message-create for the counter (msg / my-mention)

root:        pastMsg = pastMention = pastSize = 0
k = 1 (P):   pastMsg(X) = pastMsg(P) + countedMsg(P)        (and analogously
             pastMention, pastSize(X) = pastSize(P) + 1)     for the others)
k ≥ 2:       past(X) = ⋃ᵢ (past(Pᵢ) ∪ {Pᵢ})  — union cardinalities via the
             bounded meet-walk: dfsPrev from all parents with a shared visited
             set, descending until the open branches converge into a common
             covered region (the meet antichain); count counted creates seen
             strictly above the meet per branch + the meet's own labels.
             Bounded by the fork span (measured p50 = 2, p95 ≤ 28).
```

Notes:
- On the **origin** device, every created change links all local heads
  (`objecttree.go:378`), so `past(X)` = the entire local tree and the origin can
  stamp from running totals — an optimization only; receivers always run the
  fold (a received peer change links the *author's* heads, a subset of the
  receiver's tree).
- Mentions fold exactly like messages: `HasMention` is computed once at create
  (`chathandler.go:101-105`) and **never recomputed on edit** (the `ContentKey`
  modify path re-marshals without mention detection, `chathandler.go:227-234`) —
  mention state is immutable post-create, so no cut-dependent edit logic exists.
- Deletes: a delete change's past always contains its target's create (the
  deleting device had the create attached; Property M). Deletes do **not** enter
  `pastMsg` (the count identity uses the live collection, Theorem 1); the fold
  just labels the delete change like any other (its own labels are needed if it
  ever becomes a seen head).
- Reactions: covered in **§14 (Stage 4)**. Reaction changes are folded like any
  other change; the sidecar gains a `pastReact` column (counted reaction-add
  events in the strict past). The dedup-to-messages machinery is §14's — only
  the *deduped count* needs more than label arithmetic.

---

## 4. Storage

**Label sidecar** — new device-local collection in crdt.db (per chat object),
never synced, rows immutable, **never deleted** (a deleted message's create
change can still be a seen head, so labels must outlive the message row — this
is why labels do NOT live on the `chats` docs; note the old `_change_orders`
precedent is gone, migrated to `_meta` and dropped on init,
`state.go:32-39,127-144`; the local-only-field precedent is `firstAddSeq`,
`chatmodel.go:55-58`):

```
readlabels: { id (change id), o (OrderId), kind (countedMsg/countedMention/other),
              pastMsg, pastMention, pastSize }
index: (o)            — serves createdRank(cut) for the certificate
```

Size: ~40–60 B/row → ~5 MB for a 100k-change chat. Keyed by change id ⇒ stable
under treemigrator renumber (`treemigrator.go:89-91` resets only AddSeq) — an
explicit improvement over every AddSeq-based scheme.

**Frontier cache** — device-local collection in crdt.db (NOT the synced
seen-heads KV — that value is merged across devices, `store.go:183-195,420-437`):

```
readfrontier: { counterType, frontierHash (resolved head ids, sorted),
                maxF, bandIds [], updatedAt }
```

Persisted ⇒ cold start reads it directly; no tree open, no walk.

---

## 5. Runtime flows

**Query (count / ids / status)** — never walks, never opens the tree:
indexed range COUNT/scan on the live `chats` collection (`_o.id` index exists,
`repository.go:147`) + the cached `bandIds`. Status is stamped in-memory at
marshal time (own messages: always read).

**Change arrives (replay or live):** fold labels (§3, O(1) linear / meet-walk on
merges); apply Theorem 3's O(1) `bandSet` rule; if the change is a delete of a
band member, remove it from `bandIds`.

**Frontier changes** (`MarkSeenHeads` advance, markunread re-seed, a pending
head attaching, cross-device KV update):
1. Resolve heads (unresolved → pending, omitted — the existing safe over-count,
   `readwatermark.go:54-59`).
2. Compute `maxF`; run the certificate (Theorem 2). If 0 → `bandIds = ∅`, done.
3. Else run the bounded walk (hybrid doc §5 mechanics: backward from
   `Hloc ∪ F` in decreasing `g`; F-reachable = read; stop at the read boundary),
   self-terminated by the certificate bound. Worst case O(unread + band) point
   lookups — paid once per frontier change, then cached. (The certificate makes
   the expensive case rare: a huge-tail cold frontier with `bandCreated = 0`
   skips the walk entirely.)
4. Persist the cache; emit transition events:
   - read-advance `F_old → F_new`: newly-read =
     `(band_old ∖ band_new) ∪ (range (maxF_old, maxF_new] ∖ band_new)`
   - markunread regression: newly-unread =
     `(band_new ∖ band_old) ∪ (range (maxF_new, maxF_old] ∖ band_old)`
   One range scan + two tiny set diffs — replaces `SetReadFlag.idsModified`.

**Unlabeled head** (legacy change, backfill not done): skip the certificate,
run the walk (CORE mode) — still exact and convergent, just not O(1).

---

## 6. Plumbing changes (verified against code)

1. **`PreviousIds` into the handler.** `storestate.ChangeSet`/`Change` carry no
   parent ids today (`state.go:83-104`; `store_apply.applyChange` drops them,
   `store_apply.go:49-67`). Add `PrevIds []string` and thread it from
   `objecttree.Change` through `ChangeSet` → `Change` → `ChangeOp` (same shape
   as the existing `AddSeq` threading).
2. **Hook** = `ChatHandler.BeforeCreate/BeforeModify/BeforeDelete`
   (`chathandler.go:73,149,156`) — co-located with the existing
   `OrderId`/`HasMention` stamping; sidecar writes in the same tx as doc writes.
3. **Repository**: `unreadCount`/`unreadIds`/status from §5's query path;
   frontier-cache read/write; certificate query on the sidecar.
4. **Removed** (after cutover soak): `read`/`mentionRead` bools + their indexes,
   `SetReadFlag`, `getReadFilter` paths, `firstAddSeq` field + index,
   `GetUnreadMessagesComputed`, `GetReadFrontier`/`ReadFrontierProvider`, the
   AddSeq conjunct of `dominated` (the watermark object may remain solely as the
   seen-heads set manager), `shadowComputedReadCount` scaffolding.
5. **Kept**: seen-heads KV sync + cross-device merge + pending re-resolution
   (`store.go:374-433`), `MarkReadMessages`/`MarkSeenHeads` flow, AddSeq as the
   replay cursor.

---

## 7. Migration & staged rollout

The CORE works with zero labels — that makes the rollout safely incremental:

- **Stage 1 (CORE):** ship `bandSet` + tail queries computing read state via the
  walk at frontier changes (no certificate). Already exact + convergent; cold
  start = cached frontier, no tree. The bool path runs as shadow oracle.
- **Stage 2 (LABELS):** ship the fold for new changes + the **one-time async
  per-tree backfill fold** for history (topological pass; the
  `cmd/readcounter-bandsize` bitset machinery processed 76k real changes in
  seconds), in the existing gated backfill slot (`chatobject.go:343-364`) with
  the established hardening: resolve under the tree lock, async + batched,
  per-object done-marker, update-by-id. Until a tree's done-marker is set, it
  runs in CORE mode.
- **Stage 3 (cutover):** counters served by D; bool kept one release as a
  divergence-logging shadow; then D3 deletions land.
- **Stage 4 (reactions, §14):** the reaction counter moves onto the same
  machinery (event sidecar + refcounted unread-message set), retiring the
  per-message mutable reaction-unread state and `ClearUnreadReactions`.
  Independent of Stages 1–3; does not block them.

---

## 8. Mention counter

Identical machinery with the `hasMention` filter and its **own frontier**
(`diffManagerMentions`) → own `maxF`, own `bandIds`, `pastMention` label.
Mention immutability post-create (§3) makes the fold exact. Own mentions are
excluded by the counted predicate (own messages are never counted).

---

## 9. What AddSeq/OrderId remain for

`OrderId` keeps every role it has (storage order, range queries, UI ordering,
the tail cut). `AddSeq` returns to its original storage role — the replay
cursor (`GetAfterAddSeq`) — and exits the read model entirely. The
(OrderId, AddSeq) dominance predicate is retired with the bool.

---

## 10. The snapshot contract (guarding D1)

D's labels under-count if history below a snapshot horizon becomes unreadable —
the one *unsafe* failure direction in the design. D1 says this will not happen
for chat trees; the contract makes it observable instead of assumed:

- **Invariant (documented here + in chatobject):** chat store-trees retain full
  change history locally; the only snapshot is the genesis root.
- **Runtime guard:** at chat tree open, if the common snapshot is not the
  genesis root (or the root change is absent from storage), log loudly + mark
  the object **label-untrusted**: certificates disabled, walks attempt CORE
  mode; if the walk hits a missing ancestor, fall back to
  "everything `g > maxF` unread + band unknown → report the tail count only and
  flag over-count" — degrading toward **over-counting unread** (the safe
  direction), never silent under-count.
- If any-sync ever introduces store-tree snapshotting, labels must be anchored
  on snapshot changes (the snapshot creator stamps cumulative counts) — a
  designed extension point, not an emergency.

---

## 11. Tests

Carried forward: the full 25-scenario catalog via a D engine in the existing
harness (replacing the computed/watermark engines' role), the repository
differentials, and the marshal/property suites that remain meaningful.

New, in priority order:
1. **Cross-device convergence gate** — extend the harness to per-device trees
   with independent apply orders (the dimension it currently collapses,
   `scenario_harness_test.go:174`): device A applies `[a1],[b1]`, device B
   `[b1],[a1]`, `F={b1}` → assert **equal** counts (D: 1 = 1; the watermark's
   0 ≠ 1 documented as the fixed regression).
2. **Fold invariance property test** — random DAGs, random batch partitions per
   replica → identical labels (Theorem 4).
3. **Count identity property test** — random DAGs + frontiers + deletes:
   `tail + |bandSet|` ≡ brute-force `read*` complement (Theorem 1), and
   certificate soundness: `bandCreated(H*) = 0 ⟹ band = ∅` (Theorem 2).
4. **Theorem 3 unit** — attach-after-resolve never needs an ancestry check;
   in-past insert lands in `bandSet`; delete removes it.
5. **Frontier flows** — advance / markunread / pending-head-attach event diffs
   match `SetReadFlag.idsModified` semantics (S-cat17 class).
6. **Snapshot guard** — synthetic non-genesis snapshot → label-untrusted mode,
   over-count direction, loud log.
7. **Sign-off flip** — `S-concurrent-merge`/`S-cat12`/`S-cat19` → MATCH;
   `watermarkAcceptedDivergence` emptied (D2).

---

## 12. Costs

| Path | Cost |
|---|---|
| Count / ids / status query (incl. cold start) | O(1) + one indexed range; no tree, no walk |
| Change apply | + one fold (O(1) linear; meet-walk O(fork-span p95 ≤ 28) on merges) + one sidecar write |
| Frontier change | certificate O(log N); walk only if `bandCreated > 0` (≈13% of trees overall; ~0 for chats), self-terminated |
| Storage | ~40–60 B/change sidecar + tiny frontier cache, local-only |
| Migration | one async O(N) fold pass per tree (seconds at 76k changes), once |

vs the watermark: same or better query cost, **convergent**, no `SetReadFlag`
write amplification, no bool indexes. vs the old DiffManager: same semantics,
without the O(tree) cold-start rebuild (~18.6 s) that motivated its removal.

---

## 13. Risks & open items

- **R1 — meet-walk implementation** (k≥2 fold): the union-cardinality walk is
  the subtlest code; pin with the Theorem 4 property test before relying on it.
  (Stage 1 doesn't need it — CORE has no fold.)
- **R2 — frontier-change walk worst case** is O(unread + band) when the
  certificate is non-zero on a huge-tail frontier (e.g. markunread-to-ancient in
  a 100k chat with concurrency below the cut). Once per frontier change, cached;
  still ≪ O(tree). Accepted.
- **R3 — sidecar growth** is unbounded-with-history by design (labels must
  outlive deletes). Acceptable at ~5 MB/100k; revisit only if chat trees grow
  orders beyond that.
- **R4 — D1 dependency**: §10's guard is mandatory in the first shipped stage,
  not a follow-up.
- **R5 — reactions** are covered by §14 (Stage 4). Residual risk lives in the
  two semantics pins (S1 event definition, S2 counted predicate) — both must be
  resolved against current behavior **before** implementing Stage 4; the §14
  machinery is otherwise the same as the message counter's.

---

## 14. Reactions counter (Stage 4)

The reaction counter ("messages with at least one unread reaction",
deduplicated per message) moves onto the same CORE machinery, with one extra
sidecar row type and one honest limitation: **the deduped count is not pure
label arithmetic**, because count-distinct over targets does not fold into
cardinalities. Everything else carries over verbatim.

### 14.1 Today's surfaces (develop base — what Stage 4 retires)

Per-message mutable unread-reaction state cleared by
`ClearUnreadReactions(maxOrderId)` (`repository.go:710-733` — the known
over-clear hazard), `GetNewestUnreadReactionOrderId` recompute scans
(`repository.go:734`; consumed by the subscription's `UnreadReactionOrderId`,
`chatsubscription/manager.go:217-222,506,570`), `GetAllUnreadReactionChangeIds`,
and the `diffManagerReactions` full-tree stream. The apply-time add/remove
transition detection in `handleReactionsModify`
(`chathandler.go:245-270` — `wasPresent`/`isPresent` → `onReactionAdded`)
**stays**: it is exactly the event source §14.3 logs.

### 14.2 Model

- **Counted entity:** a *reaction event* `r` — a counted reaction-add detected
  at apply — with coordinates `(g(r), msgId(r), reactor(r), emoji(r))`.
- **Counter:** `|{ live messages m : ∃ counted reaction event r → m, ¬read*(r) }|`
  against the reactions frontier `F_r` (the existing `diffManagerReactions`
  seen-heads set, kept as-is).
- **Decomposition (Theorem 1, verbatim):** `r` unread ⟺ `g(r) > maxF_r` or
  `r ∈ bandSet_r(F_r)`. Reaction events are changes; nothing in §2 is
  message-specific. Theorem 3's O(1) maintenance applies unchanged.

### 14.3 Storage and flows

**Event rows** — sibling sidecar collection in crdt.db, immutable, local-only,
never deleted (they must survive message deletion; liveness is checked at
count time against the `chats` collection):

```
reactevents: { changeId, o, msgId, reactor, emoji, op (add|remove) }
index: (o)
```

Written from the existing apply-time transition detection (`onReactionAdded`
and its remove counterpart). The `readlabels` fold (§3) gains a `pastReact`
column: counted reaction-**add** events in the strict past.

**Unread set** — cached, device-local, persisted (the `bandSet` pattern):

```
unreadReactionMsgs: { msgId → refcount of unread counted events }
counter = |keys(unreadReactionMsgs) ∩ live messages|
```

- **Frontier change (`F_r`):** zero-certificate first —
  `unreadAddEvents = addEventRank(∞) − pastReact(H*) − [H* itself a counted add]`
  over the create universe of event rows (sound upper bound by the Theorem 2
  argument: un-reacts and message deletes only shrink the live set). If 0 →
  counter is 0, done. Else one indexed scan of event rows with `o > maxF_r`
  plus the band walk for `F_r`, refcounted into the map. O(events since the
  read point), once, then cached.
- **Event arrives:** O(1) — add → `refcount++` (Theorem 3: it cannot be covered
  by a resolved head); remove → `refcount−−` under net-state semantics (S1);
  message delete → drop the key.
- **Subscription:** `UnreadReactionOrderId` (newest unread event's `g`) is read
  off the cached map — the `GetNewestUnreadReactionOrderId` repo scan goes away.

### 14.4 Semantics pins (resolve against current behavior BEFORE Stage 4)

- **S1 — event definition.** Two options with different convergence strength:
  **(a) content-based toggle-event** (any peer toggle on a counted message is
  notify-worthy, add and remove alike): a pure function of the change content →
  *fully* replica-invariant, the strongest convergence. **(b) apply-time
  add-detection** (today's `onReactionAdded`): matches current behavior
  exactly, but the add/remove classification of two *concurrent toggles of the
  same (emoji, identity)* depends on local apply order — a pre-existing corner
  of the current system that (b) inherits and (a) eliminates. Decide after
  checking what the toggle change op actually carries (full reactions state vs
  delta). The per-device counter is internally consistent either way (cert and
  set derive from the same local event log); cross-device convergence is exactly
  as strong as the chosen event definition.
- **S2 — counted predicate.** Peer reactor only (`reactor ≠ me`); whose
  messages count (all vs own-only is a product choice — match current); the
  existing `reactionsCounterEpoch` cutoff stays (change `Timestamp` is in the
  signed change → replica-invariant → convergent).

### 14.5 Tests (additions to §11)

8. **Reactions differential** — random event logs + frontier moves: cached
   refcount map ≡ brute-force recompute from the event rows; counter matches
   the legacy `ClearUnreadReactions` path on linear histories (where legacy is
   correct), and documents the over-clear divergence where legacy is not.
9. **Reactions cross-device gate** — the per-device-tree harness (§11.1) with
   concurrent reaction events: equal counters on both devices under S1(a);
   under S1(b), equal except the documented concurrent-toggle corner.
10. **Lifecycle** — un-react retraction per S1, message-delete drops the key,
    epoch cutoff respected, event rows survive message deletion.

### 14.6 Cost

Zero-certificate O(1); recompute O(unread events since the read point), once
per frontier change, cached + persisted; O(1) per arriving event; ~40 B per
reaction event in the sidecar. Retired: the per-message mutable
reaction-unread writes, `ClearUnreadReactions`'s clear-storms (and over-clear
bug), the `GetNewestUnreadReactionOrderId` scans, and the last full-tree diff
manager stream.
