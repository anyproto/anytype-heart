package filedownloader

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonfile/fileblockstore"
	"github.com/anyproto/any-sync/commonfile/fileservice"
	blocks "github.com/ipfs/go-block-format"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/block/cache/mock_cache"
	"github.com/anyproto/anytype-heart/core/device/mock_device"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files/filestorage"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/subscription/crossspacesub"
	"github.com/anyproto/anytype-heart/core/subscription/crossspacesub/mock_crossspacesub"
	"github.com/anyproto/anytype-heart/core/wallet/mock_wallet"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

type fixture struct {
	*downloader
}

func newFixture(t *testing.T) *fixture {
	s := New().(*service)
	return &fixture{
		downloader: s.newDownloader(20 * 1024 * 1024),
	}
}

type serviceFixture struct {
	*service
	subscribeCh chan subscription.SubscribeRequest
	config      *config.Config
}

func newServiceFixture(t *testing.T) *serviceFixture {
	s := New()
	crossSpaceSub := mock_crossspacesub.NewMockService(t)
	objectGetter := mock_cache.NewMockObjectGetterComponent(t)
	networkState := mock_device.NewMockNetworkState(t)
	networkState.EXPECT().RegisterHook(mock.Anything).Return().Maybe()

	cfg := config.New(config.DisableFileConfig(true))
	cfg.PeferYamuxTransport = true

	walletMock := mock_wallet.NewMockWallet(t)
	walletMock.EXPECT().RepoPath().Return(t.TempDir()).Maybe()

	subscribeCh := make(chan subscription.SubscribeRequest, 10)
	crossSpaceSub.EXPECT().Subscribe(mock.Anything, mock.Anything).RunAndReturn(
		func(req subscription.SubscribeRequest, _ crossspacesub.Predicate) (*subscription.SubscribeResponse, error) {
			subscribeCh <- req
			return &subscription.SubscribeResponse{Records: []*domain.Details{}}, nil
		},
	).Maybe()

	ctx := context.Background()
	a := new(app.App)
	a.Register(testutil.PrepareMock(ctx, a, crossSpaceSub))
	a.Register(testutil.PrepareMock(ctx, a, objectGetter))
	a.Register(testutil.PrepareMock(ctx, a, networkState))
	a.Register(testutil.PrepareMock(ctx, a, walletMock))
	a.Register(cfg)
	a.Register(filestorage.NewInMemory())
	a.Register(fileservice.New())

	err := a.Start(ctx)
	require.NoError(t, err)

	err = s.Init(a)
	require.NoError(t, err)

	t.Cleanup(func() {
		s.Close(ctx)
		a.Close(ctx)
	})

	return &serviceFixture{
		service:     s.(*service),
		subscribeCh: subscribeCh,
		config:      cfg,
	}
}

func (fx *serviceFixture) waitForSubscribe(t *testing.T) subscription.SubscribeRequest {
	t.Helper()
	select {
	case req := <-fx.subscribeCh:
		return req
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for Subscribe call")
		return subscription.SubscribeRequest{}
	}
}

func findSizeFilter(filters []database.FilterRequest) (domain.Value, bool) {
	for _, f := range filters {
		if f.RelationKey == bundle.RelationKeySizeInBytes {
			return f.Value, true
		}
	}
	return domain.Value{}, false
}

func TestCacheFileRootBlockCheck(t *testing.T) {
	fx := newServiceFixture(t)

	// Replace the warmer with an observable one so we can tell whether a
	// warm-up was actually scheduled.
	downloadCh := make(chan domain.FileId, 4)
	fx.cacheWarmer = newCacheWarmer(fx.ctx, 10, 20, time.Minute,
		func(ctx context.Context, spaceId string, fileCid domain.FileId, blocksLimit int) error {
			downloadCh <- fileCid
			return nil
		})
	go fx.cacheWarmer.run()
	go fx.cacheWarmer.runWorker()

	t.Run("skips warm-up when root block already cached", func(t *testing.T) {
		ctx := fileblockstore.CtxWithSpaceId(context.Background(), "space1")
		node, err := fx.commonFile.AddFile(ctx, bytes.NewReader([]byte("already cached file")))
		require.NoError(t, err)
		cachedFileId := domain.FileId(node.Cid().String())

		fx.CacheFile("space1", cachedFileId)

		select {
		case got := <-downloadCh:
			t.Fatalf("expected warm-up to be skipped, but download ran for %s", got)
		case <-time.After(200 * time.Millisecond):
		}
	})

	t.Run("warms up when root block is missing", func(t *testing.T) {
		absentCid := blocks.NewBlock([]byte("never stored in the blockstore")).Cid()
		missingFileId := domain.FileId(absentCid.String())

		fx.CacheFile("space1", missingFileId)

		select {
		case got := <-downloadCh:
			assert.Equal(t, missingFileId, got)
		case <-time.After(2 * time.Second):
			t.Fatal("expected warm-up to run for a missing root block")
		}
	})

	t.Run("reports skipped, enqueued and warmed-up counters", func(t *testing.T) {
		want := stat{SkippedFiles: 1, EnqueuedFiles: 1, WarmedUpFiles: 1}
		assert.Eventually(t, func() bool {
			return fx.ProvideStat() == want
		}, 2*time.Second, 10*time.Millisecond)
	})
}

func TestCacheWarmerCancelBeforeStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// No worker is started, so the enqueued task stays queued and is cancelled
	// before its warm-up can begin.
	w := newCacheWarmer(ctx, 10, 10, time.Minute, func(context.Context, string, domain.FileId, int) error {
		return nil
	})
	go w.run()

	w.enqueue("space1", "file1")
	w.cancelTask("file1")

	assert.Eventually(t, func() bool {
		return w.cancelledBeforeStart.Load() == 1 && w.warmedUp.Load() == 0
	}, time.Second, 10*time.Millisecond)
}

func TestManger(t *testing.T) {
	t.Run("get one task", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.stop()

		go fx.runManager()

		want := downloadTask{
			objectId: "1",
			fileId:   "fileId1",
		}
		fx.addTaskCh <- want

		task, ok := fx.getNextTask()
		require.True(t, ok)

		assert.Equal(t, want, task)
	})

	t.Run("wait for task", func(t *testing.T) {
		fx := newFixture(t)

		go fx.runManager()

		const n = 10

		var wg sync.WaitGroup
		gotTasksCh := make(chan downloadTask, n)
		for range n {
			wg.Add(1)
			go func() {
				defer wg.Done()
				task, ok := fx.getNextTask()
				require.True(t, ok)
				gotTasksCh <- task
			}()
		}

		var wantTasks []downloadTask
		for i := range n {
			want := downloadTask{
				objectId: fmt.Sprintf("%d", i),
				fileId:   domain.FileId(fmt.Sprintf("fileId%d", i)),
			}
			fx.addTaskCh <- want
			wantTasks = append(wantTasks, want)
		}

		wg.Wait()

		var gotTasks []downloadTask
		for range n {
			got := <-gotTasksCh
			gotTasks = append(gotTasks, got)
		}

		assert.ElementsMatch(t, wantTasks, gotTasks)
	})
}

func TestSubscriptionReloadOnLimitChange(t *testing.T) {
	t.Run("subscribe with initial size limit", func(t *testing.T) {
		fx := newServiceFixture(t)

		fx.setDownloadState(true, false, 100)

		req := fx.waitForSubscribe(t)
		val, ok := findSizeFilter(req.Filters)
		require.True(t, ok, "expected SizeInBytes filter")
		assert.Equal(t, domain.Int64(100*1024*1024), val)
	})

	t.Run("changing limit resubscribes with new filter", func(t *testing.T) {
		fx := newServiceFixture(t)

		// Enable with initial limit
		fx.setDownloadState(true, false, 100)
		req := fx.waitForSubscribe(t)
		val, ok := findSizeFilter(req.Filters)
		require.True(t, ok)
		assert.Equal(t, domain.Int64(100*1024*1024), val)

		// Change limit — downloader restarts, new subscription
		fx.setDownloadState(true, false, 200)
		req = fx.waitForSubscribe(t)
		val, ok = findSizeFilter(req.Filters)
		require.True(t, ok, "expected SizeInBytes filter with new limit")
		assert.Equal(t, domain.Int64(200*1024*1024), val)
	})

	t.Run("zero limit subscribes without size filter", func(t *testing.T) {
		fx := newServiceFixture(t)

		fx.setDownloadState(true, false, 0)

		req := fx.waitForSubscribe(t)
		_, ok := findSizeFilter(req.Filters)
		assert.False(t, ok, "expected no SizeInBytes filter for unlimited")
	})

	t.Run("changing from limit to unlimited removes size filter", func(t *testing.T) {
		fx := newServiceFixture(t)

		fx.setDownloadState(true, false, 500) // 500 MiB
		req := fx.waitForSubscribe(t)
		val, ok := findSizeFilter(req.Filters)
		require.True(t, ok)
		assert.Equal(t, domain.Int64(500*1024*1024), val)

		// Switch to unlimited
		fx.setDownloadState(true, false, 0)
		req = fx.waitForSubscribe(t)
		_, ok = findSizeFilter(req.Filters)
		assert.False(t, ok, "expected no SizeInBytes filter after switching to unlimited")
	})
}
