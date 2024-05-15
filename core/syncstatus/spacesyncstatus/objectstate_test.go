package spacesyncstatus

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/syncstatus/helpers"
)

func TestObjectState_GetSyncObjectCount(t *testing.T) {
	t.Run("GetSyncObjectCount", func(t *testing.T) {
		// given
		objectState := NewObjectState()

		// when
		objectState.objectSyncCountBySpace["spaceId"] = 1
		objectCount := objectState.GetSyncObjectCount("spaceId")

		// then
		assert.Equal(t, 1, objectCount)
	})
	t.Run("GetSyncObjectCount: zero value", func(t *testing.T) {
		// given
		objectState := NewObjectState()

		// when
		objectCount := objectState.GetSyncObjectCount("spaceId")

		// then
		assert.Equal(t, 0, objectCount)
	})
}

func TestObjectState_GetSyncStatus(t *testing.T) {
	t.Run("GetSyncStatus", func(t *testing.T) {
		// given
		objectState := NewObjectState()

		// when
		objectState.objectSyncStatusBySpace["spaceId"] = helpers.Syncing
		syncStatus := objectState.GetSyncStatus("spaceId")

		// then
		assert.Equal(t, helpers.Syncing, syncStatus)
	})
	t.Run("GetSyncStatus: zero value", func(t *testing.T) {
		// given
		objectState := NewObjectState()

		// when
		syncStatus := objectState.GetSyncStatus("spaceId")

		// then
		assert.Equal(t, helpers.Synced, syncStatus)
	})
}

func TestObjectState_SetObjectsNumber(t *testing.T) {
	t.Run("SetObjectsNumber", func(t *testing.T) {
		// given
		objectState := NewObjectState()
		syncStatus := helpers.MakeSyncStatus("spaceId", helpers.Syncing, 2, helpers.Null, helpers.Objects)

		// when
		objectState.SetObjectsNumber(syncStatus)

		// then
		assert.Equal(t, 2, objectState.GetSyncObjectCount("spaceId"))
	})
	t.Run("SetObjectsNumber: no object", func(t *testing.T) {
		// given
		objectState := NewObjectState()
		syncStatus := helpers.MakeSyncStatus("spaceId", helpers.Synced, 0, helpers.Null, helpers.Objects)

		// when
		objectState.SetObjectsNumber(syncStatus)

		// then
		assert.Equal(t, 0, objectState.GetSyncObjectCount("spaceId"))
	})
}

func TestObjectState_IsSyncFinished(t *testing.T) {
	t.Run("IsSyncFinished, sync is finished", func(t *testing.T) {
		// given
		objectState := NewObjectState()

		// when
		finished := objectState.IsSyncFinished("spaceId")

		// then
		assert.True(t, finished)
	})
	t.Run("IsSyncFinished, sync is finished", func(t *testing.T) {
		// given
		objectState := NewObjectState()

		// when
		syncStatus := helpers.MakeSyncStatus("spaceId", helpers.Synced, 0, helpers.Null, helpers.Objects)
		objectState.SetSyncStatus(syncStatus)
		finished := objectState.IsSyncFinished("spaceId")

		// then
		assert.True(t, finished)
	})
	t.Run("IsSyncFinished, sync is not finished", func(t *testing.T) {
		// given
		objectState := NewObjectState()

		// when
		syncStatus := helpers.MakeSyncStatus("spaceId", helpers.Offline, 3, helpers.Null, helpers.Objects)
		objectState.SetSyncStatus(syncStatus)
		finished := objectState.IsSyncFinished("spaceId")

		// then
		assert.False(t, finished)
	})
}

func TestObjectState_SetSyncStatus(t *testing.T) {
	t.Run("SetSyncStatus, status synced", func(t *testing.T) {
		// given
		objectState := NewObjectState()

		// when
		syncStatus := helpers.MakeSyncStatus("spaceId", helpers.Synced, 0, helpers.Null, helpers.Objects)
		objectState.SetSyncStatus(syncStatus)

		// then
		assert.Equal(t, helpers.Synced, objectState.GetSyncStatus("spaceId"))
	})
	t.Run("SetSyncStatus, sync in progress", func(t *testing.T) {
		// given
		objectState := NewObjectState()

		// when
		syncStatus := helpers.MakeSyncStatus("spaceId", helpers.Syncing, 1, helpers.Null, helpers.Objects)
		objectState.SetSyncStatus(syncStatus)

		// then
		assert.Equal(t, helpers.Syncing, objectState.GetSyncStatus("spaceId"))
	})
	t.Run("SetSyncStatus, sync is finished with error", func(t *testing.T) {
		// given
		objectState := NewObjectState()

		// when
		syncStatus := helpers.MakeSyncStatus("spaceId", helpers.Error, 3, helpers.Null, helpers.Objects)
		objectState.SetSyncStatus(syncStatus)

		// then
		assert.Equal(t, helpers.Error, objectState.GetSyncStatus("spaceId"))
	})
	t.Run("SetSyncStatus, offline", func(t *testing.T) {
		// given
		objectState := NewObjectState()

		// when
		syncStatus := helpers.MakeSyncStatus("spaceId", helpers.Offline, 3, helpers.Null, helpers.Objects)
		objectState.SetSyncStatus(syncStatus)

		// then
		assert.Equal(t, helpers.Offline, objectState.GetSyncStatus("spaceId"))
	})
}
