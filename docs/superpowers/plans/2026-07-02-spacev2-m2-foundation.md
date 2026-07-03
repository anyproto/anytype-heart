# spacev2 Milestone 2 — Foundation (bootstrap, registry, watcher) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the spacev2 unidirectional foundation — service Init/Run bootstrap, tech-space resolution, controller registry with retryable ready-futures, and the reactive SpaceView watcher — to the point where an existing account's SpaceViews are enumerated and tracked, with parity oracle tests against v1.

**Architecture:** Unidirectional lifecycle (user-confirmed 2026-07-02): writing a SpaceView is the ONLY way a controller comes to exist; the watcher is the single builder path. Outward verbs (Create/Join/Delete — Milestone 3+) write the SpaceView + prep storage, then await the registry. The registry replaces v1's `spaceControllers` + `waiting` maps with one entry type carrying a retryable ready-future (eliminates the §9.8 waiting-map poisoning and §9.9 dual-path dedup races by construction).

**Tech Stack:** Go, any-sync `app.App` component model, `core/subscription/objectsubscription`, `space/dedupqueue` (reused), testify + mockery mocks (existing `mock_spacecore`, `mock_techspace` packages).

## Global Constraints

(from `space/spacev2/HANDOFF.md` + `docs/SpaceController.md` + `CLAUDE.md` — apply to every task)

- Do NOT delete or edit v1 `space/` (it is the oracle). New code goes under `space/spacev2/` only.
- `space/spacev2` stays UNREGISTERED in `core/anytype/bootstrap.go` (CName stays `client.space.v2`).
- `go build ./...` green after every task; `go test ./space/spacev2/...` green after every task.
- The `spacev2.Service` interface in `service.go` must not change (mirrors v1 verbatim).
- Deterministic ids: personal = `spaceCore.DeriveID(ctx, spacedomain.SpaceTypeRegular)`, tech = `spaceCore.DeriveID(ctx, spacedomain.SpaceTypeTech)` — same calls as v1, never reimplemented.
- Account metadata: only via `domain.DeriveAccountMetadata(signKey)` (SLIP-0021, byte-exact contract).
- §9.1: every SpaceView status write stays `Equal()`-guarded (that guard lives inside `techspace`/spaceview editor — reused, not reimplemented; do not add unconditional Apply paths).
- §9.2: never build controllers synchronously on the watcher/dedupqueue loop goroutine — spawn per-space work; per-space serialization via the entry work mutex.
- Error wrapping per CLAUDE.md: `fmt.Errorf("operation: %w", err)`, no bare returns, no "failed to" prefixes.
- Tests: fixture pattern + `want` structs + testify (`assert`/`require`) per CLAUDE.md.
- Commits: `GO-7348 spacev2: <description>` (issue confirmed by the user 2026-07-02; branch renamed to `go-7348-spacecontroller-refactor`). Do not push.

## Locked design decisions (confirmed against v1 + user)

1. **Unidirectional lifecycle** (user-confirmed). Watcher = only builder. `Wait` awaits the registry future (no 500ms polling like v1 `waiter.go`).
2. **Marketplace stays, but as a static registry entry.** `objectcreator/installer.go:28,74` and `templateimpl/impl.go:356` still call `Get(addr.AnytypeMarketplaceWorkspace)`, so dropping it entirely is blocked on GO-6259 consumer migration. v2 ports the v1 `marketplacespace` controller essence (VirtualSpace + reindex-once on WaitLoad + virtualspaceservice registration) as a state-machine-free static controller.
3. **`CreatePersonalSpace` is dead code in v1** (only `NewPersonalSpace` is called, from `startStatus`); v2 does not port it. New accounts create a random-id first shareable space (`spaceCore.Create`), old accounts materialize the deterministic personal space via the SpaceView path.
4. **`AccountStatusRemoving` collapsed**: mapped to Deleted/offload at the single read boundary (M3 mode mapping); never written.
5. **Old-account restore drops v1's `SkipCheckSpaceViewKey` ctx hack**: the tech-space resolution step explicitly `SpaceViewCreate`s the personal space's view when missing, then the watcher builds the controller uniformly.
6. **Reused unchanged:** `spacecore`, `techspace`, `clientspace`, `spaceinfo`, `spacedomain`, `dedupqueue`, `virtualspaceservice`, `internal/components/dependencies` (seams). **Reimplemented in v2:** service orchestration, registry, watcher, bootstrap, factory+controllers+processes (M3+).

## Roadmap context (this plan = Milestone 2)

- M1 skeleton — DONE (scaffold: `service.go`, `controller.go`, `doc.go`).
- **M2 (this plan):** registry, subscription+watcher, Init, tech-space resolution, marketplace entry, Run/Close. Parity: load existing account, enumerate SpaceViews.
- M3: factory + controllers + state machine + load pipeline; `Get`/`Wait` return loaded spaces; `Create`. Separate plan.
- M4: join/offload/delete/remote-status/deletion controller. Separate plan.
- M5: lazy load + ModePaused + memory caps. Separate plan.
- M6: storage migration + platform layer wiring. Separate plan.
- M7: cutover. Separate plan.

---

### Task 1: Controller registry with retryable ready-futures

**Files:**
- Create: `space/spacev2/registry.go`
- Test: `space/spacev2/registry_test.go`

**Interfaces:**
- Consumes: `SpaceController` (controller.go, exists).
- Produces (used by Tasks 3, 7, 8):
  - `newRegistry() *registry`
  - `(*registry) await(ctx context.Context, spaceId string) (SpaceController, error)` — blocks until an attempt completes; placeholder-creates the entry so waiters can precede builders.
  - `(*registry) get(ctx context.Context, spaceId string) (SpaceController, error)` — non-blocking for unknown ids: `ErrSpaceNotExists` if no build was ever started (placeholder or absent); blocks only on an in-flight build.
  - `(*registry) ensure(ctx context.Context, spaceId string, build builderFunc) (SpaceController, error)` — watcher-only single build path; retries failed entries with a fresh future.
  - `(*registry) addStatic(spaceId string, ctrl SpaceController)` — pre-loaded entries (marketplace).
  - `(*registry) all() []SpaceController`, `(*registry) allIds() []string`
  - `(*registry) workLock(spaceId string) *sync.Mutex` — per-space serialization for watcher apply (§9.2).
  - `(*registry) closeAll(ctx context.Context) error` — marks closing (later ensure/await return `ErrSpaceIsClosing`), closes all controllers concurrently.
  - `type builderFunc func(ctx context.Context) (SpaceController, error)`

