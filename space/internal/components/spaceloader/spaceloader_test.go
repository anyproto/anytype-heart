package spaceloader

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/space/clientspace"
	"github.com/anyproto/anytype-heart/space/internal/components/spacestatus/mock_spacestatus"
	"github.com/anyproto/anytype-heart/space/spacecore/storage/mock_storage"
	"github.com/anyproto/anytype-heart/space/spaceinfo"
)

// stubSpace is a minimal clientspace.Space used only as a non-nil sentinel return value.
type stubSpace struct {
	clientspace.Space
}

// blockingBuilder is a SpaceBuilder stub whose BuildSpace blocks until released, so the
// synchronous status decision in startLoad can be asserted before the background goroutine runs.
type blockingBuilder struct {
	release chan struct{}
}

func (b *blockingBuilder) Init(_ *app.App) error { return nil }
func (b *blockingBuilder) Name() string          { return "test.builder" }
func (b *blockingBuilder) BuildSpace(ctx context.Context, _ bool) (clientspace.Space, error) {
	<-b.release
	return nil, errors.New("build stopped by test")
}

// loaderTestFixture records every SpaceLocalInfo written via SetLocalInfo so assertions do not
// depend on testify expectation precedence (the background goroutine writes Missing on cleanup).
type loaderTestFixture struct {
	loader   *spaceLoader
	status   *mock_spacestatus.MockSpaceStatus
	mu       sync.Mutex
	setInfos []spaceinfo.SpaceLocalInfo
}

func (f *loaderTestFixture) recordedStatuses() []spaceinfo.LocalStatus {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]spaceinfo.LocalStatus, len(f.setInfos))
	for i, info := range f.setInfos {
		out[i] = info.GetLocalStatus()
	}
	return out
}

func (f *loaderTestFixture) recordedInfos() []spaceinfo.SpaceLocalInfo {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]spaceinfo.SpaceLocalInfo, len(f.setInfos))
	copy(out, f.setInfos)
	return out
}

func newLoaderForTest(t *testing.T, localStatus spaceinfo.LocalStatus, spaceExists bool) *loaderTestFixture {
	status := mock_spacestatus.NewMockSpaceStatus(t)
	store := mock_storage.NewMockClientStorage(t)

	status.EXPECT().SpaceId().Return("spaceId").Maybe()
	status.EXPECT().GetPersistentStatus().Return(spaceinfo.AccountStatusActive).Maybe()
	status.EXPECT().GetLatestAclHeadId().Return("").Maybe()
	status.EXPECT().GetLocalStatus().Return(localStatus).Maybe()
	store.EXPECT().SpaceExists("spaceId").Return(spaceExists).Maybe()

	fx := &loaderTestFixture{status: status}
	status.EXPECT().SetLocalInfo(mock.Anything).RunAndReturn(func(info spaceinfo.SpaceLocalInfo) error {
		fx.mu.Lock()
		fx.setInfos = append(fx.setInfos, info)
		fx.mu.Unlock()
		return nil
	}).Maybe()

	release := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		close(release)
	})

	fx.loader = &spaceLoader{
		status:         status,
		builder:        &blockingBuilder{release: release},
		storageService: store,
		ctx:            ctx,
		cancel:         cancel,
	}
	return fx
}

func containsStatus(statuses []spaceinfo.LocalStatus, target spaceinfo.LocalStatus) bool {
	for _, s := range statuses {
		if s == target {
			return true
		}
	}
	return false
}

func TestStartLoad_FastPath(t *testing.T) {
	t.Run("on-disk previously-Ok space skips Loading", func(t *testing.T) {
		// given
		fx := newLoaderForTest(t, spaceinfo.LocalStatusOk, true)

		// when
		require.NoError(t, fx.loader.startLoad(fx.loader.ctx))

		// then: the fast path must issue NO SetLocalInfo write at all (not just "no Loading").
		// A spurious SetLocalInfo with a freshly-built SpaceLocalInfo would carry localStatus=Ok
		// but unset remoteStatus/limits, silently clobbering the persisted values; require.Empty
		// pins that startLoad leaves the optimistic Ok completely untouched.
		require.Empty(t, fx.recordedStatuses(), "fast path must not write any localStatus")
	})

	t.Run("Unknown status sets Loading", func(t *testing.T) {
		// given
		fx := newLoaderForTest(t, spaceinfo.LocalStatusUnknown, false)

		// when
		require.NoError(t, fx.loader.startLoad(fx.loader.ctx))

		// then
		require.True(t, containsStatus(fx.recordedStatuses(), spaceinfo.LocalStatusLoading),
			"Unknown space must publish Loading")
	})

	t.Run("Ok status but storage missing sets Loading (stale correction)", func(t *testing.T) {
		// given
		fx := newLoaderForTest(t, spaceinfo.LocalStatusOk, false)

		// when
		require.NoError(t, fx.loader.startLoad(fx.loader.ctx))

		// then
		require.True(t, containsStatus(fx.recordedStatuses(), spaceinfo.LocalStatusLoading),
			"stale Ok without storage must publish Loading")
	})
}

