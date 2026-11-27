package filesync

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/anyproto/any-sync/commonfile/fileproto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/files/filestorage/rpcstore/mock_rpcstore"
	"github.com/anyproto/anytype-heart/pb"
)

func TestSpaceLimit(t *testing.T) {
	t.Run("concurrent file upload", func(t *testing.T) {
		for _, tc := range []struct {
			name          string
			limit         int
			usage         int
			filesToUpload int
			fileSize      int

			errorsCount int
		}{
			{
				name:          "3 concurrent files, 2 ok, 1 out of limits",
				limit:         100_000_000,
				usage:         0,
				filesToUpload: 3,
				fileSize:      40_000_000,
				errorsCount:   1,
			},
			{
				name:          "3 concurrent files, some used space, 1 ok, 2 out of limits",
				limit:         100_000_000,
				usage:         40_000_000,
				filesToUpload: 3,
				fileSize:      40_000_000,
				errorsCount:   2,
			},
			{
				name:          "10 concurrent files, 1 ok, 9 out of limits",
				limit:         100_000_000,
				usage:         0,
				filesToUpload: 10,
				fileSize:      90_000_000,
				errorsCount:   9,
			},
			{
				name:          "10 concurrent files, 10 ok",
				limit:         100_000_000,
				usage:         0,
				filesToUpload: 10,
				fileSize:      1_000,
				errorsCount:   0,
			},
			{
				name:          "10 concurrent files, no free space, 10 out of limits",
				limit:         100_000_000,
				usage:         99_999_999,
				filesToUpload: 10,
				fileSize:      1_000,
				errorsCount:   10,
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				rpcStore := mock_rpcstore.NewMockRpcStore(t)
				rpcStore.EXPECT().SpaceInfo(mock.Anything, mock.Anything).RunAndReturn(func(ctx2 context.Context, spaceId string) (*fileproto.SpaceInfoResponse, error) {
					return &fileproto.SpaceInfoResponse{
						SpaceId:         spaceId,
						LimitBytes:      uint64(tc.limit),
						SpaceUsageBytes: 0,
						TotalUsageBytes: uint64(tc.usage),
					}, nil
				})

				ctx := context.Background()
				spaceId := "space1"
				updateCh := make(chan updateMessage, 1)

				usage := newSpaceUsage(ctx, spaceId, rpcStore, updateCh)

				var wg sync.WaitGroup
				errorsCh := make(chan error, tc.filesToUpload)
				for i := 0; i < tc.filesToUpload; i++ {
					wg.Add(1)
					go func() {
						defer wg.Done()

						fileKey := fmt.Sprintf("file%d", i)
						errorsCh <- usage.allocateFile(ctx, fileKey, tc.fileSize)
					}()
				}

				wg.Wait()
				close(errorsCh)

				var errorsCount int
				for err := range errorsCh {
					if err != nil {
						errorsCount++
					}
				}

				assert.Equal(t, tc.errorsCount, errorsCount)
			})
		}
	})

	t.Run("test allocate then remove", func(t *testing.T) {
		rpcStore := mock_rpcstore.NewMockRpcStore(t)
		rpcStore.EXPECT().SpaceInfo(mock.Anything, mock.Anything).RunAndReturn(func(ctx2 context.Context, spaceId string) (*fileproto.SpaceInfoResponse, error) {
			return &fileproto.SpaceInfoResponse{
				SpaceId:         spaceId,
				LimitBytes:      uint64(100_000_000),
				SpaceUsageBytes: uint64(0),
			}, nil
		})

		ctx := context.Background()
		spaceId := "space1"
		updateCh := make(chan updateMessage, 1)

		usage := newSpaceUsage(ctx, spaceId, rpcStore, updateCh)

		err := usage.allocateFile(ctx, "file1", 90_000_000)
		require.NoError(t, err)

		err = usage.allocateFile(ctx, "file2", 80_000_000)
		require.Error(t, err)

		usage.deallocateFile("file1")

		err = usage.allocateFile(ctx, "file2", 80_000_000)
		require.NoError(t, err)
	})

	t.Run("test allocate then mark as uploaded", func(t *testing.T) {
		rpcStore := mock_rpcstore.NewMockRpcStore(t)
		rpcStore.EXPECT().SpaceInfo(mock.Anything, mock.Anything).RunAndReturn(func(ctx2 context.Context, spaceId string) (*fileproto.SpaceInfoResponse, error) {
			return &fileproto.SpaceInfoResponse{
				SpaceId:         spaceId,
				LimitBytes:      uint64(100_000_000),
				SpaceUsageBytes: uint64(0),
			}, nil
		})

		ctx := context.Background()
		spaceId := "space1"

		updateCh := make(chan updateMessage, 1)

		usage := newSpaceUsage(ctx, spaceId, rpcStore, updateCh)

		err := usage.allocateFile(ctx, "file1", 90_000_000)
		require.NoError(t, err)

		err = usage.allocateFile(ctx, "file2", 80_000_000)
		require.Error(t, err)

		usage.markFileUploaded("file1")

		err = usage.allocateFile(ctx, "file2", 80_000_000)
		require.Error(t, err)
	})
}