- [ ] **Step 1: Write the failing tests**

```go
// space/spacev2/registry_test.go
package spacev2

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

type fakeController struct {
	spaceId string
	closed  bool
	mu      sync.Mutex
}

func (f *fakeController) SpaceId() string                  { return f.spaceId }
func (f *fakeController) Start(ctx context.Context) error  { return nil }
func (f *fakeController) Mode() Mode                       { return ModeLoading }
func (f *fakeController) WaitLoad(ctx context.Context) (clientspace.Space, error) {
	return nil, nil
}
func (f *fakeController) Update() error { return nil }
func (f *fakeController) SetPersistentInfo(ctx context.Context, info spaceinfo.SpacePersistentInfo) error {
	return nil
}
func (f *fakeController) SetLocalInfo(ctx context.Context, info spaceinfo.SpaceLocalInfo) error {
	return nil
}
func (f *fakeController) GetStatus() spaceinfo.AccountStatus  { return spaceinfo.AccountStatusActive }
func (f *fakeController) GetLocalStatus() spaceinfo.LocalStatus { return spaceinfo.LocalStatusOk }
func (f *fakeController) Close(ctx context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

func TestRegistry_EnsureThenAwait(t *testing.T) {
	// given
	r := newRegistry()
	want := &fakeController{spaceId: "space1"}

	// when
	got, err := r.ensure(context.Background(), "space1", func(ctx context.Context) (SpaceController, error) {
		return want, nil
	})

	// then
	require.NoError(t, err)
	assert.Same(t, want, got)
	awaited, err := r.await(context.Background(), "space1")
	require.NoError(t, err)
	assert.Same(t, want, awaited)
}

func TestRegistry_AwaitBeforeEnsure(t *testing.T) {
	// given: a waiter arrives before any build (unidirectional Wait semantics)
	r := newRegistry()
	want := &fakeController{spaceId: "space1"}
	done := make(chan struct{})
	var got SpaceController
	var gotErr error
	go func() {
		got, gotErr = r.await(context.Background(), "space1")
		close(done)
	}()

	// when: the watcher builds later
	time.Sleep(10 * time.Millisecond)
	_, err := r.ensure(context.Background(), "space1", func(ctx context.Context) (SpaceController, error) {
		return want, nil
	})

	// then
	require.NoError(t, err)
	<-done
	require.NoError(t, gotErr)
	assert.Same(t, want, got)
}

func TestRegistry_FailedBuildIsRetryable(t *testing.T) {
	// given: first build fails (v1 would poison the waiting map for the session — §9.8)
	r := newRegistry()
	buildErr := errors.New("boom")
	_, err := r.ensure(context.Background(), "space1", func(ctx context.Context) (SpaceController, error) {
		return nil, buildErr
	})
	require.ErrorIs(t, err, buildErr)

	// await between failure and retry returns the failure
	_, err = r.await(context.Background(), "space1")
	require.ErrorIs(t, err, buildErr)

	// when: the next watcher event retries
	want := &fakeController{spaceId: "space1"}
	got, err := r.ensure(context.Background(), "space1", func(ctx context.Context) (SpaceController, error) {
		return want, nil
	})

	// then: the retry wins and later awaits see the controller
	require.NoError(t, err)
	assert.Same(t, want, got)
	awaited, err := r.await(context.Background(), "space1")
	require.NoError(t, err)
	assert.Same(t, want, awaited)
}

func TestRegistry_GetSemantics(t *testing.T) {
	r := newRegistry()

	// unknown id: not-exists, does not block
	_, err := r.get(context.Background(), "nope")
	require.ErrorIs(t, err, ErrSpaceNotExists)

	// placeholder created by a waiter still reads as not-exists for get()
	go func() { _, _ = r.await(context.Background(), "waited") }()
	time.Sleep(10 * time.Millisecond)
	_, err = r.get(context.Background(), "waited")
	require.ErrorIs(t, err, ErrSpaceNotExists)

	// ready entry returns the controller
	want := &fakeController{spaceId: "ready"}
	_, err = r.ensure(context.Background(), "ready", func(ctx context.Context) (SpaceController, error) {
		return want, nil
	})
	require.NoError(t, err)
	got, err := r.get(context.Background(), "ready")
	require.NoError(t, err)
	assert.Same(t, want, got)
}

func TestRegistry_StaticEntry(t *testing.T) {
	r := newRegistry()
	want := &fakeController{spaceId: "marketplace"}
	r.addStatic("marketplace", want)

	got, err := r.get(context.Background(), "marketplace")
	require.NoError(t, err)
	assert.Same(t, want, got)
	assert.Equal(t, []string{"marketplace"}, r.allIds())
}

func TestRegistry_CloseAll(t *testing.T) {
	r := newRegistry()
	c1 := &fakeController{spaceId: "s1"}
	_, err := r.ensure(context.Background(), "s1", func(ctx context.Context) (SpaceController, error) {
		return c1, nil
	})
	require.NoError(t, err)

	require.NoError(t, r.closeAll(context.Background()))
	assert.True(t, c1.closed)

	// post-close: ensure and await refuse
	_, err = r.ensure(context.Background(), "s2", func(ctx context.Context) (SpaceController, error) {
		return &fakeController{spaceId: "s2"}, nil
	})
	require.ErrorIs(t, err, ErrSpaceIsClosing)
	_, err = r.await(context.Background(), "s2")
	require.ErrorIs(t, err, ErrSpaceIsClosing)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./space/spacev2/ -run TestRegistry -v`
Expected: FAIL — `undefined: newRegistry`

- [ ] **Step 3: Implement the registry**

