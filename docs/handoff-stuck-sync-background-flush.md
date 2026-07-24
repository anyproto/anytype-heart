# Handoff: iOS "stuck sync" / 76s `AppSetDeviceState` hang + WAL checkpoint durability

**Status:** Diagnosis complete, no code changed yet. This doc is a cold-start handoff for the
agent implementing the fixes.

**Source of investigation:** a user debug report (iOS `0.48.0`, middleware `v0.50.17`,
any-sync `v0.12.16`, `ios/arm64`) auto-captured with reason `LONG_RPC`:
`AppSetDeviceState` took **76,711 ms**. The archive contained a goroutine dump, a
`debugStat.json`, and a `bundle.log` covering ~2.5h. All conclusions below are backed by that
evidence plus the current source tree.

---

## TL;DR — what to build

Two **independent** problems were found in the report. Fixes are independent; do them separately.

1. **Primary (the reported hang): the mobile background DB flush blocks the RPC thread for
   minutes.** On `AppSetDeviceState(BACKGROUND)`, `durability` runs a **passive WAL checkpoint of
   every open any-store DB in parallel** and `wg.Wait()`s with no hard upper bound. A per-DB 10s
   `context` deadline exists but **cannot preempt an in-flight SQLite checkpoint**, so on iOS's
   throttled background I/O the wait overran to 76s+ (goroutine dump shows 2+ min).
   - **Fix A1 (behavioral):** on background, do a cheap **fsync** (`FlushModeFsync`) instead of a
     passive checkpoint. Durability is preserved; the expensive/throttled page-copy I/O is
     dropped. Space reclamation stays on the idle/auto-checkpoint paths that already exist.
   - **Fix A2 (belt-and-suspenders):** bound `provider.Flush`'s `wg.Wait()` with an overall
     deadline so `AppSetDeviceState` can never overrun its budget regardless of what SQLite does.
   - **Fix A3 (durability correctness, Apple):** set `PRAGMA fullfsync=ON` (or at least
     `checkpoint_fullfsync=ON`) on Apple platforms. On Darwin plain `fsync()` does not flush the
     drive cache; without this, both commits and checkpoints are not power-safe. Currently unset.

2. **Secondary (the literal "stuck sync"): one space is permanently stuck loading.** A space fails
   to build with `any-store: collection not found` and, because that error is treated as
   **retryable**, `spaceloader` retries forever (~20s cap) for the whole session. The space stays
   `localStatus: Loading` and every `WaitLoad` caller on it blocks. **Platform-independent.**
   - **Fix B:** classify `anystore.ErrCollectionNotFound` on space build as a corrupt/incomplete
     local store — either mark it non-retryable and surface an error status, or trigger a storage
     rebuild/re-download for that one space — instead of an infinite retry that wedges dependents.

Commit prefix convention (see `CLAUDE.md`): `GO-XXXX Short description`. Target branch: `develop`.
This investigation ran on branch `claude/stuck-sync-report-gfuzj6`.

---

## Problem 1 — background flush hang (the reported LONG_RPC)

### Definitive evidence (goroutine dump)

The `AppSetDeviceState` goroutine was blocked in `sync.WaitGroup.Wait` for 2+ minutes:

```
core.(*Middleware).AppSetDeviceState                         core/core.go:54
 → any-sync app.(*App).SetDeviceState                        (synchronous fan-out to all ComponentStatable)
  → core/durability.(*durability).StateChange                core/durability/durability.go:74   (AppWentBackground)
   → anystoreprovider.(*provider).Flush                      pkg/lib/datastore/anystoreprovider/provider.go:564  (wg.Wait)
      → 5× provider.Flush.func1 goroutines, state [runnable], each in:
         any-store/internal/durability.NewFlushFunc.func2
          → driver.(*Conn).ExecNoResult "PRAGMA wal_checkpoint(PASSIVE);"
           → modernc.org/sqlite _sqlite3Checkpoint / _sqlite3BtreeCheckpoint
              (one goroutine parked in _unixRead — a disk-read syscall)
```

Key reads: the flush goroutines are `[runnable]` (actively grinding a WAL checkpoint, one blocked
on disk I/O), **not deadlocked**. It is slow, unpreemptable checkpoint I/O, amplified by scale.

### Mechanism / root cause

- `AppSetDeviceState` → `app.SetDeviceState` notifies every `ComponentStatable` **synchronously**,
  so any slow `StateChange` blocks the whole RPC (`core/core.go:53-60`).
- `durability.StateChange` (`core/durability/durability.go:59-81`):
  - `CompStateAppWentBackground` → `spaceCore.Flush(10s, true)` then
    `anystoreProvider.Flush(10s, true)` — **blocking**, `waitPending=true`.
  - `CompStateAppClosingInitiated` → `Flush(3s, false)` — non-blocking, best-effort.
