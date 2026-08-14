# ImportV2 Deferred Materialization — Fetch/Convert/Spool, Then Materialize

Status: design revision, 2026-08-14. Revises
`docs/superpowers/specs/2026-08-13-importv2-durable-queue-design.md` ("08-13" below), whose
Phase A is **shipped** (commits `7220d9113..ff8d37db3`) and whose runstore/journal/sweep
machinery this design builds on, not around. §2 lists exactly what is superseded and what
stands. Two of 08-13's decisions are **reversed** here — D3's write-through-only ledger and
§3.2's rejection of the spool — and §5 re-argues them rather than papering over the reversal.

## 1. What changes, and why

**The requirement (product, not optimization):** a large Notion import is pacer-bound at
3 rps, ~2 requests per page (`notion/prefetch.go:11-16`) — hours for a big workspace. Today
the engine persists from the first seconds: the sink dispatches each converted object straight
into the worker pool (`engine/sink.go:77-92` → `engine/engine.go:273-330`), so objects trickle
into the user's space for the entire crawl. A space that is half-built for three hours is the
thing being forbidden. The space must stay clean until the import is essentially done.

**The shape:** split today's pass 2 in two.

```
pass 1  identity          unchanged: claims, dedup match, local id minting
pass 2  fetch/convert/spool   crawl + convert + download attachments; write converted
                              output to the run dir. NOTHING enters the Anytype space.
pass 3  materialize       read the spool in order, assign identity, resolve references,
                          create trees, upload files, journal effects. No network
                          except file-sync's own background push.
finalize                  root collection, widget, report — end of pass 3, as today
```

Pass 3 runs at persist speed — 50–200 objects/s estimated (08-13 §8; the store, not the
network, is the bottleneck) — so the window in which the space is visibly half-built shrinks
from hours to minutes. Everything before pass 3 is abortable by deleting the run dir: nothing
to compensate, because nothing was done.

**The core design move, stated up front:** the spool reader implements `importv2.Converter`.
Pass 3 is today's engine back-half — sink, identity assignment, channels, workers, resolver,
persister, journal — fed by a converter that replays the spool instead of crawling Notion.
The §2 converter contract, the file-future mechanism, the `C+K` gauge test, the Phase-A
journal/compensation machinery: all of it applies to pass 3 verbatim, because pass 3 *is* the
pipeline it was built for. What this design adds is a durable seam between the two halves,
not a new pipeline.

## 2. Relationship to the 08-13 spec

Superseded:

| 08-13 | Disposition |
|---|---|
| D3 ("the durable layer is a write-through ledger, **not** an absorbing queue") | **Reversed in part.** The run dir now holds an absorbing queue — the spool — between pass 2 and pass 3. The *effect ledger* remains write-through exactly as shipped; the reversal is scoped to what sits between converter and persister (§5.1). |
| §3.2 (converted object stream deliberately not persisted) | **Reversed.** The premise no longer holds (§5.2). |
| §5.5 (backpressure: blocking channel end-to-end) | Superseded for pass 2 (§5.4); stands for pass 3. |
| §6.2–§6.3 resume semantics (skip-set re-crawl, `ResumableConverter` as the driver-2 payoff) | Reworked (§8): pass-3 restart from the spool subsumes most of it; the skip-set/seam survives only as pass-2 crawl resume. |
| §10 phase plan B/C | Reshaped (§9). |

Stands unchanged: §4.1 (per-run DB, disposal by `os.RemoveAll` — the spool strengthens it),
§4.2 manifest/entries/files schemas and merge rules (extended, §6), §4.4 frozen core (the
spool is explicitly **not** frozen, §6.3), §5.1–§5.4 invariants (now scoped to pass 3), §6.1
sweep (extended with two states, §8.2), §6.4–§6.5 suspend/cancel, §7 lifecycle (extended),
§8 measurements, §9 rejected alternatives 1–2 and 5–9. OQ1–OQ8 all still open except as
noted in §11.

## 3. Verified facts this design rests on

1. **Final ids exist before anything touches the space.** `CreateTreePayload` builds and
   signs the root change in memory (`objectcache/tree.go:31-41` →
   any-sync `objecttreebuilder/treebuilder.go:189-203`, `objecttree.CreateObjectTreeRoot`
   against the ACL); the storage write is a separate call (`PutTree`, used only by
   `CreateTreeObjectWithPayload`, `tree.go:56-60`). The id is the root-change hash, fully
   known at the end of pass 1 today. No new id machinery is needed or proposed; the 08-13
   payload store (durable `payloads` collection) carries these across the pass boundary and
   across restarts — the same §4.2 collection Phase B already specified, pulled forward.