```go
// space/spacev2/registry.go
package spacev2

import (
	"context"
	"sync"
)

// builderFunc builds and starts a controller. Invoked only from the watcher
// apply path (unidirectional single-builder invariant).
type builderFunc func(ctx context.Context) (SpaceController, error)

type entryState int

const (
	// statePlaceholder: created by a waiter (await) before any build was
	// attempted. get() treats it as not-exists (v1 Get parity); await blocks.
	statePlaceholder entryState = iota
	stateBuilding
	stateReady
	stateFailed
)

type entry struct {
	state entryState
	ctrl  SpaceController
	err   error
	// ready is closed when the CURRENT attempt completes; a retry after a
	// failure swaps in a fresh channel, so waiters loop and re-read state.
	ready chan struct{}
	// work serializes the watcher apply (build + Update) per space (§9.2):
	// events for different spaces run concurrently, per-space strictly ordered.
	work sync.Mutex
}

type registry struct {
	mu      sync.Mutex
	entries map[string]*entry
	closing bool
}

func newRegistry() *registry {
	return &registry{entries: map[string]*entry{}}
}

func (r *registry) entryFor(spaceId string) *entry {
	if e, ok := r.entries[spaceId]; ok {
		return e
	}
	e := &entry{state: statePlaceholder, ready: make(chan struct{})}
	r.entries[spaceId] = e
	return e
}

func (r *registry) workLock(spaceId string) *sync.Mutex {
	r.mu.Lock()
	defer r.mu.Unlock()
	return &r.entryFor(spaceId).work
}

// await blocks until some attempt for spaceId completes. It resolves with the
// result of the attempt that completes while waiting; if the latest completed
// attempt failed and no retry is in flight, it returns that failure.
func (r *registry) await(ctx context.Context, spaceId string) (SpaceController, error) {
	for {
		r.mu.Lock()
		if r.closing {
			r.mu.Unlock()
			return nil, ErrSpaceIsClosing
		}
		e := r.entryFor(spaceId)
		state, ctrl, err, ready := e.state, e.ctrl, e.err, e.ready
		r.mu.Unlock()
		switch state {
		case stateReady:
			return ctrl, nil
		case stateFailed:
			return nil, err
		}
		select {
		case <-ready:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// get is the non-blocking-for-unknown-ids read (v1 Get parity): a space no
// build was ever attempted for reads as ErrSpaceNotExists even if waiters
// exist; an in-flight build is awaited; ready/failed resolve immediately.
func (r *registry) get(ctx context.Context, spaceId string) (SpaceController, error) {
	r.mu.Lock()
	e, ok := r.entries[spaceId]
	if !ok || e.state == statePlaceholder {
		r.mu.Unlock()
		return nil, ErrSpaceNotExists
	}
	r.mu.Unlock()
	return r.await(ctx, spaceId)
}

// ensure runs one build attempt for spaceId unless a controller is already
// ready. A previously failed entry is retried with a fresh ready channel —
// failures are never sticky for the session (fixes v1 §9.8).
func (r *registry) ensure(ctx context.Context, spaceId string, build builderFunc) (SpaceController, error) {
	r.mu.Lock()
	if r.closing {
		r.mu.Unlock()
		return nil, ErrSpaceIsClosing
	}
	e := r.entryFor(spaceId)
	switch e.state {
	case stateReady:
		ctrl := e.ctrl
		r.mu.Unlock()
		return ctrl, nil
	case stateBuilding:
		// The per-space work lock serializes watcher applies, so a concurrent
		// build for the same space cannot happen; treat defensively as await.
		r.mu.Unlock()
		return r.await(ctx, spaceId)
	case stateFailed:
		e.ready = make(chan struct{})
		e.err = nil
	}
	e.state = stateBuilding
	ready := e.ready
	r.mu.Unlock()

	ctrl, err := build(ctx)

	r.mu.Lock()
	if err != nil {
		e.state = stateFailed
		e.err = err
	} else {
		e.state = stateReady
		e.ctrl = ctrl
	}
	close(ready)
	r.mu.Unlock()
	return ctrl, err
}

// addStatic registers an already-started controller (marketplace).
func (r *registry) addStatic(spaceId string, ctrl SpaceController) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e := r.entryFor(spaceId)
	e.state = stateReady
	e.ctrl = ctrl
	close(e.ready)
}

func (r *registry) all() []SpaceController {
	r.mu.Lock()
	defer r.mu.Unlock()
	ctrls := make([]SpaceController, 0, len(r.entries))
	for _, e := range r.entries {
		if e.state == stateReady {
			ctrls = append(ctrls, e.ctrl)
		}
	}
	return ctrls
}

func (r *registry) allIds() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	ids := make([]string, 0, len(r.entries))
	for id, e := range r.entries {
		if e.state == stateReady {
			ids = append(ids, id)
		}
	}
	return ids
}

// closeAll marks the registry closing (later ensure/await refuse with
// ErrSpaceIsClosing) and closes all ready controllers concurrently.
func (r *registry) closeAll(ctx context.Context) error {
	r.mu.Lock()
	r.closing = true
	ctrls := make([]SpaceController, 0, len(r.entries))
	for _, e := range r.entries {
		if e.state == stateReady {
			ctrls = append(ctrls, e.ctrl)
		}
		// Wake blocked waiters so they observe closing.
		if e.state == statePlaceholder || e.state == stateBuilding {
			select {
			case <-e.ready:
			default:
				close(e.ready)
			}
			e.state = stateFailed
			e.err = ErrSpaceIsClosing
		}
	}
	r.mu.Unlock()

	var wg sync.WaitGroup
	for _, ctrl := range ctrls {
		wg.Add(1)
		go func(c SpaceController) {
			defer wg.Done()
			if err := c.Close(ctx); err != nil {
				log.Error("close space controller", zapSpaceId(c.SpaceId()), zapError(err))
			}
		}(ctrl)
	}
	wg.Wait()
	return nil
}
```

Note: `log`, `zapSpaceId`, `zapError` don't exist yet — add a tiny `log.go`:

```go
// space/spacev2/log.go
package spacev2

import (
	"github.com/anyproto/any-sync/app/logger"
	"go.uber.org/zap"
)

var log = logger.NewNamed(CName)

func zapSpaceId(id string) zap.Field { return zap.String("spaceId", id) }
func zapError(err error) zap.Field   { return zap.Error(err) }
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./space/spacev2/ -run TestRegistry -v && go build ./...`
Expected: PASS, build green.

