# ImportV2 Durable Runs — Per-Run any-store Ledger, Crash Compensation, Resume

Status: design, 2026-08-13. Builds on `docs/ImportV2Design.md` — this is the work §7 explicitly
deferred ("Crash-during-import leaves orphans exactly as v1 does; a persisted journal for crash
recovery is noted as future work, out of scope", ImportV2Design.md:346-347) and the disk-backed
payload store §4 already reserved an interface seat for ("in-memory map by default, with a
disk-spill implementation behind the same interface", ImportV2Design.md:222-228). Nothing here
relitigates the engine architecture: the streaming pipeline, the identity classes, the single
runCtx, the compensation-not-rollback failure story all stand. This spec makes the run *state*
survive the process.

Decided constraint from the requester, worked within: the durable state lives in a **new,
dedicated any-store database** — not a collection in an existing DB — because disposal must be
file deletion, not tombstones awaiting a vacuum.

## 1. Why — three drivers, all real

**Driver 1 — imports target existing spaces, so cleanup must be precise.** "Throw away the
space" is not available: an import can land in a space with years of user data. Compensation
already exists and is precise (`persist/journal.go:77-92` deletes exactly what the run created,
newest first; pre-existing deduped files are never touched, journal.go:73-76) — but the journal
is a plain in-memory struct (`persist/journal.go:13-24`), built fresh per run
(`adapter/adapter.go:371`). A crash loses the only record of what this run put into the user's
space. The orphans are then indistinguishable from user data forever.

**Driver 2 — hour-long Notion runs cannot survive a laptop lid-close.** Notion throughput is
capped at 3 rps by the shared pacer with ~2 requests per page (`notion/prefetch.go:11-16`,
ImportV2Design.md:852-859): a 5,000-page workspace is ~55 minutes of crawl, 10,000 pages is
~110. There is no checkpoint anywhere — the identity index, the create-payload store, the
journal and all converter state are process memory (`identity/service.go:75-99`). A crash at
minute 100 forces a full re-crawl from zero *and* (driver 1) leaves unattributable orphans.

**Driver 3 — server-side imports block deploys.** Imports also run in a headless heart sidecar
driven over gRPC for hosted orgs. `adapter.Close()` cancels every in-flight run and waits up to
30 s (`adapter/adapter.go:113-126`); cancellation triggers compensation
(`engine/engine.go:565-570` classifies `context.Canceled` as `IssueCancelled`, which is fatal
→ `compensate()`), so **every redeploy destroys every in-flight import's work**. Worse, the
compensation itself is budgeted at 5 minutes (`engine/engine.go:35`) against a 30 s close grace
— a large abort can't even finish cleaning up before the process is gone. The server currently
cannot be restarted while any import runs. That is a hard operational blocker.

One fact forces the shape of the whole design. Minted tree ids are the hash of a root change
that contains **random seed bytes** ("Seed is a random bytes to make root change unique",
any-sync@v0.12.16 `commonspace/object/tree/treechangeproto/treechange.pb.go:87-88`; minted via
`space.CreateTreePayload`, `identity/service.go:121-133`). Re-minting after a restart therefore
yields *different* ids — while every object persisted before the crash carries references
already rewritten to the *old* ids (the resolver rewrites to final ids at persist time,
`resolve/resolver.go:58-98`). Without the identity ledger on disk, a resumed run cannot ever
reconnect to its first half. This is why resume is impossible to bolt on later and why the
identity index + payload store is in scope below, not optional.

## 2. Decision summary

| # | Decision | Where argued |
|---|---|---|
| D1 | One dedicated any-store DB **per engine run**, in its own directory next to the run's file-spill dir; disposal = `os.RemoveAll(runDir)` | §4.1 |
| D2 | Durable scope = run manifest + identity ledger (entries, create-payloads, derived memo) + effect/outcome ledger + files ledger + issues. The **converted object stream is deliberately not persisted** | §3 |
| D3 | The in-memory bounded channels stay; the durable layer is a write-through ledger, **not** an absorbing queue. The `C + K` memory invariant and end-to-end backpressure are unchanged | §3.2, §5.5 |
| D4 | Resume = reload ledger, re-run both passes with a **skip set** (persisted source keys); converters gain one optional, advisory seam. Replay of unfinished creates rides the existing `ErrTreeExists` fallback, upgraded to a heal-by-update on resumed incarnations | §6 |
| D5 | `Close()` **suspends** instead of cancel-and-compensate: flush, mark `suspended`, return. User cancel (`ProcessCancel`) keeps compensating, now durably (`cancelling` state survives a crash mid-cleanup) | §6.4, §6.5 |
| D6 | Startup sweep in the adapter: enumerate run dirs, then per manifest state — finish deleting, finish compensating, or resume (capped attempts) | §6.1 |
| D7 | Effect rows are write-through (one tx per persisted object); only pass-1 claims are batched (their loss is harmless — no side effects exist yet) | §5.2, §8 |
| D8 | Compensation-critical fields are a **version-frozen core** (additive-only evolution), so a future schema bump can still clean up an old run | §4.4, §7.3 |
| D9 | Volatile mode remains: `Deps.RunStore == nil` keeps today's in-memory journal/identity for tests, the golden harness and any sync caller | §5.1 |

## 3. What becomes durable — and what deliberately does not

The task named three candidate kinds of state. Verdicts:

### 3.1 In scope

**(a) The run journal / effect ledger** — required by drivers 1 and 3. Without it, crash =
permanent orphans in a user's space. Cost: one small row-write per effect, at persist rates
(§8). This is the §7 "future work" made real.

**(b) The identity index + tree-payload store** — required by the seed argument in §1: resume
is *impossible* without it, and even compensation-after-crash needs it (a minted id recorded at
claim time is the only durable proof that an object found in the space belongs to this run —
the current journal records ids only *after* the effect, `persist/persist.go:214`, leaving an
unattributable window). The payload store is exactly §4's anticipated disk-spill implementation
behind the same interface (ImportV2Design.md:222-228) — same seat, now with a durability
rationale on top of the memory one. Cost: bounded and small — §4's own estimate for 100k
objects is "tens of MB" of payloads, and payload rows are deleted as trees get created (§5.3).

**(c) Enough resolver-adjacent metadata to rehydrate** the format registry
(`resolve/formats.go`), the key-adoption table (`engine/keys.go`) and resolved file futures —
all currently rebuilt from the live stream (`engine/engine.go:592-606`,
`identity/future.go:51-75`). On resume the defining objects may be skipped, so the stream can
no longer be the source; the derived and files ledgers carry the few extra fields these need
(§4.3). Cheap: O(definitions), thousands of rows at most.

### 3.2 Deliberately not in scope: the converted object stream

The tempting design — spool every post-convert `Object` to disk so a resumed run replays
snapshots instead of re-converting — is **rejected**. The arithmetic kills it:

The pipeline is blocking end-to-end with `C = 16` per lane and `K = 8` workers
(`engine/engine.go:28-33`); the converter can never be more than ~`2C + K ≈ 40` objects ahead
of persistence (that is the gauge-tested invariant, `engine/engine_test.go:470-500`). So at any
crash, the set of objects that are *converted but not persisted* — the only objects a spool
would save re-converting — is **at most ~40**. For Notion that is ~80 requests ≈ 27 seconds of
re-crawl; for local markdown it is milliseconds. Against that saving, a spool costs:
serializing `Object` including the unserializable `FileSource.Open` closure
(`object.go:102-118`), schema-versioning full snapshots on disk, O(source-size) DB growth for
large imports, and a second copy of every heavy payload transiting the DB. The expensive part
of a resumed run is the *already-persisted majority*, and those need only a skip set — one
status field on rows we already store — plus a cheap re-run of pass 1 (the `/search` crawl is
~1 request per 100 entities, ImportV2Design.md:858: ~100 requests ≈ 35 s for 10k pages).

Consequence: **the serialization problem for objects disappears**. Nothing heavier than a
root-change payload (~hundreds of bytes) and issue strings ever enters the DB. File *bytes*
never do — they stay ordinary files in the run's spill dir (§4.1), which is where the notion
converter already downloads them (`notion/files.go:20-33`, tempDir wired at
`adapter/adapter.go:320-321`) and where `materialize` already spills archive entries
(`persist/file.go:92-123`). The only change is the spill dir's *location and lifetime*: today
it is an OS temp dir removed by a `defer` (`adapter/adapter.go:245-249`) — gone (or leaked) on
crash; it moves inside the run dir so spilled bytes survive restart and are collected with it.

Also not durable, with reasons: converter-internal state (rebuilt by re-running conversion with
the skip set — except the schema plan, §6.3), progress percentages (recomputed from ledger
counts), the channels themselves (nothing in flight is durable by D3), and issues beyond the
existing `IssueCap` (same cap as memory, `request.go:55`).

## 4. The store

### 4.1 File layout — one DB per run

```
<repoPath>/importv2/runs/<runId>/
    run.db            # the dedicated any-store DB (+ run.db-wal, run.db-shm, run.db.lock)
    spill/            # file bytes: archive-entry spills + notion downloads
```

`runId` = `bson.NewObjectId().Hex()` minted by the adapter per **engine run** (a multi-path
markdown request is several sequential engine runs, `adapter/adapter.go:281-303`; each gets its
own dir — the manifest carries the full request and the path index so the sweep can also
finish the *request*, §6.2). `<repoPath>` comes from `wallet.Wallet.RepoPath()` exactly as the
objectstore provider does (`pkg/lib/datastore/anystoreprovider/provider.go:165,179`).

**Per-run DB, not one queue DB with per-run collections** — the disposal constraint decides
it, and three more properties confirm it:

1. *Disposal is `os.RemoveAll`.* Dropping a collection in a shared DB frees pages inside the
   SQLite file but never shrinks it; reclaiming space means a vacuum, which the codebase never
   runs. A per-run DB is deleted with the `RemoveSqliteFiles` file set
   (`pkg/lib/localstore/objectstore/anystorehelper/helper.go:36-54`) — O(1), zero residue.
   This is the requester's stated reason and it holds.
2. *Writer isolation.* any-store v0.4.7 has a single write connection per DB (writers
   serialize; `db.go` conn manager). Concurrent imports (driver 3: several orgs' imports in one
   sidecar) in one shared DB would serialize every ledger write across runs; per-run DBs give
   each run its own writer and leave the common `objects.db` writer — which the persist path
   itself needs for indexing — uncontended.
3. *Corruption blast radius.* The provider's corruption story is delete-and-reinit
   (`provider.go:210-244`). For a shared queue DB that would erase every run's ledger; for a
   per-run DB it costs exactly one run (§7.2).
4. *Enumeration for free.* The sweep lists directories, the same idiom as
   `ListSpaceIdsFromFilesystem` (`provider.go:446-458`). No registry to keep consistent.

Cost accepted: N concurrently-open DBs (each with WAL + read conns). N = concurrent imports, in
practice single digits; connections are configured minimal (`ReadConnections: 1` — recovery is
the only reader besides the writer path).

The DB is opened with the app's standard config plus durability, mirroring the provider
(`provider.go:263-274, 383-396`): `synchronous=normal`, WAL, `Durability{AutoFlush: true,
IdleAfter: 20s, FlushMode: CheckpointPassive, Sentinel: true}`. Sentinel gives us the
dirty-detection + quick-check on reopen that the sweep relies on.

### 4.2 Collections and documents

Serialization follows the filequeue idiom exactly: hand-written `marshal(arena, T)
*anyenc.Value` / `unmarshal(*anyenc.Value) (T, error)` pairs
(`core/files/filesync/filequeue/fileinfo.go:31-59`), a `Storage`-style wrapper with tolerant
`unmarshalOrSkip` reads (`filequeue/storage.go:54-67`), and `anystorehelper.AddIndexes` for
index setup (`helper.go:56+`). The filequeue's `Queue` scheduler is *not* reused — §5.5 keeps
the in-memory channels as the scheduler; only the storage idiom carries over. Collection names
are plain (`manifest`, `entries`, …): the namespacing that made filesync write
`"filesync/queue"` (`filesync/service.go:174`) existed because it shares the common DB; a
dedicated DB needs none.

**`manifest`** — one doc, `id = "manifest"`:

```
{ id: "manifest",
  schemaVersion: 1,            // §4.4
  runId, createdAt, updatedAt,
  state: "running" | "suspended" | "cancelling" | "compensating"
       | "completed" | "failed",
  incarnation: 1,              // ++ on every resume
  resumeAttempts: 0,           // crash-loop cap, §6.1
  spaceId, importType, mode, updateExisting, noCollection,   // engine Request mirror
  request: <serialized pb.RpcObjectImportRequest>,           // headless resume needs
                                                             //   the full request (§6.2;
                                                             //   token-at-rest → OQ2)
  pathIndex: 0,                // which path of a multi-path markdown request this run is
  converter: "Notion",
  appVersion: "<heart version>" }
```

**`entries`** — the identity index made durable; `id = sourceKey`
(`identity/service.go:62-70`'s `entry` plus outcome):

```
{ id: <sourceKey>,
  objectId,                    // minted/matched/derived final id; "" for unresolved files
  mode: "minted" | "matched" | "derived" | "file",
  status: "claimed" | "persisted" | "failed" | "skipped",
  action: "created" | "updated" | "skipped",   // persist Outcome.Action, set with persisted
  rank: <int64>,               // FROZEN; assigned at the row's FIRST write, never changed.
                               //   Phase A writes rows at effect time, so rank is effect
                               //   order; phase B's claims write rows first, making it
                               //   emission order. Root membership + newest-first delete
                               //   order both read it.
  isRootCandidate: <bool>,
  incarnation: <int> }
```

`status` transitions: `claimed` on pass-1 claim (batched, §5.2); terminal
`persisted`/`failed`/`skipped` written by the worker completion path (write-through). There is
deliberately no durable "emitted" state: for minted objects, *intent* is already durable at
claim time (the minted id exists in `entries` before any tree can be created), which closes the
attribution window that a create-then-journal order leaves open (§3.1b). `failed` is terminal
across incarnations — resume does not retry failed objects (§6.3).

Effect writes MERGE into the row, never clobber it (review finding, phase A): **minted is
sticky** — once a row says the run created the object, no later effect (a phase-B
heal-by-update re-record, say) may downgrade it out of the delete set; deletion supersedes
update-rollback for an object the run made. A `files` row's **first record wins** entirely: a
re-recorded file looks pre-existing only because the first upload indexed it.

**`payloads`** — the create-payload store, §4's disk spill; `id = objectId`:

```
{ id: <objectId>, rootRawChange: <bytes>, rootId: <string>, heads: [<string>] }
```

`treestorage.TreeStorageCreatePayload` fields (any-sync `treestorage/treestorage.go:17-21`) are
proto bytes + strings — trivially marshalable. Rows are written in the claim batch and
**deleted in the same tx that marks the entry `persisted`** (§5.3), so steady-state size is the
unpersisted remainder, shrinking to zero.

**`derived`** — the derivation memo (`identity/service.go:85-87` `derived` map) plus the
rehydration metadata from §3.1c; `id = uniqueKey.Marshal()`:

```
{ id: <uniqueKey>, objectId, internalKey, isExisting: <bool>,
  emittedKey: <string>,        // converter-emitted key → KeyTable (engine/keys.go)
  relationFormat: <int>,       // → Formats registry, when SbType == relation
  freshKey: <bool> }           // deleted-key remint (service.go:222-229) — random, so it
                               //   MUST be durable or a resumed run mints a third key
```

Every `sourceKey → derived id` registration (`service.go:191-204` registers aliases on memo
hits) also writes an `entries` row with `mode: "derived"`, so reference resolution rehydrates
from one place.

**`files`** — the file-future ledger; `id = sourceKey`:

```
{ id: <sourceKey>,
  status: "registered" | "done" | "failed",
  objectId,                    // upload result — resolves the future on rehydration
  preExisting: <bool>,         // journal.CreatedFile's owned/matched split (journal.go:38-46)
  spillPath: <string>,         // relative to runDir/spill; "" when not spilled
  name, url: <string> }        // re-materialization hints (§6.3)
```

**`issues`** — append-only, `id = seq`; the fields of `importv2.Issue` (`issue.go:81-88`) with
`Err` flattened to a string. Capped at `IssueCap` rows plus a dropped counter in `manifest`
(mirrors `engine.go:191-197`). Rehydrated on resume so the final report page and
`reconcileClaims`' issued-keys exclusion set (`engine.go:167-171`) span incarnations.

**`kv`** — small singletons: the serialized `schemaplan.Plan` (§6.3), counters snapshot
(created/updated/skipped/failed at last incarnation end — recomputable from `entries`, stored
only to make sweep reporting cheap).

### 4.3 Indexes

Minimal, because nearly all access is by primary id (write path) or full scan (recovery reads
the whole ledger once — 100k rows ≈ 25 MB, sub-second):

- `entries`: `{objectId}` — compensation and diagnostics look up by final id (the persist
  journal seam is id-keyed today, `persist.go:214,261`); `{status}` is *not* indexed — status
  scans happen once per resume and a full scan is cheaper than paying index maintenance on
  every object write (write amplification, §8).
- No other collection gets a secondary index.

### 4.4 Schema version and the frozen compensation core

`manifest.schemaVersion` (integer, starts at 1). Rules:

- A sweep finding `schemaVersion` > its own: leave the dir untouched (a newer binary owns it —
  downgrade scenario), log loudly.
- `schemaVersion` < its own: **resume is refused, compensation is guaranteed**, because the
  compensation-critical subset is version-frozen by policy: the fields
  `entries.{id, objectId, mode, status, rank}`, `files.{id, objectId, status, preExisting,
  rank}` and `manifest.{schemaVersion, state, spaceId}` may only ever gain siblings, never
  change meaning, type or name (`files.rank` is frozen too — delete order depends on it). Any
  future version can therefore always run the §6.5 compensation against any older DB. This is
  what "forward-compat story" concretely means here: we promise cleanup forever, resume only
  within a version.

Two rules make the freeze bite in both directions (review hardening):

- **Reader rule — an unrecognized `entries.mode` is DELETABLE.** An older binary reading a
  newer DB (phase-B `derived` rows, say) must still compensate those objects; only `matched`
  is exempt from deletion. The flip side is a writer obligation: a future mode that must NOT
  be deleted cannot ride an old schemaVersion — it bumps it.
- **The pin is a test, both halves.** A committed v1 fixture is checked raw (presence + anyenc
  type of every frozen field), and so is a store freshly written by the current writer —
  because the black-box reader consumes only a subset of the frozen fields, a rename of
  `entries.status` passed the entire suite until this pin existed (confirmed by review).
  Fixture regeneration refuses to overwrite an existing fixture dir.

## 5. The write path — and each invariant it must preserve

### 5.1 Wiring

New package `core/block/importv2/runstore` (fits the §12 layout: subpackages import the root,
never each other): `Create(dir) (*RunStore, error)`, `Open(dir)`, typed accessors per
collection, `Sweep(root) []RunDir`. The adapter creates the store in `runImport` before the
engine starts and threads it through `engine.Deps` as a new optional seam. Three existing
components grow a durable backing behind their current APIs:

- `persist.Journal` becomes an interface (today a concrete `*Journal` in `Deps`,
  `engine/engine.go:72`); the recording methods gain the `sourceKey` (the persister has it,
  `persist.go:140`) so ledger rows are keyed consistently. In-memory implementation stays for
  volatile mode.
- `identity.Service` gains a `Ledger` seam for entries/payloads/derived writes and a
  `NewServiceFromLedger` rehydration constructor. Its in-memory maps remain the hot-path read
  side — the DB is write-through, never read during normal streaming.
- The engine's `report()` funnel (`engine.go:188-210`) appends to `issues`.

`Deps.RunStore == nil` ⇒ everything behaves exactly as today (D9). The golden harness, gauge
test and unit fixtures don't change.

### 5.2 Pass 1 — batched claims

`identityPass` (`engine.go:218-241`) claims one identity per enumerated object; each claim
mints a payload (`service.go:121-133`). Durable writes: one `entries` row + (minted only) one
`payloads` row per claim, **batched ~500 claims per write-tx** using any-store's ctx-carried
transactions (`tx := db.WriteTx(ctx)`; collection calls against `tx.Context()` join it via
savepoints — `any-store@v0.4.7/db.go:244,448-462`). A crash inside an unflushed batch loses at
most 500 claims *that had no side effects* — the resumed pass 1 simply re-mints them. That is
the entire crash cost of batching here, which is why claims are the one thing batched (D7).

100k claims ⇒ ~200 commits: seconds, not minutes (§8).

### 5.3 Pass 2 — write-through outcomes

Per object, on the worker completion path (`engine.go:326-356` `process()`), **one write-tx**
containing: `entries.status` → terminal + `action` + `incarnation`, and for creates the
`payloads` row delete; for file objects additionally the `files` row (`status: "done"`,
`objectId`, `preExisting`) replacing today's `journal.CreatedFile` call (`persist/file.go:59`).
For file objects an intent row (`files.status: "registered"`, spill path if any) is written
*before* the upload starts — the upload is the one effect whose resulting id is not known in
advance, so it is the one place needing explicit write-ahead intent. The residual window (crash
after upload, before the outcome row) is closed by content-addressed convergence: the resumed
run re-uploads the same bytes and receives the same object id (dedup inside
`UploadFile`/`CreateFromImport`, ImportV2Design.md:272-274), after which the ledger owns it.
Only the crash-then-*discard* path can leak that one file, and the design keeps the codebase's
explicit bias — leak, never delete user data (`persist.go:59-63`, `journal.go:73-76`).

**Effect writes never ride the run context** (P0 review finding, confirmed by execution: 20/20
ledger writes failed on a cancelled ctx). The effect has already happened in the user's space,
and the run context dies exactly at shutdown — which is exactly when the record matters most,
because the next start's sweep compensates *from the ledger*. The journal's record methods
therefore take no caller context at all; every ledger write runs detached, bounded by a 10 s
timeout (measured cost is sub-millisecond, §8).

Updated objects: `entries.action = "updated"` replaces `journal.UpdatedObject`
(`persist.go:261`); they remain uncompensated by decision §13.3 (ImportV2Design.md:765-768) and
are reported as uncovered, exactly as `Journal.Compensate` does today (`journal.go:63-92`).
Favorite/archive flags are applied but not journaled today (`persist.go:307-318` vs the §7
aspirational table) — the durable journal keeps parity with the *shipped* journal, not the
aspiration; flag compensation stays out (for created objects deletion subsumes it; for updated
ones it joins the uncovered report).

### 5.4 Invariant-by-invariant

**§2 converter contract (rules 1–5, `converter.go:13-25`).** Unchanged for every existing
converter — the ledger writes happen on the engine side of the sink. Rule 5 (determinism) gains
one *cross-incarnation* clause, stated in §6.3: given an unchanged source, re-running must
yield the same claims and the same definitions. Both shipped converters already satisfy it
(their §2-rule-5 determinism is per-run; nothing in them is seeded per-process) — except the
LLM plan step, which is exactly why the plan is persisted and reused (§6.3).

**§3 file futures / deadlock freedom (ImportV2Design.md:185-193).** The argument rests on FIFO
worker pull plus the dedicated file lane (`engine.go:266-277`). Neither changes — the durable
layer adds no scheduling. On resume, futures for `files.status == "done"` are rehydrated
already-resolved (they cannot deadlock); unfinished files are re-emitted by the converter in
stream order, re-establishing the same FIFO precedence within the new incarnation. A file whose
*emission site* is skipped (its defining page persisted) is by construction resolved: an object
persists only after every file it references resolved (`resolver.go:72-98` waits;
`process()` completes futures before accounting, `engine.go:335-337`) — so "referencer
persisted, file pending" cannot survive a crash in the dangerous direction.

**§4 identity semantics.** Match order, revision guard, deleted-key remint all execute
identically; the ledger is write-through behind them. The one semantic *improvement*: the
deleted-key fresh mint (`service.go:222-229`, random bson id) becomes stable across
incarnations because the memo row survives.

**§5 the `C + K` memory invariant (gauge test `engine_test.go:470-500`).** Preserved trivially:
the durable layer holds no heavy objects (D2/D3), channels and worker counts are untouched. The
gauge test runs in volatile mode unchanged; a new variant runs with a real `RunStore` and
asserts the same bound, pinning that the ledger never becomes an accidental buffer.

**§6 single-runCtx cancellation.** One context still governs everything; §6.4 adds a *cause* to
it (`context.WithCancelCause`) so the abort path can distinguish suspend from user cancel from
fatal — one mechanism, one new bit of information, no second channel.

**§7 compensation.** `Compensate` reads the ledger instead of the in-memory slices: delete
targets = `entries` rows with `mode ∈ {minted, derived-new}` (newest-first by `rank`, matching
`journal.go:85-89`) plus `files` rows with `preExisting == false`; matched rows are untouchable
by construction. After a crash, rows with non-terminal status are *possible* creates
(crash mid-`CreateTreeObjectWithPayload`): compensation attempts deletion and treats
not-found as success (idempotent — required anyway so that a crash *during* compensation can
re-run it, §6.5).

**`reconcileClaims` (`engine.go:423-445`).** The invariant — every claim persisted, skipped, or
loudly issued — now spans incarnations: `entries.status` and the rehydrated issued-keys set
provide the cross-incarnation inputs. One genuinely new case appears: a claim from a previous
incarnation whose entity no longer enumerates (source drifted — a Notion page deleted mid-run).
Under the shipped rule that's a fatal-under-ALL_OR_NOTHING invariant violation; on a resumed
run it is expected drift, not a converter bug. Rule: a stale claim with **no effects**
(`status == "claimed"`, mode minted-new) is dropped with a `Warning(dataLoss)` ("source entity
disappeared between sessions"); a stale claim already persisted keeps its object and its
count. The invariant's teeth — a *silent* converter drop is still fatal — remain for claims
made within the current incarnation.

### 5.5 Backpressure — the queue stays in memory, on purpose

A durable queue *could* absorb an unbounded convert-ahead window without blocking. Rejected.
The blocking channel is what makes a slow store stall the converter and a slow Notion API
starve the pool with neither accumulating (ImportV2Design.md:245-248) — for Notion, persistence
is far faster than the 3 rps crawl, so decoupling buys nothing; for local markdown, decoupling
would just convert the whole source onto disk ahead of persistence, reintroducing v1's
batch-shaped residency as *disk* growth (O(source) snapshots) and breaking the file-future FIFO
argument above, in exchange for zero end-to-end speedup (the store is the bottleneck there, and
it is not made faster by queueing in front of it). What bounds on-disk growth for a very large
import is therefore the same thing that bounds memory: the channels. Ledger rows are O(objects)
but tiny (§8); payload rows shrink to zero as trees are created; spill bytes are bounded by
source size and are the same bytes the run spills today, relocated.

## 6. Resume and recovery

### 6.1 Startup sweep

Runs in the adapter's `Run()` (component start), in a background goroutine joined into the
run waitgroup (Close waits for it) and gated by the same `componentCtx` the runs use
(`adapter.go:106`) — checked between dirs, so an account stop mid-sweep stops the walk
instead of deleting through a closing service. For each `<repo>/importv2/runs/<dir>`:

```
dir held open by a live Store  → SKIP (active-run guard, below)
open run.db                    → on corruption: delete dir, loud "objects may be
                                 orphaned" log. Leak, documented. (§7.2)
                                 Corruption is classified NARROWLY: anystore's
                                 quick-check/version sentinels + the go-sqlite fork's
                                 CORRUPT/NOTADB codes. CANTOPEN is deliberately NOT
                                 corruption (EACCES, fd exhaustion, disk-full — an
                                 intact ledger must survive a permission hiccup);
                                 an unreadable db → SKIP, retry next start
open ok, no manifest doc       → delete dir (creation crashed before the first write:
                                 nothing recorded ⇒ nothing done)
read manifest
  schemaVersion > ours         → skip (log)                                  (§4.4)
  state completed | failed     → delete dir            (crash between finish & cleanup)
  state cancelling
      | compensating           → run compensation from ledger; delete dir only if
                                 nothing leaked — a partial compensation keeps the dir
                                 (state stays compensating) so the next start retries:
                                 compensation is idempotent, retry is free, and
                                 dropping the dir would make the leak permanent
  state running | suspended    →
      resumeAttempts ≥ 3       → compensate, fail                            (phase B)
      space missing/deleted    → delete dir (nothing to compensate into; only a
                                 DEFINITIVE not-exists/deleted/storage-missing answer
                                 counts — any other probe error skips the dir)
      source unavailable       → compensate, fail                            (OQ4, phase B)
      else                     → resume (§6.2)          (phase B; phase A compensates,
                                                         same branch as cancelling)
```

Phase A reports each settled run as one structured log line on the import-v2 scope; a
user-facing notification is deliberately deferred to phase D — no existing wire code means
"cleaned up after a crash", and inventing one is client-facing surface.

`state == "running"` at sweep time **is** the crash detector: a live process moves its runs
through `suspended`/`cancelling`/terminal before exiting; only a dead one leaves `running`
behind. `state == "cancelling"` is the durable record that *the user said stop* — written
before compensation starts (§6.5) — which is exactly the crash-vs-cancel distinction the
drivers demand: a cancelled run is never resumed, only finished being cleaned up.

**The active-run guard is a process-global dir registry in runstore** (marked on open,
released on Close/Drop), not a per-component map: the confirmed hazard is a same-process
account stop/start where Close's 30 s grace gave up on a run still finishing while the NEW
component instance sweeps — and the db's `.lock` is a dirty *sentinel*, not a mutex, so a
second Open of a live run's db succeeds and Drop would unlink the dir under the live writer
(whose subsequent writes keep succeeding, into an unlinked file). Cross-process exclusivity
remains the platform invariant (one heart per repo dir). Run DBs are self-describing, so
enumeration/inspection needs no session — a future `ImportRunList` RPC is a read-only walk
over manifests (server operators' observability; noted, not designed here).

### 6.2 Resume — what is rebuilt, what is replayed, what is skipped

Resume constructs the run exactly like `runImport` does (`adapter.go:155-186`), except:

1. **Request** comes from the manifest (`request` blob), not a live RPC — this is the headless
   path: no client present, notification/event delivery at the end works as for any async run.
   Multi-path markdown: the manifest's `pathIndex` resumes the current engine run; remaining
   paths of the request run fresh afterwards, as `executeMarkdown`'s loop does today
   (`adapter.go:281-303`).
2. **Identity service** is built by `NewServiceFromLedger`: `entries` → the index (with
   `claimed`/`assigned` reconstructed from status), `payloads` → the payload store, `derived` →
   memo + `KeyTable` + `Formats` seeds, `files` → futures (`done` rows pre-resolved with their
   `objectId`; `failed` rows pre-resolved with an error, so references degrade identically to
   the first incarnation).
3. **Counters and root membership** rehydrate from `entries` (`action` counts;
   `isRootCandidate` ordered by `rank`), the issue ledger refills `issues`/`issuedKeys`, and
   the progress process reports total = claims, done = terminal rows — the bar resumes where
   it stopped.
4. **Pass 1 re-runs** against the live source. Claims whose sourceKey already has an `entries`
   row are no-ops (reuse the minted id — no re-mint, no dedup re-query); new keys claim
   normally (source gained entities — they import); missing keys are the §5.4 stale-claim
   case.
5. **Pass 2 re-runs with the skip set**: `Skip(sourceKey) bool` ≔ `entries.status ==
   "persisted"` (or `failed`/`skipped` — terminal). Delivered to converters as an optional
   capability (§6.3). The engine also enforces it at the sink as the backstop: a re-emitted
   object whose entry is terminal is acknowledged (`Reporter.Step`) and dropped without
   enqueue or recount — so a converter that ignores the seam entirely is merely slower, never
   incorrect.

**Replay of the unfinished (≤ ~2C+K objects).** These re-convert and re-persist. The existing
`ErrTreeExists` fallback (`persist/persist.go:220-233`) gives replay idempotency for the
crash-mid-create case — but the task's question "how far does that actually get you" has a
sharp answer: **not far enough alone**. The fallback *reads the existing object and reports
Skipped*, which is correct for its designed case (index-lag races) but wrong for resume twice
over: (a) a crash between the tree write and the state application can leave a tree with
partial state, which skip-and-read would silently keep hollow; (b) `ActionSkipped`
mis-attributes an object this run created, which would corrupt compensation accounting if the
ledger didn't already know better. Fix, scoped to resumed incarnations: when
`CreateTreeObjectWithPayload` returns `ErrTreeExists` for an entry with `mode == "minted"` and
non-terminal status, route through the update path (`updateObject`'s
`history.ResetToVersion`, `persist.go:240-275` — revision guard and all) to heal partial state,
and record `action: "created"` (the ledger's mode proves this run made it; compensation
attribution stays exact — driver 1). Half-uploaded files re-upload from spill/source and
converge by content addressing (§5.3). Flag application is a plain re-run (`applyFlags` is
reached on both create and heal paths, `persist.go:200`, and setting favorite/archived twice
is idempotent).

### 6.3 The converter seam — and per-converter resume behavior

One addition to the engine-facing surface, advisory and optional:

```go
// importv2 (root package)
// ResumableConverter is implemented by converters that can cheaply skip
// re-converting already-persisted objects. Skip is engine-provided and
// safe for concurrent use. Purely an optimization: the engine enforces
// terminal-entry dedup at the sink regardless.
type ResumableConverter interface {
    Converter
    SetSkip(skip func(sourceKey string) bool)
}
```

- **Notion**: the payoff case. `Convert`'s page loop and the prefetch producer
  (`notion/converter.go:154-199`, `prefetch.go`) consult `Skip` per stub *before* fetching —
  each skipped page saves the 2 requests that make driver 2 hurt. Database conversion re-runs
  (schemas re-fetch — ~1 request per data source, cheap) because page rows of an unfinished
  database need its property mappings in converter memory; already-persisted collection/
  relation/option re-emissions are absorbed by the sink backstop and the derived memo
  (`AssignDerived` memo hits return `IsExisting`, `service.go:187-195`). The **schema plan is
  reused from `kv`, never recomputed**: LLM output is not deterministic across calls, and a
  second plan would mint divergent type/relation identities for the run's second half —
  the one converter-state exception to "rebuild by re-running" (cross-incarnation rule 5,
  §5.4). Second-chance discoveries (`converter.go:19-23,183-194`) re-discover naturally: their
  claims are already in `entries`, so re-adoption is a reuse, not a re-mint. File URLs from a
  previous incarnation are guaranteed-expired (~1 h signing window, §16 item 5) — irrelevant by
  construction: unfinished files are re-emitted by their re-converting page with fresh URLs, or
  uploaded from surviving spill bytes; ledger URL/name fields are diagnostics, not fetch
  sources.
- **Markdown**: may ignore the seam entirely in phase B (local re-parse is fast; sink backstop
  handles everything) and adopt it later as a pure optimization. Source paths come from the
  manifest request; a vanished path is the §6.1 "source unavailable" case.

### 6.4 Suspend — graceful shutdown without losing the run (driver 3)

`adapter.Close()` today: cancel + wait ≤ 30 s (`adapter.go:113-126`), which flows into
compensation (§1). New behavior when a run has a `RunStore`:

1. Cancel with cause `errSuspend` (`context.WithCancelCause` on the runCtx the adapter already
   owns, `adapter.go:157`).
2. The converter stops at its next ctx check; workers finish or abandon in-flight objects:
   an object interrupted mid-persist keeps its non-terminal entry and is simply redone next
   incarnation. Critically, suspension must not *bake in* degradation: a resolver file-wait
   interrupted by suspend already returns an error rather than the missing-marker
   (`resolver.go:78-82` — the degrade branch fires only when ctx is alive), and the engine's
   completion path treats a persist interrupted by cancellation as **`skipped`** — not
   `failed`, not an issue (as-built; amended from this spec's original "recorded as nothing":
   the run-level counter is the honest place for "stopped before done", and phase B keys
   resume off the durable entry status, which stays non-terminal, not off in-memory
   counters). The abort itself is accounted exactly once, by the run's fatal cancellation.
   One carve-out: a *fatal* issue that merely wraps a context error (a durable-journal write
   timing out during shutdown) is never absorbed as skipped — it aborts loudly.
3. Mark manifest `suspended`, `db.Flush(ctx, 0, CheckpointPassive)`, close the DB. No
   compensation, no notification.

The engine is the single owner of the suspend verdict, carried out as `Result.Suspended` (set
iff it skipped compensation for the suspend cause). The adapter must consume that, never
re-derive it from a context: a cancel cause is one-shot, so an inner abort followed by a Close
reads differently from the engine's inner runCtx and the adapter's outer one (confirmed
disagreement — a backwards `compensating → suspended` manifest transition).

The 30 s grace now bounds only drain + flush — sufficient, unlike today where it races a 5 min
compensation budget (§1). The sweep resumes suspended runs on next start; on a server that
means a redeploy costs an import ~seconds of replay, not an hour of work. Desktop gets the
same mechanics on app quit for free (OQ1 covers the UX question of when to auto-resume).

### 6.5 Cancel — user intent, made durable

`ProcessCancel` keeps today's semantics: cancel → fatal `IssueCancelled` → compensation
(`engine.go:565-570, 514-539`). Two durable additions: the engine marks the run
**`compensating`** *before* the first delete (as built, via the `OnCompensating` hook — the
state is named for what is happening, since any abort compensates, not only cancel;
`cancelling` stays reserved for phase B's adapter-side cancel-intent record) and `"failed"`
(terminal, with `compensated`/`leaked` counts) after — so a crash mid-cleanup is finished by
the sweep, idempotently (§5.4's not-found-is-success rule exists for exactly this). The run
dir is disposed by `finishRun` *before* the result is delivered (§7.1). "User said stop" is
thus never confused with "process died" on restart: the former is on disk as
`compensating`/`failed` before the process can die uncleanly with it.

One user-cancel subtlety the review confirmed the hard way: with lanes holding `2C+K = 40`
objects, a small import is *fully buffered* when cancel fires — the converter has already
returned cleanly and cannot report the cancellation, and interrupted persists are accounted
skipped. The engine therefore reports the cancelled fatal itself whenever the run context is
dead and no fatal exists (`engine.go`, post-stream guard); without that, a ≤40-object import
cancelled mid-flight finalized as a silent SUCCESS with zero compensation (wire `ErrorCode`
NULL — confirmed by differential run).

## 7. Lifecycle and GC

### 7.1 Nominal lifecycle

Created by the adapter immediately before the engine run (manifest written `running` first,
engine started second — an empty-but-manifested dir is sweepable garbage, an un-manifested dir
is deleted on sight). On success: manifest → `completed`, then `db.Close()` +
`os.RemoveAll(runDir)`, and only then is the notification/event delivered
(`adapter.go:169-186`) — as built, `finishRun` runs inside the per-run executor, before the
request-level delivery. Disposed-before-delivered is fine in phase A (the result is complete
in memory) but is worth stating for phase B, where anything the delivery might want from the
store must be extracted into the Result first. The DB lives exactly as long as its run — the
cheap-disposal constraint honored end to end.

### 7.2 Abnormal cases

- **Corrupted run DB** (sweep or mid-run): report, delete dir, log possible orphans loudly.
  The frozen core (§4.4) protects against *version* skew, nothing protects against lost
  bytes; the bias stays leak-over-delete. Corruption classification is narrow on purpose —
  CANTOPEN is not corruption (§6.1).
- **Mid-run ledger write failure** (disk full, IO error): the write-through calls surface a
  **fatal** `IssueStoreError` — the run aborts regardless of mode, IGNORE_ERRORS included
  (amended from "normal abort predicate applies": a run that cannot journal must not keep
  creating objects, and continue-on-error would do exactly that). The in-memory record is
  kept even when the durable write failed, so in-process compensation still covers the effect
  that just happened.
- **Runs that can never resume**: covered by the sweep table (§6.1) — space gone, source
  gone, attempts exhausted, schema too old (compensate-only).
- **Dirs CAN pile up** (retraction — this section originally claimed "no fourth state where
  dirs pile up"): the skip branches (unreadable db, transient space error, newer schema,
  active run) and the partial-compensation keep are all deliberate keeps, and a condition
  that never heals accumulates them. Bounding this needs a `sweepAttempts` counter on the
  manifest plus an age bound (e.g. give up and delete-with-loud-leak-log after N failed
  sweeps or M days) — **designed here, deliberately not built in phase A**: every keep today
  is retryable and self-heals on the next start in the common case, and choosing N/M is a
  product call (OQ8).
- **The 5-minute compensation budget is inert** (pre-existing, noted for honesty):
  `ObjectAccess.DeleteObject` takes no context, so `compensationTimeout` bounds nothing
  inside the delete loop. Making it real means threading ctx through the delete seam —
  phase B at the earliest.

### 7.3 Compatibility

`schemaVersion` bumps only for incompatible changes; additive fields don't bump (anyenc docs
tolerate unknown fields; `unmarshalOrSkip` tolerates bad rows, `storage.go:54-67`). The frozen
core contract (§4.4) is pinned by tests: a fixture DB checked in at v1 must forever pass
`runstore.CompensationInputs`, and the raw-field pin (§4.4) holds both the fixture and the
current writer to the frozen field set.

## 8. Performance

Demand side — writes per object, steady state (§5.2–5.3):

| Phase | Durable work | Commits |
|---|---|---|
| Pass 1 | 1 `entries` + ≤1 `payloads` row per claim, batched 500/tx | ~2 per 1000 claims |
| Pass 2, regular object | 1 tx: entry terminal + payload delete | 1 per object |
| Pass 2, file object | +1 intent tx before upload, outcome joins the terminal tx | 2 per file |
| Issues | 1 row per issue (≤ `IssueCap`) | ≤1000/run |

Rates: Notion is pacer-bound at ≤ ~1.5 objects/s (ImportV2Design.md:852-859) — durable load is
noise there. Local markdown persist throughput is bounded by tree creation (signing) + space
index writes; **estimate** 50–200 objects/s across the 8 workers (basis: not measured for v2;
the persist path per object does strictly more SQLite work against the *common* objects.db
than our 1–2 ledger commits do against a private DB). Worst-case ledger demand ≈ 400
commits/s.

Supply side: any-store v0.4.7 on SQLite WAL with `synchronous=normal` (the provider's own
setting, `provider.go:271`) makes a commit a WAL append with no per-commit fsync (fsync rides
the checkpoint, `wal_autocheckpoint=10000`).

**Measured, not estimated.** The Phase-A gate microbench has been run: real any-store v0.4.7,
the exact §4.1 durability config, a dedicated per-run DB, the one §4.3 secondary index, and
realistic row shapes (32-hex source keys, 59-char object ids, 550 B payload blobs). Apple M2,
three runs.

| Scenario | Measured (typical / stalled run) |
|---|---|
| Claims batched 500/tx, 100k claims | 23k–36k claims/s; 14–21 ms per 500-row commit |
| Claims unbatched 1/tx | 7.7k–9.2k claims/s — batching is a 3–4× win |
| Per-object outcome tx, 1 worker | 15.5k–19k tx/s / 5.5k |
| Per-object outcome tx, 8 workers | 15.8k–16.4k tx/s aggregate / 4.1k; p99 1.7 ms |
| 8 workers + 40 ms simulated persist (worst-case demand) | **194 obj/s vs 200 ideal = 3% overhead**, identical across all 3 runs; tx p99 2.7–6.7 ms |
| File shape (intent tx + outcome tx) | ~12k cycles/s; p99 ~4 ms |
| `Flush` (suspend path) | 4–9 ms — far inside the 30 s close grace (§5.6) |

Against the ≈400 commits/s worst-case demand above, that is **~40× headroom typically and
~10× on the worst observed run** (one ~530 ms stall, checkpoint or background load). The gate's
threshold was < 10% wall-clock overhead; the measured figure is 3%. **§5's write-through policy
therefore stands unmodified** — no batching or write-behind on the outcome path.

One property the bench exposed that the design should not have to rediscover: **any-store
v0.4.7 has a single write connection per DB, so ledger writes do not scale with worker count**
— 8 workers aggregate to roughly what 1 worker achieves, with per-tx p50 rising 43 µs → ~450 µs
from queueing. This is invisible at demand rates (the 3% row above) and is a further argument
for per-run DBs (§4.1): a shared queue DB would serialize every run's ledger writes against
every other run's.

Space — **measured ~180 MB peak per 100k objects**, against this section's original 50–100 MB
estimate: 96 MB live data after the payload drain, 160 MB total file plus WAL. anyenc, page and
index overhead run ~2× the raw-bytes estimate, which is where the original figure went wrong.
Note also that **the live data drains as payload rows are deleted but the file does not shrink**
— SQLite reuses freed pages rather than releasing them, so on-disk footprint stays at its peak
for the life of the run. That is not a problem here, and it is precisely why disposal is
`os.RemoveAll` of the run dir rather than a delete-and-vacuum inside a shared DB (§4.1): the
space is reclaimed by unlinking the file, which is the only cheap way to reclaim it at all.
`derived`/`files`/`issues` remain negligible; spill bytes are unchanged in magnitude from
today's temp dir, relocated. Write amplification from indexes: one secondary index total
(§4.3), by design.

Crash-window cost recap (the price of D7's batching decisions): ≤500 claims re-minted
(harmless), ≤~40 objects re-converted (§3.2), one file re-uploaded (§5.3). No window loses an
*effect record* for a completed effect except the single post-upload row (§5.3, accepted,
leak-biased).

## 9. Rejected alternatives

1. **One shared queue DB, per-run collections.** Fails the disposal constraint (dropped
   collections leave dead pages until a vacuum nobody runs), serializes all runs on one write
   connection, couples corruption blast radius across runs. §4.1.
2. **Reusing the common `objects.db` or a space DB.** Excluded by the requester's constraint,
   and independently wrong: run state is ephemeral by nature, and deleting it must not depend
   on compacting a DB whose lifetime is the account's.
3. **Durable converted-object spool** (persist the post-convert stream). The convert-ahead
   window is ≤ ~40 objects by the backpressure invariant, so the spool saves ~27 s of Notion
   re-crawl at the cost of serializing snapshots and `FileSource` closures, O(source) disk,
   and a snapshot schema-versioning liability. §3.2.
4. **Unbounded durable queue as the backpressure mechanism** (converter free-runs onto disk).
   Breaks the flow-control property §5 exists for, reintroduces batch-shaped growth, weakens
   the file-future FIFO argument, speeds nothing up. §5.5.
5. **Driving persist workers off `filequeue.Queue`.** The filequeue is a scheduler
   (locking, subscriptions, scheduled items — `filequeue/filequeue.go:21-55`) for a standing
   queue with many producers/consumers across restarts. Import's scheduling needs are two
   channels and FIFO order, already deadlock-argued; only the *storage idiom* is worth
   borrowing, and is. §4.2.
6. **Write-behind batching of effect rows.** A crash in the buffer leaves completed effects
   with no record — unattributable orphans, the exact failure driver 1 forbids. Only pass-1
   claims are batched because their loss provably has no effect to orphan. §5.2, D7.
7. **Resume via `updateExisting`-style dedup instead of a ledger** ("just re-import; dedup
   will converge"). Fails three ways: minted ids are seed-random (§1), so re-import creates
   duplicates of every not-yet-matched page unless `sourceFilePath` matching happens to be on
   (`identity/dedup.go:45-55` — it is off by default for pages); references in the first
   half's objects point at first-run ids that a fresh run knows nothing about; and there is
   still no journal, so driver 1 is unmet. This is the tempting "no new state" answer and it
   does not work.
8. **Compensate-on-every-restart** (crash ⇒ always clean up, never resume). Sound for driver 1,
   fails drivers 2 and 3: an hour of rate-limited crawl is destroyed by a redeploy. Kept only
   as the fallback branches of the sweep (§6.1).
9. **A parallel append-only journal collection** duplicating what `entries`+`files` already
   encode. One fewer collection and one fewer write per object by deriving compensation from
   the ledger itself; the newest-first ordering the journal provided survives as `rank`. §4.2,
   §5.4-§7.

## 10. Phased implementation plan

- **Phase A — durable journal + sweep + suspend (drivers 1 & 3, no resume).** `runstore`
  package (manifest, entries as effect ledger, files, frozen-core reader); journal seam swap;
  suspend-on-close with cause; startup sweep limited to the compensate/finish branches
  (`running`/`suspended` runs are *compensated* in this phase — resume lands in B, and until
  then suspend still beats today's cancel-compensate-race). Microbench gate (§8) — **run and
  passed** (3% overhead against a 10% threshold). **Built and review-hardened** (2026-08-13,
  three-way review): detached effect writes (§5.3), the engine-owned cancel fatal and suspend
  verdict (§6.4-§6.5), narrow corruption classification and the active-run guard (§6.1),
  merge-not-clobber effect rows (§4.2), and the frozen-field raw pin (§4.4). One honest
  bound: phase A **narrows** the unattributable-orphan window to one object per crash — the
  journal still records *after* the effect (`persist.go:214-220`-era ordering), and only
  phase B's claims-as-intent closes it. Ships value alone: no more journal-lost-on-crash, no
  more redeploy-races-compensation.
- **Phase B — identity ledger + resume for markdown (driver 2 mechanics).** Payloads/derived
  collections, `NewServiceFromLedger`, sink backstop dedup, stale-claim rule, ErrTreeExists
  heal-by-update, counters/issues rehydration, sweep resume branch with attempt cap.
  Golden-harness crash tests: kill at claim/convert/persist/finalize boundaries, resume,
  assert the final object set is byte-identical to an uninterrupted run.
- **Phase C — Notion resume (driver 2 payoff).** `ResumableConverter` seam; prefetch skip;
  plan persistence + reuse; schema re-fetch path; cassette-based crash/resume test (kill after
  N pages, assert the resumed run issues no fetches for persisted pages).
- **Phase D — server polish.** Manifest-driven run enumeration surface for operators,
  config knobs (resume attempt cap, auto-resume gate), structured sweep logging on the
  `import-v2` scope, load test with concurrent runs.

## 11. Open questions

- **OQ1 — desktop auto-resume UX.** Server-side auto-resume is required (driver 3). On
  desktop, silently resuming an hour-long import on app start matches the async fire-and-
  forget contract (the client sees a progress process appear, then the normal finish
  notification), but a user who force-quit *because of* the import would disagree. Proposed
  default: auto-resume everywhere, no new wire surface; a prompt would need client work and a
  new API. Needs a product call.
- **OQ2 — Notion token at rest.** The manifest embeds the serialized request, including the
  API key (`adapter.go:308-312`), because a headless resume has no client to re-supply it.
  The run dir sits in the account repo beside the wallet and objectstore — arguably the same
  trust domain — but today the token never touches disk at all. Options: store as-is (trust-
  domain parity), encrypt the request blob with an account-derived key, or make Notion resume
  require re-injection (killing unattended server resume for Notion specifically). Needs a
  security call before phase C.
- **OQ3 — retrying failed objects on resume.** §4.2 makes `failed` terminal across
  incarnations (avoids re-fail loops and double-issue noise). But some failures are
  transient (network flap on a file). A bounded per-object retry budget on resume is a
  possible refinement; deliberately out of v1 of this design.
- **OQ4 — non-resumable source policy.** §6.1 compensates when the source is gone. The
  alternative — *finish partial*: run finalize over what persisted (root collection, report)
  and deliver a partial-success result — is arguably better for a continue-on-error run that
  was 95% done. Proposed: compensate for ALL_OR_NOTHING (its contract is all or nothing),
  finish-partial for `IGNORE_ERRORS`. Flagging rather than deciding, since it changes
  user-visible semantics.
- **OQ5 — multi-path requests.** §6.2 resumes the in-flight engine run and then runs the
  remaining paths fresh. Alternative: treat each path as an independent durable run from the
  start (N manifests up front). Current proposal is simpler and matches the sequential
  execution shape (`adapter.go:281-303`); revisit if multi-path merging (§15 open item) ever
  lands.
- **OQ6 — sweep vs. account lifecycle.** The sweep runs at adapter start, which on desktop is
  account start. An account that logs out mid-import: componentCtx cancellation currently
  flows into Close → suspend (fine), but a *different* account logging in must not resume
  another account's runs — run dirs live under the per-account repo path, so isolation should
  hold by construction; needs a test, not a design change.
- **OQ7 — `Deps.Journal` API shape.** RESOLVED at phase-A implementation: the journal stays a
  concrete `*persist.Journal`; the seam is the narrow `persist.EffectLedger` write-through
  interface implemented structurally by `runstore.Store`; record methods carry `sourceKey`,
  return an error, and (P0 amendment) take no caller context — writes run detached (§5.3).
- **OQ8 — bounding run-dir accumulation.** The sweep's deliberate keeps (unreadable db,
  transient space error, newer schema, active run, partial compensation) can accumulate under
  a condition that never heals (§7.2). The design is a manifest `sweepAttempts` counter plus
  an age bound — give up and delete with a loud leaked-objects log after N failed sweeps or
  M days. Choosing N/M (and whether give-up should notify) is a product call; not built in
  phase A.