func TestSpaceUsageReported(t *testing.T) {
	// given: a space usage the node has not answered for yet
	rpcStore := mock_rpcstore.NewMockRpcStore(t)
	rpcStore.EXPECT().SpaceInfo(mock.Anything, "space1").Return(&fileproto.SpaceInfoResponse{
		SpaceId:         "space1",
		LimitBytes:      5 << 30, // over 4 GiB: must survive as 64-bit
		TotalUsageBytes: 600,
	}, nil).Once()
	usage := &spaceUsage{
		ctx:      context.Background(),
		spaceId:  "space1",
		rpcStore: rpcStore,
		updateCh: make(chan updateMessage, 1),
		files:    map[string]allocatedFile{},
	}

	// then: nothing is reported before the node answers
	_, ok := usage.reported()
	require.False(t, ok)

	// when: the node answers
	require.NoError(t, usage.Update(context.Background()))

	// then: its limit and usage are reported, with nothing confirmed since
	got, ok := usage.reported()
	require.True(t, ok)
	assert.Equal(t, int64(5<<30), got.LimitBytes)
	assert.Equal(t, int64(600), got.NodeUsageBytes)
	assert.Empty(t, got.Confirmed)
	assert.False(t, got.ReportedAt.IsZero())

	// when: an upload is in flight, nothing is confirmed
	require.NoError(t, usage.allocateFile(context.Background(), "file1", 300))
	got, _ = usage.reported()
	assert.Empty(t, got.Confirmed)

	// when: the upload is confirmed, it is listed by its key, not yet in a
	// report, and the node's usage stays the node's
	before := got
	usage.markFileUploaded("file1")
	got, _ = usage.reported()
	assert.Equal(t, map[string]ConfirmedUpload{"file1": {Bytes: 300}}, got.Confirmed)
	assert.Equal(t, int64(600), got.NodeUsageBytes)
	// and a snapshot read earlier is not changed under its reader, nor by a
	// second confirmation
	assert.Empty(t, before.Confirmed)
	held := got
	require.NoError(t, usage.allocateFile(context.Background(), "file2", 50))
	usage.markFileUploaded("file2")
	assert.Equal(t, map[string]ConfirmedUpload{"file1": {Bytes: 300}}, held.Confirmed)
}

func TestSpaceUsageConfirmedAcrossReports(t *testing.T) {
	// given: an upload confirmed after a report
	rpcStore := mock_rpcstore.NewMockRpcStore(t)
	rpcStore.EXPECT().SpaceInfo(mock.Anything, "space1").Return(&fileproto.SpaceInfoResponse{
		LimitBytes: 1000, TotalUsageBytes: 600,
	}, nil)
	usage := &spaceUsage{
		ctx: context.Background(), spaceId: "space1", rpcStore: rpcStore,
		updateCh: make(chan updateMessage, 1), files: map[string]allocatedFile{},
	}
	require.NoError(t, usage.Update(context.Background()))
	require.NoError(t, usage.allocateFile(context.Background(), "file1", 300))
	usage.markFileUploaded("file1")
	refresh := func() {
		usage.lock.Lock()
		usage.updatedAt = usage.updatedAt.Add(-time.Hour) // stale: the next update asks the node
		usage.lock.Unlock()
		require.NoError(t, usage.Update(context.Background()))
	}

	// when: the next report comes in
	refresh()

	// then: the upload stays listed, marked as in that report
	got, _ := usage.reported()
	assert.Equal(t, map[string]ConfirmedUpload{"file1": {Bytes: 300, InReport: true}}, got.Confirmed)

	// when: the next report comes long after the confirmation, but within
	// the retention of the first report that included it
	usage.reportLock.Lock()
	entry := usage.confirmed["file1"]
	entry.inReportAt = time.Now().Add(-confirmedRetention / 2)
	usage.confirmed["file1"] = entry
	usage.reportLock.Unlock()
	refresh()

	// then: it is kept
	got, _ = usage.reported()
	assert.Contains(t, got.Confirmed, "file1")

	// when: a report comes in past that retention
	usage.reportLock.Lock()
	entry = usage.confirmed["file1"]
	entry.inReportAt = time.Now().Add(-2 * confirmedRetention)
	usage.confirmed["file1"] = entry
	usage.reportLock.Unlock()
	refresh()

	// then: it is dropped
	got, _ = usage.reported()
	assert.Empty(t, got.Confirmed)
}

