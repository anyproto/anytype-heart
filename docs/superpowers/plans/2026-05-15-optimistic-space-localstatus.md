# Optimistic localStatus for spaces already on disk — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stop the `Unknown -> Loading -> Ok` localStatus churn for spaces that already exist on disk and were previously `Ok`, so mobile clients show the spaces list instantly on cold start.

**Architecture:** `SpaceView.Init` stops force-resetting `localStatus` to `Unknown` and instead preserves the value persisted from the previous session. `spaceLoader.startLoad` skips publishing `Loading` when the persisted status is already `Ok` and the on-disk store still exists, while still running the background build. `spaceLoader.WaitLoad` is reworked to gate open-on-demand on the loader's *internal* lifecycle (`loading`/`space`/`loadErr`) instead of the client-facing persisted status, so an optimistic `Ok` cannot return a not-yet-built space.

**Tech Stack:** Go, testify, uber gomock + mockery mocks (`mock_spacestatus`, `mock_storage`), `smarttest` editor harness.

**Spec:** `docs/superpowers/specs/2026-05-15-optimistic-space-localstatus-design.md`

**Commit convention (repo CLAUDE.md):** every commit message MUST start with `GO-7289 `.

---

## File Structure

- `core/block/editor/spaceview.go` — `Init` preserves persisted local/remote status (modify `Init`, ~lines 94-98).
- `core/block/editor/spaceview_test.go` — new tests for Init preservation + the state-carry verification.
- `space/internal/components/spaceloader/spaceloader.go` — add `storageService` dependency in `Init`; fast-path branch in `startLoad`; internal-state rework of `WaitLoad`.
- `space/internal/components/spaceloader/spaceloader_test.go` — new test file (none exists today) for `startLoad` decision and `WaitLoad` behavior.

No file is large enough to warrant splitting. All changes follow existing patterns in these files.

---

## Task 1: Verify persisted localStatus reaches `ctx.State` at `SpaceView.Init`, and preserve it

This task carries the spec's "Key implementation risk to verify first": the design assumes the
previously persisted `spaceLocalStatus` local detail is present in `ctx.State` during
`SpaceView.Init`. The test in Step 1 asserts that explicitly. If Step 2 shows the seeded value is
**not** visible at Init at all (test fails on the pre-Init assertion rather than the post-Init
one), STOP and revisit the spec's fallback note before continuing.

**Files:**
- Modify: `core/block/editor/spaceview.go:94-98`
- Test: `core/block/editor/spaceview_test.go`

- [ ] **Step 1: Write the failing test**

Add to `core/block/editor/spaceview_test.go` (the package already imports `spaceinfo`, `bundle`,
`domain`, `model`, `smarttest`, `order`, `migration`, `mock_objecttree`, `treechangeproto`,
`require`, `assert`):

