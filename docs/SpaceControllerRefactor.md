# SpaceController machinery: how it works today and how to refactor it

Research notes, June 2026. Scope: `space/` service layer — `space.Service`, the four
`SpaceController` implementations, the `mode.StateMachine`, the space processes
(loader / joiner / offloader), and the lazy-loading bolt-on.

---

## 1. How it works today

### 1.1 The layers

```
space.Service (space/service.go)
 ├── techspace (space views = persistent per-space state, synced as objects)
 ├── spaceWatcher (space/spacewatcher.go)
 │     objectstore subscription on spaceView objects in tech space
 │     → dedupqueue → onSpaceStatusUpdated (goroutine per event)
 ├── spaceControllers map[string]SpaceController   (registry)
 ├── waiting map[string]controllerWaiter           (hand-rolled singleflight + error cache)
 └── lazy-loading bolt-on: deferredStatuses, lazyMode, releasing,
     preloadOnce/preloadCh, ensureSpaceStarted, resolveDerivedInfo

SpaceController (4 implementations)
 ├── personalspace.spaceController     Initial / Loading(+migration) / Offloading
 ├── shareablespace.spaceController    Initial / Loading / Joining / Offloading
 ├── streamablespace.spaceController   Initial / Loading(guestKey) / Offloading
 └── marketplacespace.spaceController  fake: no state machine, Mode()==ModeLoading always
       each real controller owns:
       ├── child app.App with spacestatus (facade over its spaceView)
       └── mode.StateMachine (one goroutine per space)
             └── current mode.Process = ANOTHER child app.App per mode:
                  loader   → builder, spaceloader, aclnotifications,
                             aclobjectmanager, participantwatcher, migration
                  joiner   → statuschanger, aclnotifications, aclwaiter
                  offloader→ spaceoffloader
```

### 1.2 The data flow (a feedback loop through the object store)

1. Persistent intent lives in the **spaceView** object in tech space
   (`AccountStatus`: Unknown/Active/Joining/Removing/Deleted, plus aclHeadId, guestKey).
2. `spaceWatcher` subscribes to all spaceViews (`space/spacesub.go`) and feeds every
   change through a dedup queue into `service.onSpaceStatusUpdated`.
3. The service creates the controller if needed (`startStatus`, dispatch by
   id/EncodedKey → factory) and calls `ctrl.Update()`.
4. `Update()` maps AccountStatus → Mode (`Deleted/Removing→Offloading`,
   `Joining→Joining`, else `Loading`) and calls `sm.ChangeMode(mode)`.
5. The state machine tears down the current mode's child app and starts the next one.
6. The processes write status *back* into the spaceView (loader sets
   LocalStatus Loading→Ok/Missing; joiner's aclwaiter flips AccountStatus
   Joining→Active/Deleted on ACL events) — which re-triggers the subscription (2).

RPC intents (`Join`, `InviteJoin`, `Delete`, `CancelLeave`) bypass part of the loop:
they create controllers directly *and* write the spaceView, guarded by the `waiting`
map. `join.go` carries the TODO: *"refactor using unidirectional model where we
change/create space view and it asynchronously starts controller"*.

### 1.3 The state machine (`space/internal/spaceprocess/mode/statemachine.go`)

- One goroutine (`loop`) per space; `ChangeMode(next)` registers an unbuffered waiter
  channel, pokes `notify`, and **blocks** until the loop has closed the old process and
  started the new one.
- Only one pending transition is allowed: a second, different `ChangeMode` while one is
  in flight returns `ErrTransitionInProcess`.
- On process-start failure, the machine falls back to `ModeInitial` and sends `nil` to
  all waiters, which surfaces as a generic `ErrFailedToStart` (the real error is lost —
  see `// TODO: [MR] send error to waiter`).

---

## 2. Why it is error-prone (concrete defects, with locations)

These are not stylistic complaints; each is a latent or actual bug.

1. **Dropped transitions.** Every controller's `Update()` sets `lastUpdatedStatus = status`
   *before* calling `ChangeMode`. If `ChangeMode` returns `ErrTransitionInProcess`
   (status flipped while a previous transition runs, e.g. RPC `SetPersistentInfo`
   racing the watcher), the error is logged and dropped (`applySpaceStatus`,
   service.go), and the *next identical event is a no-op* because `lastUpdatedStatus`
   already matches. The space silently stays in the wrong mode until the status changes
   again. (shareable.go:122-145, personal.go:155-174, streamablespace.go:122-141)

2. **`ChangeMode` cannot be cancelled and waiters are never drained on Close.**
   `proc = <-wait` has no ctx/timeout (statemachine.go:126). If `Close()` wins the
   select race in `loop`, pending waiters never receive — those goroutines block
   forever. A hung `Process.Start` (network) blocks every caller indefinitely.

3. **Swallowed errors.** Waiters get `nil` → `ErrFailedToStart`; the actual start error
   only goes to the log. Callers (Join RPC, Get) cannot distinguish "storage missing"
   from "invalid ACL".