2. **Large spool transactions are not a memory problem.** any-store v0.4.7 is SQLite-backed
   with a per-connection page cache capped at 2 MB (`cache_size: -2000` kilobytes,
   any-store `config.go:10-12`; heart's pragmas add only `synchronous`/`wal_autocheckpoint`,
   `runstore.go storeConfig`); SQLite's pager spills dirty pages to disk mid-transaction when
   the cache overflows (`cache_spill`, on by default, not overridden anywhere in v0.4.7 or
   heart). Write-transaction memory is bounded by the page cache, not the transaction size.
   Per-document buffers above `SyncPoolElementMaxSize` (2 MiB default, `config.go:34-36`)
   allocate transiently rather than pool — fine for the rare large snapshot, which our own
   48 MB guard (`persist/persist.go:163`) bounds anyway, far under SQLite's blob limits.

3. **The `FileSource.Open` closure problem dissolves.** The closure exists because downloads
   are lazy: notion wires `Open: c.downloadOpener(...)` (`notion/files.go:80-83`) and the
   download happens when a persist worker materializes the file. Pass 2 downloads eagerly as
   it converts — wanted independently, because downloads bypass the Notion pacer entirely
   (`client/fetcher.go` is retry+stall-timeout only, no limiter; verified) and should overlap
   the crawl — so the spooled object carries a spill path, not a closure. §3.2's
   "unserializable `FileSource.Open`" cost is gone, not worked around.

4. **URL expiry shrinks to inside pass 2.** Today a pre-signed URL captured at block-fetch
   time is downloaded by a persist worker arbitrarily later (the §16-item-5 refresh exists
   for exactly this). With eager downloads the capture→download gap is the download queue's
   latency — seconds to minutes — comfortably inside the ~1 h signing window. The refresh
   path (`notion/files.go` urlRefresher) is kept as the rare-stall fallback, demoted from
   load-bearing to insurance.

## 4. The pass model

### 4.1 Pass 2 — fetch, convert, spool

The converter runs exactly as today — same `Convert(ctx, sink)`, same prefetch pipeline,
same plan step, same second-chance discovery — against a sink whose implementation changes:
instead of assigning identity and dispatching to workers (`sink.go:22-92`), it serializes
the object into the **spool** (§6.1) and returns. Late claims (`sink.Claim`) go to the
durable claims ledger. Issues go to a durable issues collection (they must survive to pass 3's
report page). Downloads: `FileSource.Open` closures are drained to the run dir's spill at
spool time through a small bounded download pool (concurrency ~4, streaming copy — the
existing `readerWithContext` discipline, `persist/file.go:139-149`); plain `FileSource.Path`
entries (markdown loose files) are spooled as paths, not copied (§10 costs, "source moved").

What pass 2 may touch: the source, the network, the run dir, the space **index for reads**
(pass-1 dedup queries and `AssignDerived`-style lookups are reads and happen in passes 1/3
respectively — pass 2 itself needs no store access at all, preserving converter rule 4).
What it must not touch: the space. No trees, no uploads, no bundled installs, no flags.

The LLM plan step stays where it is — inside `Convert`, before emission
(`notion/converter.go:159`) — because it shapes conversion output; the spooled objects
already reflect it. The plan JSON lands in the run dir `kv` (08-13 §6.3's persistence,
arriving one phase earlier), which also makes the plan-determinism concern moot for pass-3
restarts: pass 3 never replans.

Pass 2 ends by writing `RootSpec` + a **fetch-complete marker** to `kv` and moving the
manifest to `fetched`. A spool without the marker is a partial crawl, never materialized.

### 4.2 Pass 3 — materialize

A `spoolReader` implementing `importv2.Converter`: `Convert` iterates the spool in recorded
order and emits each object to the real engine sink — the one that assigns identity
(`sink.go:50-67`), enqueues to the bounded lanes, and feeds the `K` workers. Everything
downstream is the shipped code path: resolver rewrites, file futures, install coordinator,
provenance stamping, the write-through effect journal, favorite/archive flags. File objects
upload from their spill paths; content-addressed dedup and the owned/matched classification
work exactly as today (`persist/file.go:48-60`).

