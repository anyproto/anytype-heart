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
