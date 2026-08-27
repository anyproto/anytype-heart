# API v2 Phase 0+1 — code review synthesis (2026-07-23)

Three opus lenses over the committed branch `go-7383-apiv2-phase0`
(`go-7383-anyblockjson..HEAD`): spec-conformance, Go robustness/security,
tests + agent-contract. Deduped, cross-confirmation noted, severity-tiered.
The top-3 criticals were re-verified in source by the synthesizer before
listing (marked ✓verified).

Overall: the read/validate/plumbing surface is competently layered and
mostly spec-conformant (C4 split, C6 errors, C10 pagination, /validate
200-with-issues, live-state read under one lock all check out). But there is
**one reachable process-crash, two input-driven DoS holes, and one broken
core agent flow** (outline→drill-down) that the tests structurally cannot
see. None are architectural — all are surgical fixes.

---

## TIER 1 — critical (crash / DoS; fix before Phase 2)

### C1 — Live `store` pointer escapes the object lock → uncatchable process crash ✓verified [robustness #1]
`objectreadadapter.go:35` sets `Collections: st.Store()`.
`state.go:1664` `State.Store()` returns the **live** `*types.Struct` (walks
to parent, no copy) — unlike `BlocksToSave()` / `CombinedDetails().ToProto()`
on the adjacent lines, which copy. The lock releases at
`objectreadadapter.go:41`; `anyblockjson.Marshal` iterates that map at
`v2_object.go:171`, **outside the lock**. An agent reading a
set/collection/dataview object while a user mutates its store (add/remove a
collection item) → `fatal error: concurrent map read and map write`, which
`gin.Recovery` **cannot** catch → the whole middleware process dies.
**Fix:** deep-copy under the lock — `Collections: pbtypes.CopyStruct(st.Store(), true)`
in the adapter (mirror how blocks/details are already copied). Add a
concurrent read-vs-mutate test on a collection object.

### C2 — Arbitrary `space_id` path param mints an unbounded store + backing DB [robustness #2]
Every v2 space-scoped handler calls `s.store.SpaceIndex(c.Param("space_id"))`
directly (`v2_discovery.go:26,57,116,163,180,227`; `v2_object.go:171,354,426`).
`getOrInitSpaceIndex` inserts `spaceIndexes[spaceId] = New(...)` for **any**
non-deleted id and `Init()` opens/creates a backing DB. v1 avoids this by
routing through the space service (which rejects unknown spaces). An agent
looping `GET /v2/spaces/{random}/objects` grows an attacker-keyed,
never-evicted map plus DBs/dirs without bound — memory+disk DoS from
untrusted localhost input. **Fix:** validate `space_id` against the account's
known spaces before touching the store (or use a non-creating accessor).
Tree-path reads (`GetObject`/`GetType`) are safe; only the store path.

### C3 — Idempotency middleware reads the body unbounded, defeating the 10 MiB validate cap ✓verified [robustness #3]
`v2_middleware.go:125` `io.ReadAll(c.Request.Body)` with no limit, before the
handler. `/v2/validate` is wired with the idempotency MW (`router.go`), so
its `io.LimitReader(body, 10MiB+1)` guard is bypassed whenever an
`Idempotency-Key` is present (agents are told to send one on POSTs). No
server-level `MaxBytesReader`. Header + multi-GB body → OOM.
**Fix:** `io.ReadAll(io.LimitReader(body, cap+1))` in the MW; reject over cap
before hashing.

## TIER 2 — major (broken flow / spec violation an agent hits)

### M1 — Outline block labels don't round-trip to `?block=` — the outline→fetch loop 404s ✓verified [ALL THREE lenses: conformance #1, robustness #5, tests #1]
`v2_object.go:169` sets `CompactBlockLabels = plan.outline`, so `?outline=true`
emits 5-char suffix labels; a `?block=X` read leaves `CompactBlockLabels=false`
(full ids) and `filterBlockSubtree` (`v2_object.go:324`) matches
`probe.Id == blockId` exactly → 404 for every outline label. This is the
primary large-document agent flow (APIV2.md outline-then-fetch, R12). The
unit tests miss it because **every fixture id is ≤5 chars** (`v2_object_test.go:56-72`),
so label==id and compaction is a silent no-op.
**Fix:** resolve unique block-id suffixes in `filterBlockSubtree` (mirror the
§9a/C4 write-side suffix allowance, applied to reads), OR keep full block ids
in the outline. **Add a fixture with realistic long ids** asserting the
outline label round-trips through `?block=` — without it every future change
here is unguarded.

### M2 — `format=md` derives etag and content from two separate locked reads (§8 "same state") [conformance #2]
`v2_object.go:154-158` computes etag from the reader's `ReadObject`; then
`markdownEnvelope` (`:215`) acquires a **second** lock via `s.mw.ObjectExport`
whose markdown is a different state read. A concurrent edit between them makes
the returned markdown newer than the etag advertises — the inconsistency §8
exists to prevent. **Fix:** render md from the reader's snapshot, or derive
md's etag from the export's own read; at minimum document md etag as
best-effort.

### M3 — Reads hard-fail (500) on unmapped/over-deep block content — violates C11 [conformance #3]
`export.go:600` errors on unknown content type and `:446` on `indent > 32`;
`Marshal` propagates → `GetObject` 500s the whole read. C11: "reads never
fail on unknown content" — degrade to `warnings`. No `OnWarning` sink is
wired, so the reserved `warnings` slot is never populated. **Fix:** wire
`OnWarning`, degrade unmapped/over-deep blocks to a warning, keep the read
succeeding. (Related: N1 below.)

