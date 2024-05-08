package spacesyncstatus

import (
	"context"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/syncstatus"
	"github.com/cheggaaa/mb/v3"
	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/core/syncstatus/filesyncstatus"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/tests/testutil"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestSpaceSyncStatus_Init(t *testing.T) {
	t.Run("init", func(t *testing.T) {
		// given
		status := NewSpaceSyncStatus()
		ctx := context.Background()

		a := new(app.App)
		eventSender := mock_event.NewMockSender(t)
		a.Register(testutil.PrepareMock(ctx, a, eventSender))
		a.Register(objectstore.NewStoreFixture(t))
		a.Register(&config.Config{NetworkMode: pb.RpcAccount_LocalOnly})

		// when
		err := status.Init(a)

		// then
		assert.Nil(t, err)
		err = status.Run(ctx)
		assert.Nil(t, err)
		err = status.Close(ctx)
		assert.Nil(t, err)
	})
}

func TestSpaceSyncStatus_updateSpaceSyncStatus(t *testing.T) {
	t.Run("local only mode", func(t *testing.T) {
		// given
		eventSender := mock_event.NewMockSender(t)
		status := spaceSyncStatus{
			eventSender:   eventSender,
			networkConfig: &config.Config{NetworkMode: pb.RpcAccount_LocalOnly},
			batcher:       mb.New[*syncstatus.SpaceSync](0),
			filesState:    NewFileState(objectstore.NewStoreFixture(t)),
			objectsState:  NewObjectState(),
		}
		syncStatus := syncstatus.MakeSyncStatus("spaceId", syncstatus.Synced, 0, syncstatus.Null, syncstatus.Files)

		// then
		status.updateSpaceSyncStatus(syncStatus)

		// when
		eventSender.AssertNotCalled(t, "Broadcast")
	})
	t.Run("don't send not needed synced event", func(t *testing.T) {
		// given
		eventSender := mock_event.NewMockSender(t)
		status := spaceSyncStatus{
			eventSender:   eventSender,
			networkConfig: &config.Config{NetworkMode: pb.RpcAccount_DefaultConfig},
			batcher:       mb.New[*syncstatus.SpaceSync](0),
			filesState:    NewFileState(objectstore.NewStoreFixture(t)),
			objectsState:  NewObjectState(),
		}
		syncStatus := syncstatus.MakeSyncStatus("spaceId", syncstatus.Synced, 0, syncstatus.Null, syncstatus.Files)

		// then
		status.updateSpaceSyncStatus(syncStatus)

		// when
		eventSender.AssertNotCalled(t, "Broadcast")
	})
	t.Run("syncing event for objects", func(t *testing.T) {
		// given
		eventSender := mock_event.NewMockSender(t)
		eventSender.EXPECT().Broadcast(&pb.Event{
			Messages: []*pb.EventMessage{{
				Value: &pb.EventMessageValueOfSpaceSyncStatusUpdate{
					SpaceSyncStatusUpdate: &pb.EventSpaceSyncStatusUpdate{
						Status:                pb.EventSpace_Syncing,
						Network:               pb.EventSpace_Anytype,
						Error:                 pb.EventSpace_Null,
						SyncingObjectsCounter: 2,
					},
				},
			}},
		})
		status := spaceSyncStatus{
			eventSender:   eventSender,
			networkConfig: &config.Config{NetworkMode: pb.RpcAccount_DefaultConfig},
			batcher:       mb.New[*syncstatus.SpaceSync](0),
			filesState:    NewFileState(objectstore.NewStoreFixture(t)),
			objectsState:  NewObjectState(),
		}
		syncStatus := syncstatus.MakeSyncStatus("spaceId", syncstatus.Syncing, 2, syncstatus.Null, syncstatus.Objects)

		// then
		status.updateSpaceSyncStatus(syncStatus)

		// when
		assert.Equal(t, syncstatus.Syncing, status.objectsState.GetSyncStatus("spaceId"))
		assert.Equal(t, 2, status.objectsState.GetSyncObjectCount("spaceId"))
		assert.Equal(t, syncstatus.Syncing, status.getSpaceSyncStatus(syncStatus))
	})
	t.Run("syncing event for files", func(t *testing.T) {
		// given
		eventSender := mock_event.NewMockSender(t)
		eventSender.EXPECT().Broadcast(&pb.Event{
			Messages: []*pb.EventMessage{{
				Value: &pb.EventMessageValueOfSpaceSyncStatusUpdate{
					SpaceSyncStatusUpdate: &pb.EventSpaceSyncStatusUpdate{
						Status:                pb.EventSpace_Syncing,
						Network:               pb.EventSpace_Anytype,
						Error:                 pb.EventSpace_Null,
						SyncingObjectsCounter: 2,
					},
				},
			}},
		})
		storeFixture := objectstore.NewStoreFixture(t)
		storeFixture.AddObjects(t, []objectstore.TestObject{
			{
				bundle.RelationKeyId:               pbtypes.String("id1"),
				bundle.RelationKeyFileBackupStatus: pbtypes.Int64(int64(filesyncstatus.Syncing)),
				bundle.RelationKeySpaceId:          pbtypes.String("spaceId"),
			},
			{
				bundle.RelationKeyId:               pbtypes.String("id2"),
				bundle.RelationKeyFileBackupStatus: pbtypes.Int64(int64(filesyncstatus.Synced)),
				bundle.RelationKeySpaceId:          pbtypes.String("spaceId"),
			},
			{
				bundle.RelationKeyId:               pbtypes.String("id3"),
				bundle.RelationKeyFileBackupStatus: pbtypes.Int64(int64(filesyncstatus.Syncing)),
				bundle.RelationKeySpaceId:          pbtypes.String("spaceId"),
			},
		})

		status := spaceSyncStatus{
			eventSender:   eventSender,
			networkConfig: &config.Config{NetworkMode: pb.RpcAccount_DefaultConfig},
			batcher:       mb.New[*syncstatus.SpaceSync](0),
			filesState:    NewFileState(storeFixture),
			objectsState:  NewObjectState(),
		}
		syncStatus := syncstatus.MakeSyncStatus("spaceId", syncstatus.Syncing, 0, syncstatus.Null, syncstatus.Files)

		// then
		status.updateSpaceSyncStatus(syncStatus)

		// when
		assert.Equal(t, syncstatus.Syncing, status.filesState.GetSyncStatus("spaceId"))
		assert.Equal(t, 2, status.filesState.GetSyncObjectCount("spaceId"))
		assert.Equal(t, syncstatus.Syncing, status.getSpaceSyncStatus(syncStatus))
	})
	t.Run("don't send not needed synced event if files or objects are still syncing", func(t *testing.T) {
		// given
		eventSender := mock_event.NewMockSender(t)
		status := spaceSyncStatus{
			eventSender:   eventSender,
			networkConfig: &config.Config{NetworkMode: pb.RpcAccount_DefaultConfig},
			batcher:       mb.New[*syncstatus.SpaceSync](0),
			filesState:    NewFileState(objectstore.NewStoreFixture(t)),
			objectsState:  NewObjectState(),
		}
		objectsSyncStatus := syncstatus.MakeSyncStatus("spaceId", syncstatus.Syncing, 2, syncstatus.Null, syncstatus.Objects)
		status.objectsState.SetSyncStatus(objectsSyncStatus)

		// then
		syncStatus := syncstatus.MakeSyncStatus("spaceId", syncstatus.Synced, 0, syncstatus.Null, syncstatus.Files)
		status.updateSpaceSyncStatus(syncStatus)

		// when
		eventSender.AssertNotCalled(t, "Broadcast")
	})
	t.Run("send error event", func(t *testing.T) {
		// given
		eventSender := mock_event.NewMockSender(t)
		eventSender.EXPECT().Broadcast(&pb.Event{
			Messages: []*pb.EventMessage{{
				Value: &pb.EventMessageValueOfSpaceSyncStatusUpdate{
					SpaceSyncStatusUpdate: &pb.EventSpaceSyncStatusUpdate{
						Status:                pb.EventSpace_Error,
						Network:               pb.EventSpace_Anytype,
						Error:                 pb.EventSpace_Null,
						SyncingObjectsCounter: 0,
					},
				},
			}},
		})
		status := spaceSyncStatus{
			eventSender:   eventSender,
			networkConfig: &config.Config{NetworkMode: pb.RpcAccount_DefaultConfig},
			batcher:       mb.New[*syncstatus.SpaceSync](0),
			filesState:    NewFileState(objectstore.NewStoreFixture(t)),
			objectsState:  NewObjectState(),
		}
		syncStatus := syncstatus.MakeSyncStatus("spaceId", syncstatus.Error, 0, syncstatus.Null, syncstatus.Objects)

		// then
		status.updateSpaceSyncStatus(syncStatus)

		// when
		assert.Equal(t, syncstatus.Error, status.objectsState.GetSyncStatus("spaceId"))
		assert.Equal(t, 0, status.objectsState.GetSyncObjectCount("spaceId"))
		assert.Equal(t, syncstatus.Error, status.getSpaceSyncStatus(syncStatus))
	})
	t.Run("send offline event", func(t *testing.T) {
		// given
		eventSender := mock_event.NewMockSender(t)
		eventSender.EXPECT().Broadcast(&pb.Event{
			Messages: []*pb.EventMessage{{
				Value: &pb.EventMessageValueOfSpaceSyncStatusUpdate{
					SpaceSyncStatusUpdate: &pb.EventSpaceSyncStatusUpdate{
						Status:                pb.EventSpace_Offline,
						Network:               pb.EventSpace_SelfHost,
						Error:                 pb.EventSpace_Null,
						SyncingObjectsCounter: 0,
					},
				},
			}},
		})
		status := spaceSyncStatus{
			eventSender:   eventSender,
			networkConfig: &config.Config{NetworkMode: pb.RpcAccount_CustomConfig},
			batcher:       mb.New[*syncstatus.SpaceSync](0),
			filesState:    NewFileState(objectstore.NewStoreFixture(t)),
			objectsState:  NewObjectState(),
		}
		syncStatus := syncstatus.MakeSyncStatus("spaceId", syncstatus.Offline, 0, syncstatus.Null, syncstatus.Objects)

		// then
		status.updateSpaceSyncStatus(syncStatus)

		// when
		assert.Equal(t, syncstatus.Offline, status.objectsState.GetSyncStatus("spaceId"))
		assert.Equal(t, 0, status.objectsState.GetSyncObjectCount("spaceId"))
		assert.Equal(t, syncstatus.Offline, status.getSpaceSyncStatus(syncStatus))
	})
	t.Run("send synced event", func(t *testing.T) {
		// given
		eventSender := mock_event.NewMockSender(t)
		eventSender.EXPECT().Broadcast(&pb.Event{
			Messages: []*pb.EventMessage{{
				Value: &pb.EventMessageValueOfSpaceSyncStatusUpdate{
					SpaceSyncStatusUpdate: &pb.EventSpaceSyncStatusUpdate{
						Status:                pb.EventSpace_Synced,
						Network:               pb.EventSpace_SelfHost,
						Error:                 pb.EventSpace_Null,
						SyncingObjectsCounter: 0,
					},
				},
			}},
		})
		status := spaceSyncStatus{
			eventSender:   eventSender,
			networkConfig: &config.Config{NetworkMode: pb.RpcAccount_CustomConfig},
			batcher:       mb.New[*syncstatus.SpaceSync](0),
			filesState:    NewFileState(objectstore.NewStoreFixture(t)),
			objectsState:  NewObjectState(),
		}
		syncStatus := syncstatus.MakeSyncStatus("spaceId", syncstatus.Syncing, 2, syncstatus.Null, syncstatus.Objects)
		status.objectsState.SetObjectsNumber(syncStatus)
		status.objectsState.SetSyncStatus(syncStatus)

		// then
		syncStatus = syncstatus.MakeSyncStatus("spaceId", syncstatus.Synced, 0, syncstatus.Null, syncstatus.Objects)
		status.updateSpaceSyncStatus(syncStatus)

		// when
		assert.Equal(t, syncstatus.Synced, status.objectsState.GetSyncStatus("spaceId"))
		assert.Equal(t, 0, status.objectsState.GetSyncObjectCount("spaceId"))
		assert.Equal(t, syncstatus.Synced, status.filesState.GetSyncStatus("spaceId"))
		assert.Equal(t, 0, status.filesState.GetSyncObjectCount("spaceId"))
		assert.Equal(t, syncstatus.Synced, status.getSpaceSyncStatus(syncStatus))
	})
}

func TestSpaceSyncStatus_SendUpdate(t *testing.T) {
	t.Run("SendUpdate success", func(t *testing.T) {
		// given
		eventSender := mock_event.NewMockSender(t)
		spaceStatus := spaceSyncStatus{
			eventSender:   eventSender,
			networkConfig: &config.Config{NetworkMode: pb.RpcAccount_DefaultConfig},
			batcher:       mb.New[*syncstatus.SpaceSync](0),
			filesState:    NewFileState(objectstore.NewStoreFixture(t)),
			objectsState:  NewObjectState(),
		}
		syncStatus := syncstatus.MakeSyncStatus("spaceId", syncstatus.Synced, 0, syncstatus.Null, syncstatus.Files)

		// then
		spaceStatus.SendUpdate(syncStatus)

		// when
		status, err := spaceStatus.batcher.WaitOne(context.Background())
		assert.Nil(t, err)
		assert.Equal(t, status, syncStatus)
	})
}