Identity assignment deliberately happens **here**, not at spool time. Two reasons: (a) the
dedup queries (`AssignDerived` → `identity/dedup.go`) run against the space as it is at
materialize time, which after an hours-long pass 2 is *fresher* than assigning at convert
time; (b) it keeps the spool a pure record of converter output — source-keyed, unresolved —
so pass 3 is a function of (run dir, space) with no hidden coupling to pass-2-time state.
The durable inputs pass 3 needs beyond the spool are exactly pass 1's outputs: the claims
index and the create payloads (§6.2).

Finalize (root collection, widget hookup, report page) runs at the end of pass 3, unchanged
(`engine.go:414-460`, `maybeClaimReport`/`emitReport` at `engine.go:502-555`), as does
`reconcileClaims` — whose
inputs (claims vs emissions vs issues) now span the pass boundary through the durable
collections rather than process memory.

### 4.3 File uploads: pass 3, not pass 2 — decided

Uploads are local-first (`filesync.AddFile` queues, ImportV2Design.md:934-939), so they
*could* run during pass 2 — but an upload creates a **file object in the space**
(`fileobject` tree + index rows), which is precisely the dirt the requirement forbids; it
also starts background sync traffic and quota-event flow hours before the user has an
import. Uploads stay in pass 3, done by the persist workers as today. The interaction with
reference resolution therefore does not change shape: the file object's id is
upload-determined *within pass 3*, and the existing future mechanism synchronizes referencing
objects against it (§5.5).

## 5. The reversals and the invariants — re-argued

### 5.1 D3, reversed in part

08-13 D3 said the durable layer is a write-through ledger, not an absorbing queue, because
absorbing bought nothing: the store was the bottleneck and decoupling only converted memory
bounds into disk bounds for zero speedup. That reasoning assumed the two ends *should* run
concurrently. The product requirement breaks that assumption: the ends must **not** run
concurrently — the whole point is that persistence does not start until conversion is done.
Once the phases are sequential, the thing between them is definitionally an absorbing queue,
and disk is the only place it can live. The effect ledger is untouched: pass 3 journals
write-through per object exactly as shipped (Phase A commits), because its job — crash
attribution of real effects — is unchanged.

### 5.2 §3.2, reversed

§3.2 rejected the spool on this arithmetic: backpressure bounds convert-ahead to ~40
objects, so a spool saves ~27 s of re-crawl on resume, against costs of (a) serializing the
`Open` closure, (b) snapshot schema-versioning on disk, (c) O(source) DB growth, (d) heavy
payloads transiting the DB twice. That arithmetic answered the question "is the spool worth
it *as a resume optimization*" — and the answer was no, and stands. The question now is
different: the spool is the run's **primary storage**, the mechanism by which the space
stays clean. The costs must be paid or refuted on their own terms:

- (a) **gone**, not mitigated: eager downloads (§3.3) mean nothing unserializable remains.
  The envelope left to serialize is `Snapshot` (already proto-shaped —
  `object.go:65-79` `ToProto`/`NewSnapshotFromProto` round-trip exists) plus six scalar
  fields and a file descriptor of five scalars.
- (b) **bounded by ephemerality** (§6.3): the spool lives hours-to-days inside one run dir
  and carries no forward-compat promise — unlike the frozen core, an incompatible spool is
  *dropped*, not migrated. Versioning cost = one integer check.
- (c) **accepted and quantified** (§10): O(source) disk in the run dir, ~2× serialized
  snapshot bytes (the measured anyenc+page overhead, 08-13 §8) plus attachments that were
  already being spilled to disk today (`adapter` spill dir). Disposal is the same
  `os.RemoveAll` the layout was designed for.
- (d) **accepted**: every snapshot is written and read once more. At measured any-store
  rates (15k+ small-row tx/s; large rows disk-bandwidth-bound) this is seconds per hundred
  thousand objects — noise against a multi-hour crawl and a multi-minute materialize (§10).

### 5.3 The `C + K` invariant and the gauge test

