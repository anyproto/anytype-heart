# GO-7349 — open review findings, 2026-08-15

Branch `go-7349-import-llm`, verified clean at **`fa44dd964`** (build, `go vet`, full `-race` suite green; zero data races across all reviewer runs).

Two ranges reviewed, **no fixes applied yet**:

- **DM-3 fix round** — `adb98609a..46d04de84` (3 Opus lenses)
- **importStatistic push emitter** — `46d04de84..fa44dd964` (2 Opus lenses)

Every item below is CONFIRMED by execution unless marked REASONED. Recommendation was to fix 1–10 and defer the rest.

---

## CRITICAL — destroys user work

**1. `userCancelled()` reads an error *code* where it needs a *stop source*.** One flaw, two opposite failures.
- `adapter/runlifecycle.go:268` + `adapter/crawlresumerun.go:217`. `classifyFatal` (`engine/engine.go:1159`) maps anything wrapping `context.DeadlineExceeded` to `IssueCancelled` — including the Notion client's own `http.Client{Timeout: time.Minute}`. So a **60-second server hang deletes a two-hour crawl**. Strict regression: pre-round `IsRetryable` was true and the dir was quiet-kept. Also hits *fresh* imports. Second trigger, no network: a stalled-disk claim flush.
- `notion/converter.go:344-351` (`recoverOne`) issues `IssueObjectFailed` for a cancellation without consulting `ctx.Err()`, so `userCancelled` is false → **a cancelled import is kept and silently re-run** with the stored token. `errors.Join` can make it simultaneously retryable → transient-keep *refunds* the attempt → re-runs indefinitely.

Fix: consult the cancel cause, which is already unambiguous (`progress.Canceled()` fires `cancel(nil)`; shutdown fires `cancel(ErrSuspended)`).

**2. Recovery probes two of three id shapes.** `notion/converter.go:302-349` settles on `/pages` then `/data_sources`, but `crawlSearch` also yields `kindDatabase` (`notion/search.go:173-176`) and `EnumerateIdentities` claims every entity. A bare database id 404s on both rungs → reported as deleted, never imported. Verbatim the P0-A symptom the round removed. `discoverDatabase` (line 396) is the missing rung.

**3. `NothingToUndo` is an in-memory oracle; the durable scope is `MaterializeStarted`.** `engine/engine.go:1064-1079` + `adapter/runlifecycle.go:268`. A cancel early in pass 3 tears up to 8 in-flight creates with an empty journal → dir dropped → the claim rows that were the only attribution for those hollow trees go with it. Correct rule: `NothingToUndo && !manifest.MaterializeStarted`.

**4. A resumed run's first event says `Scanning` / `NothingToUndo` while carrying created objects.** `adapter/runlifecycle.go:59-71` builds the emitter in its zero value; `engine/engine.go:420` calls `Created(state.Created)` before `beginMaterialize`. §15.6 renders that as *"Cancel (nothing added yet)"* — cancelling there compensates thousands of real objects. Also poisons pull for the whole rehydration window (`trackLive` registers in `newLifecycle`).

## HIGH — the surface lies

**5. A genuine error is repainted calm.** `adapter/statistic.go:225-231` (`Throttled`) and `:233-240` (`Retrying`) lack the `state == Error` guard that `Recovered` has at `:242-254`. `notion/client/client.go:121,130` fire with no ctx check, on prefetch workers that outlive `r.cancel()`. A failed run's **terminal** event can read "waiting for Notion".

**6. A stalled run reports a healthy rate and frozen ETA forever.** `adapter/statistic.go:467-478` — `ratesLocked` never consults `now`; the 30 s window is pruned only inside `sampleLocked`, which runs only from `Completed`. Exactly the throttled-vs-stuck distinction §15.1 exists to draw.

**7. `objectsCreated` moves backwards.** `engine/engine.go:795` publishes a *level* from 8 workers; `adapter/statistic.go:206-211` is last-writer-wins. Measured: regressed on every run of a 600-page import, one settling at 598/600. It is §15.4's cancel affordance, and `runstatus.go:256` serves the exact ledger count for a dormant poll of the same run. Fix: make `Created` a delta, or the setter monotone. Note `Bytes` (`engine/spool.go:192`) is the same shape, safe only while `drainFile` stays single-goroutine.