- [ ] **Step 5: Commit**

```bash
git add space/spacev2/registry.go space/spacev2/registry_test.go space/spacev2/log.go
git commit -m "GO-0000 spacev2: controller registry with retryable ready-futures"
```

---

### Task 2: SpaceView status subscription (port of v1 spacesub)

**Files:**
- Create: `space/spacev2/spacesub.go`
- Test: `space/spacev2/spacesub_test.go`

**Interfaces:**
- Consumes: `core/subscription`, `core/subscription/objectsubscription`, `spaceinfo` (same as v1 `space/spacesub.go`).
- Produces (used by Task 3):
  - `type spaceViewStatus struct { spaceId, spaceViewId, creator, aclHeadId string; localStatus spaceinfo.LocalStatus; accountStatus spaceinfo.AccountStatus; remoteStatus spaceinfo.RemoteStatus; guestKey string }` — NOTE: no embedded `*sync.Mutex` (v1 carried one; v2 serializes via the registry entry work lock instead).
  - `newSpaceSubscription(service subscription.Service, techSpaceId string, afterRun func(*spaceViewObjectSubscription), update func(spaceViewStatus)) *spaceSubscription` with `Run()/Close()`
  - `statusToInfo(status spaceViewStatus) spaceinfo.SpacePersistentInfo`
  - `const subId = CName` (v2 uses `client.space.v2` so both services could subscribe side by side in a test app)

- [ ] **Step 1: Write the failing test**

```go
// space/spacev2/spacesub_test.go
package spacev2

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

func TestStatusToInfo(t *testing.T) {
	// given
	status := spaceViewStatus{
		spaceId:       "space1",
		aclHeadId:     "aclHead1",
		guestKey:      "guestKey1",
		accountStatus: spaceinfo.AccountStatusJoining,
	}
	want := spaceinfo.NewSpacePersistentInfo("space1")
	want.SetAccountStatus(spaceinfo.AccountStatusJoining).
		SetAclHeadId("aclHead1").
		SetEncodedKey("guestKey1")

	// when
	got := statusToInfo(status)

	// then
	assert.Equal(t, want, got)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./space/spacev2/ -run TestStatusToInfo -v`
Expected: FAIL — `undefined: spaceViewStatus`

- [ ] **Step 3: Port the subscription**

Copy `space/spacesub.go` into `space/spacev2/spacesub.go` with exactly these deltas (same relation keys, same filter — this is the §5.3/§11.7 contract read path):
- package `spacev2`
- drop the `mx *sync.Mutex` field from `spaceViewStatus`, its initialization in `SetDetails`, and keep everything else identical (`String()` included, minus mx).
- `SubId: CName` (which is `client.space.v2` here).
- Add `statusToInfo` (moved from v1 `spacewatcher.go:49`):

```go
func statusToInfo(status spaceViewStatus) spaceinfo.SpacePersistentInfo {
	persistentInfo := spaceinfo.NewSpacePersistentInfo(status.spaceId)
	persistentInfo.SetAccountStatus(status.accountStatus).
		SetAclHeadId(status.aclHeadId).
		SetEncodedKey(status.guestKey)
	return persistentInfo
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./space/spacev2/ -run TestStatusToInfo -v && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add space/spacev2/spacesub.go space/spacev2/spacesub_test.go
git commit -m "GO-0000 spacev2: port spaceview status subscription"
```

---

### Task 3: Watcher — the unidirectional spine

**Files:**
- Create: `space/spacev2/watcher.go`
- Test: `space/spacev2/watcher_test.go`

**Interfaces:**
- Consumes: Task 1 registry (via the `statusApplier` seam), Task 2 subscription, `space/dedupqueue` (reused v1 package).
- Produces (used by Task 8):
  - `type statusApplier interface { onSpaceStatusUpdated(status spaceViewStatus) }` — implemented by the service.
  - `newSpaceWatcher(techSpaceId string, service subscription.Service, applier statusApplier) *spaceWatcher` with `Run() error` / `Close() error` — structurally identical to v1 `space/spacewatcher.go` (dedupqueue coalescing per spaceId, replay via afterRun Iterate).

The remote-deleted branch and the apply logic live on the SERVICE (Task 8), not the watcher — the watcher stays a thin coalescing pipe, mirroring v1.

- [ ] **Step 1: Write the failing test**

```go
// space/spacev2/watcher_test.go
package spacev2

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/space/dedupqueue"
)

type recordingApplier struct {
	mu       sync.Mutex
	statuses []spaceViewStatus
}

func (r *recordingApplier) onSpaceStatusUpdated(status spaceViewStatus) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.statuses = append(r.statuses, status)
}

func (r *recordingApplier) spaceIds() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	ids := make([]string, 0, len(r.statuses))
	for _, s := range r.statuses {
		ids = append(ids, s.spaceId)
	}
	return ids
}

// The watcher pipes coalesced statuses to the applier. We drive the dedup
// queue directly (the subscription integration is covered in Task 8's
// service-level test with the real subscription fixture).
func TestWatcher_CoalescesPerSpace(t *testing.T) {
	// given
	applier := &recordingApplier{}
	queue := dedupqueue.New(0)
	w := &spaceWatcher{queue: queue, applier: applier}
	queue.Run()
	defer queue.Close()

	// when: many updates for one space and one for another
	for i := 0; i < 5; i++ {
		w.enqueue(spaceViewStatus{spaceId: "spaceA", spaceViewId: "viewA"})
	}
	w.enqueue(spaceViewStatus{spaceId: "spaceB", spaceViewId: "viewB"})

	// then: spaceB is applied exactly once; spaceA at least once and at most 5x
	require.Eventually(t, func() bool {
		ids := applier.spaceIds()
		return contains(ids, "spaceA") && contains(ids, "spaceB")
	}, time.Second, 5*time.Millisecond)
	assert.LessOrEqual(t, count(applier.spaceIds(), "spaceA"), 5)
	assert.Equal(t, 1, count(applier.spaceIds(), "spaceB"))
}

func contains(ids []string, id string) bool { return count(ids, id) > 0 }

func count(ids []string, id string) int {
	n := 0
	for _, i := range ids {
		if i == id {
			n++
		}
	}
	return n
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./space/spacev2/ -run TestWatcher -v`
Expected: FAIL — `undefined: spaceWatcher` / `w.enqueue`