4. **Session-permanent error poisoning.** `s.waiting` entries are never deleted; a
   failed factory call caches its error forever (load.go:31-47, join.go, streamable.go).
   A transient failure during `Join` makes that space id unusable until app restart.
   The lazy-loading code knows this ("would poison s.waiting" — service.go:580).

5. **`Wait()` is a 500 ms polling loop** (waiter.go:58-67) because there is no event for
   "controller appeared in the registry".

6. **`Current() any` + scattered type assertions.** `load.go:103`,
   `create.go:86,129` assert `ctrl.Current().(loader.LoadWaiter)`; marketplace must fake
   the whole interface; `join.go` branches on raw `Mode()` values. Mode is overloaded:
   `ModeLoading` means both "loading" and "loaded" (`AllLoadedSpaceIds`, service.go:786).

7. **Massive copy-paste.** personal/shareable/streamable controllers are ~80% identical:
   `makeStatusApp`, `Update`, `SetPersistentInfo`, `SetLocalInfo`, `Close`,
   `GetStatus`, `GetLocalStatus`, `Delete`, and the AccountStatus→Mode switch is
   duplicated **six times** (Start + Update × three controllers). Divergence has already
   crept in: shareable handles `AccountStatusJoining`, the others don't; only personal
   wires migrations.

8. **Lazy loading is a bolt-on, not a property of the design.** Because *creating* a
   controller immediately *loads* the space (controller Start → ChangeMode(Loading) →
   loader app starts building), laziness had to be implemented as "don't create the
   controller": `deferredStatuses` backlog + `releasing` + `preloadOnce` + dynamic
   fallback + `ensureSpaceStarted` + `resolveDerivedInfo` + two test-seam hooks, with
   invariants (B1/B2/B3/E2) enforced by comments and careful lock choreography in
   service.go (~150 lines).

9. **Obscure concurrency idioms.** `spaceViewStatus` carries `mx *sync.Mutex` shared
   across value copies to serialize handlers per view (spacesub.go:29);
   `onSpaceStatusUpdated` spawns a goroutine per event; loader and offloader each
   hand-roll the same retry loop (`loadingSpace.loadRetry` / `offloadingSpace`).

10. **Construction does I/O.** `personalspace.NewSpaceController` checks/creates the
    space view; factory `Create*` methods mix storage mutation, view creation and
    controller construction — so "register a controller" can block on the network.

History confirms the cost: GO-5948 ("Refactor space service to use subscriptions"),
GO-6108 ("Fix WaitSpace for tech space"), GO-5935 ("Offload space when user is kicked"),
GO-7292 (lazy loading, 3 commits of lock-choreography fixes), plus the standing TODOs.

---

## 3. Proposed refactoring: one controller, a reconciler instead of a state machine

### 3.1 Key insight

This is a **reconciliation** problem, not an RPC-style state machine problem.
Desired state is already fully described by `(AccountStatus, demand)`; actual state is
"which process app is running". The current design pushes transitions imperatively and
blocks callers on them; everything painful above follows from that. Invert it:

> A per-space goroutine owns the actual state and continuously converges it to the
> latest desired state. Callers never drive transitions; they update inputs and/or wait
> on outcomes.

This is the same TODO already written in `join.go` ("unidirectional model"), applied
consistently.

### 3.2 The pieces

**One controller for all space kinds.** Replace personal/shareable/streamable with a
single implementation parameterized by a descriptor; kind differences become data:

```go
type Descriptor struct {
    SpaceId     string
    IsPersonal  bool              // loader flag + migration component
    GuestKey    crypto.PrivKey    // streamable
    Metadata    []byte
    ExtraLoaderComponents func() []app.Component // personalmigration
    JoinSupported bool            // shareable only
}
```

Marketplace leaves the registry entirely (it is deprecated, GO-6259); `service.Get`
special-cases it exactly like it already special-cases tech space.

**Explicit states, not overloaded modes:**

```go
type State int
const (
    StateDormant State = iota // registered, nothing running  ← lazy by design
    StateLoading
    StateLoaded
    StateJoining
    StateOffloading
    StateOffloaded            // terminal
    StateFailed               // holds the error; retryable
)
```

**The reconciler loop** (replaces `mode.StateMachine`, still one goroutine per space):

```go
// inputs, written atomically by anyone, latest-wins:
//   accountStatus (from watcher / SetPersistentInfo)
//   demand        (false = dormant; true = should be loaded)
func (c *controller) run() {
    for {
        select {
        case <-c.wake:        // buffered(1); tick on any input change
        case <-c.ctx.Done():
            c.teardown(); return
        }
        target := computeTarget(c.inputs())  // pure function, ONE copy:
        // Deleted|Removing → Offloaded ; Joining → Joining(then auto-Active)
        // else: demand ? Loaded : Dormant
        if target != c.state {
            c.transitionTo(target)           // close old process app, start new one
        }
    }
}
```

Properties, each fixing a defect from §2:

- *Latest-wins coalescing*: no transition queue, no `ErrTransitionInProcess`, no
  dropped updates (§2.1). A status flip mid-transition just re-ticks `wake`; the loop
  re-reads inputs after every transition.
