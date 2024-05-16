package treesyncer

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree/mock_objecttree"
	"github.com/anyproto/any-sync/commonspace/object/tree/synctree/mock_synctree"
	"github.com/anyproto/any-sync/commonspace/object/treemanager/mock_treemanager"
	"github.com/anyproto/any-sync/commonspace/peermanager/mock_peermanager"
	"github.com/anyproto/any-sync/nodeconf/mock_nodeconf"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/tests/testutil"
)

type fixture struct {
	*treeSyncer

	missingMock  *mock_objecttree.MockObjectTree
	existingMock *mock_synctree.MockSyncTree
	treeManager  *mock_treemanager.MockTreeManager
	updater      *mock_treesyncer.MockUpdater
	manager      *mock_peermanager.MockPeerManager
	nodeConf     *mock_nodeconf.MockService
}

func newFixture(t *testing.T, spaceId string) *fixture {
	ctrl := gomock.NewController(t)
	treeManager := mock_treemanager.NewMockTreeManager(ctrl)
	missingMock := mock_objecttree.NewMockObjectTree(ctrl)
	existingMock := mock_synctree.NewMockSyncTree(ctrl)
	updater := mock_treesyncer.NewMockUpdater(t)
	updater.EXPECT().Name().Return("updater").Maybe()
	manager := mock_peermanager.NewMockPeerManager(ctrl)
	manager.EXPECT().Name().Return("manager").AnyTimes()
	nodeConf := mock_nodeconf.NewMockService(ctrl)
	nodeConf.EXPECT().Name().Return("nodeConf").AnyTimes()

	a := new(app.App)
	a.Register(testutil.PrepareMock(context.Background(), a, treeManager)).
		Register(testutil.PrepareMock(context.Background(), a, updater)).
		Register(testutil.PrepareMock(context.Background(), a, manager)).
		Register(testutil.PrepareMock(context.Background(), a, nodeConf))
	syncer := NewTreeSyncer(spaceId)
	err := syncer.Init(a)
	require.NoError(t, err)

	return &fixture{
		treeSyncer:   syncer.(*treeSyncer),
		missingMock:  missingMock,
		existingMock: existingMock,
		treeManager:  treeManager,
		updater:      updater,
		manager:      manager,
		nodeConf:     nodeConf,
	}
}

