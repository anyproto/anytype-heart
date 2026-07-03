# spacev2 design — reconciler-based space orchestration

Status: v1 of the design, written before the first code slice. This document is
normative for the implementation in this package; HANDOFF.md defines scope and
the compatibility boundary.

## 1. Core idea

v1 models a space's lifecycle as a mode state machine (Initial/Loading/
Offloading/Joining) with five controller types, four Process wrappers around
child `app.App`s, a dedup `waiting` map, and two build paths (direct + watcher)
that must stay consistent. Most of §9 of `docs/SpaceController.md` is fallout
from that shape.

v2 replaces it with a **per-space reconciler**:

- **Inputs** (re-read live on every iteration):
  - `AccountStatus` from the SpaceView (the synced source of truth), and
  - `wanted` — a local demand flag ("should this space be resident in memory").
- A **pure function** `decide(status, wanted) → Target`.
- A **single goroutine per space** that converges the current `State` to the
  `Target`, one blocking step at a time, through a narrow `Backend` interface.

```
        SpaceView write (any device, any subsystem)          Get/Wait/policy
                    │                                              │
                    ▼                                              ▼
            watcher: poke(spaceId)                        controller.SetWanted
                    │                                              │
                    └────────────► controller.poke ◄───────────────┘
                                        │
                              reconcile loop (1 goroutine)
                    read live AccountStatus + wanted → decide() → step()
                                        │
                        Backend.Load / Unload / Offload / Join
                                        │
                     reused layers: spacecore, clientspace.BuildSpace,
                     techspace SpaceView writes, storage, deletion driver
```

There is **one controller implementation** for every space kind. Per-kind
variation (personal migration, guest key, one-to-one metadata) lives entirely
in how the `Backend` for that space is assembled — not in the lifecycle logic.

## 2. States and targets

```go
State:  Idle | Loading | Loaded | Joining | Unloading | Offloading | Offloaded | Closed
Target: TargetIdle | TargetLoaded | TargetJoining | TargetOffloaded
```

- `Idle` — controller exists, nothing resident; on-disk storage may exist.
  This is also the **paused/unloaded** state (first-class pause/unload).
- `Offloaded` — local data deleted (storage, files, indexes). Distinct from
  Idle for reporting; both can transition to Loading (restore/CancelLeave).
- Transient states (`Loading`, `Joining`, `Unloading`, `Offloading`) are set by
  the loop around the corresponding blocking backend call — observable for
  debugging/waiters, but only the loop goroutine moves between states.

```go
func decide(status spaceinfo.AccountStatus, wanted bool) Target {
    switch status {
    case AccountStatusDeleted, AccountStatusRemoving: return TargetOffloaded
    case AccountStatusJoining:                        return TargetJoining
    default: /* Unknown, Active */                    if wanted { return TargetLoaded }
                                                      return TargetIdle
    }
}
```

Convergence edges (each is one loop step; strictly sequential per space):

| current | target | step |
|---|---|---|
| Loaded | ≠Loaded | `Backend.Unload` → Idle (then loop continues, e.g. to offload) |
| Idle/Offloaded | Loaded | `Backend.Load` → Loaded |
| Idle/Offloaded | Joining | `Backend.Join` (blocks until accept/reject writes a new AccountStatus) → Idle → re-decide |
| Idle | Offloaded | `Backend.Offload` → Offloaded |

Joining is a *target activity*, not an end state: `Join` completes by writing
`Active` (accepted) or `Deleted` (rejected) to the SpaceView; the next
iteration re-decides to Loaded or Offloaded. Restore (CancelLeave) is just
`Deleted→Active` on the view: Offloaded + TargetLoaded → Load.

## 3. The Backend seam

```go
// All methods are invoked sequentially from the controller's reconcile
// goroutine (plus one final Unload from Close after the loop has exited).
type Backend interface {
    // AccountStatus returns the live persistent status from the SpaceView.
    AccountStatus(ctx context.Context) (spaceinfo.AccountStatus, error)
    // Load builds a usable space: spacecore.Get → clientspace.BuildSpace,
    // publishes LocalStatus (Loading→Ok/Missing, optimistic-Ok fast path),
    // runs the post-load components, fires OnSpaceLoad.
    Load(ctx context.Context) (clientspace.Space, error)
    // Unload releases the resident space, keeping on-disk data (pause).
    // Fires OnSpaceUnload. Must NOT write LocalStatusMissing.
    Unload(ctx context.Context, sp clientspace.Space) error
    // Offload deletes local storage + files + indexes and writes
    // LocalStatusMissing. Called only when nothing is resident.
    Offload(ctx context.Context) error
    // Join runs the ACL waiter until the join is resolved; it writes the
    // resulting AccountStatus (Active/Deleted) to the SpaceView itself.
    Join(ctx context.Context) error
}
```

Error contract: any error is retried by the controller with exponential
backoff (default 1s ×1.5 → 20s, reset on success or input change) — **retry is
owned by the controller**, not hidden inside loaders. A backend wraps
non-retryable failures with `Fatal(err)`; a fatal error parks the controller
(no timer retry, error surfaced to waiters) until the next poke, i.e. until
some input actually changes. This kills v1's stuck-controller (§9.7) and
poisoned-waiting-map (§9.8) hazards: nothing caches errors past the next real
event, and every decision is recomputed from live inputs.

## 4. Unidirectional lifecycle, synchronous API

All builds happen in exactly one place — the reconciler. API verbs only write
inputs and await convergence:

