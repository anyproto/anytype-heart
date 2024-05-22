package spacesyncstatus

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
		objectState.objectSyncStatusBySpace["spaceId"] = Syncing
		syncStatus := objectState.GetSyncStatus("spaceId")

		// then
		assert.Equal(t, Syncing, syncStatus)
	})
	t.Run("GetSyncStatus: zero value", func(t *testing.T) {
		// given
		objectState := NewObjectState()

		// when
		syncStatus := objectState.GetSyncStatus("spaceId")

		// then
		assert.Equal(t, Synced, syncStatus)
	})
}

func TestObjectState_SetObjectsNumber(t *testing.T) {
	t.Run("SetObjectsNumber", func(t *testing.T) {
		// given
		objectState := NewObjectState()
		syncStatus := MakeSyncStatus("spaceId", Syncing, 2, Null, Objects)

		// when
		objectState.SetObjectsNumber(syncStatus)

		// then
		assert.Equal(t, 2, objectState.GetSyncObjectCount("spaceId"))
	})
	t.Run("SetObjectsNumber: no object", func(t *testing.T) {
		// given
		objectState := NewObjectState()
		syncStatus := MakeSyncStatus("spaceId", Synced, 0, Null, Objects)

		// when
		objectState.SetObjectsNumber(syncStatus)

		// then
		assert.Equal(t, 0, objectState.GetSyncObjectCount("spaceId"))
	})
}

func TestObjectState_IsSyncFinished(t *testing.T) {
	t.Run("IsSyncFinished, sync is not finished", func(t *testing.T) {
		// given
		objectState := NewObjectState()

		// when
		finished := objectState.IsSyncFinished("spaceId")

		// then
		assert.False(t, finished)
	})
	t.Run("IsSyncFinished, sync is finished", func(t *testing.T) {
		// given
		objectState := NewObjectState()

		// when
		syncStatus := MakeSyncStatus("spaceId", Synced, 0, Null, Objects)
		objectState.SetSyncStatus(syncStatus)
		finished := objectState.IsSyncFinished("spaceId")

		// then
		assert.True(t, finished)
	})
	t.Run("IsSyncFinished, sync is not finished", func(t *testing.T) {
		// given
		objectState := NewObjectState()

		// when
		syncStatus := MakeSyncStatus("spaceId", Offline, 3, Null, Objects)
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
		syncStatus := MakeSyncStatus("spaceId", Synced, 0, Null, Objects)
		objectState.SetSyncStatus(syncStatus)

		// then
		assert.Equal(t, Synced, objectState.GetSyncStatus("spaceId"))
	})
	t.Run("SetSyncStatus, sync in progress", func(t *testing.T) {
		// given
		objectState := NewObjectState()

		// when
		syncStatus := MakeSyncStatus("spaceId", Syncing, 1, Null, Objects)
		objectState.SetSyncStatus(syncStatus)

		// then
		assert.Equal(t, Syncing, objectState.GetSyncStatus("spaceId"))
	})
	t.Run("SetSyncStatus, sync is finished with error", func(t *testing.T) {
		// given
		objectState := NewObjectState()

		// when
		syncStatus := MakeSyncStatus("spaceId", Error, 3, Null, Objects)
		objectState.SetSyncStatus(syncStatus)

		// then
		assert.Equal(t, Error, objectState.GetSyncStatus("spaceId"))
	})
	t.Run("SetSyncStatus, offline", func(t *testing.T) {
		// given
		objectState := NewObjectState()

		// when
		syncStatus := MakeSyncStatus("spaceId", Offline, 3, Null, Objects)
		objectState.SetSyncStatus(syncStatus)

		// then
		assert.Equal(t, Offline, objectState.GetSyncStatus("spaceId"))
	})
}
