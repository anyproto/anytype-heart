package treesyncer

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/headsync/statestorage/mock_statestorage"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/object/tree/synctree/mock_synctree"
	"github.com/anyproto/any-sync/commonspace/object/treemanager/mock_treemanager"
	"github.com/anyproto/any-sync/commonspace/peermanager/mock_peermanager"
	"github.com/anyproto/any-sync/commonspace/spacestorage/mock_spacestorage"
	"github.com/anyproto/any-sync/net/peer"
	"github.com/anyproto/any-sync/net/rpc/rpctest"
	"github.com/anyproto/any-sync/nodeconf/mock_nodeconf"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/block/object/treesyncer/mock_treesyncer"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

type fixture struct {
	*treeSyncer

	peerManagerMock    *mock_peermanager.MockPeerManager
	missingMock        *mock_synctree.MockSyncTree
	existingMock       *mock_synctree.MockSyncTree
	treeManager        *mock_treemanager.MockTreeManager
	nodeConf           *mock_nodeconf.MockService
	syncStatus         *mock_treesyncer.MockSyncedTreeRemover
	syncDetailsUpdater *mock_treesyncer.MockSyncDetailsUpdater
	stateStorage       *mock_statestorage.MockStateStorage
}

func newFixture(t *testing.T, spaceId string) *fixture {
	ctrl := gomock.NewController(t)
	treeManager := mock_treemanager.NewMockTreeManager(ctrl)
	peerManager := mock_peermanager.NewMockPeerManager(ctrl)
	missingMock := mock_synctree.NewMockSyncTree(ctrl)
	existingMock := mock_synctree.NewMockSyncTree(ctrl)
	nodeConf := mock_nodeconf.NewMockService(ctrl)
	nodeConf.EXPECT().Name().Return("nodeConf").AnyTimes()
	syncStatus := mock_treesyncer.NewMockSyncedTreeRemover(t)
	syncDetailsUpdater := mock_treesyncer.NewMockSyncDetailsUpdater(t)
	spaceStorage := mock_spacestorage.NewMockSpaceStorage(ctrl)
	stateStorage := mock_statestorage.NewMockStateStorage(ctrl)
	spaceStorage.EXPECT().StateStorage().AnyTimes().Return(stateStorage)
	stateStorage.EXPECT().SettingsId().AnyTimes().Return("settingsId")

	missingMock.EXPECT().Lock().AnyTimes()
	missingMock.EXPECT().Unlock().AnyTimes()
	existingMock.EXPECT().Lock().AnyTimes()
	existingMock.EXPECT().Unlock().AnyTimes()

	a := new(app.App)
	a.Register(testutil.PrepareMock(context.Background(), a, treeManager)).
		Register(testutil.PrepareMock(context.Background(), a, spaceStorage)).
		Register(testutil.PrepareMock(context.Background(), a, syncStatus)).
		Register(testutil.PrepareMock(context.Background(), a, nodeConf)).
		Register(testutil.PrepareMock(context.Background(), a, peerManager)).
		Register(testutil.PrepareMock(context.Background(), a, syncDetailsUpdater))
	syncer := NewTreeSyncer(spaceId)
	err := syncer.Init(a)
	require.NoError(t, err)

	return &fixture{
		treeSyncer:         syncer.(*treeSyncer),
		missingMock:        missingMock,
		existingMock:       existingMock,
		treeManager:        treeManager,
		nodeConf:           nodeConf,
		syncStatus:         syncStatus,
		syncDetailsUpdater: syncDetailsUpdater,
		stateStorage:       stateStorage,
		peerManagerMock:    peerManager,
	}
}