```go
// buildSpaceViewWithSeededLocalInfo constructs a SpaceView whose underlying doc already has
// localStatus/remoteStatus persisted (simulating a reload from the object store after a
// previous session), then runs Init exactly as the production reload path does.
func buildSpaceViewWithSeededLocalInfo(t *testing.T, targetSpaceId string, seeded *spaceinfo.SpaceLocalInfo) *SpaceView {
	ctrl := gomock.NewController(t)
	tree := mock_objecttree.NewMockObjectTree(ctrl)

	sb := smarttest.NewWithTree("root", tree)

	// Seed local details on the doc BEFORE Init, mimicking a spaceview reloaded from disk.
	if seeded != nil {
		seedState := sb.NewState()
		seeded.UpdateDetails(seedState)
		require.NoError(t, sb.Apply(seedState))
	}

	sv := &SpaceView{
		SmartBlock:    sb,
		OrderSettable: order.NewOrderSettable(sb, bundle.RelationKeySpaceOrder),
		spaceService:  &spaceServiceStub{},
		log:           log,
	}

	changePayload := &model.ObjectChangePayload{Key: targetSpaceId}
	marshaled, err := changePayload.Marshal()
	require.NoError(t, err)
	tree.EXPECT().ChangeInfo().Return(&treechangeproto.TreeChangeInfo{ChangePayload: marshaled}).AnyTimes()

	initCtx := &smartblock.InitContext{IsNewObject: false}
	require.NoError(t, sv.Init(initCtx))

	// Risk gate: the seeded value MUST be visible in the Init state. If this fails, the
	// design's Init-side preservation is not viable as written — see spec fallback note.
	if seeded != nil {
		preInfo := spaceinfo.NewSpaceLocalInfoFromState(initCtx.State)
		require.Equal(t, seeded.GetLocalStatus(), preInfo.GetLocalStatus(),
			"persisted localStatus must be present in ctx.State at Init")
	}

	migration.RunMigrations(sv, initCtx)
	require.NoError(t, sv.Apply(initCtx.State))
	t.Cleanup(ctrl.Finish)
	return sv
}

func TestSpaceView_Init_PreservesPersistedLocalStatus(t *testing.T) {
	t.Run("previously Ok is preserved", func(t *testing.T) {
		// given
		seeded := spaceinfo.NewSpaceLocalInfo("spaceId")
		seeded.SetLocalStatus(spaceinfo.LocalStatusOk).
			SetRemoteStatus(spaceinfo.RemoteStatusOk)

		// when
		sv := buildSpaceViewWithSeededLocalInfo(t, "spaceId", &seeded)

		// then
		got := sv.GetLocalInfo()
		assert.Equal(t, spaceinfo.LocalStatusOk, got.GetLocalStatus())
		assert.Equal(t, spaceinfo.RemoteStatusOk, got.GetRemoteStatus())
	})

	t.Run("brand new spaceview defaults to Unknown", func(t *testing.T) {
		// given no seeded local info
		// when
		sv := buildSpaceViewWithSeededLocalInfo(t, "spaceId", nil)

		// then
		got := sv.GetLocalInfo()
		assert.Equal(t, spaceinfo.LocalStatusUnknown, got.GetLocalStatus())
		assert.Equal(t, spaceinfo.RemoteStatusUnknown, got.GetRemoteStatus())
	})
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./core/block/editor/ -run TestSpaceView_Init_PreservesPersistedLocalStatus -v`

Expected: the `previously Ok is preserved` subtest FAILS at the final assertion with
`LocalStatusUnknown != LocalStatusOk` (current `Init` force-resets to `Unknown`). The
`brand new` subtest PASSES. The risk-gate `require.Equal(... at Init)` line must PASS (proving
the seeded value reaches `ctx.State`). If the risk-gate line is what fails, STOP per the note
above.

- [ ] **Step 3: Modify `SpaceView.Init` to preserve persisted local/remote status**

In `core/block/editor/spaceview.go`, replace lines 94-98:

```go
	localInfo := spaceinfo.NewSpaceLocalInfo(spaceId)
	localInfo.SetLocalStatus(spaceinfo.LocalStatusUnknown).
		SetRemoteStatus(spaceinfo.RemoteStatusUnknown).
		UpdateDetails(ctx.State).
		Log(log)
```

with:

```go
	// Preserve the localStatus/remoteStatus persisted from the previous session instead of
	// force-resetting to Unknown. A spaceview reloaded from disk that was Ok stays Ok so the
	// client list does not churn on cold start; the spaceLoader remains the sole authority for
	// status transitions. Brand-new spaceviews have no persisted value and default to Unknown
	// (SpaceLocalInfo.GetLocalStatus/GetRemoteStatus return Unknown when unset). Explicit reset
	// paths (spacefactory recreate, joiner rejoin) still set Unknown themselves.
	prevLocalInfo := spaceinfo.NewSpaceLocalInfoFromState(ctx.State)
	localInfo := spaceinfo.NewSpaceLocalInfo(spaceId)
	localInfo.SetLocalStatus(prevLocalInfo.GetLocalStatus()).
		SetRemoteStatus(prevLocalInfo.GetRemoteStatus()).
		UpdateDetails(ctx.State).
		Log(log)
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./core/block/editor/ -run TestSpaceView_Init_PreservesPersistedLocalStatus -v`
Expected: both subtests PASS.