**8. `probeSpace` → `spaceGone` → `dropStore` has no stop check.** `adapter/sweep.go:346-361` + `:211-216`. The premise (`ErrSpaceNotExists` is definitive) is contradicted by `space/service.go:598-606`, which returns it for *unreadable* as well as absent, via `resolveDerivedInfo` reading through `s.ctx`. The startup sweep runs concurrently with techspace warm-up.

**9. A crawl-resumed run reports `pagesDone > pagesTotal`.** `engine/engine.go:655-657` (pass 1 Discovers only re-enumerated claims) vs `engine/spool.go:265-271` (census seeds Completed from the whole spool). Notion's late-discovered claims land in the seed, never the denominator. ETA then reads "unknown" for the rest of the crawl (`statistic.go:504`). Also a push/pull disagreement: dormant `2/2`, live `2/1`.

## MEDIUM

**10.** `opCtx` re-imposes an already-expired deadline verbatim (`runstore/runstore.go:505-507`; `time.Until(deadline) < dbOpTimeout` is true for negatives) → born-dead context, exactly the input the any-store connection leak needs. Unreachable from current callers; reachable once a composite's first nested op consumes the shared budget.
**11.** ERROR unreachable for an `ALL_OR_NOTHING` abort — the commonest real failure. `issue.go:155-160` aborts on `SeverityObjectError`; `statistic.go:275` escalates only on `>= SeverityFatal`; `engine.go:483-491` returns it without re-reporting. Shared root with 5: `Close()` takes no argument, so the terminal state is always a transport artifact, never the verdict. REASONED.
**12.** Every import publishes one CREATING event with `totalsKnown=true` over a zero denominator (`engine.go:308-324`).
**13.** A swallowed census failure inverts the counters and re-opens the legacy zero-total bar on a pass-3 restart (`engine.go:314-321`) — the bug `563049ff5` fixed, restored on this path.
**14.** Keep-and-retry yields 3 failure notifications across 3 app starts and extends token-on-disk lifetime that whole span.
**15.** Recovery probing is unbounded — 2 API calls per vanished claim, not capped by `lateDiscoveryCap`; 10k deleted pages ≈ 110 min of silent 404s at 3 rps.
**16.** `resumesInMs` is a duration frozen at signal time (`statistic.go:95,410`), so a poller's countdown never counts down. A deadline (unix ms) would fix it.
**17.** The advisory telemetry seam has no recover — `teeReporter` panics abort the import they observe (`engine.go:753` turns it into a fatal); a panic in `stats.Close()` unwinds before `finishRun` disposes the store. REASONED.

## LOW
`engineSink.Claim` (`engine/sink.go:149`) lacks the P0-D flush — dead today; `safeToClose` off-by-one on the final attempt (reads a counter `BeginCrawlResume` already incremented); two file-class predicates (`runstore/spool.go:266` vs `IsFileClass`); in-process compensation loses the "never wordlessly" rule the sweep's reader implements; `errorMessage` outlives the ERROR state; `Claim` hard-codes `KindPage` ignoring `claim.SbType`.

## Decisions, not defects
- **`currentItem` plaintext reaches the RPC response by design**, and `RunList` enumerates every live run — page titles flow to whatever polls a headless sidecar. Fell out of "push and pull are the same message" rather than being chosen. (`ANYTYPE_GRPC_LOG>1` also logs whole payloads — pre-existing, opt-in.)
- **`errorMessage` is a second user-content field** (may carry a Notion id or markdown path) with none of `DisplayText`'s hashing protection.

## Also outstanding
- `core/event/event_grpc.go:76` — a headless sidecar with no session servers logs `no servers to broadcast event` per Broadcast: **~40k warn lines** for a multi-hour import. Pre-existing, but the emitter turns a latent nuisance real, and driver 3 is exactly the polling case.
- `-count=60` on the `notion` package exceeds `go test`'s default 10-minute timeout — reviewers need `-timeout=90m` or a timeout reads as a panic.
- The any-store `Iter` read-tx leak is written up in `docs/AnyStoreIterReadTxLeak.md`, issue-ready, not yet filed upstream.
