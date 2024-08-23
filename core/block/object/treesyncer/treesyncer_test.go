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
	"github.com/anyproto/any-sync/net/rpc/rpctest"
	"github.com/anyproto/any-sync/nodeconf/mock_nodeconf"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/block/object/treesyncer/mock_treesyncer"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

type fixture struct {
	*treeSyncer

	missingMock        *mock_objecttree.MockObjectTree
	existingMock       *mock_synctree.MockSyncTree
	treeManager        *mock_treemanager.MockTreeManager
	nodeConf           *mock_nodeconf.MockService
	syncStatus         *mock_treesyncer.MockSyncedTreeRemover
	syncDetailsUpdater *mock_treesyncer.MockSyncDetailsUpdater
}

func newFixture(t *testing.T, spaceId string) *fixture {
	ctrl := gomock.NewController(t)
	treeManager := mock_treemanager.NewMockTreeManager(ctrl)
	missingMock := mock_objecttree.NewMockObjectTree(ctrl)
	existingMock := mock_synctree.NewMockSyncTree(ctrl)
	nodeConf := mock_nodeconf.NewMockService(ctrl)
	nodeConf.EXPECT().Name().Return("nodeConf").AnyTimes()
	syncStatus := mock_treesyncer.NewMockSyncedTreeRemover(t)
	syncDetailsUpdater := mock_treesyncer.NewMockSyncDetailsUpdater(t)

	a := new(app.App)
	a.Register(testutil.PrepareMock(context.Background(), a, treeManager)).
		Register(testutil.PrepareMock(context.Background(), a, syncStatus)).
		Register(testutil.PrepareMock(context.Background(), a, nodeConf)).
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
		fx.treeManager.EXPECT().GetTree(gomock.Any(), spaceId, missingId).Return(fx.missingMock, nil)
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
		fx.nodeConf.EXPECT().NodeIds(spaceId).Return([]string{})
		var existing []string
		fx.syncStatus.EXPECT().RemoveAllExcept(peerId, existing).Return()

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

}
