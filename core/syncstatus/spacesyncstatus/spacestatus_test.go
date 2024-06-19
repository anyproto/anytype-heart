package spacesyncstatus

import (
	"context"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/cheggaaa/mb/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/core/syncstatus/filesyncstatus"
	"github.com/anyproto/anytype-heart/core/syncstatus/spacesyncstatus/mock_spacesyncstatus"
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
		space := mock_spacesyncstatus.NewMockSpaceIdGetter(t)
		a.Register(testutil.PrepareMock(ctx, a, eventSender)).
			Register(objectstore.NewStoreFixture(t)).
			Register(&config.Config{NetworkMode: pb.RpcAccount_DefaultConfig}).
			Register(testutil.PrepareMock(ctx, a, space))

		// when
		err := status.Init(a)

		// then
		assert.Nil(t, err)

		space.EXPECT().PersonalSpaceId().Return("personalId")
		eventSender.EXPECT().Broadcast(&pb.Event{
			Messages: []*pb.EventMessage{{
				Value: &pb.EventMessageValueOfSpaceSyncStatusUpdate{
					SpaceSyncStatusUpdate: &pb.EventSpaceSyncStatusUpdate{
						Id:      "personalId",
						Status:  pb.EventSpace_Synced,
						Network: pb.EventSpace_Anytype,
					},
				},
			}},
		})
		err = status.Run(ctx)
		assert.Nil(t, err)
		err = status.Close(ctx)
		assert.Nil(t, err)
	})
	t.Run("local only mode", func(t *testing.T) {
		// given
		status := NewSpaceSyncStatus()
		ctx := context.Background()

		a := new(app.App)
		eventSender := mock_event.NewMockSender(t)
		eventSender.EXPECT().Broadcast(&pb.Event{
			Messages: []*pb.EventMessage{{
				Value: &pb.EventMessageValueOfSpaceSyncStatusUpdate{
					SpaceSyncStatusUpdate: &pb.EventSpaceSyncStatusUpdate{
						Status:  pb.EventSpace_Offline,
						Network: pb.EventSpace_LocalOnly,
					},
				},
			}},
		})
		space := mock_spacesyncstatus.NewMockSpaceIdGetter(t)

		a.Register(testutil.PrepareMock(ctx, a, eventSender)).
			Register(objectstore.NewStoreFixture(t)).
			Register(&config.Config{NetworkMode: pb.RpcAccount_LocalOnly}).
			Register(testutil.PrepareMock(ctx, a, space))

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
	t.Run("syncing event for objects", func(t *testing.T) {
		// given
		eventSender := mock_event.NewMockSender(t)
		eventSender.EXPECT().Broadcast(&pb.Event{
			Messages: []*pb.EventMessage{{
				Value: &pb.EventMessageValueOfSpaceSyncStatusUpdate{
					SpaceSyncStatusUpdate: &pb.EventSpaceSyncStatusUpdate{
						Id:                    "spaceId",
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
				bundle.RelationKeyId:         pbtypes.String("id1"),
				bundle.RelationKeySyncStatus: pbtypes.Int64(int64(domain.Syncing)),
				bundle.RelationKeySpaceId:    pbtypes.String("spaceId"),
			},
			{
				bundle.RelationKeyId:         pbtypes.String("id2"),
				bundle.RelationKeySyncStatus: pbtypes.Int64(int64(domain.Syncing)),
				bundle.RelationKeySpaceId:    pbtypes.String("spaceId"),
			},
		})
		status := spaceSyncStatus{
			eventSender:   eventSender,
			networkConfig: &config.Config{NetworkMode: pb.RpcAccount_DefaultConfig},
			batcher:       mb.New[*domain.SpaceSync](0),
			filesState:    NewFileState(storeFixture),
			objectsState:  NewObjectState(storeFixture),
		}
		syncStatus := domain.MakeSyncStatus("spaceId", domain.Syncing, domain.Null, domain.Objects)

		// then
		status.updateSpaceSyncStatus(syncStatus)

		// when
		assert.Equal(t, domain.Syncing, status.objectsState.GetSyncStatus("spaceId"))
		assert.Equal(t, 2, status.objectsState.GetSyncObjectCount("spaceId"))
		assert.Equal(t, domain.Syncing, status.getSpaceSyncStatus(syncStatus.SpaceId))
	})
	t.Run("syncing event for files", func(t *testing.T) {
		// given
		eventSender := mock_event.NewMockSender(t)
		eventSender.EXPECT().Broadcast(&pb.Event{
			Messages: []*pb.EventMessage{{
				Value: &pb.EventMessageValueOfSpaceSyncStatusUpdate{
					SpaceSyncStatusUpdate: &pb.EventSpaceSyncStatusUpdate{
						Id:                    "spaceId",
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
			batcher:       mb.New[*domain.SpaceSync](0),
			filesState:    NewFileState(storeFixture),
			objectsState:  NewObjectState(storeFixture),
		}
		syncStatus := domain.MakeSyncStatus("spaceId", domain.Syncing, domain.Null, domain.Files)

		// then
		status.updateSpaceSyncStatus(syncStatus)

		// when
		assert.Equal(t, domain.Syncing, status.filesState.GetSyncStatus("spaceId"))
		assert.Equal(t, 2, status.filesState.GetSyncObjectCount("spaceId"))
		assert.Equal(t, domain.Syncing, status.getSpaceSyncStatus(syncStatus.SpaceId))
	})
	t.Run("don't send not needed synced event if files or objects are still syncing", func(t *testing.T) {
		// given
		eventSender := mock_event.NewMockSender(t)
		status := spaceSyncStatus{
			eventSender:   eventSender,
			networkConfig: &config.Config{NetworkMode: pb.RpcAccount_DefaultConfig},
			batcher:       mb.New[*domain.SpaceSync](0),
			filesState:    NewFileState(objectstore.NewStoreFixture(t)),
			objectsState:  NewObjectState(objectstore.NewStoreFixture(t)),
		}
		objectsSyncStatus := domain.MakeSyncStatus("spaceId", domain.Syncing, domain.Null, domain.Objects)
		status.objectsState.SetSyncStatusAndErr(objectsSyncStatus)

		// then
		syncStatus := domain.MakeSyncStatus("spaceId", domain.Synced, domain.Null, domain.Files)
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
						Id:                    "spaceId",
						Status:                pb.EventSpace_Error,
						Network:               pb.EventSpace_Anytype,
						Error:                 pb.EventSpace_NetworkError,
						SyncingObjectsCounter: 0,
					},
				},
			}},
		})
		status := spaceSyncStatus{
			eventSender:   eventSender,
			networkConfig: &config.Config{NetworkMode: pb.RpcAccount_DefaultConfig},
			batcher:       mb.New[*domain.SpaceSync](0),
			filesState:    NewFileState(objectstore.NewStoreFixture(t)),
			objectsState:  NewObjectState(objectstore.NewStoreFixture(t)),
		}
		syncStatus := domain.MakeSyncStatus("spaceId", domain.Error, domain.NetworkError, domain.Objects)

		// then
		status.updateSpaceSyncStatus(syncStatus)

		// when
		assert.Equal(t, domain.Error, status.objectsState.GetSyncStatus("spaceId"))
		assert.Equal(t, 0, status.objectsState.GetSyncObjectCount("spaceId"))
		assert.Equal(t, domain.Error, status.getSpaceSyncStatus(syncStatus.SpaceId))
	})
	t.Run("send storage error event", func(t *testing.T) {
		// given
		eventSender := mock_event.NewMockSender(t)
		eventSender.EXPECT().Broadcast(&pb.Event{
			Messages: []*pb.EventMessage{{
				Value: &pb.EventMessageValueOfSpaceSyncStatusUpdate{
					SpaceSyncStatusUpdate: &pb.EventSpaceSyncStatusUpdate{
						Id:                    "spaceId",
						Status:                pb.EventSpace_Error,
						Network:               pb.EventSpace_Anytype,
						Error:                 pb.EventSpace_StorageLimitExceed,
						SyncingObjectsCounter: 0,
					},
				},
			}},
		})
		status := spaceSyncStatus{
			eventSender:   eventSender,
			networkConfig: &config.Config{NetworkMode: pb.RpcAccount_DefaultConfig},
			batcher:       mb.New[*domain.SpaceSync](0),
			filesState:    NewFileState(objectstore.NewStoreFixture(t)),
			objectsState:  NewObjectState(objectstore.NewStoreFixture(t)),
		}
		syncStatus := domain.MakeSyncStatus("spaceId", domain.Error, domain.StorageLimitExceed, domain.Files)

		// then
		status.updateSpaceSyncStatus(syncStatus)

		// when
		assert.Equal(t, domain.Error, status.filesState.GetSyncStatus("spaceId"))
		assert.Equal(t, 0, status.filesState.GetSyncObjectCount("spaceId"))
		assert.Equal(t, domain.Error, status.getSpaceSyncStatus(syncStatus.SpaceId))
	})
	t.Run("send incompatible error event", func(t *testing.T) {
		// given
		eventSender := mock_event.NewMockSender(t)
		eventSender.EXPECT().Broadcast(&pb.Event{
			Messages: []*pb.EventMessage{{
				Value: &pb.EventMessageValueOfSpaceSyncStatusUpdate{
					SpaceSyncStatusUpdate: &pb.EventSpaceSyncStatusUpdate{
						Id:                    "spaceId",
						Status:                pb.EventSpace_Error,
						Network:               pb.EventSpace_Anytype,
						Error:                 pb.EventSpace_IncompatibleVersion,
						SyncingObjectsCounter: 0,
					},
				},
			}},
		})
		status := spaceSyncStatus{
			eventSender:   eventSender,
			networkConfig: &config.Config{NetworkMode: pb.RpcAccount_DefaultConfig},
			batcher:       mb.New[*domain.SpaceSync](0),
			filesState:    NewFileState(objectstore.NewStoreFixture(t)),
			objectsState:  NewObjectState(objectstore.NewStoreFixture(t)),
		}
		syncStatus := domain.MakeSyncStatus("spaceId", domain.Error, domain.IncompatibleVersion, domain.Objects)

		// then
		status.updateSpaceSyncStatus(syncStatus)

		// when
		assert.Equal(t, domain.Error, status.objectsState.GetSyncStatus("spaceId"))
		assert.Equal(t, 0, status.objectsState.GetSyncObjectCount("spaceId"))
		assert.Equal(t, domain.Error, status.getSpaceSyncStatus(syncStatus.SpaceId))
	})
	t.Run("send offline event", func(t *testing.T) {
		// given
		eventSender := mock_event.NewMockSender(t)
		eventSender.EXPECT().Broadcast(&pb.Event{
			Messages: []*pb.EventMessage{{
				Value: &pb.EventMessageValueOfSpaceSyncStatusUpdate{
					SpaceSyncStatusUpdate: &pb.EventSpaceSyncStatusUpdate{
						Id:                    "spaceId",
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
			batcher:       mb.New[*domain.SpaceSync](0),
			filesState:    NewFileState(objectstore.NewStoreFixture(t)),
			objectsState:  NewObjectState(objectstore.NewStoreFixture(t)),
		}
		syncStatus := domain.MakeSyncStatus("spaceId", domain.Offline, domain.Null, domain.Objects)

		// then
		status.updateSpaceSyncStatus(syncStatus)

		// when
		assert.Equal(t, domain.Offline, status.objectsState.GetSyncStatus("spaceId"))
		assert.Equal(t, 0, status.objectsState.GetSyncObjectCount("spaceId"))
		assert.Equal(t, domain.Offline, status.getSpaceSyncStatus(syncStatus.SpaceId))
	})
	t.Run("send synced event", func(t *testing.T) {
		// given
		eventSender := mock_event.NewMockSender(t)
		eventSender.EXPECT().Broadcast(&pb.Event{
			Messages: []*pb.EventMessage{{
				Value: &pb.EventMessageValueOfSpaceSyncStatusUpdate{
					SpaceSyncStatusUpdate: &pb.EventSpaceSyncStatusUpdate{
						Id:                    "spaceId",
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
			batcher:       mb.New[*domain.SpaceSync](0),
			filesState:    NewFileState(objectstore.NewStoreFixture(t)),
			objectsState:  NewObjectState(objectstore.NewStoreFixture(t)),
		}
		syncStatus := domain.MakeSyncStatus("spaceId", domain.Syncing, domain.Null, domain.Objects)
		status.objectsState.SetObjectsNumber(syncStatus)
		status.objectsState.SetSyncStatusAndErr(syncStatus)

		// then
		syncStatus = domain.MakeSyncStatus("spaceId", domain.Synced, domain.Null, domain.Objects)
		status.updateSpaceSyncStatus(syncStatus)

		// when
		assert.Equal(t, domain.Synced, status.objectsState.GetSyncStatus("spaceId"))
		assert.Equal(t, 0, status.objectsState.GetSyncObjectCount("spaceId"))
		assert.Equal(t, domain.Synced, status.filesState.GetSyncStatus("spaceId"))
		assert.Equal(t, 0, status.filesState.GetSyncObjectCount("spaceId"))
		assert.Equal(t, domain.Synced, status.getSpaceSyncStatus(syncStatus.SpaceId))
	})
}

func TestSpaceSyncStatus_SendUpdate(t *testing.T) {
	t.Run("SendUpdate success", func(t *testing.T) {
		// given
		eventSender := mock_event.NewMockSender(t)
		spaceStatus := spaceSyncStatus{
			eventSender:   eventSender,
			networkConfig: &config.Config{NetworkMode: pb.RpcAccount_DefaultConfig},
			batcher:       mb.New[*domain.SpaceSync](0),
			filesState:    NewFileState(objectstore.NewStoreFixture(t)),
			objectsState:  NewObjectState(objectstore.NewStoreFixture(t)),
		}
		syncStatus := domain.MakeSyncStatus("spaceId", domain.Synced, domain.Null, domain.Files)

		// then
		spaceStatus.SendUpdate(syncStatus)

		// when
		status, err := spaceStatus.batcher.WaitOne(context.Background())
		assert.Nil(t, err)
		assert.Equal(t, status, syncStatus)
	})
}

func TestSpaceSyncStatus_Notify(t *testing.T) {
	t.Run("Notify success", func(t *testing.T) {
		// given
		eventSender := mock_event.NewMockSender(t)
		spaceIdGetter := mock_spacesyncstatus.NewMockSpaceIdGetter(t)
		spaceStatus := spaceSyncStatus{
			eventSender:   eventSender,
			networkConfig: &config.Config{NetworkMode: pb.RpcAccount_DefaultConfig},
			batcher:       mb.New[*domain.SpaceSync](0),
			filesState:    NewFileState(objectstore.NewStoreFixture(t)),
			objectsState:  NewObjectState(objectstore.NewStoreFixture(t)),
			spaceIdGetter: spaceIdGetter,
		}
		// then
		spaceIdGetter.EXPECT().AllSpaceIds().Return([]string{"id1", "id2"})
		eventSender.EXPECT().SendToSession(mock.Anything, &pb.Event{
			Messages: []*pb.EventMessage{{
				Value: &pb.EventMessageValueOfSpaceSyncStatusUpdate{
					SpaceSyncStatusUpdate: &pb.EventSpaceSyncStatusUpdate{
						Id: "id1",
					},
				},
			}},
		})
		eventSender.EXPECT().SendToSession(mock.Anything, &pb.Event{
			Messages: []*pb.EventMessage{{
				Value: &pb.EventMessageValueOfSpaceSyncStatusUpdate{
					SpaceSyncStatusUpdate: &pb.EventSpaceSyncStatusUpdate{
						Id: "id2",
					},
				},
			}},
		})
		spaceStatus.Notify(session.NewContext())
	})
}