func TestTreeSyncer(t *testing.T) {
	spaceId := "spaceId"
	peerId := "peerId"
	existingId := "existing"
	missingId := "missing"
	pr := rpctest.MockPeer{}

	t.Run("delayed sync", func(t *testing.T) {
		ctx := context.Background()
		fx := newFixture(t, spaceId)
		fx.treeManager.EXPECT().GetTree(gomock.Any(), spaceId, existingId).Return(fx.existingMock, nil)
		fx.existingMock.EXPECT().SyncWithPeer(gomock.Any(), pr).Return(nil)
		fx.treeManager.EXPECT().GetTree(gomock.Any(), spaceId, missingId).Return(nil, fmt.Errorf("not found"))
		fx.nodeConf.EXPECT().NodeIds(spaceId).Return([]string{})
		fx.syncStatus.EXPECT().RemoveAllExcept(peerId, []string{existingId}).Return()
		err := fx.SyncAll(context.Background(), pr, []string{existingId}, []string{missingId})
		require.NoError(t, err)
		require.NotNil(t, fx.requestPools[peerId])
		require.NotNil(t, fx.headPools[peerId])

		fx.StartSync()
		time.Sleep(100 * time.Millisecond)
		fx.Close(ctx)
	})

	t.Run("delayed sync empty derived", func(t *testing.T) {
		ctx := context.Background()
		fx := newFixture(t, spaceId)
		fx.treeManager.EXPECT().GetTree(gomock.Any(), spaceId, existingId).Return(fx.existingMock, nil)
		fx.existingMock.EXPECT().SyncWithPeer(gomock.Any(), pr).Return(nil)
		fx.treeManager.EXPECT().GetTree(gomock.Any(), spaceId, missingId).Return(fx.missingMock, nil)
		fx.missingMock.EXPECT().IsDerived().Return(true)
		fx.missingMock.EXPECT().Len().Return(1)
		fx.missingMock.EXPECT().Root().Return(&objecttree.Change{Id: "id"})
		fx.missingMock.EXPECT().Id().Return("id")
		fx.missingMock.EXPECT().SyncWithPeer(gomock.Any(), pr).Return(nil)
		fx.nodeConf.EXPECT().NodeIds(spaceId).Return([]string{})
		fx.syncStatus.EXPECT().RemoveAllExcept(peerId, []string{existingId}).Return()
		err := fx.SyncAll(context.Background(), pr, []string{existingId}, []string{missingId})
		require.NoError(t, err)
		require.NotNil(t, fx.requestPools[peerId])
		require.NotNil(t, fx.headPools[peerId])

		fx.StartSync()
		time.Sleep(100 * time.Millisecond)
		fx.Close(ctx)
	})

	t.Run("delayed sync notify sync status", func(t *testing.T) {
		ctx := context.Background()
		fx := newFixture(t, spaceId)
		fx.treeManager.EXPECT().GetTree(gomock.Any(), spaceId, existingId).Return(fx.existingMock, nil)
		fx.existingMock.EXPECT().SyncWithPeer(gomock.Any(), pr).Return(nil)
		fx.treeManager.EXPECT().GetTree(gomock.Any(), spaceId, missingId).Return(fx.missingMock, nil)
		fx.nodeConf.EXPECT().NodeIds(spaceId).Return([]string{peerId})
		fx.missingMock.EXPECT().IsDerived().Return(false)
		fx.syncDetailsUpdater.EXPECT().UpdateSpaceDetails([]string{existingId}, []string{missingId}, spaceId)
		fx.syncStatus.EXPECT().RemoveAllExcept(peerId, []string{existingId}).Return()
		err := fx.SyncAll(context.Background(), pr, []string{existingId}, []string{missingId})
		require.NoError(t, err)
		require.NotNil(t, fx.requestPools[peerId])
		require.NotNil(t, fx.headPools[peerId])

		fx.StartSync()
		time.Sleep(100 * time.Millisecond)
		fx.Close(ctx)
	})

	t.Run("sync after run", func(t *testing.T) {
		ctx := context.Background()
		fx := newFixture(t, spaceId)
		fx.treeManager.EXPECT().GetTree(gomock.Any(), spaceId, existingId).Return(fx.existingMock, nil)
		fx.existingMock.EXPECT().SyncWithPeer(gomock.Any(), pr).Return(nil)
		fx.treeManager.EXPECT().GetTree(gomock.Any(), spaceId, missingId).Return(fx.missingMock, nil)
		fx.missingMock.EXPECT().IsDerived().Return(false)
		fx.nodeConf.EXPECT().NodeIds(spaceId).Return([]string{})
		fx.syncStatus.EXPECT().RemoveAllExcept(peerId, []string{existingId}).Return()

		fx.StartSync()
		err := fx.SyncAll(context.Background(), pr, []string{existingId}, []string{missingId})
		require.NoError(t, err)
		require.NotNil(t, fx.requestPools[peerId])
		require.NotNil(t, fx.headPools[peerId])

		time.Sleep(100 * time.Millisecond)
		fx.Close(ctx)
	})

	t.Run("sync same ids", func(t *testing.T) {
		ctx := context.Background()
		fx := newFixture(t, spaceId)
		fx.treeManager.EXPECT().GetTree(gomock.Any(), spaceId, existingId).Return(fx.existingMock, nil)
		fx.existingMock.EXPECT().SyncWithPeer(gomock.Any(), pr).Return(nil)
		fx.treeManager.EXPECT().GetTree(gomock.Any(), spaceId, missingId).Return(fx.missingMock, nil)
		fx.missingMock.EXPECT().IsDerived().Return(false)
		fx.nodeConf.EXPECT().NodeIds(spaceId).Return([]string{})
		fx.syncStatus.EXPECT().RemoveAllExcept(peerId, []string{existingId, existingId}).Return()

		fx.StartSync()
		err := fx.SyncAll(context.Background(), pr, []string{existingId, existingId}, []string{missingId, missingId, missingId})
		require.NoError(t, err)
		require.NotNil(t, fx.requestPools[peerId])
		require.NotNil(t, fx.headPools[peerId])

		time.Sleep(100 * time.Millisecond)
		fx.Close(ctx)
	})

	t.Run("sync spaceSettingsId", func(t *testing.T) {
		ctx := context.Background()
		fx := newFixture(t, spaceId)
		fx.nodeConf.EXPECT().NodeIds(spaceId).Return(nil)
		fx.syncStatus.EXPECT().RemoveAllExcept(peerId, mock.Anything).RunAndReturn(func(s string, strings []string) {
			require.Empty(t, strings)
		})
		ch := make(chan struct{})
		fx.treeManager.EXPECT().GetTree(gomock.Any(), spaceId, "spaceSettingsId").Return(fx.existingMock, nil)
		fx.existingMock.EXPECT().SyncWithPeer(gomock.Any(), pr).DoAndReturn(func(ctx context.Context, peer peer.Peer) error {
			close(ch)
			return nil
		})

		fx.StartSync()
		err := fx.SyncAll(context.Background(), pr, []string{"spaceSettingsId"}, nil)
		require.NoError(t, err)
		<-ch
		fx.Close(ctx)
	})

	t.Run("sync concurrent ids", func(t *testing.T) {
		ctx := context.Background()
		ch := make(chan struct{}, 2)
		fx := newFixture(t, spaceId)
		fx.treeManager.EXPECT().GetTree(gomock.Any(), spaceId, existingId).Return(fx.existingMock, nil)
		fx.existingMock.EXPECT().SyncWithPeer(gomock.Any(), pr).Return(nil)
		fx.treeManager.EXPECT().GetTree(gomock.Any(), spaceId, missingId+"1").DoAndReturn(func(ctx context.Context, spaceId, treeId string) (objecttree.ObjectTree, error) {
			<-ch
			return fx.missingMock, nil
		})
		fx.treeManager.EXPECT().GetTree(gomock.Any(), spaceId, missingId+"2").DoAndReturn(func(ctx context.Context, spaceId, treeId string) (objecttree.ObjectTree, error) {
			<-ch
			return fx.missingMock, nil
		})
		fx.missingMock.EXPECT().IsDerived().AnyTimes().Return(false)
		fx.nodeConf.EXPECT().NodeIds(spaceId).Return([]string{})
		fx.syncStatus.EXPECT().RemoveAllExcept(peerId, []string{existingId}).Return()

		fx.StartSync()
		err := fx.SyncAll(context.Background(), pr, []string{existingId}, []string{missingId + "1", missingId + "2"})
		require.NoError(t, err)
		require.NotNil(t, fx.requestPools[peerId])
		require.NotNil(t, fx.headPools[peerId])
		time.Sleep(100 * time.Millisecond)
		fx.Close(ctx)
		for i := 0; i < 2; i++ {
			ch <- struct{}{}
		}
	})

	t.Run("sync context cancel", func(t *testing.T) {
		ctx := context.Background()
		var events []string
		fx := newFixture(t, spaceId)
		mutex := sync.Mutex{}
		fx.treeManager.EXPECT().GetTree(gomock.Any(), spaceId, missingId).DoAndReturn(func(ctx context.Context, spaceId, treeId string) (objecttree.ObjectTree, error) {
			<-ctx.Done()
			mutex.Lock()
			events = append(events, "after done")
			mutex.Unlock()
			return fx.missingMock, nil
		})
		fx.missingMock.EXPECT().IsDerived().Return(false)
		fx.nodeConf.EXPECT().NodeIds(spaceId).Return(nil)
		var existing []string
		fx.syncStatus.EXPECT().RemoveAllExcept(peerId, mock.Anything).RunAndReturn(func(s string, strings []string) {
			require.Empty(t, strings)
		})

		fx.StartSync()
		err := fx.SyncAll(context.Background(), pr, existing, []string{missingId})
		require.NoError(t, err)
		require.NotNil(t, fx.requestPools[peerId])
		require.NotNil(t, fx.headPools[peerId])
		time.Sleep(100 * time.Millisecond)
		mutex.Lock()
		events = append(events, "before close")
		mutex.Unlock()
		fx.Close(ctx)
		time.Sleep(100 * time.Millisecond)
		mutex.Lock()
		require.Equal(t, []string{"before close", "after done"}, events)
		mutex.Unlock()
	})

	t.Run("refresh tree", func(t *testing.T) {
		pr := rpctest.MockPeer{}
		ch := make(chan struct{})
		fx := newFixture(t, spaceId)
		fx.peerManagerMock.EXPECT().GetResponsiblePeers(gomock.Any()).Return([]peer.Peer{pr}, nil)
		fx.treeManager.EXPECT().GetTree(gomock.Any(), spaceId, existingId).Return(fx.existingMock, nil)
		fx.existingMock.EXPECT().SyncWithPeer(gomock.Any(), pr).DoAndReturn(func(ctx context.Context, peer peer.Peer) error {
			close(ch)
			return nil
		})
		fx.StartSync()
		require.NoError(t, fx.RefreshTrees([]string{existingId}))
		<-ch
	})
}