Split by pass. **Pass 3 keeps it verbatim**: the spool reader is a Converter feeding the
same lanes and workers, so peak heavy-object residency is the same `2C + K` and the gauge
test (`TestRunMemoryBound`, `engine/engine_test.go:646`) applies unchanged — it should run against the spool
reader too (same synthetic 2000-object source, spooled then materialized). **Pass 2 gets a
new, tighter bound**: one object under conversion + one being serialized + D≈4 streaming
downloads (no full-file buffering). A pass-2 gauge test asserts heavy-object residency ≤ a
small constant while spooling a synthetic source. What bounds the *spool's* growth is the
source itself — the spool is O(converted source), there is nothing else to bound it with,
and that is the accepted cost of (c) above. Pass-2 concurrency is bounded by the converter's
own internals (notion prefetch: 6 in flight, `prefetch.go:16`) plus the download pool.

### 5.4 Backpressure

Nothing stops a fast converter outrunning the spool, and nothing needs to: the consumer is
a sequential disk write measured at 15k+ tx/s on small rows and disk-bandwidth on large ones
(08-13 §8), which outruns every producer we have — Notion is pacer-bound at ~1.5 objects/s,
and local markdown conversion feeds a writer ~two orders of magnitude faster than the old
consumer (the store). The blocking-channel flow control survives only where it still has a
job: inside pass 3, where a slow store must stall the spool reader — which it does, through
the same bounded lanes as today. The one genuinely new unbounded resource is disk (§10).

### 5.5 File futures and deadlock freedom

The futures do not cross the pass boundary, because the problem they solve does not: a
future synchronizes a *referencing object's resolution* against a *file object's upload*,
and both of those happen in pass 3. The spool preserves emission order; the spool reader
re-emits file objects before their referencers (definitions-before-use, recorded); the sink
registers futures in stream order (`future.go:51-58`); the dedicated file lane and FIFO
argument (`engine.go:297-308`, ImportV2Design.md:185-193) apply to pass 3 exactly as
written. No replacement mechanism is designed because none is needed — this is a direct
payoff of making the spool reader a Converter instead of restructuring the back-half
(the restructure is rejected alternative R1, §12).

### 5.6 The converter contract and determinism

All five §2 rules survive, unchanged in text:

1. *One object at a time* — the spool sink blocks per object (a fast disk write); the spool
   reader emits one at a time.
2. *Definitions before use* — now a property the spool **records** (converter order in) and
   **replays** (pass 3 order out). Still load-bearing: the format registry, key table and
   file futures are all seeded in stream order in pass 3.
3. *No silent drops* — unchanged; pass-2 issues are durable so they reach the report.
4. *No store access* — pass 2 now satisfies a strictly stronger form (no store access even
   by the engine on the pass-2 path).
5. *Determinism* — still required, for the same reasons as before (golden tests, idempotent
   re-import) plus a new one: pass-2 resume (§8.3) depends on a re-crawl claiming the same
   keys. The spool reader is deterministic by construction — it replays a recording.

## 6. Durable state — schema additions

All in the existing per-run `run.db` (§4.1 stands). New collections:

### 6.1 `spool`

```
{ id: <int seq>,               // monotonic emission counter; pass 3 reads Sort("id")
  sourceKey, sbType,
  snapshot: <binary>,          // model.SmartBlockSnapshotBase proto (ToProto round-trip,
                               //   object.go:65-96); source-keyed, UNRESOLVED
  isRootCandidate, favorite, archived: <bool>,
  file: null | {               // present for file objects
    spillPath: <string>,       // run-dir-relative; set when pass 2 drained an Open closure
    path: <string>,            // original absolute path (markdown loose files; not copied)
    name, url: <string>,       // url = provenance + the §16-item-5 refresh fallback input
    imageKind: <int>,
    encryptionKeys: {<string>: <string>} },
  status: "spooled" | "materialized" | "failed" }   // pass-3 consumption cursor
```