- `provider.Flush(timeout, waitPending)` (`pkg/lib/datastore/anystoreprovider/provider.go:526-565`):
  spawns one goroutine per DB, each `db.Flush(ctx, idleDuration, FlushModeCheckpointPassive)` under
  `context.WithTimeout(componentCtx, timeout)`, then a **plain `wg.Wait()` with no overall bound**.
- The 10s `ctx` is wired to `sqlite3_interrupt` (you can see `go-sqlite (*Conn).SetInterrupt`
  watcher goroutines in the dump), **but a bulk WAL checkpoint does not honor an interrupt
  promptly**, and a goroutine blocked in `_unixRead` can't be interrupted until the syscall
  returns. So `db.Flush` runs far past 10s and `wg.Wait()` overruns without bound.

### Why iOS and not desktop / Android

- **Desktop is structurally exempt.** The blocking path only runs for `CompStateAppWentBackground`,
  which arrives *only* via the `AppSetDeviceState(BACKGROUND)` RPC — a mobile lifecycle signal.
  Desktop's only durability trigger is `CompStateAppClosingInitiated`, set internally on shutdown
  (`core/application/application.go:82`), which takes the **non-blocking** path.
- **iOS ≫ Android in practice** (the code path is identical for both, so this is "mobile", reported
  on iOS):
  1. iOS suspends backgrounded apps aggressively on a hard clock with a watchdog — the synchronous
     background flush is on the critical suspension path every time the user leaves the app.
  2. iOS throttles/locks background filesystem I/O (Data Protection can lock files on screen-lock),
     turning a normally-fast checkpoint into a multi-minute stall — matches `_unixRead` in the dump.
  3. This account is large: `debugStat` shows ~95 space peer-managers → ~95+ WAL DBs checkpointed
     in parallel at background time; RSS 978 MB.

### The checkpoint types in play (all PASSIVE today)

Confirmed against `any-store@v0.4.7`:

| Trigger | Where | Mode | When |
|---|---|---|---|
| Background | `durability.StateChange(AppWentBackground)` → `provider.Flush` → `db.Flush(ctx, 30ms, CheckpointPassive)` (`provider.go:558`) | `wal_checkpoint(PASSIVE)` | on `AppSetDeviceState(BACKGROUND)`, all DBs in parallel, then `wg.Wait()` |
| Idle | any-store `Durability{AutoFlush:true, IdleAfter:20s, FlushMode:CheckpointPassive}` (`provider.go:389-393`) | `wal_checkpoint(PASSIVE)` | 20s after a DB's last write, per-DB, while quiet |
| SQLite auto | `wal_autocheckpoint = 10000` (`provider.go:272`) | PASSIVE (built-in) | when a WAL reaches 10000 pages (~40MB) |

- `FlushModeCheckpointPassive` → `conn.ExecNoResult(ctx, "PRAGMA wal_checkpoint(PASSIVE)")`
  (`any-store/internal/durability/flush.go:33-36`).
- `db.Flush(ctx, waitIdleTime, mode)` (`any-store/db.go:603`) → `Controller.Flush`
  (`internal/durability/controller.go:147`) retries every 10ms until the DB is write-idle for
  `waitIdleTime` (30ms on background because `waitPending=true`) or ctx cancels, then runs the
  checkpoint pragma.

### Why fsync-on-background is correct (SQLite-doc-backed)

- **Durability needs only the WAL fsynced.** Docs: "The WAL must be synced to persistent storage
  prior to moving content from the WAL into the database." The frame-copy (WAL→DB) and final DB
  sync are for **space reclamation + read performance**, not durability.
- **At `synchronous=NORMAL` (our setting), commits do not sync — the checkpoint is the only sync.**
  So there is genuinely un-fsynced committed data at background time; we need exactly one WAL fsync
  to be power-safe, not a full checkpoint.
- **any-store `FlushModeFsync`** does precisely that: temporarily `synchronous=FULL` + an empty
  `BEGIN IMMEDIATE`/`COMMIT` to force a WAL fsync (`any-store/internal/driver/conn.go:190-228`),
  with no page-copy I/O.
- Committed data already survives an app kill regardless of sync; only power loss needs the fsync,
  and an iOS backgrounded app is *suspended* (memory preserved), so one WAL fsync covers the only
  real risk (power loss while suspended).

### Interrupted checkpoint = corruption risk on iOS (why fsync is also *safer*, not just faster)