- `Create`: `spacecore.Create` (network op, random id) → `MarkSpaceCreated` →
  `techspace.SpaceViewCreate(status: Unknown)` → `SetWanted(true)` →
  `WaitLoaded(ctx)`. Synchronous error surface preserved.
- `Join`: `SpaceViewCreate(status: Joining, aclHeadId)` → poke. Returns once
  recorded (acceptance is asynchronous, as in v1).
- `Delete` / leave / remote-delete: write `AccountStatusDeleted` on the view
  (plus deletion-driver enqueue for owner deletes); reconciler offloads.
- Watcher events, `Get`/`Wait` promotion, pause policy — all reduce to
  `getOrCreate + SetWanted + poke`.

Because `getOrCreate` is idempotent and registration happens before any
blocking work, the v1 dual-path dedup (`waiting` map, §9.8/9.9) disappears
structurally: watcher and direct callers converge on the same controller.

`WaitLoaded(ctx)` waits for `Loaded` (returns the space), fails fast with the
fatal/load error of the current attempt, or fails with a terminal error when
the current target is `TargetOffloaded` (deleted space). Demand is explicit:
callers that want the space loaded call `SetWanted(true)` first (the service's
`Get`/`Wait` do this).

**Decision freshness.** Every poke advances an `inputSeq`; each reconcile
iteration captures the seq *before* reading the live status and records it as
`decidedSeq` when the target is decided. Two rules follow: (a) `WaitLoaded`
trusts terminal answers (deleted, fatal error) only when `decidedSeq ==
inputSeq` — a caller that just restored/promoted a space can never be failed
by the pre-change decision; (b) a parked controller (converged, or fatal
error) waits for `inputSeq > decidedSeq`, so a leftover poke token from an
input change the decision already incorporated cannot trigger a spurious
re-attempt.

## 5. Registry and service

- `registry`: `map[spaceId]*controller` + a controller factory; `getOrCreate`,
  `get`, `all`, `close` (concurrent close of all controllers, then refuses new
  ones). No waiting map, no error caching.
- `service` (later slice): outward API (`Get/Wait/Create/Join/Delete/...`),
  account bootstrap (explicit tech-space resolution step with the old-account
  fallbacks), the SpaceView watcher (objectstore subscription → poke; no
  dedupqueue needed — the buffered-1 poke coalesces bursts per space), and the
  demand policy (eager vs lazy/preferred, preload drain, future LRU caps).

Pause/unload and lazy loading are the same mechanism: `wanted=false` on a
loaded space unloads it; `Get`/`Wait` re-promote. A future memory-cap policy is
just a component that flips `wanted` flags; the engine already supports it.

## 6. How §9 hazards are addressed (structurally)

| §9 hazard | v2 answer |
|---|---|
| 1. Equal-guarded status writes | status writes go through the reused techspace SpaceView methods, which carry the Equal early-out; the engine itself writes no statuses |
| 2. No per-space ordering of watcher events | events don't carry payload at all — a poke only wakes the loop, which re-reads live state; ordering is irrelevant by construction |
| 3. components must not drive their controller | backends/pipeline components write the SpaceView; the watcher pokes asynchronously — no path joins the goroutine closing its own app |
| 4. close-before-build serialization on storage | all mutating steps run on one goroutine per space; Unload strictly precedes Offload/Load |
| 5. fresh Process per transition | every Load/Join/Offload call builds fresh resources inside the backend; nothing app-shaped is cached across transitions |
| 6. transition dedup/rejection | no ChangeMode API to misuse; concurrent intents are just input writes + pokes; buffered-1 poke provably loses no wakeup (single consumer) |
| 7. lastUpdatedStatus stuck controller | no change-detection guard; loop always reconverges from live inputs; failures retry with backoff |
| 8/9. waiting-map poisoning / dual-path dedup | no waiting map; idempotent getOrCreate; single build path |
| tech-space lock reentrancy | v2 code never nests `DoSpaceView`/`DoAccountObject` closures |

Shutdown: `service.Close` closes the registry; each controller cancels its
ctx, the loop drains (interrupting blocking backend calls via ctx), and a
final `Unload` releases a resident space. In-flight `WaitLoaded` callers get
`ErrClosed`. No `isClosing` re-check scatter: after `registry.close`, new
`getOrCreate` calls fail.

## 7. What v2 explicitly does not rebuild

Reused as-is (per HANDOFF): `clientspace` (incl. `BuildSpace`, TechSpace
wrapper), `techspace`, `spacecore`, `spaceinfo`, storage + migration, crypto
derivations (account metadata, push keys, KV encoding), coordinator/ACL
clients. The post-load domain components (`aclobjectmanager`,
`participantwatcher`, `aclnotifications`, `migration`, `personalmigration`)
are load-bearing (push keys, participants, notifications) and are reused by
the production Backend; how they are hosted (child app vs direct invocation)
is a Backend implementation detail, invisible to the engine.

The deprecated marketplace virtual space is not a controller in v2; whatever
minimal registration consumers still need is handled statically at the service
layer (decided at the bootstrap slice).

## 8. File map (this package)

- `state.go` — `State`, `Target`, `decide()` (pure), `Fatal()` error marker
- `controller.go` — the reconciler actor (`SetWanted`, `Poke`, `WaitLoaded`,
  `State`, `SpaceIfLoaded`, `Close`)
- `registry.go` — controller registry
- later slices: `backend_*.go` (production backends), `service.go`,
  `bootstrap.go`, `watcher.go`, compat tests