- [ ] **Step 5: Run the existing spaceview tests for regressions**

Run: `go test ./core/block/editor/ -run TestSpaceView -v`
Expected: all existing `TestSpaceView_*` tests PASS (notably `TestSpaceView_Info` whose
fixture starts from an empty doc and still expects `LocalStatusUnknown` initially).

- [ ] **Step 6: Commit**

```bash
git add core/block/editor/spaceview.go core/block/editor/spaceview_test.go
git commit -m "GO-7289 Preserve persisted space localStatus in SpaceView.Init"
```

---

## Task 2: Add `storageService` dependency to `spaceLoader`

`startLoad` (Task 3) needs the synchronous `SpaceExists` check. Wire the dependency first as an
isolated, separately-committable change.

**Files:**
- Modify: `space/internal/components/spaceloader/spaceloader.go` (imports, struct, `Init`)

- [ ] **Step 1: Add the import**

In `space/internal/components/spaceloader/spaceloader.go`, add to the import block (alongside the
existing `space/...` imports):

```go
	"github.com/anyproto/anytype-heart/space/spacecore/storage"
```

- [ ] **Step 2: Add the field to the struct**

In the `spaceLoader` struct (currently lines 47-58), add the field after `builder`:

```go
	storageService      storage.ClientStorage
```

- [ ] **Step 3: Resolve the component in `Init`**

In `func (s *spaceLoader) Init`, after the `s.builder = app.MustComponent[builder.SpaceBuilder](a)`
line, add:

```go
	s.storageService = app.MustComponent[storage.ClientStorage](a)
```

- [ ] **Step 4: Verify the package builds**

Run: `go build ./space/internal/components/spaceloader/`
Expected: builds with no errors.

- [ ] **Step 5: Commit**

```bash
git add space/internal/components/spaceloader/spaceloader.go
git commit -m "GO-7289 Add storage service dependency to spaceLoader"
```

---

## Task 3: Skip `Loading` in `startLoad` for on-disk, previously-Ok spaces

**Files:**
- Modify: `space/internal/components/spaceloader/spaceloader.go:97-111` (`startLoad`)
- Test: `space/internal/components/spaceloader/spaceloader_test.go` (new file)

- [ ] **Step 1: Write the failing test**

Create `space/internal/components/spaceloader/spaceloader_test.go`:

```go
package spaceloader

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/internal/components/spacestatus/mock_spacestatus"
	"github.com/anyproto/anytype-heart/space/spacecore/storage/mock_storage"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

// blockingBuilder is a SpaceBuilder stub whose BuildSpace blocks until released, so the
// synchronous status decision in startLoad can be asserted before the background goroutine runs.
type blockingBuilder struct {
	release chan struct{}
}

func (b *blockingBuilder) Init(_ *app.App) error { return nil }
func (b *blockingBuilder) Name() string           { return "test.builder" }
func (b *blockingBuilder) BuildSpace(ctx context.Context, _ bool) (clientspace.Space, error) {
	<-b.release
	return nil, errors.New("build stopped by test")
}

func newLoaderForTest(t *testing.T, localStatus spaceinfo.LocalStatus, spaceExists bool) (*spaceLoader, *mock_spacestatus.MockSpaceStatus, chan struct{}) {
	status := mock_spacestatus.NewMockSpaceStatus(t)
	store := mock_storage.NewMockClientStorage(t)

	status.EXPECT().SpaceId().Return("spaceId").Maybe()
	status.EXPECT().GetPersistentStatus().Return(spaceinfo.AccountStatusActive).Maybe()
	status.EXPECT().GetLatestAclHeadId().Return("").Maybe()
	status.EXPECT().GetLocalStatus().Return(localStatus).Maybe()
	store.EXPECT().SpaceExists("spaceId").Return(spaceExists).Maybe()

	release := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		close(release)
	})

	s := &spaceLoader{
		status:         status,
		builder:        &blockingBuilder{release: release},
		storageService: store,
		ctx:            ctx,
		cancel:         cancel,
	}
	return s, status, release
}

func TestStartLoad_FastPath(t *testing.T) {
	t.Run("on-disk previously-Ok space skips Loading", func(t *testing.T) {
		s, status, _ := newLoaderForTest(t, spaceinfo.LocalStatusOk, true)
		// SetLocalInfo MUST NOT be called with Loading on the fast path.
		status.EXPECT().SetLocalInfo(mock.MatchedBy(func(info spaceinfo.SpaceLocalInfo) bool {
			return info.GetLocalStatus() == spaceinfo.LocalStatusLoading
		})).Return(nil).Times(0)

		require.NoError(t, s.startLoad(s.ctx))
	})

	t.Run("Unknown status sets Loading", func(t *testing.T) {
		s, status, _ := newLoaderForTest(t, spaceinfo.LocalStatusUnknown, false)
		status.EXPECT().SetLocalInfo(mock.MatchedBy(func(info spaceinfo.SpaceLocalInfo) bool {
			return info.GetLocalStatus() == spaceinfo.LocalStatusLoading
		})).Return(nil).Once()

		require.NoError(t, s.startLoad(s.ctx))
	})

	t.Run("Ok status but storage missing sets Loading (stale correction)", func(t *testing.T) {
		s, status, _ := newLoaderForTest(t, spaceinfo.LocalStatusOk, false)
		status.EXPECT().SetLocalInfo(mock.MatchedBy(func(info spaceinfo.SpaceLocalInfo) bool {
			return info.GetLocalStatus() == spaceinfo.LocalStatusLoading
		})).Return(nil).Once()

		require.NoError(t, s.startLoad(s.ctx))
	})
}
```

Add the missing `app` import to the test file's import block:
`"github.com/anyproto/any-sync/app"`.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./space/internal/components/spaceloader/ -run TestStartLoad_FastPath -v`
Expected: `on-disk previously-Ok space skips Loading` FAILS — current `startLoad` always calls
`SetLocalInfo` with `Loading`. The other two subtests PASS.

- [ ] **Step 3: Implement the fast path in `startLoad`**

In `space/internal/components/spaceloader/spaceloader.go`, replace the body of `startLoad`
(lines 97-111) — specifically the block:

```go
	if s.status.GetPersistentStatus() == spaceinfo.AccountStatusDeleted {
		return ErrSpaceDeleted
	}
	info := spaceinfo.NewSpaceLocalInfo(s.status.SpaceId())
	info.SetLocalStatus(spaceinfo.LocalStatusLoading)
	if err = s.status.SetLocalInfo(info); err != nil {
		return
	}
	s.loading = s.newLoadingSpace(s.ctx, s.stopIfMandatoryFail, s.disableRemoteLoad, s.status.GetLatestAclHeadId())
	return