- **Being interrupted is safe by design:** SQLite doesn't truncate/reset the WAL until after the DB
  file is successfully synced (Dan Kennedy: "SQLite does not delete or truncate the wal file until
  after the successful sync"). A checkpoint killed mid-copy just restarts next launch; the WAL
  stays authoritative. App kill (no power loss) is always safe.
- **The one corruption window:** checkpoint copies frames → syncs DB → truncates/overwrites WAL. If
  that DB sync was a *lie* (data still in a volatile cache) and power dies as the WAL is discarded,
  the DB is left partially updated with its authoritative WAL gone → B-tree corruption. Docs: "In
  WAL mode, the only time that a failed sync operation can cause database corruption is during a
  checkpoint operation."
- **On Darwin this window is real:** plain `fsync()` does not flush the drive cache; `F_FULLFSYNC`
  is required, exposed via `PRAGMA fullfsync` / `checkpoint_fullfsync` (both default **off**). The
  codebase sets **neither** (only `synchronous=normal`, `wal_autocheckpoint=10000` in
  `provider.setDefaultConfig`, `provider.go:263-274`). SQLite forum documents iOS hard-reset
  corruption fixed by enabling fullfsync.
- **Degradation asymmetry (the decisive point):**
  - WAL append + fsync: WAL frames are checksummed & append-only → a torn write at power loss is
    detected and the tail discarded → **lost last transaction(s), no corruption**.
  - Checkpoint: the failure mode at the sync boundary (if the sync is not honored) is **main-DB
    corruption**.
  - So a passive checkpoint of ~95 DBs at suspend time — exactly when iOS may freeze I/O and the
    battery may be low — maximizes exposure to the corruption window at the worst moment. Fsync-only
    removes those windows from that moment and degrades gracefully.

### Recommended change for Problem 1

`core/durability/durability.go` — `CompStateAppWentBackground` branch:
- Flush with an **fsync** semantics instead of a passive checkpoint. Two options:
  - Add an fsync-only entry point to the `Flusher` interface / `provider.Flush` (e.g. a mode
    parameter) and call it here; **or**
  - Change `provider.Flush` to take a `FlushMode` and pass `FlushModeFsync` from the background
    path (keep checkpoint for any explicit-checkpoint callers, if any).
- Keep the closing (`AppClosingInitiated`) path as-is (best-effort).

`pkg/lib/datastore/anystoreprovider/provider.go` — `Flush`:
- Replace the unbounded `wg.Wait()` with a bounded wait: wait on the WaitGroup up to
  `timeout + slack`; on elapse, `log` and return, leaving the per-DB goroutines to finish on their
  own `componentCtx`. This guarantees `AppSetDeviceState` returns within budget even if a flush
  stalls in SQLite. (Note the existing `dbsAreFlushing` CAS guard at `provider.go:527` prevents
  overlapping flushes — make sure a bounded-wait early return still `Store(false)` via the existing
  `defer`, which it does.)

`pkg/lib/datastore/anystoreprovider/provider.go` — `setDefaultConfig` (`:263-274`):
- On Apple platforms (`runtime.GOOS == "darwin"` or `"ios"`; confirm how gomobile reports GOOS for
  the iOS build — likely `"ios"`), set `SQLiteConnectionOptions["fullfsync"] = "1"` (or at minimum
  `["checkpoint_fullfsync"] = "1"`). Verify any-store passes unknown pragma keys straight through as
  `PRAGMA <key> = <value>` connection pragmas (it clones `SQLiteConnectionOptions` and applies them;
  confirm `fullfsync`/`checkpoint_fullfsync` are accepted). Weigh the sync-latency cost — acceptable
  on mobile where sync rate is low and correctness wins.

### Regression test (deterministic, any OS)

The platform-independent core is: *a checkpoint slower than the timeout is not preempted, and
`provider.Flush` waits unbounded.* Pin it with a Go test:
- Open several DBs via `anystoreprovider`, write enough rows to build a large WAL (disable/avoid the
  20s idle auto-checkpoint so pages stay in the WAL). Optionally add a slow read shim / slow FS to
  make the checkpoint read-bound like iOS background I/O.
- Assert current behavior overruns; after Fix A2, assert `provider.Flush(10s, true)` returns within
  `timeout + slack`; after Fix A1, assert the background path performs an fsync (WAL fsynced) and
  does **not** run a full checkpoint (e.g. WAL not drained to the DB file).

---

## Problem 2 — space permanently stuck loading (the literal "stuck sync")

### Evidence

- `debugStat.json` `client.components.spacestatus`: exactly one space not in a terminal state —
  `localStatus: Loading`:
  `bafyreifhyp67ivvylqs5hfcg4d7c3e6k4tsdcnn6qbfyk46435nfymytmm.39nao6yeeozk9`
  (accountStatus `Unknown`, remoteStatus `Ok`).
- `bundle.log`: **37×** `client.components.spaceloader "space load: build space error"`,
  `error:"any-store: collection not found"`, `notRetryable:false`, spanning the entire session
  (10:33:19 → 12:54:28), never succeeding.
- Goroutine dump: many `spaceLoader.WaitLoad` / `ocache.(*entry).waitLoad` goroutines parked —
  callers blocked waiting on this space (e.g. `aclobjectmanager`).

### Mechanism

- `spaceloader/loadingspace.go`:
  - `loadRetry` (`:73-103`) retries `load` forever with backoff capped at `loadingRetryTimeout`
    (20s) until `load` returns `shouldReturn=true`.
  - `load` (`:121-145`) returns `shouldReturn` only when `isNotRetryable(err)` is true.
  - `isNotRetryable` (`:117-119`) is true only for `objecttree.ErrHasInvalidChanges`,
    `spacedomain.ErrUnexpectedSpaceType`, or `disableRemoteLoad`.
  - `open` (`spaceloader.go:156-158`) → `builder.BuildSpace`, which surfaces
    `any-store: collection not found` (a missing collection in the space's local store). This is
    **not** in the non-retryable set → infinite retry.
- Consequence: the loading space's `loadCh` never closes, so `spaceLoader.WaitLoad`
  (`spaceloader.go:160-189`) → `space.(*service).Get` (`space/service.go:420-429`) blocks
  indefinitely for anything that touches this space. `any-store.ErrCollectionNotFound` is already
  a recognized sentinel elsewhere (`core/indexer/reindex.go:354,396`,
  `core/block/chats/chatrepository/repository.go:131`).

### Recommended change for Problem 2

Decide direction with the maintainer; two reasonable options:
- **(b1) Stop the infinite loop:** treat `anystore.ErrCollectionNotFound` on build as non-retryable
  (extend `isNotRetryable`) and surface an error `localStatus` so the space stops wedging
  `WaitLoad` dependents and the UI can show a real state.
- **(b2) Self-heal:** trigger a storage rebuild/re-download for just that space (drop the corrupt
  local store and re-acquire from peers) rather than surfacing an error.

b1 is smaller/safer; b2 is better UX. Either way, the infinite silent retry must go.

---

## Benign / ruled-out (do not chase these)

- **45× `common.commonspace.headsync "can't sync with peer" … broken pipe`** all at 10:44:16 — a
  single transient network drop; connectivity recovery re-dialed and peers are healthy in
  `common.net.pool` at snapshot time. Normal.
- **72 spaces `accountStatus: Unknown`** with `aclobjectmanager loaded:false` — lazy/not-yet-opened
  spaces on mobile. Expected, not a bug. (17 `Deleted`, 23 `Active` are the terminal ones.)
- `heart.inboxsender` resend errors, `anytype-doc-indexer "should be more than just id"`,
  `ChatObjectDeprecated` reindex, `filesync "update space usage"` — low-level noise unrelated to
  either problem.

---

## Key file references

| Area | File:line |
|---|---|
| RPC entry (synchronous) | `core/core.go:53-60` |
| Device-state fan-out | any-sync `app/app.go` `SetDeviceState` (synchronous over `ComponentStatable`) |
| Background vs closing flush policy | `core/durability/durability.go:59-81` |
| Unbounded parallel flush + `wg.Wait()` | `pkg/lib/datastore/anystoreprovider/provider.go:526-565` |
| anystore config (synchronous/autocheckpoint/durability) | `pkg/lib/datastore/anystoreprovider/provider.go:263-274, 383-396` |
| Desktop-only closing trigger | `core/application/application.go:82` |
| LONG_RPC reporter (threshold `maxDuration`) | `metrics/interceptors.go:215-268` |
| Space load retry loop | `space/internal/components/spaceloader/loadingspace.go:73-145` |
| `WaitLoad` blocks on `loadCh` | `space/internal/components/spaceloader/spaceloader.go:160-189` |
| `space.Get` → `WaitLoad` | `space/service.go:420-429` |
| any-store FlushMode → pragma | `any-store@v0.4.7/internal/durability/flush.go:33-47` |
| any-store fsync primitive | `any-store@v0.4.7/internal/driver/conn.go:190-228` |
| any-store `db.Flush` / `Controller.Flush` | `any-store@v0.4.7/db.go:603`, `internal/durability/controller.go:147-214` |

## Sources (external, SQLite)

- Write-Ahead Logging: https://sqlite.org/wal.html
- wal_checkpoint_v2 (checkpoint modes): https://sqlite.org/c3ref/wal_checkpoint_v2.html
- Checkpoint mode values: https://sqlite.org/c3ref/c_checkpoint_full.html
- How corruption happens during WAL checkpoint (forum): https://sqlite.org/forum/info/47107ab818977549
- Hard reset causing DB corruption on iOS (forum): https://sqlite.org/forum/info/71d804068ce5bdcf46019173364789ef78ecba9009339adf67fb5d3a9ae2c8e5
- PRAGMA fullfsync / checkpoint_fullfsync: https://www.sqlite.org/pragma.html
- Powersafe overwrite: https://sqlite.org/psow.html