### M4 — Idempotency store has no in-flight reservation → concurrent retries double-execute [robustness #4, latent Phase 2]
`ensureIdempotency` is get→`c.Next()`→put with no reservation between miss and
put. Two concurrent same-key POSTs both miss, both execute, both put. Correct
today (no mutations wired) but the moment Phase 2 attaches it to mutations,
duplicate concurrent submissions each mutate — exactly what the key exists to
prevent. **Fix now while it's cheap:** reserve the key on first miss (pending
marker; second caller blocks or 409/replays).

## TIER 3 — medium (correctness / coverage / contract)

- **T1 — `objectreadadapter` (the etag-consistency crux) has zero tests** [tests #3]. Every service test mocks `ObjectReader`, so the same-lock snapshot+heads capture — the whole optimistic-concurrency invariant — is never run. Needs a focused test (editor/cache infra required); at minimum the snapshot assembly.
- **T2 — 404 mapping untested; real misses may 500** [tests #4]. `mapReadError` (`v2_object.go:130`) maps only `ErrUnknownTreeId` + space-not-exist to 404; the only test hits the 500 branch. A miss with another sentinel → opaque 500 a 3B can't recover from. Test the 404 paths; widen the sentinel set to real miss errors.
- **T3 — Corruption metric is structure/order-blind and misses added details** [tests #5]. `snapshotdiff.Compare:52` scans only `orig` detail keys (added details invisible); no block-order/nesting signal, so the `restructure-section` fixture scores Clean regardless of whether order was restored — it can't measure the thing it tests. Scan `got`-only keys too; add an order signal or drop restructure from backtranslation scoring.
- **T4 — `ListObjects` can emit empty `type`, violating C5** [tests NOTE]. `v2_object.go:398` `Type: typeKeys[...]` is `""` when the type id isn't in `typeKeysById` (hidden/bundled/edge). Fall back to resolving the key directly; test it.
- **T5 — ETag emitted unquoted (RFC 7232)** [conformance #4]. `c.Header("ETag", etag)` bare; `EtagMatches` compares raw. A conformant client quoting `If-Match` (or `W/"…"`) never matches once Phase 3 wires preconditions. Emit `ETag: "<hash>"`, strip quotes/`W/` in `EtagMatches`.
- **T6 — C6 `hint` field almost never populated** [tests #7, conformance]. Repair guidance lives in `message`; structured `issues[].hint` set only for the newer-version case. A model keying on `hint` per the C6 contract finds it empty. Move guidance into `hint`.
- **T7 — Outline heading text keeps compacted ref labels after `refs` is dropped → unresolvable** [conformance #5]. `v2_object.go:168` keeps `CompactObjectRefs=true` for outline; `buildOutlineEnvelope:294` then deletes `refs` when properties aren't kept, leaving a 5-char mention label with no legend. Keep `refs` when retained text may carry a compacted ref, or disable ref compaction for outline.

## TIER 4 — minor / polish

- **P1 — Every object read echoes the 60-char `$schema` URL + `version`** (~20 tokens/read of constant waste on the token-cheap read path) [tests #6]. Strip from read envelopes (the md envelope already omits them — shapes also diverge). `export.go:167`.
- **P2 — Two divergent pagination strategies; `ListSpaces`/`ListPropertyOptions` load all rows in memory then paginate** [tests #9, robustness]. The "thousands of options" C10 case is bounded only by the prefix filter, not the DB. `v2_discovery.go:48,253`. Untested: offset-past-end, `limit=0`, empty list, no-match prefix.
- **P3 — Idempotency replay drops response headers (ETag/Location)** [robustness #6]. `storedResult` records only status/contentType/body; a replayed create loses `ETag`/`Location`.
- **P4 — 500s leak raw internal error strings to the agent** [robustness #7]. `RespondV2Error` puts `err.Error()` in the client envelope (`mapReadError` fallback, `ObjectExport` desc, idempotency body-read). Low impact on localhost; prefer generic 500 + server-side log.
- **P5 — `dry_run` scaffold runs on every v2 GET** [conformance NOTE]. `GET …?dry_run=xyz` 400s though reads never mutate. Attach to mutation routes in Phase 2 only.
- **P6 — literal `501` not `http.StatusNotImplemented`** [robustness #9], and **`/v2/validate` has no rate limiting** [robustness #10] (v1 attaches a write limiter to every write route). **empty-heads objects share one etag** [conformance NOTE] — treat empty heads as absent etag.

## Verified sound (not defects — recorded so they aren't re-raised)
objectreadadapter holds the lock across snapshot+heads and copies the heads
slice; `headsHash` sorts before hashing (order-independent, `\n`-delimited,
stable); `suffixLabels`/`setToSlice` deterministic → canonical output stable;
pagination clamps negative/overflow limit+offset and caps at 1000; `Details.Get`
nil-safe; path params are exact-match store keys (no traversal); v2 sits under
engine-level `gin.Recovery`; /validate 200-with-issues + newer-version hint
correct and tested; C4 object-ref/block-label split correct; resolvers wired so
custom date/select props round-trip.

## Disposition
Tier 1 (C1–C3) are must-fix-now — a crash and two DoS reachable from
untrusted localhost agent input, all surgical (copy-under-lock, validate
space_id, LimitReader). Tier 2 M1 is the one broken *feature* (outline
navigation) and needs the long-id test alongside the fix. M2–M4 and Tier 3
are correctness/coverage that should land before Phase 2 builds on this
surface. Nothing invalidates the architecture or the spec. Recommended: one
fix pass for Tier 1 + M1 (+ its test) immediately, Tier 2/3 folded into the
same pass, Tier 4 as cleanup. Two spec touch-ups also fall out (write the
`/validate` 200 and `block=`-absolute-indent rules into APIV2.md so Phase 2
doesn't re-derive them; T5/T7 confirm the outline shape needs a spec note).