- *Nobody blocks on transitions*: `Update`/`SetPersistentInfo` only store inputs +
  tick. Waiting moves to outcome futures: `WaitLoad(ctx)` sets `demand=true` and waits
  on a promise the reconciler resolves on entering `StateLoaded` (with the real error
  on `StateFailed`) — cancellable by caller ctx (§2.2, §2.3, §2.5).
- *Failures are states, not poison*: a failed load → `StateFailed{err}` with
  backoff-retry inside the reconciler; the next demand or status change retries.
  The `waiting` error cache dies (§2.4).
- *Lazy by design*: the registry creates a (cheap, no-I/O) Dormant controller for
  **every** space view at startup. Loading starts only when something sets demand:
  `Get/Wait` (user opened the space), preload release, eager mode (demand=true at
  registration), preferred-space (demand only that one). The entire bolt-on —
  `deferredStatuses`, `releasing`, `preloadOnce`, `ensureSpaceStarted`,
  `resolveDerivedInfo`, `lazyMode` branches — reduces to a per-controller boolean
  (§2.8). Offload/join ignore demand because `computeTarget` ranks status first.

**Processes stay.** The child-app-per-mode pattern (loader/joiner/offloader bundles)
is the *good* part of the current design — keep it, but give processes one uniform
shape and pull the duplicated retry loops up into a shared helper in the reconciler.

**Unidirectional intents.** `Join`/`InviteJoin`/`Delete`/`CancelLeave` only write the
spaceView (create it if needed) and then wait on the controller future. Controller
creation happens in exactly two places: the watcher (view appeared) and
`registry.GetOrCreate` (demand for an existing view). `Wait()`'s polling loop becomes
"wait for view to exist (subscription event), then `GetOrCreate(id).WaitLoad(ctx)`".

**Typed interface.** `Current() any` disappears:

```go
type SpaceController interface {
    SpaceId() string
    State() State
    SetStatusInfo(spaceinfo.SpacePersistentInfo) // input write + wake
    Demand()                                     // input write + wake
    WaitLoad(ctx) (clientspace.Space, error)     // demand + future
    WaitOffload(ctx) error
    Close(ctx) error
}
```

### 3.3 What gets deleted

| Today | After |
|---|---|
| 3 near-identical controller packages (~600 LOC) | 1 controller (~250 LOC) |
| `mode.StateMachine` + waiters + notify dance | ~60-line reconciler loop |
| `waiting` map + error caching + close/wait choreography in 5 files | controller futures |
| 6 copies of the AccountStatus→Mode switch | 1 pure `computeTarget` |
| lazy bolt-on (~150 LOC + 2 test hooks + B1/B2/B3/E2 comments) | `demand` flag |
| `Wait` 500 ms polling | event-driven wait |
| `Current().(loader.LoadWaiter)` assertions | `WaitLoad` on the interface |
| marketplace fake controller | service-level special case (then delete with GO-6259) |

### 3.4 Incremental migration (each phase ships independently)

1. **Dedup controllers** (mechanical, low risk): merge personal/shareable/streamable
   into one implementation + `Descriptor`, keeping `mode.StateMachine` and the existing
   `SpaceController` interface byte-for-byte. Kills §2.7 and halves the surface for the
   next phases. Existing tests keep passing.
2. **Swap the state machine internals for the reconciler**, preserving the public
   interface (`Mode()` derived from `State` for `join.go`/`AllLoadedSpaceIds` compat).
   Fixes dropped transitions, blocked waiters, swallowed errors.
3. **Introduce `StateDormant` + `Demand()`** and move lazy-mode logic out of
   service.go; registry creates Dormant controllers for all views; preferred/preload
   becomes demand wiring.
4. **Unidirectional intents**: rewrite `Join`/`InviteJoin`/`Delete`/`AddStreamable` to
   write-view-then-wait; delete the `waiting` map and the polling waiter.
5. **Cleanups**: typed interface (`Current()` removal), retire marketplace controller
   (GO-6259), unify loader/offloader retry helpers.

### 3.5 Risks / open questions

- `clientspace.Space` consumers cache the pointer returned by `Get`; with
  Dormant→Loaded→Dormant cycles (future unload support) those references go stale.
  V1: no automatic unloading — Dormant is only an *initial* state, same semantics as
  today's deferral. Unloading-on-idle becomes possible later precisely because the
  reconciler makes Loaded→Dormant a legal transition.
- Join flow timing: today `Join` returns once the controller exists; in the
  unidirectional model it should wait for `StateJoining` to be reached (subscribe to
  the controller future), not just for the view write — needs an explicit
  `WaitState(ctx, StateJoining)` or a joining future.
- The optimistic `LocalStatusOk` fast-path in `spaceloader.startLoad` (cold-start UX)
  must survive: it is process-internal and unaffected by the reconciler, but tests
  around it should be carried over.
- Personal-space migrations (`WaitMigrations`) are reachable via the `Personal`
  interface; with a unified controller this becomes a descriptor-provided component
  handle — verify `core/` callers.
