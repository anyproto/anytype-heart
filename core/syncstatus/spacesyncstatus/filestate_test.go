package spacesyncstatus

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/syncstatus/filesyncstatus"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestFileState_GetSyncObjectCount(t *testing.T) {
	t.Run("GetSyncObjectCount", func(t *testing.T) {
		// given
		fileState := NewFileState(nil)

		// when
		fileState.fileSyncCountBySpace["spaceId"] = 1
		objectCount := fileState.GetSyncObjectCount("spaceId")

		// then
		assert.Equal(t, 1, objectCount)
	})
	t.Run("GetSyncObjectCount: zero value", func(t *testing.T) {
		// given
		fileState := NewFileState(nil)

		// when
		objectCount := fileState.GetSyncObjectCount("spaceId")

		// then
		assert.Equal(t, 0, objectCount)
	})
}

func TestFileState_GetSyncStatus(t *testing.T) {
	t.Run("GetSyncStatus", func(t *testing.T) {
		// given
		fileState := NewFileState(nil)

		// when
		fileState.fileSyncStatusBySpace["spaceId"] = domain.Syncing
		syncStatus := fileState.GetSyncStatus("spaceId")

		// then
		assert.Equal(t, domain.Syncing, syncStatus)
	})
	t.Run("GetSyncStatus: zero value", func(t *testing.T) {
		// given
		fileState := NewFileState(nil)

		// when
		syncStatus := fileState.GetSyncStatus("spaceId")

		// then
		assert.Equal(t, domain.Synced, syncStatus)
	})
}

func TestFileState_SetObjectsNumber(t *testing.T) {
	t.Run("SetObjectsNumber", func(t *testing.T) {
		// given
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
		fileState := NewFileState(storeFixture)
		syncStatus := domain.MakeSyncStatus("spaceId", domain.Syncing, domain.Null, domain.Files)

		// when
		fileState.SetObjectsNumber(syncStatus)

		// then
		assert.Equal(t, 2, fileState.GetSyncObjectCount("spaceId"))
	})
	t.Run("SetObjectsNumber: no file object", func(t *testing.T) {
		// given
		storeFixture := objectstore.NewStoreFixture(t)
		fileState := NewFileState(storeFixture)
		syncStatus := domain.MakeSyncStatus("spaceId", domain.Synced, domain.Null, domain.Files)

		// when
		fileState.SetObjectsNumber(syncStatus)

		// then
		assert.Equal(t, 0, fileState.GetSyncObjectCount("spaceId"))
	})
}

func TestFileState_SetSyncStatus(t *testing.T) {
	t.Run("SetSyncStatusAndErr, status synced", func(t *testing.T) {
		// given
		fileState := NewFileState(objectstore.NewStoreFixture(t))

		// when
		fileState.SetSyncStatusAndErr(domain.Synced, domain.Null, "spaceId")

		// then
		assert.Equal(t, domain.Synced, fileState.GetSyncStatus("spaceId"))
	})
	t.Run("SetSyncStatusAndErr, sync in progress", func(t *testing.T) {
		// given
		fileState := NewFileState(objectstore.NewStoreFixture(t))

		// when
		syncStatus := domain.MakeSyncStatus("spaceId", domain.Syncing, domain.Null, domain.Files)
		fileState.SetSyncStatusAndErr(syncStatus.Status, domain.Null, syncStatus.SpaceId)

		// then
		assert.Equal(t, domain.Syncing, fileState.GetSyncStatus("spaceId"))
	})
	t.Run("SetSyncStatusAndErr, sync is finished with error", func(t *testing.T) {
		// given
		fileState := NewFileState(objectstore.NewStoreFixture(t))

		// when
		syncStatus := domain.MakeSyncStatus("spaceId", domain.Error, domain.Null, domain.Files)
		fileState.SetSyncStatusAndErr(syncStatus.Status, domain.Null, syncStatus.SpaceId)

		// then
		assert.Equal(t, domain.Error, fileState.GetSyncStatus("spaceId"))
	})
	t.Run("SetSyncStatusAndErr, offline", func(t *testing.T) {
		// given
		fileState := NewFileState(objectstore.NewStoreFixture(t))

		// when
		fileState.SetSyncStatusAndErr(domain.Offline, domain.Null, "spaceId")

		// then
		assert.Equal(t, domain.Offline, fileState.GetSyncStatus("spaceId"))
	})
}