The 48 MB oversize guard (`persist.go:163-178`) moves its *first* check to spool time — an
oversized object fails in pass 2, hours earlier, with the same typed issue; pass 3 keeps the
check as a backstop. `status` is the pass-3 restart cursor (§8.1); it is written by the
pass-3 completion path in the same per-object transaction as the effect row (one tx, same
write-through discipline and measured cost envelope as Phase A's outcome tx).

### 6.2 Pulled forward from 08-13 Phase B (now prerequisites, unchanged in shape)

- **`claims`/`entries` identity rows + `payloads`** (08-13 §4.2): pass 3 after a restart
  must mint nothing — the seed argument (08-13 §1) now applies within a single logical run
  whose halves may be separated by a process restart. Claims batched 500/tx as specified and
  measured.
- **`issues`**: pass-2 issues must survive to the pass-3 report page. Capped at `IssueCap`
  as in memory.
- **`kv`**: plan JSON, `RootSpec`, fetch-complete marker, spool sequence high-water mark.

### 6.3 Versioning: the spool is NOT frozen

The §4.4 frozen core stays exactly its thirteen fields. Spool, claims, payloads, issues, kv
carry no cross-version promise: `kv.spoolVersion` = writer's `SchemaVersion`; a mismatch at
materialize time means the run cannot continue — compensate whatever pass 3 did (possibly
nothing), log loudly, drop the dir. This is the deliberate answer to §3.2's versioning cost:
an ephemeral store may simply refuse, where the frozen core must forever read. An unknown
manifest state read by an older binary already lands in the sweep's compensate-by-default
branch (`sweep.go` switch — states not terminal/newer fall through), which is the safe
outcome for both new states below.

### 6.4 Manifest states

`running → fetching → fetched → materializing → {completed | failed | suspended |
compensating}`. `fetching`/`fetched`/`materializing` are new **values** in the frozen
`state` field — additive, and safe under old readers per the default-compensate sweep
branch (verified reasoning above; pinned by a test in the phase plan).

## 7. Compensation, cancel, suspend — per pass

- **Abort/cancel during passes 1–2** (the dominant hours): nothing is in the space.
  Compensation is `Drop()` — instant, total, and `ProcessCancel` becomes as cheap as the
  requirement wants it. The effect ledger is empty by construction; `CompensationInputs`
  returns nothing; the shipped machinery handles it with zero special cases. What driver 1
  still needs — precise deletion in an existing space — is scoped to pass 3, where the
  Phase-A journal (write-through, detached writes, minted-sticky merge) continues verbatim.
- **Crash during pass 3** (window: minutes, still non-atomic): exactly Phase A's story —
  the sweep compensates from the effect ledger. With §8.1, the sweep instead *finishes*
  pass 3 when the run is resumable; compensation remains the fallback (mode ALL_OR_NOTHING
  aborts, unresumable spool, attempts exhausted).
- **Suspend**: unchanged mechanics (engine verdict, `Result.Suspended`, dir kept).
  Suspend during pass 3 → resume pass 3 (§8.1). Suspend during pass 2 → resume the crawl
  (§8.3); until §8.3 ships, a suspended mid-crawl run is swept as today (compensate —
  trivially nothing — and drop): the crawl is lost but nothing is ever wrong.
- **Cancel during finalize** (outstanding CONFIRMED finding, §13.1): finalize still exists
  and still runs last, so the fix is required by this design, not obsoleted: the engine's
  success path must re-check the run context before declaring success (the post-stream
  guard repeated after finalize/reconcile). Specified as part of DM-1 (§9).

## 8. Resume, reworked

### 8.1 Pass-3 restart — the first resume that ships, and the cheapest

A run dir in `fetched` or `materializing` is self-sufficient: spool + claims + payloads +
manifest flags (spaceId, mode, updateExisting, noCollection) are everything pass 3 consumes.
Notably **it needs no source and no credentials** — the sweep can finish a materialization
headlessly without the Notion token ever being stored, so OQ2 (token at rest) stays entirely
avoided for this resume class. Restart algorithm: rehydrate identity from claims/payloads
(the 08-13 §6.2 rehydration, minus converter concerns), skip spool rows with terminal
`status`, replay the rest; `ErrTreeExists` heal-by-update (08-13 §6.2, D4) covers the
crash-window row whose tree exists but whose status write was lost. Sweep table gains:

```
state fetched | materializing  → re-run pass 3 from the dir (attempts-capped;
                                 exhaustion → compensate + drop, as 08-13 §6.1)
```

### 8.2 What this does to the old Phase C payoff

08-13's driver-2 resume (skip-set re-crawl) attacked "don't lose the crawl". The spool
attacks it more directly: once pass 2 completes, the crawl can never be lost to a crash
again — the expensive artifact is on disk. The skip-set machinery (`ResumableConverter`)
survives with a smaller job: resuming an *interrupted* crawl (§8.3).

### 8.3 Pass-2 crawl resume (later phase)

Suspend/crash mid-crawl → re-run pass 2 with `Skip(sourceKey)` ≔ "already spooled" (the
spool itself is the skip set — no separate status bookkeeping). Notion re-search (~1 req /
100 entities) then skips fetching spooled pages. This needs the request/token (OQ2 returns,
scoped to this phase only) and converter cooperation (the 08-13 §6.3 seam, unchanged).

## 9. Phase plan, reshaped

- **DM-1 — the split itself** (replaces 08-13 Phase B): claims/payloads/issues durable
  (08-13 §4.2/§5.2 as specified); spool collection + spool sink + spool reader; engine
  restructure `identityPass → spoolPass → materializePass`; manifest states; pass-3 gauge +
  pass-2 residency tests; golden harness runs every fixture through spool→materialize
  (byte-identical object sets vs today's direct path — the equivalence gate); **plus the
  outstanding fixes that interact** (§13): finalize-cancel guard, finishRun leak-keep,
  identityPass suspend verdict.
- **DM-2 — pass-3 restart + sweep branch** (§8.1). Crash tests: kill during pass 3 at
  create/upload/finalize boundaries; resume; assert final object set identical.
- **DM-3 — pass-2 crawl resume** (§8.3; the old Phase C, shrunk). OQ2 decision needed here
  and not before.
- **Phase D — unchanged** (operator surface, notifications, config knobs), plus the §13.4
  lifecycle test harness which should land *early* in DM-1, not at the end.

Markdown goes through the spool too — one pipeline, one golden harness, one equivalence
proof; the cost to a fast local import is seconds (§10) and the clean-space property is as
desirable there as for Notion. A per-format split (spool for notion, direct for markdown)
is rejected alternative R4.

## 10. Costs — what this is worse at, honestly

- **Disk, worst case**: spool ≈ 2× serialized snapshot bytes (measured overhead factor,
  08-13 §8) + attachments (spill — same bytes as today, same place) + payloads (~180 MB per
  100k objects, measured) + the effect ledger. A 100k-object markdown import: roughly
  1–4 GB spool where today's peak was the ~180 MB ledger + spill. Bounded by O(source),
  deleted with the dir, and the never-shrinks property is irrelevant to a dir that is
  removed whole. A pathological source (huge attachments) costs what the attachments cost —
  true today as well. Disk-full during pass 2 → fatal `IssueStoreError`, drop; nothing to
  compensate. Worth stating: this is the first time the run dir is *expected* to be large.
- **Double handling**: every snapshot is serialized once and parsed once more. Measured
  small-row rates (15k+ tx/s) and disk bandwidth for large rows put this at seconds per
  100k objects — but it is pure overhead for small imports: a 50-page markdown import pays
  a spool round-trip it does not need. Accepted for pipeline uniformity (R4).
- **Failure surfacing moves late.** Today a broken import fails visibly in its first
  minute of persisting; now store-side failures (quota-adjacent, space read-only,
  local-store trouble) surface only in pass 3, after hours of crawl. Mitigation: the crawl
  artifact survives (§8.1) — the user retries materialization, not the crawl. But the
  first-failure latency is genuinely worse and should be named in review.
- **Dedup staleness for minted-class claims widens.** Pass-1 `sourceFilePath` matching
  happens at claim time, as today — but create now happens hours later. An object the user
  creates *during the crawl* that would have matched is missed → duplicate on materialize.
  Derived-class matching actually *improves* (assigned at materialize, fresher). Edge case,
  accepted, noted.
- **A half-crawled run holds the user's content on disk unencrypted** exactly as the spill
  dir already does today (downloads sat in the temp/spill dir before; snapshots now join
  them). Same trust domain as the objectstore (08-13 §4.1 argument); flagged, not solved —
  it is not a new class, but it is more bytes of it.
- **What the user sees**: pass 2 reports phase "Fetching content" with per-entity progress
  ticks (the `Reporter.Phase`/`Step` seam, `engine.go:40-44`, already exists); pass 3
  reports "Creating objects". Total = 2× claims so the bar moves through both passes; the
  wire `process` surface needs no proto change, but whether clients *display* the phase
  string is OQ-DM3 (§11). During pass 2 the space shows nothing — that is the feature — and
  the import surfaces only in the progress process.

## 11. Open questions

- **OQ-DM1 — spool row granularity for huge objects.** One row per object with a 48 MB cap
  is simple and verified safe (fact 2); if profiling ever shows large-row parse spikes in
  pass 3, chunked snapshot storage is the escape. Not designed now.
- **OQ-DM2 — should pass 3 start automatically?** Default: yes, immediately after
  `fetched`, same run, no user gate. A "review before materialize" gate (user confirms
  after seeing the fetch summary) is a product option this architecture makes cheap —
  flagged for product, not designed.
- **OQ-DM3 — progress UX.** Whether clients render the phase name, and whether a
  long-fetching import needs an intermediate notification ("fetched, creating objects").
  Client-facing; phase D territory.
- **OQ-DM4 — spool of updated-object targets.** `updateExisting` runs reset existing
  objects in pass 3; the half-built-window argument applies to *modifications* too
  (updates trickling over hours today → minutes under this design). No extra machinery
  needed, but the review should confirm updates need no staging beyond this.
- **OQ2 (08-13) narrows**: token at rest is now needed only for pass-2 crawl resume (DM-3),
  not for the flagship pass-3 restart. OQ1 (auto-resume UX) now splits per pass with
  different stakes (resuming pass 3 is invisible and safe; resuming a crawl re-opens OQ2).
- **OQ8 (08-13) grows teeth**: run dirs are now large (§10), so the sweepAttempts/age bound
  design should be scheduled with DM-2 rather than indefinitely deferred.

## 12. Rejected alternatives

- **R1 — files-first two-scan pass 3** (upload all files, then materialize objects with a
  complete id map; eliminates futures). Rejected: it removes a mechanism that is already
  built, tested, and deadlock-argued, in exchange for a second spool scan, a restructured
  order, and a new invariant to argue. The spool-reader-as-Converter design gets pass 3 for
  ~zero new pipeline code; R1 spends code to delete working code.
- **R2 — flat-file spool (length-prefixed protobuf log) instead of any-store collections.**
  Cheaper writes, but: no crash-safe partial-write story without inventing framing+fsync
  discipline, no per-row status for the pass-3 cursor without a sidecar, and a second
  storage idiom in a package that just standardized on one. The measured any-store rates
  make the performance argument for R2 moot.
- **R3 — materialize into a hidden/staging area of the space** (archived flags, a staging
  space, deferred indexing). Rejected outright: CRDT trees sync from creation — "hidden"
  objects still hit sync traffic, quota, and other devices; a staging *space* doubles every
  id and requires cross-space moves (a heavier machine than the spool); and any flag-based
  hiding is one client bug away from visible half-built state. The only clean space is one
  you haven't written to.
- **R4 — per-format split** (markdown direct, notion spooled). Rejected: two pipelines to
  golden-test, two failure models, and the clean-space property silently absent for half
  the formats. The uniform cost is seconds (§10).
- **R5 — throttle-free direct persist with UI-side hiding** (client filters import-origin
  objects until done). Rejected: pushes correctness to every client, breaks on shared
  spaces (other members see the trickle), and leaves sync/quota effects in place.
- **R6 — `fileuploader`'s two-phase preload as the pass-2/pass-3 file seam** (raised and
  rejected by the requester; recorded so it is not re-litigated). The mechanism is real and
  looks like a natural fit: `Preload()` uploads a file to storage and returns a preloadId
  *without* creating the object, and a later `Upload()` with `SetPreloadId()` commits the
  batch and creates the file object (`core/files/fileuploader/uploader.go:12-22` — the
  documented "upload speculatively, discard if not needed" flow). On its face that maps
  onto pass 2 (preload during the crawl) / pass 3 (commit at materialize), and would
  overlap the encrypt-and-chunk work with the crawl. Three reasons it is the wrong tool:
  1. *It breaks pass 2's invariant.* Preload writes into the account's real file storage —
     the blocks occupy account storage and can begin syncing. Pass 2's whole point is
     "nothing outside the run dir is touched"; preload weakens that to "no *objects*
     appear, but the blocks do." It also re-introduces the cleanup problem this design
     eliminates: abandoning a run would mean tracking and discarding preloaded batches
     instead of a single `os.RemoveAll(runDir)`.
  2. *The bookkeeping is process-local.* `GetPreloadResult(preloadId)` /
     `RemovePreloadResult(preloadId)` (`uploader.go:83-84`) are backed by the in-memory
     `preloadEntry` map (`uploader.go:88-93`); a restart loses the preloadId → `AddResult`
     mapping, so pass 3 could no longer materialize what pass 2 preloaded. That directly
     defeats DM-2's headline property — pass 3 restarting from the run dir alone, with no
     source and no token.
  3. *Consequently it is self-defeating.* Because of (2), the bytes must be kept on local
     disk anyway as the restart fallback — the design would carry both mechanisms and gain
     nothing over the spill dir it already has.
  The one thing preload would buy — overlapping encrypt/chunk with the crawl — is a poor
  trade: pass 3 is already minutes and uploads are local-first (ImportV2Design.md:934-939).
  The accepted design stands as specified (§4.3): download to `spill/` during pass 2;
  upload *and* object creation together in pass 3.

## 13. Outstanding review findings, accounted for

These are unfixed as of `ff8d37db3`; none is obsoleted by this design, three are folded
into DM-1 because the redesign touches the same code:

1. **Cancel during finalize** (CONFIRMED; IGNORE_ERRORS + cancel after the post-stream
   guard → `Err=nil`, wire `Import_NULL`, zero compensation, dir dropped as `completed`).
   Finalize survives this redesign at the end of pass 3 → the fix is in DM-1: repeat the
   dead-runCtx guard before the final success return, so no run whose context died can
   build a success result. Pinned by an IGNORE_ERRORS finalize-cancel test.
2. **`finishRun` drops the dir on leaked in-process compensation** (`runlifecycle.go:85-94`)
   — the sweep keeps the dir on `Leaked > 0`, the engine path must too (state
   `compensating`, dir kept, sweep retries). DM-1.
3. **`identityPass` early return never sets `Suspended`** → shutdown during pass 1 fires a
   spurious cancelled notification. The pass restructure in DM-1 rewrites exactly this
   return path; the suspend verdict check must cover it (and now also the pass-2 return
   path it creates).
4. **No lifecycle test harness** — no test calls `service.Close()` or `sweepAbandoned()`,
   none runs two imports concurrently; every shutdown finding so far has been reasoned-only.
   This design *increases* the lifecycle surface (two more manifest states, two resume
   classes), so DM-1 ships a harness before the states multiply: an adapter-level fixture
   constructing `service` with fakes (the `runlifecycle_test.go` pattern scaled up — fake
   space service, fake block service, real runstore), driving `Import` + `Close` mid-pass
   for each pass, asserting manifest state + dir fate + notification behavior; plus a
   two-concurrent-imports test (independent run dirs, shared registry) and a
   `sweepAbandoned`-through-the-service test.
5. **Registry is a set, not a refcount; a leaked entry blocks sweeping forever.** The
   entry's lifetime is already tied to `Store` open/close, so a leak requires a leaked
   Store (a bug), but the design needs a backstop since spool dirs are now large: fold into
   OQ8's age bound — an "active" dir older than the bound is logged loudly and left (never
   deleted under a live writer; loud is the point). Also: `Close` can still block up to
   30 s behind a sweep delete that ignores ctx — bounded to one dir's compensation by the
   between-dirs check; the real fix is the ctx-threaded delete seam, which DM-2's
   delete-by-`FullID` change (recommended in the review reply) should carry.
