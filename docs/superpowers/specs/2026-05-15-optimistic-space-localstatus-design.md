# Optimistic localStatus for spaces already on disk

Date: 2026-05-15
Status: Approved (Approach A)

## Problem

Mobile clients render and subscribe the spaces list based on `spaceView.spaceLocalStatus == Ok`
(`spaceinfo.LocalStatusOk`, value `2`). On a cold app open with N existing spaces, every space
goes through `Unknown (0) -> Loading (1) -> Ok (2)` before the client will show it:

1. `SpaceView.Init` loads the spaceview from the object store — where `localStatus` was persisted
   as `Ok` from the previous session — and **unconditionally overwrites it with `Unknown`**
   (`core/block/editor/spaceview.go:95`). The client sees the space drop out of the list.
2. The space controller starts the loader; `spaceLoader.startLoad` sets `Loading`
   (`space/internal/components/spaceloader/spaceloader.go:104-106`).
3. `loadingSpace.loadRetry` runs `BuildSpace` + `WaitMandatoryObjects` (ReindexSpace + loading
   the derived/mandatory object trees from local storage). This is the genuinely slow part and
   it is effectively serialized across all spaces.
4. `spaceLoader.onLoad` finally sets `Ok` (`spaceloader.go:118-120`). The client re-shows the space.

The result: with many spaces, the list is empty/churning for a noticeable time after opening the
app from a closed state, even though all the data is already on disk.

Per product clarification:

- The client uses `Ok` **only to populate the spaces list**. Opening a specific space triggers a
  separate load that already blocks until the space is ready (via `spaceLoader.WaitLoad`).
- An `Ok -> Missing` transition in the rare case where a background rebuild genuinely fails
  (invalid changes, space deleted remotely while offline) is acceptable.

This makes an **optimistic `Ok`** strategy viable: for a space whose store already exists on disk
and which was successfully loaded before, report `Ok` immediately and rebuild in the background,
keeping the open-on-demand path correct.

## Goals

- No `Unknown` or `Loading` churn for spaces that already exist on disk and were previously `Ok`.
- The spaces list is stable and instant across app restarts.
- Opening a space still blocks correctly until its mandatory objects are actually loaded.
- No new client-facing status or contract change.

## Non-goals

- Speeding up `ReindexSpace` / `WaitMandatoryObjects` itself. The heavy work still runs in the
  background; we only remove it from the path that gates list visibility.
- Changing behavior for brand-new spaces, joined spaces, or spaces with no local storage — those
  keep the existing `Unknown -> Loading -> Ok` flow.
- Eager/parallel preloading of all spaces.

## Design (Approach A)

Three coordinated changes, plus one correctness-critical decoupling.

### 1. `SpaceView.Init` — preserve persisted localStatus instead of resetting

`core/block/editor/spaceview.go:94-98` currently does:

```go
localInfo := spaceinfo.NewSpaceLocalInfo(spaceId)
localInfo.SetLocalStatus(spaceinfo.LocalStatusUnknown).
    SetRemoteStatus(spaceinfo.RemoteStatusUnknown).
    UpdateDetails(ctx.State).
    Log(log)
```

Change it to **preserve the previously persisted local status/remote status** (read from
`ctx.State` via `spaceinfo.NewSpaceLocalInfoFromState`) rather than forcing `Unknown`:

- If the spaceview already has a persisted `localStatus` (loaded from the object store), keep it.
- If there is no persisted value (brand-new spaceview), it defaults to `Unknown` exactly as
  before (`SpaceLocalInfo.GetLocalStatus()` returns `LocalStatusUnknown` when unset).

Init must not be the component that destroys a good `Ok`. The loader becomes the sole authority
for transitions. The explicit reset paths that *should* force `Unknown` remain unchanged and still
work:

- `space/spacefactory/spacefactory.go:350-354` (recreate a removed space, when not `Ok`)
- `space/internal/spaceprocess/joiner/statuschanger.go:32` (rejoin)

Rationale for keeping the storage existence guard out of the editor layer: `core/block/editor`
should not depend on `space/spacecore/storage`. The "is the persisted `Ok` still trustworthy"
decision is made in the loader (next section), which already depends on the storage service.

### 2. `spaceLoader.startLoad` — skip `Loading` when on disk and previously `Ok`

`space/internal/components/spaceloader/spaceloader.go:97-111`. Today it unconditionally sets
`Loading`. New logic:

```
current := s.status.GetLocalStatus()
if current == LocalStatusOk && storageService.SpaceExists(spaceId) {
    // optimistic fast path: leave localStatus == Ok, just start the background build
} else {
    // current behavior: set Loading
}
// in both cases: start the background loadingSpace as today
```

- `storageService.SpaceExists(spaceId)` is the synchronous directory check at
  `space/spacecore/storage/anystorage/storageservice.go:203-212`. It guards against a stale
  persisted `Ok` when the on-disk store was wiped/corrupted externally — in that case we fall
  back to `Loading`, and the build will correct it.
- The space loader package already depends on the storage service via the builder; the loader
  needs a direct reference to `storageService` (added as an `app` component dependency in
  `spaceLoader.Init`).
- The background `loadingSpace` (retry loop, `BuildSpace`, `WaitMandatoryObjects`) is started in
  **both** branches — the only change is whether we publish `Loading`.

`onLoad` (`spaceloader.go:113-132`) is unchanged: success -> `Ok` (an idempotent no-op in the
fast path), failure -> `Missing`. The accepted `Ok -> Missing` regression flows naturally here.