```

with:

```go
	if s.status.GetPersistentStatus() == spaceinfo.AccountStatusDeleted {
		return ErrSpaceDeleted
	}
	// Fast path: a space whose store still exists on disk and was already Ok in the previous
	// session keeps reporting Ok to clients. We still run the background build below; we just
	// do not publish a transient Loading (which would make the client hide the space on cold
	// start). If onLoad later fails, it sets Missing (accepted Ok->Missing regression).
	onDiskAndOk := s.status.GetLocalStatus() == spaceinfo.LocalStatusOk &&
		s.storageService.SpaceExists(s.status.SpaceId())
	if !onDiskAndOk {
		info := spaceinfo.NewSpaceLocalInfo(s.status.SpaceId())
		info.SetLocalStatus(spaceinfo.LocalStatusLoading)
		if err = s.status.SetLocalInfo(info); err != nil {
			return
		}
	}
	s.loading = s.newLoadingSpace(s.ctx, s.stopIfMandatoryFail, s.disableRemoteLoad, s.status.GetLatestAclHeadId())
	return
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./space/internal/components/spaceloader/ -run TestStartLoad_FastPath -v`
Expected: all three subtests PASS.

- [ ] **Step 5: Commit**

```bash
git add space/internal/components/spaceloader/spaceloader.go space/internal/components/spaceloader/spaceloader_test.go
git commit -m "GO-7289 Skip Loading status for on-disk previously-Ok spaces"
```

---

## Task 4: Rework `WaitLoad` to gate on internal loader lifecycle

With an optimistic `Ok`, the current `case spaceinfo.LocalStatusOk: sp = s.space` would return a
nil space because the build has not finished. `WaitLoad` must instead key off
`s.loading`/`s.space`/`loadErr`.

**Files:**
- Modify: `space/internal/components/spaceloader/spaceloader.go:138-170` (`WaitLoad`)
- Test: `space/internal/components/spaceloader/spaceloader_test.go` (extend)

- [ ] **Step 1: Write the failing test**

Append to `space/internal/components/spaceloader/spaceloader_test.go`:

```go
func TestWaitLoad_InternalState(t *testing.T) {
	t.Run("loader not started returns error", func(t *testing.T) {
		status := mock_spacestatus.NewMockSpaceStatus(t)
		s := &spaceLoader{status: status}
		sp, err := s.WaitLoad(context.Background())
		require.Nil(t, sp)
		require.Error(t, err)
	})

	t.Run("optimistic Ok but build not finished blocks then returns space", func(t *testing.T) {
		s, _, _ := newLoaderForTest(t, spaceinfo.LocalStatusOk, true)
		s.loading = &loadingSpace{loadCh: make(chan struct{})}

		done := make(chan struct{})
		var gotErr error
		var gotSp clientspace.Space
		go func() {
			gotSp, gotErr = s.WaitLoad(context.Background())
			close(done)
		}()

		// WaitLoad must still be blocked: build not finished, space nil, no error.
		select {
		case <-done:
			t.Fatal("WaitLoad returned before the build finished")
		default:
		}

		// Simulate onLoad success then loadCh close (mirrors loadRetry's defer ordering).
		s.mx.Lock()
		s.space = stubSpace{}
		s.mx.Unlock()
		close(s.loading.loadCh)

		<-done
		require.NoError(t, gotErr)
		require.NotNil(t, gotSp)
	})

	t.Run("build failed returns load error", func(t *testing.T) {
		s, _, _ := newLoaderForTest(t, spaceinfo.LocalStatusOk, true)
		ls := &loadingSpace{loadCh: make(chan struct{})}
		ls.setLoadErr(errors.New("boom"))
		close(ls.loadCh)
		s.loading = ls

		sp, err := s.WaitLoad(context.Background())
		require.Nil(t, sp)
		require.EqualError(t, err, "boom")
	})
}

// stubSpace is a minimal clientspace.Space used only as a non-nil sentinel return value.
type stubSpace struct {
	clientspace.Space
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./space/internal/components/spaceloader/ -run TestWaitLoad_InternalState -v`
Expected: FAILS — current `WaitLoad` switches on `s.status.GetLocalStatus()`; with these mocks /
internal state it does not behave as asserted (e.g. nil-space return, or wrong branch).

- [ ] **Step 3: Rework `WaitLoad`**

In `space/internal/components/spaceloader/spaceloader.go`, replace the whole `WaitLoad` function
(lines 138-170) with:

```go
func (s *spaceLoader) WaitLoad(ctx context.Context) (sp clientspace.Space, err error) {
	s.mx.Lock()
	// Readiness is driven by the loader's own lifecycle, NOT by the client-facing persisted
	// localStatus. localStatus may be an optimistic Ok (set before the background build
	// finished); returning s.space here without the loadCh wait would hand back a nil space.
	if s.loading == nil {
		s.mx.Unlock()
		return nil, fmt.Errorf("waitLoad for a not started space")
	}
	if s.space != nil {
		sp = s.space
		s.mx.Unlock()
		return sp, nil
	}
	loading := s.loading
	loadErr := loading.getLoadErr()
	if loadErr != nil {
		s.mx.Unlock()
		return nil, loadErr
	}
	waitCh := loading.loadCh
	s.mx.Unlock()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-waitCh:
	}
	return s.WaitLoad(ctx)
}
```

(Confirm `fmt` is still imported — it is, used elsewhere in the file.)

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./space/internal/components/spaceloader/ -run TestWaitLoad_InternalState -v`
Expected: all subtests PASS.