func TestBuildSemaphore(t *testing.T) {
	spaceId := "spaceId"
	peerId := "peerId"
	pr := rpctest.MockPeer{}

	t.Run("limits concurrent requestTree calls", func(t *testing.T) {
		fx := newFixture(t, spaceId)
		// Track the number of concurrent GetTree calls
		var concurrent int64
		var maxConcurrent int64
		var mu sync.Mutex

		ids := []string{"a", "b", "c", "d", "e"}
		blocker := make(chan struct{})

		for _, id := range ids {
			fx.treeManager.EXPECT().GetTree(gomock.Any(), spaceId, id).DoAndReturn(
				func(ctx context.Context, spaceId, treeId string) (objecttree.ObjectTree, error) {
					mu.Lock()
					concurrent++
					if concurrent > maxConcurrent {
						maxConcurrent = concurrent
					}
					mu.Unlock()
					<-blocker
					mu.Lock()
					concurrent--
					mu.Unlock()
					return fx.missingMock, nil
				},
			)
		}
		fx.missingMock.EXPECT().IsDerived().AnyTimes().Return(false)
		fx.nodeConf.EXPECT().NodeIds(spaceId).Return([]string{})
		fx.syncStatus.EXPECT().RemoveAllExcept(peerId, mock.Anything).Return()

		fx.StartSync()
		err := fx.SyncAll(context.Background(), pr, nil, ids)
		require.NoError(t, err)

		// Wait for workers to hit the semaphore
		time.Sleep(200 * time.Millisecond)

		mu.Lock()
		mc := maxConcurrent
		mu.Unlock()
		// buildSem capacity is 3, so max concurrent should not exceed 3
		require.LessOrEqual(t, mc, int64(buildSemCapacity))

		close(blocker)
		time.Sleep(200 * time.Millisecond)
		fx.Close(context.Background())
	})

	t.Run("slow objects use separate semaphore", func(t *testing.T) {
		fx := newFixture(t, spaceId)
		slowId := "slow-tree"
		normalId := "normal-tree"

		// Pre-mark the slow object
		fx.slowObjects.Store(slowId, struct{}{})

		slowStarted := make(chan struct{})
		slowBlocker := make(chan struct{})
		normalDone := make(chan struct{})

		fx.treeManager.EXPECT().GetTree(gomock.Any(), spaceId, slowId).DoAndReturn(
			func(ctx context.Context, spaceId, treeId string) (objecttree.ObjectTree, error) {
				close(slowStarted)
				<-slowBlocker
				return fx.existingMock, nil
			},
		)
		fx.existingMock.EXPECT().SyncWithPeer(gomock.Any(), pr).AnyTimes().Return(nil)

		fx.treeManager.EXPECT().GetTree(gomock.Any(), spaceId, normalId).DoAndReturn(
			func(ctx context.Context, spaceId, treeId string) (objecttree.ObjectTree, error) {
				defer close(normalDone)
				return fx.missingMock, nil
			},
		)
		fx.missingMock.EXPECT().IsDerived().AnyTimes().Return(false)
		fx.nodeConf.EXPECT().NodeIds(spaceId).Return([]string{})
		fx.syncStatus.EXPECT().RemoveAllExcept(peerId, mock.Anything).Return()

		fx.StartSync()
		// Queue slow first, then normal
		err := fx.SyncAll(context.Background(), pr, []string{slowId}, []string{normalId})
		require.NoError(t, err)

		// Wait for slow to start
		<-slowStarted

		// Normal should still complete even though slow is blocked
		// (they use different semaphores)
		select {
		case <-normalDone:
			// ok
		case <-time.After(2 * time.Second):
			t.Fatal("normal tree was blocked by slow tree")
		}

		close(slowBlocker)
		time.Sleep(100 * time.Millisecond)
		fx.Close(context.Background())
	})

	t.Run("context cancellation unblocks acquire", func(t *testing.T) {
		fx := newFixture(t, spaceId)

		// Fill the build semaphore completely
		for i := int64(0); i < buildSemCapacity; i++ {
			require.NoError(t, fx.buildSem.Acquire(context.Background(), 1))
		}

		// Try to acquire with a short timeout — should fail
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		err := fx.acquireBuildSlot(ctx, "blocked-id")
		require.Error(t, err)

		// Release all
		for i := int64(0); i < buildSemCapacity; i++ {
			fx.buildSem.Release(1)
		}
	})

	t.Run("updateTree acquires build slot", func(t *testing.T) {
		fx := newFixture(t, spaceId)
		ch := make(chan struct{})

		fx.treeManager.EXPECT().GetTree(gomock.Any(), spaceId, "update-tree").DoAndReturn(
			func(ctx context.Context, spaceId, treeId string) (objecttree.ObjectTree, error) {
				close(ch)
				return fx.existingMock, nil
			},
		)
		fx.existingMock.EXPECT().SyncWithPeer(gomock.Any(), pr).Return(nil)
		fx.nodeConf.EXPECT().NodeIds(spaceId).Return([]string{})
		fx.syncStatus.EXPECT().RemoveAllExcept(peerId, []string{"update-tree"}).Return()

		fx.StartSync()
		err := fx.SyncAll(context.Background(), pr, []string{"update-tree"}, nil)
		require.NoError(t, err)
		<-ch
		time.Sleep(100 * time.Millisecond)
		fx.Close(context.Background())
	})
}