### 3. `spaceLoader.WaitLoad` — drive readiness off internal loader state, not localStatus (correctness-critical)

`space/internal/components/spaceloader/spaceloader.go:138-170` currently switches on
`s.status.GetLocalStatus()`. With an optimistic `Ok`, the existing
`case LocalStatusOk: sp = s.space` would return a **nil space** because the build has not
finished. This must change.

Rework `WaitLoad` to be driven by the loader's own lifecycle, independent of the persisted
client-facing status:

- `s.loading == nil` -> loader not started yet -> return an error
  (equivalent to today's "waitLoad for an unknown space").
- Otherwise (mutex held, mirroring current locking):
  - if the load finished and `s.space != nil` -> return `s.space`;
  - if the load finished with an error (`loading.getLoadErr() != nil`) -> return that error;
  - otherwise wait on `loading.loadCh` (respecting `ctx.Done()`), then re-evaluate (recurse as today).

This keeps open-on-demand fully correct: while the list shows an optimistic `Ok`, the first
attempt to actually open the space blocks on `loadCh` until the real build completes (or returns
the real error). The persisted `localStatus` and the loader's internal readiness become two
separate concerns, which is what makes optimistic `Ok` safe.

## Data flow (cold start, existing space already on disk, previously Ok)

```
object store ──> SpaceView.Init: localStatus preserved as Ok (no write of Unknown)
              └─> client subscription: space already shown as Ok, no churn
controller.Run ──> spaceLoader.startLoad:
                     GetLocalStatus()==Ok && SpaceExists==true
                     => do NOT set Loading; start loadingSpace in background
background      ──> BuildSpace + WaitMandatoryObjects (ReindexSpace, load derived trees)
                 └─> onLoad(nil): SetLocalStatus(Ok)  // idempotent no-op
client opens X  ──> spaceLoader.WaitLoad: loading!=nil, space==nil
                     => block on loadCh until build done => return built space
```

Failure variant: `onLoad(err)` -> `SetLocalStatus(Missing)` -> client list flips `Ok -> Missing`,
`WaitLoad` returns the error. Accepted, rare.

Brand-new / joined / no-storage variant: `GetLocalStatus()` is `Unknown` (or storage absent) ->
`startLoad` sets `Loading` -> unchanged existing behavior.

## Edge cases

- **Stale `Ok`, storage wiped externally:** `SpaceExists` is false -> fall back to `Loading`,
  build corrects to `Ok`/`Missing`.
- **Space deleted remotely while app was closed:** persisted `Ok` -> optimistic `Ok` ->
  background build fails with `ErrSpaceIsDeleted` -> `onLoad` sets `Missing` (+ remote status).
  Accepted `Ok -> Missing`.
- **Client opens a space before `startLoad` ran:** `s.loading == nil` -> `WaitLoad` returns the
  "not started" error, same observable behavior as today's `Unknown` branch. Window is small
  (controller `Run` calls `startLoad` immediately after Init).
- **Marketplace space:** hardcodes `LocalStatusOk` with no loader — unaffected.
- **Explicit reset paths (recreate-after-removal, rejoin):** still set `Unknown` explicitly;
  unaffected by removing the unconditional reset in `Init`.

## Key implementation risk to verify first

`localStatus` is stored in the spaceview's **local details** (`RelationKeySpaceLocalStatus`).
The design assumes the previously persisted value is present in `ctx.State` during
`SpaceView.Init` (hydrated from the object store, the same mechanism that keeps the spaces list
populated across restarts). This must be confirmed before relying on the Init-side preservation.
If local details are *not* populated in `ctx.State` at `Init` time, the preservation read must
move to the appropriate hook or read the object store directly. The implementation plan must
front-load this verification with a focused test/spike.

## Testing

- `spaceLoader.startLoad`:
  - status `Ok` + `SpaceExists` true -> does not write `Loading`; background load started.
  - status `Ok` + `SpaceExists` false -> writes `Loading` (stale-status correction).
  - status `Unknown` -> writes `Loading` (unchanged behavior).
- `spaceLoader.WaitLoad` (internal-state driven):
  - loader not started -> error.
  - optimistic `Ok`, build in progress -> blocks on `loadCh`, then returns the built space.
  - build failed -> returns the load error.
  - happy path after normal `Loading` -> returns space.
- `SpaceView.Init`:
  - previously persisted `localStatus == Ok` -> preserved as `Ok` (no `Unknown` write).
  - no persisted value (new spaceview) -> `Unknown`.
- `spaceLoader.onLoad`: success keeps/sets `Ok` (idempotent), failure sets `Missing` — unchanged.
- Regression scope check: existing spaceloader / spacefactory / joiner tests still pass; the
  explicit `Unknown` reset paths still produce `Unknown`.
- Behavioral check: simulate cold start with an existing on-disk space previously `Ok`; assert
  the spaceview detail for `spaceLocalStatus` never transitions through `Unknown`/`Loading`.

## Files in scope

- `core/block/editor/spaceview.go` (`Init` — preserve persisted local/remote status)
- `space/internal/components/spaceloader/spaceloader.go` (`Init` dep on storage service,
  `startLoad` fast path, `WaitLoad` internal-state rework)
- `space/internal/components/spaceloader/loadingspace.go` (only if `WaitLoad` rework needs
  internal accessors)
- Tests alongside the above.