- [ ] **Step 5: Run the full spaceloader package tests**

Run: `go test ./space/internal/components/spaceloader/ -v`
Expected: all tests PASS (`TestStartLoad_FastPath` and `TestWaitLoad_InternalState`).

- [ ] **Step 6: Commit**

```bash
git add space/internal/components/spaceloader/spaceloader.go space/internal/components/spaceloader/spaceloader_test.go
git commit -m "GO-7289 Gate WaitLoad on internal loader state, not persisted localStatus"
```

---

## Task 5: Regression sweep across affected packages

**Files:** none modified — verification only.

- [ ] **Step 1: Build the whole module**

Run: `go build ./...`
Expected: no errors.

- [ ] **Step 2: Run the affected package test suites**

Run: `go test ./space/internal/components/spaceloader/ ./core/block/editor/ ./space/internal/components/spacestatus/ ./space/spacefactory/ ./space/internal/spaceprocess/joiner/`
Expected: all PASS. These cover the loader changes plus the explicit `Unknown`-reset paths
(`spacefactory` recreate, `joiner` rejoin) that the spec requires to remain unaffected.

- [ ] **Step 3: Manual verification note (record result in the PR description)**

On a device/profile with several existing spaces, cold-start the app from a fully closed state
and confirm via the spaceview subscription / debug stat (`spaceStatusStat.LocalStatus`) that
existing spaces report `Ok` immediately without transitioning through `Unknown`/`Loading`, and
that opening a space still works (blocks until built). Confirm a space deleted remotely while the
app was closed correctly transitions `Ok -> Missing`.

- [ ] **Step 4: Final commit (if any doc/PR notes were added); otherwise skip**

```bash
git commit --allow-empty -m "GO-7289 Verify optimistic localStatus regression sweep"
```

---

## Self-Review

**Spec coverage:**
- Design §1 (Init preserves persisted status) → Task 1.
- Design §2 (`startLoad` skips Loading when on disk + Ok; `SpaceExists` guard; build still runs) → Tasks 2 & 3.
- Design §3 (`WaitLoad` internal-state rework, correctness-critical) → Task 4.
- Design "Key implementation risk to verify first" → Task 1 Step 1/2 risk gate, with explicit STOP instruction.
- Design "Edge cases" (stale Ok/storage wiped → Loading; remote-deleted → Missing; not-started → error; explicit reset paths unchanged) → Task 3 subtests, Task 4 subtests, Task 5 Step 2.
- Design "Testing" bullets → Tasks 1, 3, 4 tests; Task 5 regression sweep.
- `onLoad` unchanged (success Ok / failure Missing) → no task needed (no code change), covered by existing behavior + Task 5 sweep.

**Placeholder scan:** No TBD/TODO; all steps contain concrete code and exact commands.

**Type consistency:** `spaceLoader` fields (`status`, `builder`, `storageService`, `ctx`,
`cancel`, `loading`, `space`, `mx`) match `spaceloader.go`. `storage.ClientStorage` +
`mock_storage.NewMockClientStorage` and `spacestatus.SpaceStatus` +
`mock_spacestatus.NewMockSpaceStatus` match `.mockery.yaml`. `loadingSpace` fields used in tests
(`loadCh`, `setLoadErr`, `getLoadErr`) match `loadingspace.go`. `spaceinfo` API
(`NewSpaceLocalInfo`, `NewSpaceLocalInfoFromState`, `SetLocalStatus`, `SetRemoteStatus`,
`GetLocalStatus`, `GetRemoteStatus`, `UpdateDetails`) matches `spaceinfo.go`/`spacelocalinfo.go`.
`SpaceExists(string) bool` is on `storage.ClientStorage` via the embedded
`spacestorage.SpaceStorageProvider`.