6. **Merge rules drop a same-key effect with a different objectId.** Today unreachable;
   under pass-3 restart it would indicate an identity-ledger violation. DM-1: the entries
   merge, on seeing a *different* objectId for an existing key, records the new id under a
   synthetic key (`sourceKey + "#dup" + n`) so the delete set holds both, and reports a
   loud invariant issue — never silently prefer either id.
7. **`journal.go:34-36` comment overstates in-process coverage** ("the in-memory record is
   kept, so in-process compensation still covers the effect"): true on abort paths, false
   under suspend (compensation is gated on the suspend verdict) — post-P0-1 the exposure
   needs a failed *detached* write (disk-full-shaped) and is one object, the same magnitude
   as §10's disclosed window. The comment gets the qualifier next time persist is touched
   (DM-1 touches it).

## 14. Performance summary

| Quantity | Value | Basis |
|---|---|---|
| Half-built-space window | hours → minutes (pass 3 at 50–200 obj/s est.) | 08-13 §8 demand analysis; persist-bound |
| Spool write rate vs producers | 15k+ tx/s small rows / disk-bandwidth large vs 1.5 obj/s (Notion) or convert-speed (md) | measured, 08-13 §8 |
| Spool round-trip overhead, 100k objects | seconds (write + read once) | measured rates + 2× size factor |
| Spool size | ~2× snapshot bytes + attachments; 1–4 GB per 100k md objects worst case | measured overhead factor; §10 |
| Pass-2 memory | O(1) objects + ~4 streaming downloads | §5.3; new residency test |
| Pass-3 memory | `2C + K` unchanged | gauge test retargeted to spool reader |
| Cancel during crawl | O(1): drop dir | §7 |