func TestSpaceUsageConfirmedBeforeAnyReport(t *testing.T) {
	// given: refreshes failed for longer than the retention; an upload was
	// confirmed meanwhile
	rpcStore := mock_rpcstore.NewMockRpcStore(t)
	rpcStore.EXPECT().SpaceInfo(mock.Anything, "space1").Return(&fileproto.SpaceInfoResponse{
		LimitBytes: 1000, TotalUsageBytes: 900,
	}, nil)
	usage := &spaceUsage{
		ctx: context.Background(), spaceId: "space1", rpcStore: rpcStore,
		updateCh: make(chan updateMessage, 1), files: map[string]allocatedFile{},
	}
	usage.files["file1"] = allocatedFile{size: 300}
	usage.allocatedUsage = 300
	usage.markFileUploaded("file1")

	// when: the first report finally comes in
	require.NoError(t, usage.Update(context.Background()))

	// then: the upload is kept, as in that report, for the retention from now
	got, ok := usage.reported()
	require.True(t, ok)
	assert.Equal(t, map[string]ConfirmedUpload{"file1": {Bytes: 300, InReport: true}}, got.Confirmed)
}

func TestFileSyncSpaceLimits(t *testing.T) {
	t.Run("the space's limits once the node has answered", func(t *testing.T) {
		// given
		fx := newFixture(t, 1024)
		t.Cleanup(func() { fx.Finish(t) })

		// when, then: the first report runs in the background
		require.Eventually(t, func() bool {
			got, ok, err := fx.SpaceLimits("space1")
			return err == nil && ok && got.LimitBytes == 1024 && got.NodeUsageBytes == 0
		}, 5*time.Second, 10*time.Millisecond)
	})
	t.Run("the account report the node sends proves the limit is the plan's", func(t *testing.T) {
		// given: the node's account report and space report, ingested as the
		// app ingests them
		fx := newFixture(t, 1024)
		t.Cleanup(func() { fx.Finish(t) })

		// when, then
		require.Eventually(t, func() bool {
			got, ok, _ := fx.SpaceLimits("space1")
			return ok && got.PlanLimit
		}, 5*time.Second, 10*time.Millisecond)
	})
	t.Run("the limit is the plan's only on recent, conclusive account evidence", func(t *testing.T) {
		// given: the space reports a limit of 1000
		fx := newFixture(t, 1000)
		t.Cleanup(func() { fx.Finish(t) })
		require.Eventually(t, func() bool {
			_, ok, _ := fx.SpaceLimits("space1")
			return ok
		}, 5*time.Second, 10*time.Millisecond)
		fiveGiB := int64(5 << 30)
		for name, tc := range map[string]struct {
			account accountLimits
			limit   int64
			want    bool
		}{
			"whole: nothing given to a space of its own":       {accountLimits{shared: 1000, total: 1000, at: time.Now()}, 1000, true},
			"shared, though another space has 200 of its own":  {accountLimits{shared: 1000, total: 1200, at: time.Now()}, 1000, true},
			"equal to the shared limit, but could be isolated": {accountLimits{shared: 1000, total: 3000, at: time.Now()}, 1000, false},
			"a limit apart from the shared one":                {accountLimits{shared: 800, total: 1000, at: time.Now()}, 1000, false},
			"a stale account report":                           {accountLimits{shared: 1000, total: 1000, at: time.Now().Add(-time.Hour)}, 1000, false},
			"a node that sends no whole limit":                 {accountLimits{shared: 1000, at: time.Now()}, 1000, false},
			"above 4 GiB, on any build":                        {accountLimits{shared: fiveGiB, total: fiveGiB, at: time.Now()}, fiveGiB, true},
		} {
			t.Run(name, func(t *testing.T) {
				fx.accountLimits.Store(&tc.account)

				// when
				got := fx.isPlanLimit(tc.limit)

				// then
				assert.Equal(t, tc.want, got)
			})
		}
	})
	t.Run("the tech space's usage is not started by a read", func(t *testing.T) {
		// given
		fx := newFixture(t, 1024)
		t.Cleanup(func() { fx.Finish(t) })

		// when
		_, ok, err := fx.SpaceLimits(techSpaceId)

		// then
		require.NoError(t, err)
		assert.False(t, ok)
		fx.limitManager.techSpaceLock.Lock()
		defer fx.limitManager.techSpaceLock.Unlock()
		assert.Nil(t, fx.limitManager.techSpaceUsage)
	})
	t.Run("a space without a space view", func(t *testing.T) {
		// given
		fx := newFixture(t, 1024)
		t.Cleanup(func() { fx.Finish(t) })

		// when
		_, ok, err := fx.SpaceLimits("unknownSpace")

		// then
		require.ErrorContains(t, err, "spaceView not found")
		assert.False(t, ok)
	})
	t.Run("local-only mode reports nothing, even with a report at hand", func(t *testing.T) {
		// given: a report the node already gave
		fx := newFixture(t, 1024)
		t.Cleanup(func() { fx.Finish(t) })
		require.Eventually(t, func() bool {
			_, ok, _ := fx.SpaceLimits("space1")
			return ok
		}, 5*time.Second, 10*time.Millisecond)
		fx.cfg.NetworkMode = pb.RpcAccount_LocalOnly

		// when
		_, ok, err := fx.SpaceLimits("space1")

		// then
		require.NoError(t, err)
		assert.False(t, ok)
	})
}

