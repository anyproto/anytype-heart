# Convergent, Index-Cheap Read Counters over a Replicated Causal DAG — A Formal Problem

## 0. Purpose and how to read this

This is a **self-contained, domain-free problem statement** for a strong reasoning
solver. No knowledge of any particular product, database, or codebase is required;
all needed structure is defined in §1. The optional appendix sketches the concrete
system this abstracts, but the core (§1–§9) stands alone.

We ask the solver to:

1. **Confirm or correct** the two base results, Theorem A and Theorem B (§5), and
   their corollary.
2. **Solve the open problem in §8**: design a read-counter scheme that is
   *simultaneously* **cheap** (no graph traversal at query time, no mutable
   per-event state) and **convergent** across a user's replicas — or **prove it
   impossible** under those constraints and characterize the best achievable.

The tension in one line: the only *correct, convergent* scheme we know requires a
graph walk (too slow); the only *cheap* scheme we know is non-convergent across
devices. Bridge them, or prove the gap is fundamental.

---

## 1. Model and notation

### 1.1 Events and the causal DAG

- A growing set `V` of immutable **events**. Each event names a set of **parents**
  (other events). This induces a DAG; let `≺` be its transitive closure, a strict
  **partial order** ("causally precedes"). Write `⪯` for "`≺` or `=`", and `X ∥ Y`
  for **concurrent** (`X ⋠ Y` and `Y ⋠ X`).
- Events are **content-addressed and append-only**, so `≺` is **replica-invariant**:
  any two replicas that both hold `X, Y` agree on which of `X ≺ Y`, `Y ≺ X`,
  `X ∥ Y` holds. The DAG only grows; edges are immutable.

### 1.2 Replicas and delivery

- One **user** owns several **replicas** (devices). Each replica `r` holds a subset
  of `V` that grows monotonically toward `V` (eventual, lossless delivery).
- Events may arrive in **different orders** and grouped into different **batches**
  on different replicas. There is no global arrival order.

### 1.3 Two linear extensions of `≺` (the crux)

Both schemes below number events with a *total* order that extends `≺`. The whole
problem turns on the difference between these two:

- **Global order `⊏` (convergent).** A canonical **linear extension of `≺`**: a
  deterministic function of the DAG that is **identical on every replica**. It
  extends `≺` (`X ≺ Y ⟹ X ⊏ Y`) and, being total, also orders **concurrent**
  events by a fixed deterministic tiebreak. Store its key as `g(X)` (a comparable
  scalar). *Because it is total, `g` orders concurrent `x ∥ h` too — this is the
  source of "over-reading" below.*

- **Local rank `aᵣ` (device-local).** Each replica `r` assigns each event an
  integer `aᵣ(X)` = its position in `r`'s **arrival/apply** linearization. Its
  properties:
  - **(S1)** extends `≺`: `X ≺ Y ⟹ aᵣ(X) < aᵣ(Y)` on every replica;
  - **(S1-batch)** events delivered in one batch share a single rank value (ties);
  - **(S2)** **not replica-invariant for concurrent events**: if `X ∥ Y`, the sign
    of `aᵣ(X) − aᵣ(Y)` may differ between replicas (it encodes local arrival order).

  Note: a linear extension of `≺` that were *also* replica-invariant would be
  essentially `⊏` itself. So `aᵣ`'s *only* information beyond `g` is local and
  concurrency-sensitive — exactly the part that does not converge.

### 1.4 The read frontier

- The user marks "read up to here." The state is a **set** `F ⊆ V` of frontier
  events ("heads" — the maximal events the user has read).
- `F` is **synced across the user's replicas as a set of event-IDs**, *not* as
  coordinates. So every replica eventually holds the same `F` (as IDs) but
  **resolves** each `H ∈ F` to its *local* coordinates: `g(H)` (shared) and
  `aᵣ(H)` (local).
- `F` **evolves**: it grows as the user reads more, and it can **regress** (a
  "mark-unread" operation shrinks the read region). An ID in `F` may reference an
  event **not yet locally present** (delivered late).

### 1.5 Counted events and filters

