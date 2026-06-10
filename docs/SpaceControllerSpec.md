# Spec: space lifecycle architecture (v2)

Target architecture for the `space/` service layer. Background and evidence for each
decision: [SpaceControllerRefactor.md](SpaceControllerRefactor.md).

## 1. Goals

- **G1** One controller implementation for all account spaces (personal, shareable,
  streamable, one-to-one). Kind differences are data, not code.
- **G2** Lazy loading is a first-class state: registering a space is cheap and does not
  load it. Loading happens only on demand.
- **G3** Unidirectional flow: intents mutate the spaceView; a reconciler converges the
  running state to it. Nothing drives transitions imperatively.
- **G4** Errors are values: callers receive the real error; failures are retryable
  states, never session-permanent.
- **G5** Every wait is cancellable; no caller ever blocks on a transition.
- **G6** The status→behavior mapping exists in exactly one place.

## 2. Non-goals (v1)

- Unload-on-idle (Loaded→Dormant). The design must allow it; v1 does not implement it.
- Changing the spaceView data model, techspace, or tech-space bootstrap.
- Changing process internals (loader / joiner / offloader component bundles).
- Marketplace redesign: it leaves the controller registry and becomes a `service.Get`
  special case (like tech space) until deleted with GO-6259.

## 3. Core model

### 3.1 States

```
Dormant     registered, nothing running            ← the lazy state
Loading     loader process running (incl. its internal retries)
Loaded      loader finished; clientspace.Space available
Joining     joiner process running (aclwaiter)
Offloading  offloader process running
Offloaded   local data removed; re-entrant (see 3.3)
Failed      last transition failed non-retryably; holds the error
```

### 3.2 Inputs and target function

Each controller has exactly two inputs, written latest-wins:

- `status` — `spaceinfo.AccountStatus` from the spaceView (watcher or direct set)
- `demand` — local bool: someone wants this space loaded

One pure function defines all behavior (the only copy in the codebase):

```go
func computeTarget(status AccountStatus, demand bool) State {
    switch status {
    case Deleted, Removing:        return Offloaded
    case Joining:                  return Joining
    default: /* Active, Unknown */ if demand { return Loaded } else { return Dormant }
    }
}
```

### 3.3 Reconciler

One goroutine per controller owns the actual state. It is the **single writer**: only
it starts/stops process apps.

```
loop:
  wait for wake (buffered-1 chan, ticked by any input write) or ctx.Done
  target := computeTarget(inputs())
  if target != state: transition (close current process app, start next)
  resolve/refresh waiters' futures
```

- Latest-wins: a status flip mid-transition re-ticks `wake`; the loop re-reads inputs
  after every transition. There is no transition queue and no "transition in process"
  error.
- Retry policy: errors the process classifies as retryable stay inside the process
  (loader keeps its internal backoff loop, as today). A non-retryable transition
  failure → `Failed{err}`; the reconciler retries on the next input change.
- `Offloaded` is re-entrant: if status returns to Active (CancelLeave) and demand
  exists, the reconciler loads again (re-fetch from network). No terminal-state special
  case.

### 3.4 Controller interface

```go
type SpaceController interface {
    SpaceId() string
    State() State                       // also exposes Failed error
    SetStatusInfo(spaceinfo.SpacePersistentInfo) error // persist to view + input write
    SetLocalInfo(spaceinfo.SpaceLocalInfo) error       // passthrough to view
    Demand()                                           // input write
    WaitLoad(ctx) (clientspace.Space, error)           // Demand() + future
    WaitState(ctx, State) error                        // join/offload waits
    Close(ctx) error
}
```

`WaitLoad` blocks until `Loaded` (returns the space), `Failed` (returns the real
error), controller close (`ErrSpaceIsClosing`), or ctx cancellation. No `Current() any`,
no type assertions, no `Mode()`.

### 3.5 Descriptor

Controller construction takes a descriptor and performs **no I/O**:

```go
type Descriptor struct {
    SpaceId               string
    IsPersonal            bool            // loader flag
    GuestKey              crypto.PrivKey  // streamable; nil otherwise
    OwnerMetadata         []byte
    ExtraLoaderComponents func() []app.Component // e.g. personalmigration
}
```

The descriptor is derived from the spaceView (`EncodedKey` → GuestKey, id ==
personalSpaceId → IsPersonal), in one place in the registry.

## 4. Service layer

### 4.1 Registry

`registry.GetOrCreate(id) SpaceController` — creates a Dormant controller from the
spaceView-derived descriptor. Replaces both `spaceControllers` and the `waiting`
singleflight/error-cache maps. Plain map + mutex; creation is cheap and synchronous.

### 4.2 Flows

- **Watcher** (unchanged subscription): view added → `GetOrCreate(id)`; view changed →
  `ctrl.SetStatusInfo(...)`. Delivery is a non-blocking input write — no
  goroutine-per-event, no shared-mutex-in-copied-struct idiom.
- **Get/Wait**: tech space and marketplace special-cased; else
  `GetOrCreate(id).WaitLoad(ctx)`. `Wait` differs from `Get` only by first waiting
  (event-driven, on the watcher) for the spaceView to exist. No polling.
- **Create*** (new space / one-to-one / streamable): create storage + spaceView as
  today, then `GetOrCreate(id)` + `WaitLoad(ctx)`. Factory builds data, not
  controllers.
- **Join / InviteJoin / Delete / CancelLeave**: write the spaceView (create if absent),
  then `GetOrCreate(id).WaitState(ctx, Joining / Loaded / …)` as the RPC requires.
  This is the unidirectional model from the `join.go` TODO.

### 4.3 Demand wiring (lazy loading)

| Trigger | Effect |
|---|---|
| eager mode (no preferredSpaceId) | `Demand()` on every controller at registration |
| lazy mode | `Demand()` only on the preferred space |
| `Get`/`Wait` on a space | `WaitLoad` ⇒ implicit `Demand()` |
| preload release (RPC / timer / preferred-broken fallback) | `Demand()` on all registered controllers |

This table replaces `deferredStatuses`, `releasing`, `lazyMode` branches,
`ensureSpaceStarted`, `resolveDerivedInfo`, and both test-seam hooks.

## 5. Invariants

1. Only the reconciler goroutine starts or stops process apps (single writer).
2. Target is always computed from current inputs by `computeTarget`; transitions are
   never queued or requested by name.
3. Input writes never block; callers block only in `Wait*`, always with ctx.
4. Every outstanding `Wait*` resolves on controller close.
5. Deletion/offload outranks demand (encoded in `computeTarget`'s ordering).
6. Controller construction and registry registration perform no I/O.
7. An error can only be observed through a `Wait*` return or `State()`; no error is
   cached past the next input change.

## 6. Shutdown

`service.Close` cancels all controller contexts in parallel and waits. Each reconciler
tears down its current process app; all futures resolve with `ErrSpaceIsClosing`.

## 7. Open questions

- **Deletion controller interplay**: offloader currently calls
  `delController.AddSpaceToDelete`; with re-entrant Offloaded, confirm a
  CancelLeave→reload also removes the space from the deletion queue.
- **`WaitMigrations`** (personal space): exposed today via the `Personal` interface;
  becomes a handle returned by `ExtraLoaderComponents` wiring — verify `core/` callers.
- **Status enum for clients**: `Loaded` vs `Loading` is now observable; decide whether
  to surface it in `spaceinfo.LocalStatus` or keep mapping both to `Ok`/`Loading` as
  today (optimistic-Ok fast path in `spaceloader` is unaffected either way).