func TestSpaceUsageReportedDoesNotWaitForTheNode(t *testing.T) {
	// given: a space usage the node has answered for once, then a refresh
	// that hangs on the node while holding the lock
	rpcStore := mock_rpcstore.NewMockRpcStore(t)
	rpcStore.EXPECT().SpaceInfo(mock.Anything, "space1").Return(&fileproto.SpaceInfoResponse{
		LimitBytes: 1000, TotalUsageBytes: 600,
	}, nil).Once()
	release := make(chan struct{})
	rpcStore.EXPECT().SpaceInfo(mock.Anything, "space1").RunAndReturn(func(context.Context, string) (*fileproto.SpaceInfoResponse, error) {
		<-release
		return &fileproto.SpaceInfoResponse{LimitBytes: 1000, TotalUsageBytes: 700}, nil
	}).Once()
	usage := &spaceUsage{
		ctx: context.Background(), spaceId: "space1", rpcStore: rpcStore,
		updateCh: make(chan updateMessage, 1), files: map[string]allocatedFile{},
	}
	require.NoError(t, usage.Update(context.Background()))
	usage.lock.Lock()
	usage.updatedAt = time.Time{} // stale: the next update asks the node
	usage.lock.Unlock()
	refreshed := make(chan struct{})
	go func() {
		defer close(refreshed)
		assert.NoError(t, usage.Update(context.Background()))
	}()
	require.Eventually(t, func() bool {
		if !usage.lock.TryLock() {
			return true // the refresh holds the lock, waiting on the node
		}
		usage.lock.Unlock()
		return false
	}, 5*time.Second, time.Millisecond)

	// when
	answered := make(chan SpaceLimits, 1)
	go func() {
		got, _ := usage.reported()
		answered <- got
	}()

	// then: the last report is served at once
	select {
	case got := <-answered:
		assert.Equal(t, int64(600), got.NodeUsageBytes)
	case <-time.After(time.Second):
		t.Fatal("reported() waited for the node")
	}
	close(release)
	<-refreshed
	got, _ := usage.reported()
	assert.Equal(t, int64(700), got.NodeUsageBytes)
}
