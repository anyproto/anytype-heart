package spacesyncstatus

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestObjectState_GetSyncObjectCount(t *testing.T) {
	t.Run("GetSyncObjectCount", func(t *testing.T) {
		// given
		objectState := NewObjectState(objectstore.NewStoreFixture(t))

		// when
		objectState.objectSyncCountBySpace["spaceId"] = 1
		objectCount := objectState.GetSyncObjectCount("spaceId")

		// then
		assert.Equal(t, 1, objectCount)
	})
	t.Run("GetSyncObjectCount: zero value", func(t *testing.T) {
		// given
		objectState := NewObjectState(objectstore.NewStoreFixture(t))

		// when
		objectCount := objectState.GetSyncObjectCount("spaceId")

		// then
		assert.Equal(t, 0, objectCount)
	})
}

func TestObjectState_GetSyncStatus(t *testing.T) {
	t.Run("GetSyncStatus", func(t *testing.T) {
		// given
		objectState := NewObjectState(objectstore.NewStoreFixture(t))

		// when
		objectState.objectSyncStatusBySpace["spaceId"] = domain.Syncing
		syncStatus := objectState.GetSyncStatus("spaceId")

		// then
		assert.Equal(t, domain.Syncing, syncStatus)
	})
	t.Run("GetSyncStatus: zero value", func(t *testing.T) {
		// given
		objectState := NewObjectState(objectstore.NewStoreFixture(t))

		// when
		syncStatus := objectState.GetSyncStatus("spaceId")

		// then
		assert.Equal(t, domain.Unknown, syncStatus)
	})
}

func TestObjectState_SetObjectsNumber(t *testing.T) {
	t.Run("SetObjectsNumber", func(t *testing.T) {
		// given
		storeFixture := objectstore.NewStoreFixture(t)
		objectState := NewObjectState(storeFixture)
		syncStatus := domain.MakeSyncStatus("spaceId", domain.Syncing, domain.Null, domain.Objects)
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

		// when
		objectState.SetObjectsNumber(syncStatus)

		// then
		assert.Equal(t, 2, objectState.GetSyncObjectCount("spaceId"))
	})
	t.Run("SetObjectsNumber: no object", func(t *testing.T) {
		// given
		objectState := NewObjectState(objectstore.NewStoreFixture(t))
		syncStatus := domain.MakeSyncStatus("spaceId", domain.Synced, domain.Null, domain.Objects)

		// when
		objectState.SetObjectsNumber(syncStatus)

		// then
		assert.Equal(t, 0, objectState.GetSyncObjectCount("spaceId"))
	})
}

func TestObjectState_SetSyncStatus(t *testing.T) {
	t.Run("SetSyncStatusAndErr, status synced", func(t *testing.T) {
		// given
		objectState := NewObjectState(objectstore.NewStoreFixture(t))

		// when
		syncStatus := domain.MakeSyncStatus("spaceId", domain.Synced, domain.Null, domain.Objects)
		objectState.SetSyncStatusAndErr(syncStatus.Status, domain.Null, syncStatus.SpaceId)

		// then
		assert.Equal(t, domain.Synced, objectState.GetSyncStatus("spaceId"))
	})
	t.Run("SetSyncStatusAndErr, sync in progress", func(t *testing.T) {
		// given
		objectState := NewObjectState(objectstore.NewStoreFixture(t))

		// when
		syncStatus := domain.MakeSyncStatus("spaceId", domain.Syncing, domain.Null, domain.Objects)
		objectState.SetSyncStatusAndErr(syncStatus.Status, domain.Null, syncStatus.SpaceId)

		// then
		assert.Equal(t, domain.Syncing, objectState.GetSyncStatus("spaceId"))
	})
	t.Run("SetSyncStatusAndErr, sync is finished with error", func(t *testing.T) {
		// given
		objectState := NewObjectState(objectstore.NewStoreFixture(t))

		// when
		syncStatus := domain.MakeSyncStatus("spaceId", domain.Error, domain.Null, domain.Objects)
		objectState.SetSyncStatusAndErr(syncStatus.Status, domain.Null, syncStatus.SpaceId)

		// then
		assert.Equal(t, domain.Error, objectState.GetSyncStatus("spaceId"))
	})
	t.Run("SetSyncStatusAndErr, offline", func(t *testing.T) {
		// given
		objectState := NewObjectState(objectstore.NewStoreFixture(t))

		// when
		syncStatus := domain.MakeSyncStatus("spaceId", domain.Offline, domain.Null, domain.Objects)
		objectState.SetSyncStatusAndErr(syncStatus.Status, domain.Null, syncStatus.SpaceId)

		// then
		assert.Equal(t, domain.Offline, objectState.GetSyncStatus("spaceId"))
	})
}
