# spacev2 Milestone 3 — Controllers, state machine, load pipeline

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Real controllers behind the M2 registry: one kind-parameterized controller + a cleaned state machine + the (reused) v1 load pipeline, so `Get`/`Wait` return loaded `clientspace.Space`s and `Create`/`createFirstSpace` work through the unidirectional path.

**Architecture:** v2 keeps M2's unidirectional spine and fills `builderFor`. ONE `controllerImpl` replaces v1's personal/shareable/streamable triplicate (they differ only in mode mapping + loader params). The v1 `space/internal/` packages `loader`, `initial`, `spacestatus`, and all pipeline components are REUSED verbatim (importable from `space/spacev2` — Go internal visibility roots at `space/`); they encode the byte-exact contracts (§10 crypto, LocalStatus writes) and stay v1-owned until a later cleanup milestone.

**Tech Stack:** as M2; plus `space/internal/spaceprocess/{loader,initial}`, `space/internal/components/{spacestatus,personalmigration}`.

## Global Constraints

Same as the M2 plan (`2026-07-02-spacev2-m2-foundation.md`) — v1 untouched, v2 unregistered, build+tests green per task, CLAUDE.md error wrapping + fixture/`want` test style — plus:

- Commits: `GO-7348 spacev2: <description>`; branch `go-7348-spacecontroller-refactor`.
- §9.4/§9.5: the state machine closes the old process (drains) strictly BEFORE starting the next; every `Process(md)` result is a FRESH instance.
- §9.6: same-mode → no-op; different in-flight target → `ErrTransitionInProcess`; same-target callers piggyback; buffer-1 notify; start-failure falls back to `ModeInitial` (must always start).
- §9.3: processes publish lifecycle changes through the SpaceView (spacestatus) only.

## Locked design decisions