func TestTreeSyncer(t *testing.T) {

	spaceId := "spaceId"
	peerId := "peerId"
	existingId := "existing"
	missingId := "missing"

	t.Run("delayed sync", func(t *testing.T) {
		ctx := context.Background()
		fx := newFixture(t, spaceId)
		fx.treeManager.EXPECT().GetTree(gomock.Any(), spaceId, existingId).Return(fx.existingMock, nil)
		fx.existingMock.EXPECT().SyncWithPeer(gomock.Any(), peerId).Return(nil)
		fx.treeManager.EXPECT().GetTree(gomock.Any(), spaceId, missingId).Return(fx.missingMock, nil)
		fx.nodeConf.EXPECT().NodeIds(spaceId).Return([]string{})
		err := fx.SyncAll(context.Background(), peerId, []string{existingId}, []string{missingId})
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
		fx.existingMock.EXPECT().SyncWithPeer(gomock.Any(), peerId).Return(nil)
		fx.treeManager.EXPECT().GetTree(gomock.Any(), spaceId, missingId).Return(fx.missingMock, nil)
		fx.nodeConf.EXPECT().NodeIds(spaceId).Return([]string{})

		fx.StartSync()
		err := fx.SyncAll(context.Background(), peerId, []string{existingId}, []string{missingId})
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
		fx.existingMock.EXPECT().SyncWithPeer(gomock.Any(), peerId).Return(nil)
		fx.treeManager.EXPECT().GetTree(gomock.Any(), spaceId, missingId).Return(fx.missingMock, nil)
		fx.nodeConf.EXPECT().NodeIds(spaceId).Return([]string{})

		fx.StartSync()
		err := fx.SyncAll(context.Background(), peerId, []string{existingId, existingId}, []string{missingId, missingId, missingId})
		require.NoError(t, err)
		require.NotNil(t, fx.requestPools[peerId])
		require.NotNil(t, fx.headPools[peerId])

		time.Sleep(100 * time.Millisecond)
		fx.Close(ctx)
	})

	t.Run("sync concurrent ids", func(t *testing.T) {
		ctx := context.Background()
		ch := make(chan struct{}, 2)
		fx := newFixture(t, spaceId)
		fx.treeManager.EXPECT().GetTree(gomock.Any(), spaceId, existingId).Return(fx.existingMock, nil)
		fx.existingMock.EXPECT().SyncWithPeer(gomock.Any(), peerId).Return(nil)
		fx.treeManager.EXPECT().GetTree(gomock.Any(), spaceId, missingId+"1").DoAndReturn(func(ctx context.Context, spaceId, treeId string) (objecttree.ObjectTree, error) {
			<-ch
			return fx.missingMock, nil
		})
		fx.treeManager.EXPECT().GetTree(gomock.Any(), spaceId, missingId+"2").DoAndReturn(func(ctx context.Context, spaceId, treeId string) (objecttree.ObjectTree, error) {
			<-ch
			return fx.missingMock, nil
		})
		fx.nodeConf.EXPECT().NodeIds(spaceId).Return([]string{})

		fx.StartSync()
		err := fx.SyncAll(context.Background(), peerId, []string{existingId}, []string{missingId + "1", missingId + "2"})
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
		fx.nodeConf.EXPECT().NodeIds(spaceId).Return([]string{})

		fx.StartSync()
		err := fx.SyncAll(context.Background(), peerId, nil, []string{missingId})
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
	t.Run("send offline event", func(t *testing.T) {
		ctx := context.Background()
		fx := newFixture(t, spaceId)
		fx.treeManager.EXPECT().GetTree(gomock.Any(), spaceId, existingId).Return(fx.existingMock, nil)
		fx.existingMock.EXPECT().SyncWithPeer(gomock.Any(), peerId).Return(nil)
		fx.treeManager.EXPECT().GetTree(gomock.Any(), spaceId, missingId).Return(fx.missingMock, nil)
		fx.nodeConf.EXPECT().NodeIds(spaceId).Return([]string{peerId})
		fx.manager.EXPECT().IsPeerOffline(peerId).Return(true).AnyTimes()
		fx.updater.EXPECT().SendUpdate(helpers.MakeSyncStatus(spaceId, helpers.Offline, 0, helpers.Null, helpers.Objects))

		fx.StartSync()
		err := fx.SyncAll(context.Background(), peerId, []string{existingId}, []string{missingId})
		require.NoError(t, err)
		require.NotNil(t, fx.requestPools[peerId])
		require.NotNil(t, fx.headPools[peerId])

		time.Sleep(100 * time.Millisecond)
		fx.Close(ctx)
	})
	t.Run("send syncing and synced event", func(t *testing.T) {
		ctx := context.Background()
		fx := newFixture(t, spaceId)
		fx.treeManager.EXPECT().GetTree(gomock.Any(), spaceId, existingId).Return(fx.existingMock, nil)
		fx.existingMock.EXPECT().SyncWithPeer(gomock.Any(), peerId).Return(nil)
		fx.treeManager.EXPECT().GetTree(gomock.Any(), spaceId, missingId).Return(fx.missingMock, nil)
		fx.nodeConf.EXPECT().NodeIds(spaceId).Return([]string{peerId})
		fx.manager.EXPECT().IsPeerOffline(peerId).Return(false).AnyTimes()
		fx.updater.EXPECT().SendUpdate(helpers.MakeSyncStatus(spaceId, helpers.Syncing, 2, helpers.Null, helpers.Objects))
		fx.updater.EXPECT().SendUpdate(helpers.MakeSyncStatus(spaceId, helpers.Synced, 0, helpers.Null, helpers.Objects))

		fx.StartSync()
		err := fx.SyncAll(context.Background(), peerId, []string{existingId}, []string{missingId})
		require.NoError(t, err)
		require.NotNil(t, fx.requestPools[peerId])
		require.NotNil(t, fx.headPools[peerId])

		time.Sleep(100 * time.Millisecond)
		fx.Close(ctx)
	})
}