func TestWaitLoad_InternalState(t *testing.T) {
	t.Run("loader not started returns error", func(t *testing.T) {
		// given a loader whose background load was never started
		s := &spaceLoader{status: mock_spacestatus.NewMockSpaceStatus(t)}

		// when
		sp, err := s.WaitLoad(context.Background())

		// then
		require.Nil(t, sp)
		require.Error(t, err)
	})

	t.Run("optimistic Ok but build not finished blocks then returns space", func(t *testing.T) {
		// given a loader with an in-progress build (loadCh open, space not set yet)
		fx := newLoaderForTest(t, spaceinfo.LocalStatusOk, true)
		s := fx.loader
		s.loading = &loadingSpace{loadCh: make(chan struct{})}

		done := make(chan struct{})
		var gotSp clientspace.Space
		var gotErr error
		go func() {
			gotSp, gotErr = s.WaitLoad(context.Background())
			close(done)
		}()

		// WaitLoad must still be blocked: build not finished, space nil, no error.
		select {
		case <-done:
			t.Fatal("WaitLoad returned before the build finished")
		case <-time.After(50 * time.Millisecond):
		}

		// when the build finishes successfully (mirrors loadRetry's defer ordering)
		s.mx.Lock()
		s.space = stubSpace{}
		s.mx.Unlock()
		close(s.loading.loadCh)

		// then
		<-done
		require.NoError(t, gotErr)
		require.NotNil(t, gotSp)
	})

	t.Run("build failed returns load error", func(t *testing.T) {
		// given a loader whose background build finished with an error
		fx := newLoaderForTest(t, spaceinfo.LocalStatusOk, true)
		s := fx.loader
		ls := &loadingSpace{loadCh: make(chan struct{})}
		ls.setLoadErr(errors.New("boom"))
		close(ls.loadCh)
		s.loading = ls

		// when
		sp, err := s.WaitLoad(context.Background())

		// then
		require.Nil(t, sp)
		require.EqualError(t, err, "boom")
	})

	t.Run("context cancelled mid-wait returns ctx error", func(t *testing.T) {
		// given a loader with an in-progress build (loadCh open, space not set yet). This is the
		// path Close() relies on: it cancels the ctx then WaitLoads while the build is running.
		fx := newLoaderForTest(t, spaceinfo.LocalStatusOk, true)
		s := fx.loader
		s.loading = &loadingSpace{loadCh: make(chan struct{})}

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		var gotSp clientspace.Space
		var gotErr error
		go func() {
			gotSp, gotErr = s.WaitLoad(ctx)
			close(done)
		}()

		// WaitLoad must be blocked on the loadCh: build not finished, space nil, no error.
		select {
		case <-done:
			t.Fatal("WaitLoad returned before the build finished or ctx was cancelled")
		case <-time.After(50 * time.Millisecond):
		}

		// when the caller cancels the context (build never finishes; loadCh stays open)
		cancel()

		// then WaitLoad returns promptly with the ctx error and no space
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("WaitLoad did not return after ctx cancellation")
		}
		require.Nil(t, gotSp)
		require.ErrorIs(t, gotErr, context.Canceled)
	})
}

// TestOnLoad_AfterFastPath covers the spec's most safety-critical claim: after the fast path
// leaves an optimistic Ok, the background build's onLoad still drives the final status. onLoad
// is unchanged by this PR but was previously unreachable by any test (blockingBuilder never
// releases before assertions run), so the Ok->Missing flip and the idempotent-Ok no-op were
// unverified. These call onLoad directly to pin both branches deterministically.
func TestOnLoad_AfterFastPath(t *testing.T) {
	t.Run("build success keeps localStatus Ok and leaves remoteStatus untouched", func(t *testing.T) {
		// given a fast-path space already optimistically Ok with a persisted remoteStatus
		fx := newLoaderForTest(t, spaceinfo.LocalStatusOk, true)

		// when the background build succeeds
		require.NoError(t, fx.loader.onLoad(stubSpace{}, nil))

		// then onLoad sets localStatus=Ok and does NOT write remoteStatus (it is left unset on
		// the SpaceLocalInfo, so SetLocalInfo->UpdateDetails preserves the persisted value).
		infos := fx.recordedInfos()
		require.Len(t, infos, 1)
		assert.Equal(t, spaceinfo.LocalStatusOk, infos[0].GetLocalStatus())
		assert.Equal(t, spaceinfo.RemoteStatusUnknown, infos[0].GetRemoteStatus(),
			"onLoad success must not overwrite the persisted remoteStatus")
		// the built space becomes available to open-on-demand callers
		assert.Equal(t, stubSpace{}, fx.loader.space)
	})

	t.Run("build failure flips an optimistic Ok to Missing", func(t *testing.T) {
		// given a fast-path space already optimistically Ok
		fx := newLoaderForTest(t, spaceinfo.LocalStatusOk, true)

		// when the background build fails (the accepted Ok->Missing regression)
		require.NoError(t, fx.loader.onLoad(nil, errors.New("build failed")))

		// then the recorded status sequence ends in Missing and no space is stored
		statuses := fx.recordedStatuses()
		require.NotEmpty(t, statuses)
		assert.Equal(t, spaceinfo.LocalStatusMissing, statuses[len(statuses)-1])
		assert.Nil(t, fx.loader.space)
	})

	t.Run("shutdown cancellation must not persist Missing", func(t *testing.T) {
		// given a fast-path space already optimistically Ok whose Close() cancelled the build
		fx := newLoaderForTest(t, spaceinfo.LocalStatusOk, true)

		// when the background build is interrupted by context cancellation (clean shutdown), even
		// when wrapped by intermediate layers
		require.NoError(t, fx.loader.onLoad(nil, fmt.Errorf("build space: %w", context.Canceled)))

		// then onLoad writes NO status: persisting Missing would knock the healthy space off the
		// optimistic-Ok fast path on the next cold start.
		require.Empty(t, fx.recordedStatuses(), "shutdown cancellation must leave persisted status untouched")
		assert.Nil(t, fx.loader.space)
	})
}