1. **One controller type** (`controllerImpl`), `spaceKind ∈ {personal, shareable, streamable}`; one-to-one spaces build as shareable (v1 parity).
2. **v2 state machine**: `Process` = `{Start, Close}` — `CanTransition` DROPPED (§11.3; gating lives in the controller's mode mapping). Waiters receive `(Process, error)` — fixes v1's `TODO [MR] send error to waiter` (a failed start now reports its cause, not bare `ErrFailedToStart`). All other §9.6 semantics preserved.
3. **Offloading is NOT terminal**: Offloading→Loading allowed (CancelLeave/restore). v1's test `DeletingInvalid` (expects rejection via a test-only `CanTransition=false`) is deliberately NOT ported — production v1 processes all returned `true` anyway.
4. **`AccountStatusRemoving` collapsed** → Offloading for every kind (§11.5). Deviation from v1 personal/streamable (which mapped Removing→Loading): Removing is never written, so unobservable.
5. **§9.7 fixed**: `lastUpdatedStatus` rolls back when `ChangeMode` fails, so the next event with the same status retries instead of wedging the controller. Initialized to a sentinel (`accountStatusUnset = -1`) so `Start` (= `Update`) always drives the first transition.
6. **Typed `WaitLoad`** asserts the current process to `loader.LoadWaiter` in ONE place; wrong mode → `ErrFailedToLoad` (v1 `service.waitLoad` parity).
7. **`WaitMigrations` dropped** — no callers outside `space/internal` (grep-verified; streamablespace's `Personal` iface is a copy-paste artifact).
8. **Loader params by kind (v1 parity, byte-for-byte):** personal → `{SpaceId, IsPersonal: true, OwnerMetadata: metadata, AdditionalComps: [personalmigration.New()]}`; shareable → `{SpaceId}` (NO metadata — v1 quirk, keep); streamable → `{SpaceId, OwnerMetadata: metadata, GuestKey: decoded ed25519}`.
9. **Offloading/Joining processes are M4**: `Process()` returns a stub whose `Start` fails with `errModeNotImplemented` → state machine falls back to Initial and reports the error to waiters. A Deleted space at M3 sits in Initial (logged), never loads — acceptable until M4's offloader.
10. **Controller construction**: `makeStatusApp` port — child app of `techProvider.ControllerApp()` (the app with the tech space registered; new accessor on the `techSpaceProvider` seam) + `spacestatus.New(spaceId)` registered + started. Process factory injectable for tests (`procs processFactory` param; production default built from kind).
11. **`create()` is unidirectional**: `spaceCore.Create` → `MarkSpaceCreated` → `SpaceViewCreate(ctx, id, true, info(Unknown), desc)` → `registry.await(ctx, id)` (the watcher builds) → `ctrl.WaitLoad`. `Create` routes OneToOne → `errNotImplemented` (M4); `createFirstSpace` = `create(ctx, nil)` + set `firstCreatedSpaceId`. Coordinator-status kick stays M4.

---

### Task 1: State machine

**Files:** Create `space/spacev2/statemachine.go`, `space/spacev2/statemachine_test.go`.

**Produces:**
```go
type smProcess interface {
	Start(ctx context.Context) error
	Close(ctx context.Context) error
}
type processFactory interface{ Process(md Mode) smProcess }
var ErrTransitionInProcess = errors.New("transition in process")
var ErrFailedToStart = errors.New("failed to start")
func newStateMachine(factory processFactory, log logger.CtxLogger) (*stateMachine, error)
func (s *stateMachine) ChangeMode(next Mode) (smProcess, error) // blocks until done
func (s *stateMachine) GetMode() Mode
func (s *stateMachine) GetProcess() smProcess
func (s *stateMachine) Close()
```

Implementation = v1 `mode/statemachine.go` port with: v2 `Mode`, no `CanTransition`, waiters as `chan waitResult{proc, err}` so start failures carry the cause (`fmt.Errorf("start %v process: %w ... ErrFailedToStart", ...)` wrapping the real error), same loop/notify/fallback structure otherwise.

**Tests (port + extend):** initial mode is Initial and its process started; ChangeMode same mode no-ops (same process returned, no rebuild); divergent concurrent target → `ErrTransitionInProcess`; same-target concurrent callers piggyback (10 goroutines, one transition — v1 `MultipleWaiters` shape at SM level); start failure → waiters get error wrapping the cause + fallback to Initial + a later ChangeMode(Loading) succeeds (retry after failure); close-old-before-start-new ordering (recording processes assert Close(old) happens-before Start(new)); fresh instance per transition (factory call count); Close drains.

Steps: failing tests → run (`go test ./space/spacev2/ -run TestStateMachine -race`) → implement → pass → commit `GO-7348 spacev2: state machine (typed errors, no CanTransition)`.

### Task 2: Controller

**Files:** Create `space/spacev2/controllerimpl.go`, `space/spacev2/controllerimpl_test.go`.

**Produces:**
```go
type spaceKind int // kindPersonal, kindShareable, kindStreamable
type controllerParams struct {
	spaceId  string
	kind     spaceKind
	metadata []byte           // account metadata payload (owner metadata for loader)
	guestKey crypto.PrivKey   // streamable only
	parent   *app.App         // techProvider.ControllerApp()
	procs    processFactory   // nil → production factory
}
func newController(params controllerParams) (SpaceController, error)
```

`controllerImpl` implements the v2 `SpaceController` interface (controller.go): status app via `makeStatusApp` port (child app + `spacestatus.New` + Start); `newStateMachine`; `targetMode(status)`:

```go
func (c *controllerImpl) targetMode(status spaceinfo.AccountStatus) Mode {
	switch status {
	case spaceinfo.AccountStatusDeleted, spaceinfo.AccountStatusRemoving: // Removing collapsed, §11.5
		return ModeOffloading
	case spaceinfo.AccountStatusJoining:
		if c.kind == kindShareable {
			return ModeJoining
		}
		return ModeLoading
	default:
		return ModeLoading
	}
}
```

`Update` with the §9.7 rollback (full shape locked):

```go
const accountStatusUnset = spaceinfo.AccountStatus(-1)

func (c *controllerImpl) Update() error {
	c.mx.Lock()
	status := c.status.GetPersistentStatus()
	if c.lastUpdatedStatus == status {
		c.mx.Unlock()
		return nil
	}
	prev := c.lastUpdatedStatus
	c.lastUpdatedStatus = status
	c.mx.Unlock()
	_, err := c.sm.ChangeMode(c.targetMode(status))
	if err != nil {
		// §9.7 fix: roll back so the next event with this status retries
		// instead of no-opping forever.
		c.mx.Lock()
		if c.lastUpdatedStatus == status {
			c.lastUpdatedStatus = prev
		}
		c.mx.Unlock()
		return fmt.Errorf("change mode: %w", err)
	}
	return nil
}
```

`Start(ctx)` = `c.Update()` (sentinel init makes the first call always transition). `WaitLoad` per locked decision 6. `SetPersistentInfo` = status write + `Update` (v1 parity); `SetLocalInfo` = status write. `Close` = `sm.Close()` then `statusApp.Close(ctx)`. Production `processFactory` (same file): Initial → `initial.New()` (v1 reused; adapterless — it has Start/Close), Loading → `loader.New(c.statusApp, params by kind)` fresh per call, Offloading/Joining → `notImplementedProcess{mode}` failing Start with `errModeNotImplemented`.

**Tests** (spaceStatusStub port from v1 `shareable_test.go:111` + fake processes + modeRegister): Joining→invite-accept→Loading (shareable); Loading→Deleted→Offloading; 10-goroutine Update storm → one transition; Deleted at start → Offloading; **Offloading→Active→Loading succeeds** (CancelLeave, replaces v1 `DeletingInvalid`); Removing→Offloading for all three kinds (table); Joining→Loading for personal/streamable (table); WaitLoad in non-loading mode → `ErrFailedToLoad`; §9.7: failing process factory → Update errors, then a retry with the same status transitions (v1 would no-op forever).

Steps: failing tests → implement → pass (`-race`) → commit `GO-7348 spacev2: kind-parameterized controller replacing the v1 triplicate`.

### Task 3: builderFor dispatch + ControllerApp seam

**Files:** Modify `space/spacev2/service.go` (real `builderFor`), `space/spacev2/techprovider.go` (+`ControllerApp()`), `space/spacev2/bootstrap_test.go`/`service_test.go` (fake provider + dispatch tests).

- `techSpaceProvider` gains `ControllerApp() *app.App` (real: the childApp created in `run`; fake: a test app carrying mock techspace).
- `builderFor(info)`: personal id → kindPersonal(metadata); `info.EncodedKey != ""` → kindStreamable (decode via `crypto.DecodeKeyFromString(info.EncodedKey, crypto.UnmarshalEd25519PrivateKey, nil)`, error → build failure, retryable); else kindShareable. Passes `s.testProcFactory` (test seam field, nil in production) into `controllerParams.procs`; calls `newController` then `ctrl.Start(ctx)`; Start failure closes the controller and returns the error (registry keeps it retryable).

**Tests:** dispatch table (personal id / encoded key / plain → expected kind, probed via a `testProcFactory` recording the loader params or via controller kind accessor); broken encoded key → build error surfaces through `registry.ensure` and a later good event retries.

Commit `GO-7348 spacev2: builderFor dispatch — watcher builds real controllers`.

### Task 4: create() / Create / createFirstSpace

**Files:** Modify `space/spacev2/service.go` + `service_test.go`. Uses `anystorage.ClientSpaceStorage` for `MarkSpaceCreated` (v1 factory parity).

```go
func (s *service) create(ctx context.Context, description *spaceinfo.SpaceDescription) (clientspace.Space, error) {
	if s.isClosing.Load() {
		return nil, ErrSpaceIsClosing
	}
	coreSpace, err := s.spaceCore.Create(ctx, spacedomain.SpaceTypeRegular, s.repKey, s.accountMetadataPayload)
	if err != nil {
		return nil, fmt.Errorf("create space: %w", err)
	}
	if err = coreSpace.Storage().(anystorage.ClientSpaceStorage).MarkSpaceCreated(ctx); err != nil {
		return nil, fmt.Errorf("mark space created: %w", err)
	}
	info := spaceinfo.NewSpacePersistentInfo(coreSpace.Id())
	info.SetAccountStatus(spaceinfo.AccountStatusUnknown)
	if err = s.techSpace.SpaceViewCreate(ctx, coreSpace.Id(), true, info, description); err != nil {
		return nil, fmt.Errorf("create space view: %w", err)
	}
	// Unidirectional: the watcher reacts to the new SpaceView and builds the
	// controller; await the registry future (caller ctx bounds the wait).
	ctrl, err := s.registry.await(ctx, coreSpace.Id())
	if err != nil {
		return nil, fmt.Errorf("await space controller: %w", err)
	}
	sp, err := ctrl.WaitLoad(ctx)
	if err != nil {
		return nil, convertSpaceError(err)
	}
	// TODO(M4): updater.UpdateCoordinatorStatus()
	return sp, nil
}
```

`Create`: isClosing guard; OneToOne description → `errNotImplemented` (M4); else `create`. `createFirstSpace`: hook if set (existing tests), else `sp, err := s.create(ctx, nil)`; `s.firstCreatedSpaceId = sp.Id()`.

**Tests:** create happy path — mocks for `spaceCore.Create` (AnySpace with mock commonspace + mock `ClientSpaceStorage` for MarkSpaceCreated — see v1 `service_test.go:317-319` for the AnySpace mock shape) and techspace `SpaceViewCreate` whose `.Run` side-effect simulates the watcher (`registry.ensure` with a fake controller returning a mock space) → returns that space; SpaceViewCreate error → wrapped error, no await; isClosing → `ErrSpaceIsClosing`.

Commit `GO-7348 spacev2: unidirectional create/Create/createFirstSpace`.

### Task 5: End-to-end integration + ordering stress

**Files:** Modify `space/spacev2/service_test.go`.

- **Update the M2 enumeration test**: with `testProcFactory` injected (fake loading process exposing `WaitLoad` → mock space per spaceId) and techMock `GetSpaceView` returning a stub SpaceView (port v1's `spaceStatusStub` approach — the real `spacestatus` component needs `GetSpaceView`; give the fixture a `mock_techspace.MockSpaceView` with `Lock/Unlock/GetPersistentInfo/GetLocalInfo` stubs), the three seeded SpaceViews now build REAL v2 controllers: assert `registryEntryState == stateReady`, `ctrl.Mode() == ModeLoading`, and `fx.Get(ctx, spaceId)` returns the mock space (M3 gate: spaces open across spaces at the service level).
- **Per-space ordering stress** (the M2 caveat): one space, 50 rapid `onSpaceStatusUpdated` events alternating Active/Deleted statuses against a recording controller; assert `Update` calls are strictly serialized (no overlap — recording enter/exit counters) and the final state settles. Different spaces proceed concurrently (two spaces, blocked first build on A doesn't delay B — channel-gated fake builder).

Run everything: `go test ./space/... -count=1` + `go build ./...`. Commit `GO-7348 spacev2: M3 integration gate — Get returns loaded spaces end-to-end`.

### Task 6: Bookkeeping

- HANDOFF.md milestone 3 → DONE with summary; note M4 preconditions (offloader/joiner processes, remote-deleted branch, updater kick, one-to-one create).
- Update memory file `spacev2-clean-room-project.md` (M3 state).
- Full suite paste; commit.

## Self-review notes

- v1 behaviors deliberately changed (all sanctioned): CanTransition dropped (§11.3), Removing collapsed (§11.5), error-carrying waiters (v1 TODO), §9.7 rollback, WaitMigrations dropped, one controller type. Everything else byte-parity, incl. the shareable-loader-has-no-metadata quirk.
- Reuse boundary: v1 `loader/initial/spacestatus/personalmigration` imported as-is; v2 owns statemachine + controller + dispatch. `offloader`/`joiner` intentionally not touched until M4.
- Type consistency: `smProcess`/`processFactory` (T1) consumed by controller (T2) and test fakes (T2/T5); `controllerParams.procs` seam threads T2→T3→T5; v1 `loader.Loader` satisfies `smProcess` + `loader.LoadWaiter` structurally.