- [ ] **Step 3: Implement the watcher**

```go
// space/spacev2/watcher.go
package spacev2

import (
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/space/dedupqueue"
)

// statusApplier receives coalesced SpaceView status updates. Implemented by
// the service; the watcher itself carries no lifecycle logic (unidirectional
// spine stays a thin pipe, v1 parity).
type statusApplier interface {
	onSpaceStatusUpdated(status spaceViewStatus)
}

type spaceWatcher struct {
	sub     *spaceSubscription
	queue   *dedupqueue.DedupQueue
	applier statusApplier
}

func newSpaceWatcher(techSpaceId string, service subscription.Service, applier statusApplier) *spaceWatcher {
	w := &spaceWatcher{
		queue:   dedupqueue.New(0),
		applier: applier,
	}
	w.sub = newSpaceSubscription(
		service,
		techSpaceId,
		func(sub *spaceViewObjectSubscription) {
			sub.Iterate(func(id string, status spaceViewStatus) bool {
				w.enqueue(status)
				return true
			})
		},
		w.enqueue,
	)
	return w
}

// enqueue coalesces bursts per space id: only the latest pending status for a
// space is applied (dedupqueue Replace semantics, v1 parity).
func (w *spaceWatcher) enqueue(status spaceViewStatus) {
	w.queue.Replace(status.spaceId, func() {
		w.applier.onSpaceStatusUpdated(status)
	})
}

func (w *spaceWatcher) Run() error {
	w.queue.Run()
	return w.sub.Run()
}

func (w *spaceWatcher) Close() error {
	w.sub.Close()
	return w.queue.Close()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./space/spacev2/ -run TestWatcher -v && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add space/spacev2/watcher.go space/spacev2/watcher_test.go
git commit -m "GO-0000 spacev2: spaceview watcher (dedup-coalescing pipe)"
```

---

### Task 4: Service state + Init (id derivation, account metadata, deps)

**Files:**
- Modify: `space/spacev2/service.go` (fill the `service` struct, `Init`, accessors; leave verbs stubbed)
- Test: `space/spacev2/service_test.go` (fixture + Init tests + metadata/id parity oracle)

**Interfaces:**
- Consumes: `spacecore.SpaceCoreService`, `accountservice.Service`, `*config.Config`, `subscription.Service`, `domain.DeriveAccountMetadata`, Tasks 1–3 types.
- Produces (used by Tasks 5–8): populated `service` struct fields:

```go
type service struct {
	registry  *registry
	watcher   *spaceWatcher
	spaceCore spacecore.SpaceCoreService

	accountService      accountservice.Service
	config              *config.Config
	notificationService NotificationSender          // declared here, used in M4 remote-deleted notification
	spaceNameGetter     objectstore.SpaceNameGetter // ditto
	spaceLoaderListener aclobjectmanager.SpaceLoaderListener
	techProvider        techSpaceProvider // Task 5 seam
	marketplace         *marketplaceController // Task 7

	techSpace      *clientspace.TechSpace
	techSpaceReady chan struct{}

	personalSpaceId        string
	techSpaceId            string
	newAccount             bool
	repKey                 uint64
	accountMetadataSymKey  crypto.SymKey
	accountMetadataPayload []byte
	firstCreatedSpaceId    string

	ctx       context.Context
	ctxCancel context.CancelFunc
	isClosing atomic.Bool
}
```

plus `NotificationSender` iface (verbatim from v1 `service.go:123`). `Init` resolves deps via `app.MustComponent`, derives ids via `spaceCore.DeriveID` (Regular + Tech), derives metadata via `domain.DeriveAccountMetadata`, computes `repKey` (port v1 `getRepKey` — it lives in `space/metadata.go` or near; grep `func getRepKey` and port verbatim), creates registry/watcher/ctx. Accessors `TechSpaceId/PersonalSpaceId/FirstCreatedSpaceId/AccountMetadataSymKey/AccountMetadataPayload/TechSpace` return the fields.

