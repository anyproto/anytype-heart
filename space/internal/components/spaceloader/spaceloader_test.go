package spaceloader

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/anyproto/any-sync/app"
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
func (b *blockingBuilder) Name() string          { return "test.builder" }
func (b *blockingBuilder) BuildSpace(ctx context.Context, _ bool) (clientspace.Space, error) {
	<-b.release
	return nil, errors.New("build stopped by test")
}

// loaderTestFixture records every localStatus written via SetLocalInfo so assertions do not
// depend on testify expectation precedence (the background goroutine writes Missing on cleanup).
type loaderTestFixture struct {
	loader      *spaceLoader
	status      *mock_spacestatus.MockSpaceStatus
	mu          sync.Mutex
	setStatuses []spaceinfo.LocalStatus
}

func (f *loaderTestFixture) recordedStatuses() []spaceinfo.LocalStatus {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]spaceinfo.LocalStatus, len(f.setStatuses))
	copy(out, f.setStatuses)
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
		fx.setStatuses = append(fx.setStatuses, info.GetLocalStatus())
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

		// then: startLoad must NOT have published a transient Loading
		require.False(t, containsStatus(fx.recordedStatuses(), spaceinfo.LocalStatusLoading),
			"fast path must not publish Loading")
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