- A fixed, locally-known boolean selects the **counted** events (e.g. exclude the
  user's own events; or restrict to a labeled sub-class). The **counter** = number
  of counted events that are unread. The product also needs the **set** of unread
  event-IDs, not only the count.

---

## 2. What "read" should mean — the ground truth

The intended semantics is **causal coverage**:

> **`read*(X) ⟺ ∃ H ∈ F : X ⪯ H`**  — `X` is read iff it causally precedes (or is)
> some event the user marked read; i.e. `X` is in the **causal down-set** of `F`.

`read*` is a function of `(≺, F-as-set)` **only**, hence **replica-invariant /
convergent**: once two replicas hold the same events and the same `F`, they compute
the identical read set and count. This is the correctness baseline.

Unread count `U = #{ counted X : ¬read*(X) }`.

---

## 3. Constraints (why the obvious implementations are disallowed)

A valid implementation must satisfy all three:

- **(C1) No graph traversal at query time.** The read/unread status of events, and
  the unread count/enumeration, must be answerable from **per-event stored scalar
  labels** plus the (resolved) frontier, via **indexed range queries** — *not* by
  traversing `≺` at query time. (Traversal is `O(|V|)` and dominates cold-start
  latency.)
- **(C2) No mutable per-event read flag.** We may not store a boolean "read" per
  event and flip it on each read action. Read status must be **computed on demand**.
  (Each read marks many events; flag writes are write-amplified and costly.)
- **(C3) Cheap.** Count and unread-enumeration in `O(polylog |V|)` index operations
  plus `O(#unread)` output — a few indexed range scans, sub-linear in `|V|`.

**Allowed:** per-event **immutable** labels assigned once at event creation
(these do *not* violate C2). Disallowed: anything updated per read action, or any
per-query walk of `≺`.

The ground-truth scheme `read*` computes a causal down-set, which needs `≺` at
query time ⇒ it violates **C1/C3**. That is exactly why an index-cheap proxy was
sought.

---

## 4. The scheme under study — scalar "dominance"

Store two immutable scalars per event: the global key `g(X)` and the local rank
`aᵣ(X)`. Define

> **`read_d(X) ⟺ ∃ H ∈ F : g(X) ≤ g(H) ∧ aᵣ(X) ≤ aᵣ(H)`**  — *dominance* on two axes.

**Cheap enumeration.** With `gmax = max_{H∈F} g(H)` and `amin = min_{H∈F} aᵣ(H)`,

  `¬read_d(X) ⟹ g(X) > gmax ∨ aᵣ(X) > amin`,

so the unread set is contained in (range `g > gmax`) ∪ (range `aᵣ > amin`) — **two
indexed range scans** — after which the dominated events and the non-counted events
are filtered out. This satisfies **C1–C3**.

Intuition: `g` is the convergent "timeline position"; `aᵣ` is a freshness guard so
that an event with a *small* `g` that *arrived late* (after the user read past that
position) stays unread.

---

## 5. Base theorems to confirm (or correct)

**Theorem A (cheap enumeration is exact for `read_d`).** The two-range union, minus
the dominated events, minus non-counted events, equals exactly
`{ counted X : ¬read_d(X) }` — for every `F` (including `|F| > 1`, empty `F`, equal
`g`-keys, tied ranks, and integer-encoding boundaries).

**Theorem B (when does `read_d` equal the ground truth `read*`?).** Define the
causal down-set `Stable(F) = { X : ∃ H ∈ F, X ⪯ H }` and the **ambiguous band**

  `Amb(F) = { X ∉ Stable(F) : ∃ H ∈ F, X ∥ H ∧ g(X) ≤ g(H) }`.

Then:
- for every `X ∉ Amb(F)`: `read_d(X) = read*(X)` (`= [X ∈ Stable(F)]`),
  **independent of the local ranks `aᵣ`**;
- for `X ∈ Amb(F)`: `read_d(X)` **depends on `aᵣ`** and can take either value.

Hence `read_d = read*` (equivalently, `read_d` is rank-/apply-order-invariant)
**iff `Amb(F) = ∅`**.

Supporting lemmas:
- **(B1) causal pairs are stable.** If `X ⪯ H` or `H ⪯ X`, then `g` and *every*
  `aᵣ` decide `(X, H)` the same way on every replica (both extend `≺`). So such
  pairs never flip.
- **(B2) concurrent pairs can flip.** If `X ∥ H`, there exist apply orders giving
  `aᵣ(X) ≤ aᵣ(H)` and others giving `aᵣ(X) > aᵣ(H)` (S2). Combined with
  `g(X) ≤ g(H)`, `H` dominates `X` under one and not the other.

**Corollary (the multi-device defect).** For `X ∈ Amb(F)`, since `aᵣ` is
replica-local (S2), two replicas of the same user holding the **same** `V` and the
**same** `F` can compute `read_d(X)` **differently** ⇒ a **permanent** divergence of
the unread count between the user's devices that **does not heal** after full
delivery. `read*` has no such divergence.

*(Please confirm A, B, the lemmas, and the corollary, or produce a counterexample.)*

---

## 6. Minimal worked example (abstract)

Two concurrent events `x ∥ h`, both children of a common parent, with `g(x) < g(h)`.
Frontier `F = {h}` (the user marked `h` read). `x` is delivered late on one replica.

- **Ground truth `read*(x)`:** `x ∥ h ⇒ x ⋠ h ⇒ x ∉ Stable({h}) ⇒ x UNREAD`, on
  **every** replica. Count = 1 everywhere. (The user read `h`; `x` is on a branch
  they never read.)
- **Dominance `read_d(x)`:**
  - Replica **B** received `h` first (`a_B(h)=1`) then `x` (`a_B(x)=2`):
    `a_B(x) ≤ a_B(h)`? `2 ≤ 1` false ⇒ `x` **unread** ⇒ count 1.
  - Replica **A** received `x` first (`a_A(x)=1`) then `h` (`a_A(h)=2`):
    `g(x) < g(h)` ✓ and `a_A(x) ≤ a_A(h)` ✓ ⇒ `x` **read** ⇒ count 0.
  - **A = 0, B = 1, permanently.** `x ∈ Amb({h})`.

(If `x` and `h` arrive in the **same batch** on a replica, their ranks tie (S1-batch)
and the decision collapses onto `g` ⇒ `x` read there. So the divergence requires
**split-batch** delivery across devices — the normal case when two devices receive
two concurrent events over the network independently.)

---

## 7. The property we need — convergence

> **(P2) Convergence.** For any two replicas of one user with equal local event sets
> and equal `F`-as-ID-set, the computed unread set (hence count) is identical.

`read*` satisfies P2 (it ignores all local data). `read_d` violates P2 exactly on
`Amb(F)`. The two known schemes trade off:

| scheme | cheap (C1–C3) | convergent (P2) | faithful to intent |
|---|:---:|:---:|:---:|
| causal down-set `read*` (graph walk) | ✗ (needs `≺` at query) | ✓ | ✓ |
| scalar dominance `read_d` | ✓ | ✗ (on `Amb(F)`) | ✓ off band, ✗ on band |

---

## 8. The open problem — the missing piece

Find a scheme `(L, read_L)` such that:

1. each event carries an **immutable label `L(X)`** assigned at creation (respects
   **C2**), and `L(X)` is **replica-invariant** (same on every replica, never
   updated);
2. `read_L(X, F)` is computable under **C1 and C3** — indexed range queries over
   stored scalars + the resolved frontier, no per-query traversal of `≺`, and the
   unread set is **cheaply enumerable and countable**;
3. `read_L = read*` — i.e. **convergent (P2)** and faithful to causal coverage; at
   minimum it must decide the ambiguous band `Amb(F)` the way `read*` does
   (`x ∥ h`, `g(x) ≤ g(h)` ⇒ **unread**).

In words: **recover the convergence of the causal-down-set scheme at the cost
profile of the scalar-dominance scheme.**

**Equivalent core question.** The global order `g` alone over-reads (it orders a
concurrent `x` before `h`). The single local axis that fixed that (`aᵣ`) is
non-convergent. *Is there a **replica-invariant** per-event encoding that
distinguishes, among events with `g(X) ≤ g(H)`, those with `X ≺ H` (read) from those
with `X ∥ H` (unread) — and that supports cheap **range** enumeration of the unread
set — or is that impossible under C1/C3?*

A complete answer is **either**:

- **(a) A construction** with proofs that it meets **C1–C3** and **P2**, including
  its space/time cost, and that it handles all of §9; **or**
- **(b) An impossibility / lower bound** showing no scheme can meet **C1–C3** and
  **P2** simultaneously (e.g. reduced to reachability-labeling lower bounds, or to
  the DAG's width / antichain size), **together with the best achievable** — for
  instance a **hybrid** that uses the cheap convergent-off-band test plus a bounded
  causal check confined to `Amb(F)` (note `Amb(F) = ∅` for linear histories and is
  small when concurrency at the frontier is rare), with the fallback cost
  characterized; or a scheme parameterized by width `w` with cost growing in `w`.

**Non-binding directions** the solver may use or discard: replica-invariant causal
labels (version/vector clocks, Lamport-style, 2-hop labels, interval or
chain-decomposition reachability labels) as a convergent replacement for `aᵣ`;
restricting to **low-width (near-linear)** DAGs; precomputed per-event
"concurrent-with-the-frontier" summaries (must stay **immutable** to respect C2);
or a proof that *any* single replica-invariant total order is essentially `g` and
therefore insufficient, forcing **≥ 2 dimensions**.

---

## 9. What a strong answer must address

- Confirm or correct **Theorem A**, **Theorem B**, the lemmas, and the corollary.
- Then deliver **(a)** a construction (with proofs + cost) or **(b)** an
  impossibility result + the best achievable.
- Explicitly handle:
  - **multiple frontier heads** (`|F| > 1`);
  - the **counted-subset filter**;
  - **frontier evolution** — both **growth** and **regression** (mark-unread);
  - **frontier IDs referencing not-yet-local events** — the scheme must not
    *over-read* due to an unresolved head (a safe, eventually-consistent treatment
    is acceptable here, but state it);
  - cheap **enumeration / count** of the unread set, not only point queries;
  - the **failure direction**: if any approximation is unavoidable, prefer
    *over-counting unread* (showing a read item as unread) to *under-counting*
    (silently hiding an unread item).

---

## Appendix A — Notation

| symbol | meaning |
|---|---|
| `V`, `≺`, `⪯`, `∥` | events; causal partial order; "≺ or ="; concurrent |
| `g(X)` (`⊏`) | global order key — a **convergent** (replica-invariant) linear extension of `≺` |
| `aᵣ(X)` | local rank on replica `r` — arrival/apply linear extension of `≺`, **device-local** for concurrent events |
| `F` | read frontier (set of "head" events), synced as IDs, resolved to local coordinates |
| `read*` | ground truth: `∃H∈F: X ⪯ H` (causal down-set) — convergent but needs a graph walk |
| `read_d` | dominance proxy: `∃H∈F: g(X) ≤ g(H) ∧ aᵣ(X) ≤ aᵣ(H)` — cheap but non-convergent on `Amb(F)` |
| `Stable(F)` | causal down-set `{X : ∃H∈F, X ⪯ H}` |
| `Amb(F)` | ambiguous band `{X ∉ Stable(F) : ∃H∈F, X ∥ H ∧ g(X) ≤ g(H)}` |
| C1 / C2 / C3 | no query-time graph walk / no mutable per-event flag / cheap (indexed) |
| P2 | cross-replica convergence of the result |

## Appendix B — (Optional) concrete instantiation

This abstracts **unread-counters in a CRDT-based group chat** synced peer-to-peer
across a user's devices. Events = chat messages (nodes of an append-only,
content-addressed change DAG); `≺` = the change DAG's ancestry; `g` = a
deterministic per-change order key; `aᵣ` = the device-local sequence in which
changes were applied to local storage (assigned in arrival batches); `F` = the
synced "read markers" (seen heads); counted events = messages from other authors
(optionally a "mention" sub-class with its own frontier). The original, correct
implementation computed `read*` by walking the DAG from the read markers — but that
walk over the whole history on every cold start was the dominant startup cost, which
motivated the scalar `read_d` proxy. The proxy is fast but exhibits the §6
divergence between a user's own devices. The core problem (§8) is to regain
convergence without reintroducing the per-query walk or a per-message read flag.
*(This appendix is for grounding only; the problem in §1–§9 is fully abstract.)*
