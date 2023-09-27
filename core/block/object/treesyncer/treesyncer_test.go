package treesyncer

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree"
	"github.com/anyproto/any-sync/commonspace/object/tree/objecttree/mock_objecttree"
	"github.com/anyproto/any-sync/commonspace/object/tree/synctree/mock_synctree"
	"github.com/anyproto/any-sync/commonspace/object/treemanager/mock_treemanager"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/block/object/objectcache"
)

func TestTreeSyncer(t *testing.T) {
	ctrl := gomock.NewController(t)
	managerMock := mock_treemanager.NewMockTreeManager(ctrl)
	spaceId := "spaceId"
	peerId := "peerId"
	existingId := "existing"
	missingId := "missing"
	missingMock := mock_objecttree.NewMockObjectTree(ctrl)
	existingMock := mock_synctree.NewMockSyncTree(ctrl)

	t.Run("delayed sync", func(t *testing.T) {
		syncer := NewTreeSyncer(spaceId)
		syncer.Init()
		managerMock.EXPECT().GetTree(gomock.Any(), spaceId, existingId).Return(existingMock, nil)
		existingMock.EXPECT().SyncWithPeer(gomock.Any(), peerId).Return(nil)
		managerMock.EXPECT().GetTree(gomock.Any(), spaceId, missingId).Return(missingMock, nil)
		err := syncer.SyncAll(context.Background(), peerId, []string{existingId}, []string{missingId})
		require.NoError(t, err)
		require.NotNil(t, syncer.requestPools[peerId])
		require.NotNil(t, syncer.headPools[peerId])

		syncer.Run()
		time.Sleep(100 * time.Millisecond)
		syncer.Close()
	})

	t.Run("sync after run", func(t *testing.T) {
		syncer := NewTreeSyncer(spaceId)
		managerMock.EXPECT().GetTree(gomock.Any(), spaceId, existingId).Return(existingMock, nil)
		existingMock.EXPECT().SyncWithPeer(gomock.Any(), peerId).Return(nil)
		managerMock.EXPECT().GetTree(gomock.Any(), spaceId, missingId).Return(missingMock, nil)
		syncer.Init()
		syncer.Run()
		err := syncer.SyncAll(context.Background(), peerId, []string{existingId}, []string{missingId})
		require.NoError(t, err)
		require.NotNil(t, syncer.requestPools[peerId])
		require.NotNil(t, syncer.headPools[peerId])

		time.Sleep(100 * time.Millisecond)
		syncer.Close()
	})

	t.Run("sync same ids", func(t *testing.T) {
		syncer := NewTreeSyncer(spaceId)
		managerMock.EXPECT().GetTree(gomock.Any(), spaceId, existingId).Return(existingMock, nil)
		existingMock.EXPECT().SyncWithPeer(gomock.Any(), peerId).Return(nil)
		managerMock.EXPECT().GetTree(gomock.Any(), spaceId, missingId).Return(missingMock, nil)
		syncer.Init()
		syncer.Run()
		err := syncer.SyncAll(context.Background(), peerId, []string{existingId, existingId}, []string{missingId, missingId, missingId})
		require.NoError(t, err)
		require.NotNil(t, syncer.requestPools[peerId])
		require.NotNil(t, syncer.headPools[peerId])

		time.Sleep(100 * time.Millisecond)
		syncer.Close()
	})

	t.Run("sync concurrent ids", func(t *testing.T) {
		ch := make(chan struct{}, 2)
		syncer := NewTreeSyncer(spaceId, objectcache.ObjectLoadTimeout, 2, managerMock)
		managerMock.EXPECT().GetTree(gomock.Any(), spaceId, existingId).Return(existingMock, nil)
		existingMock.EXPECT().SyncWithPeer(gomock.Any(), peerId).Return(nil)
		managerMock.EXPECT().GetTree(gomock.Any(), spaceId, missingId+"1").DoAndReturn(func(ctx context.Context, spaceId, treeId string) (objecttree.ObjectTree, error) {
			<-ch
			return missingMock, nil
		})
		managerMock.EXPECT().GetTree(gomock.Any(), spaceId, missingId+"2").DoAndReturn(func(ctx context.Context, spaceId, treeId string) (objecttree.ObjectTree, error) {
			<-ch
			return missingMock, nil
		})
		syncer.Init()
		syncer.Run()
		err := syncer.SyncAll(context.Background(), peerId, []string{existingId}, []string{missingId + "1", missingId + "2"})
		require.NoError(t, err)
		require.NotNil(t, syncer.requestPools[peerId])
		require.NotNil(t, syncer.headPools[peerId])
		time.Sleep(100 * time.Millisecond)
		syncer.Close()
		for i := 0; i < 2; i++ {
			ch <- struct{}{}
		}
	})

	t.Run("sync context cancel", func(t *testing.T) {
		var events []string
		syncer := NewTreeSyncer(spaceId, objectcache.ObjectLoadTimeout, 1, managerMock)
		mutex := sync.Mutex{}
		managerMock.EXPECT().GetTree(gomock.Any(), spaceId, missingId).DoAndReturn(func(ctx context.Context, spaceId, treeId string) (objecttree.ObjectTree, error) {
			<-ctx.Done()
			mutex.Lock()
			events = append(events, "after done")
			mutex.Unlock()
			return missingMock, nil
		})
		syncer.Init()
		syncer.Run()
		err := syncer.SyncAll(context.Background(), peerId, nil, []string{missingId})
		require.NoError(t, err)
		require.NotNil(t, syncer.requestPools[peerId])
		require.NotNil(t, syncer.headPools[peerId])
		time.Sleep(100 * time.Millisecond)
		mutex.Lock()
		events = append(events, "before close")
		mutex.Unlock()
		syncer.Close()
		time.Sleep(100 * time.Millisecond)
		mutex.Lock()
		require.Equal(t, []string{"before close", "after done"}, events)
		mutex.Unlock()
	})
}