- [ ] **Step 1: Write the failing tests** — fixture with `mock_spacecore` + `accounttest.AccountTestService` (see v1 `space/service_test.go` fixture for the exact mock wiring; v2's fixture only needs spaceCore, account service, config, subscription service stub, and fakes for the seams declared above; use `testutil.PrepareMock`):

```go
func TestService_Init(t *testing.T) {
	t.Run("derives ids and metadata byte-identical to v1 derivations", func(t *testing.T) {
		// given
		fx := newFixture(t)

		// then: ids come from the same DeriveID calls the fixture stubbed
		assert.Equal(t, testPersonalSpaceId, fx.PersonalSpaceId())
		assert.Equal(t, testTechSpaceId, fx.TechSpaceId())

		// parity oracle: metadata payload/symkey equal the direct v1 derivation
		wantMeta, wantKey, err := domain.DeriveAccountMetadata(fx.accountKeys.SignKey)
		require.NoError(t, err)
		wantPayload, err := wantMeta.Marshal()
		require.NoError(t, err)
		assert.Equal(t, wantPayload, fx.AccountMetadataPayload())
		assert.Equal(t, wantKey, fx.AccountMetadataSymKey())
	})
}
```

with fixture:

```go
const (
	testPersonalSpaceId = "personal.12345"
	testTechSpaceId     = "tech.12345"
)

type fixture struct {
	*service
	spaceCore   *mock_spacecore.MockSpaceCoreService
	accountKeys *accountdata.AccountKeys
}

func newFixture(t *testing.T) *fixture {
	serv := New().(*service)
	spaceCore := mock_spacecore.NewMockSpaceCoreService(t)
	spaceCore.EXPECT().DeriveID(mock.Anything, spacedomain.SpaceTypeRegular).Return(testPersonalSpaceId, nil)
	spaceCore.EXPECT().DeriveID(mock.Anything, spacedomain.SpaceTypeTech).Return(testTechSpaceId, nil)

	accountServ := &accounttest.AccountTestService{}
	// ... app assembly via testutil.PrepareMock + app.Register, mirroring the
	// v1 fixture but with spacev2 seams faked (techProvider, marketplace deps
	// arrive in Tasks 5/7 — register no-op fakes registered as components).
	// require.NoError(t, serv.Init(a))
	return &fixture{service: serv, spaceCore: spaceCore, accountKeys: accountServ.Account()}
}
```

(Adapt exact app wiring from v1 `newFixture` at execution time; the assertions above are the contract.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./space/spacev2/ -run TestService_Init -v`
Expected: FAIL — Init returns errNotImplemented / nil fields.

- [ ] **Step 3: Implement Init + accessors** (replace stubs in `service.go`; keep verbs like Create/Join stubbed with `errNotImplemented`). Port `getRepKey` verbatim from v1. Wire: registry = newRegistry(), watcher = newSpaceWatcher(techSpaceId, subService, s), techSpaceReady = make(chan struct{}), ctx/ctxCancel.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./space/spacev2/ -run TestService_Init -v && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add space/spacev2/service.go space/spacev2/service_test.go
git commit -m "GO-0000 spacev2: service Init — id derivation, account metadata, deps"
```

---

### Task 5: Tech-space provider (create/load, port of factory tech methods)

**Files:**
- Create: `space/spacev2/techprovider.go`
- Test: exercised via Task 6's resolution tests (the provider itself is a thin port; unit-test the seam wiring only).

**Interfaces:**
- Produces (used by Task 6):

```go
// techSpaceProvider isolates tech-space construction (v1: spacefactory
// CreateAndSetTechSpace / LoadAndSetTechSpace) behind a seam so bootstrap
// fallbacks are testable (§11.7).
type techSpaceProvider interface {
	Create(ctx context.Context) (*clientspace.TechSpace, error)
	Load(ctx context.Context) (*clientspace.TechSpace, error)
}
```

- [ ] **Step 1: Port the two methods** from `space/spacefactory/spacefactory.go:140-210` into a `techProvider` struct holding `{app *app.App, spaceCore, accountService, objectFactory, indexer, installer, personalSpaceId}` — logic verbatim (Derive vs Get, `clientspace.TechSpaceDeps`, ChildApp registration, `ts.Run(techCoreSpace, ts.Cache, isCreate)`, `indexer.ReindexSpace(ts)` on Load). Resolve deps in the service's `Init` and construct the provider there (`s.techProvider = newTechProvider(a, ...)`).

- [ ] **Step 2: Build**

Run: `go build ./...`
Expected: green.

- [ ] **Step 3: Commit**

```bash
git add space/spacev2/techprovider.go space/spacev2/service.go
git commit -m "GO-0000 spacev2: tech-space provider (create/load port)"
```

---

### Task 6: Tech-space resolution (bootstrap fallbacks, explicit + tested)

**Files:**
- Create: `space/spacev2/bootstrap.go`
- Test: add cases to `space/spacev2/service_test.go`

**Interfaces:**
- Consumes: Task 5 `techSpaceProvider`, `spaceCore.StorageExistsLocally/Get`, `techspace.SpaceViewCreate` (via the loaded TechSpace), `spacesyncproto.ErrSpaceMissing`.
- Produces (used by Task 8): `(s *service) resolveTechSpace(ctx context.Context) error` — sets `s.techSpace`, closes `s.techSpaceReady` exactly once, on success only (per §9: on total init failure leave it unclosed so `Get(techSpaceId)` blocks rather than returning a nil space).

Decision table (verbatim from v1 `initAccount`, `service.go:273-313` — each row becomes a test):

| # | Load(15s) result | personal exists locally | then | outcome |
|---|---|---|---|---|
| 1 | ok | — | — | loaded |
| 2 | DeadlineExceeded | no | retry Load(ctx) ok | loaded |
| 3 | DeadlineExceeded | yes | spaceCore.Get(personal) ok → Create → ensure personal SpaceView | created (old account) |
| 4 | ErrSpaceMissing | — | spaceCore.Get(personal) ok → Create → ensure personal SpaceView | created (old account) |
| 5 | other error | — | — | error out |

"ensure personal SpaceView": `techSpace.SpaceViewExists(ctx, personalSpaceId)`; if false → `SpaceViewCreate(ctx, personalSpaceId, true, info(AccountStatusUnknown), nil)`. This replaces v1's `SkipCheckSpaceViewKey` direct-build hack — the watcher then builds the personal controller through the one path.

- [ ] **Step 1: Write the failing tests** — port the five `TestService_Init` bootstrap cases from v1 `space/service_test.go:98-137` to the v2 shape: fake `techSpaceProvider` (hand-written, records calls, returns scripted results), `mock_spacecore` for `StorageExistsLocally`/`Get`, `mock_techspace.MockTechSpace` for `SpaceViewExists`/`SpaceViewCreate` expectations. Assert per row: which provider methods were called, whether SpaceViewCreate happened, and that `techSpaceReady` is closed on success (`s.getTechSpace(ctx)` returns promptly) and NOT closed on row-5 failure (times out).

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./space/spacev2/ -run TestService_ResolveTechSpace -v`
Expected: FAIL — `undefined: resolveTechSpace`

- [ ] **Step 3: Implement `bootstrap.go`**

```go
// space/spacev2/bootstrap.go
package spacev2

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/anyproto/any-sync/commonspace/spacesyncproto"

	"github.com/anyproto/anytype-heart/space/spaceinfo"
	"github.com/anyproto/anytype-heart/space/techspace"
)

var loadTechSpaceDeadline = 15 * time.Second

// resolveTechSpace makes s.techSpace usable, isolating the old-account /
// offline-restore fallbacks behind one explicit step (SpaceController.md §11.7,
// decision table in the M2 plan). Closes techSpaceReady on success only.
func (s *service) resolveTechSpace(ctx context.Context) (err error) {
	if s.newAccount {
		return s.createTechSpace(ctx)
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, loadTechSpaceDeadline)
	err = s.loadTechSpace(timeoutCtx)
	cancel()
	if errors.Is(err, context.DeadlineExceeded) {
		var personalExists bool
		personalExists, err = s.spaceCore.StorageExistsLocally(ctx, s.personalSpaceId)
		if err != nil {
			return fmt.Errorf("check personal space storage: %w", err)
		}
		if !personalExists {
			err = s.loadTechSpace(ctx)
		} else {
			return s.createTechSpaceForOldAccount(ctx)
		}
	}
	if errors.Is(err, spacesyncproto.ErrSpaceMissing) {
		return s.createTechSpaceForOldAccount(ctx)
	}
	if err != nil {
		return fmt.Errorf("resolve tech space: %w", err)
	}
	return nil
}

func (s *service) createTechSpace(ctx context.Context) (err error) {
	if s.techSpace, err = s.techProvider.Create(ctx); err != nil {
		return fmt.Errorf("create tech space: %w", err)
	}
	close(s.techSpaceReady)
	return nil
}

func (s *service) loadTechSpace(ctx context.Context) (err error) {
	ts, err := s.techProvider.Load(ctx)
	if err != nil {
		return err // sentinel errors inspected by resolveTechSpace; wrapped there
	}
	s.techSpace = ts
	close(s.techSpaceReady)
	return nil
}

// createTechSpaceForOldAccount handles the offline-restore path: the personal
// space exists locally (or the nodes report no tech space), so the account
// predates tech spaces. Create the tech space and ensure the personal space's
// SpaceView exists; the watcher then builds the personal controller through
// the regular unidirectional path (replaces v1's SkipCheckSpaceViewKey hack).
func (s *service) createTechSpaceForOldAccount(ctx context.Context) error {
	if _, err := s.spaceCore.Get(ctx, s.personalSpaceId); err != nil {
		return fmt.Errorf("get personal space: %w", err)
	}
	if err := s.createTechSpace(ctx); err != nil {
		return err
	}
	exists, err := s.techSpace.SpaceViewExists(ctx, s.personalSpaceId)
	if err != nil {
		return fmt.Errorf("check personal space view: %w", err)
	}
	if !exists {
		info := spaceinfo.NewSpacePersistentInfo(s.personalSpaceId)
		info.SetAccountStatus(spaceinfo.AccountStatusUnknown)
		if err = s.techSpace.SpaceViewCreate(ctx, s.personalSpaceId, true, info, nil); err != nil {
			return fmt.Errorf("create personal space view: %w", err)
		}
	}
	return nil
}

func (s *service) getTechSpace(ctx context.Context) (*clientspace.TechSpace, error) {
	select {
	case <-s.techSpaceReady:
		return s.techSpace, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
```

(Import `clientspace` as needed.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./space/spacev2/ -v && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add space/spacev2/bootstrap.go space/spacev2/service_test.go
git commit -m "GO-0000 spacev2: explicit tech-space resolution with old-account fallbacks"
```

---

### Task 7: Marketplace static entry + Get/Wait plumbing

**Files:**
- Create: `space/spacev2/marketplace.go`
- Modify: `space/spacev2/service.go` (Get, Wait, GetPersonalSpace, GetTechSpace, SpaceViewId)
- Test: add cases to `space/spacev2/service_test.go`

**Interfaces:**
- Consumes: `clientspace.NewVirtualSpace`, `virtualspaceservice.VirtualSpaceService`, `dependencies.SpaceIndexer`, registry `addStatic`/`get`/`await`.
- Produces: `marketplaceController` implementing v2 `SpaceController` — port of v1 `space/internal/marketplacespace/marketplace.go` reshaped: `WaitLoad` does the reindex-once + returns the VirtualSpace; `Mode()` returns `ModeLoading`; all Set*/Update are no-ops; statuses `AccountStatusUnknown`/`LocalStatusOk`. Keep the `virtualspaceservice.RegisterVirtualSpace` call and the `TODO GO-6259 deprecated` comment.

Get/Wait shape (M2 — real loaded spaces arrive in M3, but the routing is final):

```go
func (s *service) Get(ctx context.Context, spaceId string) (clientspace.Space, error) {
	if spaceId == s.techSpaceId {
		return s.getTechSpace(ctx)
	}
	ctrl, err := s.registry.get(ctx, spaceId)
	if err != nil {
		return nil, err
	}
	sp, err := ctrl.WaitLoad(ctx)
	if err != nil {
		return nil, convertSpaceError(err)
	}
	return sp, nil
}

func (s *service) Wait(ctx context.Context, spaceId string) (clientspace.Space, error) {
	techSpace, err := s.getTechSpace(ctx)
	if err != nil {
		return nil, fmt.Errorf("get tech space: %w", err)
	}
	if spaceId == s.techSpaceId {
		return techSpace, nil
	}
	if spaceId != addr.AnytypeMarketplaceWorkspace {
		exists, err := techSpace.SpaceViewExists(ctx, spaceId)
		if err != nil {
			return nil, fmt.Errorf("check space view: %w", err)
		}
		if !exists {
			return nil, ErrSpaceNotExists
		}
	}
	ctrl, err := s.registry.await(ctx, spaceId) // no polling: future resolves when the watcher builds
	if err != nil {
		return nil, err
	}
	sp, err := ctrl.WaitLoad(ctx)
	if err != nil {
		return nil, convertSpaceError(err)
	}
	return sp, nil
}
```

plus `convertSpaceError` ported verbatim from v1 `load.go:113`, and `SpaceViewId` delegating to `s.techSpace.SpaceViewId` (guard: return error if techSpace not ready yet — check `techSpaceReady` non-blockingly).

- [ ] **Step 1: Write the failing tests** — (a) `Get(marketplaceId)` after `initMarketplaceSpace` returns the virtual space and triggers `ReindexMarketplaceSpace` exactly once across two calls (mock `dependencies.SpaceIndexer` via `mock_dependencies`); (b) `Get(techSpaceId)` blocks until `techSpaceReady` (port v1 "tech space getter" test from `service_test.go:74-97`); (c) `Get(unknown)` → `ErrSpaceNotExists`; (d) `Wait(unknown)` with `SpaceViewExists=false` → `ErrSpaceNotExists`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./space/spacev2/ -run 'TestService_Get|TestService_Wait' -v`
Expected: FAIL.

- [ ] **Step 3: Implement** marketplace.go + the Get/Wait bodies above + `initMarketplaceSpace` (builds the controller, calls its `Start`, `registry.addStatic(addr.AnytypeMarketplaceWorkspace, ctrl)`).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./space/spacev2/ -v && go build ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add space/spacev2/marketplace.go space/spacev2/service.go space/spacev2/service_test.go
git commit -m "GO-0000 spacev2: marketplace static entry, Get/Wait routing via registry"
```

---

### Task 8: Run/Close + the apply path (enumeration parity gate)

**Files:**
- Modify: `space/spacev2/service.go` (Run, Close, onSpaceStatusUpdated, buildController stub)
- Modify: `space/spacev2/bootstrap.go` (createAccount/initAccount split)
- Test: add cases to `space/spacev2/service_test.go`

**Interfaces:**
- Consumes: everything above.
- Produces:
  - `(s *service) Run(ctx)` — new account: initMarketplace → resolveTechSpace (create) → `s.createFirstSpace(ctx)` hook (M2: sets `firstCreatedSpaceId` only if hook non-nil; real implementation in M3) → watcher.Run → StartSync → PersistAccountNetworkId. Existing: initMarketplace → `spaceLoaderListener.OnSpaceLoad(marketplace)` → resolveTechSpace → watcher.Run → StartSync → PersistAccountNetworkId. (Lazy mode + drain: M5. AutoJoinStream: M4.)
  - `(s *service) onSpaceStatusUpdated(status spaceViewStatus)` — the unidirectional apply: spawn goroutine; take `registry.workLock(spaceId)`; re-check `isClosing`; remote-deleted branch (M4 — for M2 leave a TODO comment referencing M4 plan, apply only the build path); `registry.ensure(spaceId, s.builderFor(statusToInfo(status)))`; on success `ctrl.Update()`.
  - `(s *service) builderFor(info spaceinfo.SpacePersistentInfo) builderFunc` — M2 stub: returns `errBuilderNotImplemented = errors.New("spacev2: controller builder not implemented (M3)")`. M3 replaces the body with the personal/shareable/streamable dispatch (v1 `load.go:80-86`).
  - `(s *service) Close(ctx)` — `ctxCancel()` BEFORE `isClosing.Store(true)` (§9 ordering), `registry.closeAll`, `techSpace.Close` (only if resolved), `watcher.Close()`.

- [ ] **Step 1: Write the failing tests**
  - **Enumeration parity gate (the M2 done-criterion):** fixture with the real `core/subscription` fixture over an `objectstore.StoreFixture` seeded with 3 spaceView objects in `objectstore.TestTechSpaceId` (see CLAUDE.md test guidelines §4 for the seeding shape; add `TargetSpaceId`, `SpaceAccountStatus`, `SpaceLocalStatus`, `SpaceRemoteStatus`, `GuestKey`, `LatestAclHeadId` keys). Run the v2 watcher; assert `require.Eventually` that all 3 space ids reached `onSpaceStatusUpdated` and each has a registry entry in `stateFailed` with `errBuilderNotImplemented` (proving: subscription → dedup → apply → single build path all wired). Check how v1's `space_lazy_test.go` assembles the subscription fixture and mirror it.
  - **Close ordering:** after Close, a late `onSpaceStatusUpdated` is a no-op (no new entries); `await` returns `ErrSpaceIsClosing`.
  - **Run existing-account:** provider.Load scripted ok → watcher ran (subscription active), `PersistAccountNetworkId` called (config fake), StartSync called (mock techspace).

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./space/spacev2/ -run 'TestService_Run|TestService_Enumerate|TestService_Close' -v
`
Expected: FAIL.

- [ ] **Step 3: Implement** Run/Close/onSpaceStatusUpdated/builderFor per the shapes above.

- [ ] **Step 4: Run the full package + build + v1 tests untouched**

Run: `go test ./space/... && go build ./...`
Expected: all PASS (v1 suite proves the oracle is untouched).

- [ ] **Step 5: Commit**

```bash
git add space/spacev2/ 
git commit -m "GO-0000 spacev2: Run/Close + unidirectional apply path; M2 enumeration gate"
```

---

### Task 9: Milestone bookkeeping

- [ ] Update `space/spacev2/HANDOFF.md` milestone list: mark M2 done with a one-line summary of what exists.
- [ ] Ask the user for the real GO- issue number; then reword all `GO-0000` commits on this branch (`git rebase -i` is unavailable — use `git filter-branch --msg-filter` or `git rebase --exec` non-interactive alternative: `git filter-branch -f --msg-filter 'sed "s/^GO-0000/GO-XXXX/"' <base>..HEAD`).
- [ ] Run: `go build ./... && go test ./space/...` one final time; paste output into the milestone summary.

## Self-review notes

- Spec coverage: M2 scope from HANDOFF ("load/derive spaces, tech-space + SpaceView CRUD, the reactive watcher; parity: create/load an existing account, enumerate SpaceViews") maps to Tasks 4 (derive), 5–6 (tech-space CRUD + resolution), 2–3+8 (watcher + enumeration gate). SpaceView *creation* for real spaces beyond the personal-view fallback is M3 (Create verb) — intentionally out of M2.
- Types cross-checked: `builderFunc` (T1) consumed in T8; `statusApplier` (T3) implemented by service (T8); `techSpaceProvider` (T5) consumed by T6; `spaceViewStatus` (T2) consumed by T3/T8.
- Known deliberate deviations from v1, all user-confirmed or handoff-sanctioned: unidirectional (no dual-path dedup), retryable build failures (§9.8 fix), no 500ms Wait polling, no `SkipCheckSpaceViewKey`, marketplace as static entry, `CreatePersonalSpace` dropped.
- Plan-code caveat: exact mock/fixture assembly (Task 4 Step 1, Task 8 Step 1) is adapted from the v1 fixtures at execution time; the asserted contracts are fixed here.
