package filesync

import (
	"os"
	"testing"
	"time"

	"github.com/dgraph-io/badger/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileSyncStore_QueueUpload(t *testing.T) {
	fx := newStoreFixture(t)
	defer fx.Finish()
	require.NoError(t, fx.QueueUpload("spaceId1", "fileId1", true))
	l, err := fx.QueueLen()
	require.NoError(t, err)
	assert.Equal(t, 1, l)
	it, err := fx.GetUpload()
	require.NoError(t, err)
	assert.Equal(t, "spaceId1", it.SpaceID)
	assert.Equal(t, "fileId1", it.FileID)
	assert.True(t, it.AddedByUser)
}

func TestFileSyncStore_QueueRemove(t *testing.T) {
	fx := newStoreFixture(t)
	defer fx.Finish()
	require.NoError(t, fx.QueueRemove("spaceId1", "fileId1"))
	l, err := fx.QueueLen()
	require.NoError(t, err)
	assert.Equal(t, 1, l)
	it, err := fx.GetRemove()
	require.NoError(t, err)
	assert.Equal(t, "spaceId1", it.SpaceID)
	assert.Equal(t, "fileId1", it.FileID)
}

func TestFileSyncStore_DoneUpload(t *testing.T) {
	fx := newStoreFixture(t)
	defer fx.Finish()
	require.NoError(t, fx.QueueUpload("spaceId1", "fileId1", false))
	require.NoError(t, fx.DoneUpload("spaceId1", "fileId1"))
	l, err := fx.QueueLen()
	require.NoError(t, err)
	assert.Equal(t, 0, l)
}

func TestFileSyncStore_DoneRemove(t *testing.T) {
	fx := newStoreFixture(t)
	defer fx.Finish()
	require.NoError(t, fx.QueueRemove("spaceId1", "fileId1"))
	require.NoError(t, fx.DoneRemove("spaceId1", "fileId1"))
	l, err := fx.QueueLen()
	require.NoError(t, err)
	assert.Equal(t, 0, l)
}

func TestFileSyncStore_GetUpload(t *testing.T) {
	fx := newStoreFixture(t)
	defer fx.Finish()
	require.NoError(t, fx.QueueUpload("spaceId1", "fileId1", true))
	it, err := fx.GetUpload()
	require.NoError(t, err)
	assert.Equal(t, "spaceId1", it.SpaceID)
	assert.Equal(t, "fileId1", it.FileID)
	require.NoError(t, fx.DoneUpload(it.SpaceID, it.FileID))
	_, err = fx.GetUpload()
	assert.EqualError(t, err, errQueueIsEmpty.Error())
}

func TestFileSyncStore_PushBackToQueue(t *testing.T) {
	fx := newStoreFixture(t)
	defer fx.Finish()

	require.NoError(t, fx.QueueUpload("spaceId1", "fileId1", false))
	time.Sleep(2 * time.Millisecond)
	require.NoError(t, fx.QueueUpload("spaceId2", "fileId2", false))

	it, err := fx.GetUpload()
	require.NoError(t, err)
	assert.Equal(t, "spaceId1", it.SpaceID)
	assert.Equal(t, "fileId1", it.FileID)
	assert.False(t, it.AddedByUser)

	time.Sleep(2 * time.Millisecond)
	require.NoError(t, fx.QueueUpload("spaceId1", "fileId1", false))

	it, err = fx.GetUpload()
	require.NoError(t, err)
	assert.Equal(t, "spaceId2", it.SpaceID)
	assert.Equal(t, "fileId2", it.FileID)
	assert.False(t, it.AddedByUser)
}

func TestFileSyncStore_PrioritizeAddedByUser(t *testing.T) {
	fx := newStoreFixture(t)
	defer fx.Finish()

	require.NoError(t, fx.QueueUpload("spaceId1", "fileId1", false))
	time.Sleep(2 * time.Millisecond)
	require.NoError(t, fx.QueueUpload("spaceId1", "fileId2", true))

	it, err := fx.GetUpload()
	require.NoError(t, err)
	assert.Equal(t, "spaceId1", it.SpaceID)
	assert.Equal(t, "fileId2", it.FileID)
	assert.True(t, it.AddedByUser)
}

func TestFileSyncStore_IsDone(t *testing.T) {
	fx := newStoreFixture(t)
	defer fx.Finish()
	require.NoError(t, fx.QueueUpload("spaceId1", "fileId1", false))
	done, err := fx.IsAlreadyUploaded("spaceId1", "fileId1")
	require.NoError(t, err)
	assert.False(t, done)
	require.NoError(t, fx.DoneUpload("spaceId1", "fileId1"))
	done, err = fx.IsAlreadyUploaded("spaceId1", "fileId1")
	require.NoError(t, err)
	assert.True(t, done)
}

func TestMigration(t *testing.T) {
	fx := newStoreFixture(t)
	defer fx.Finish()

	wantUploadItem := &QueueItem{
		SpaceID:     "spaceId1",
		FileID:      "fileId1",
		Timestamp:   time.Now().UnixMilli(),
		AddedByUser: false,
	}
	wantDiscardedItem := &QueueItem{
		SpaceID:     "spaceId1",
		FileID:      "fileId2",
		Timestamp:   time.Now().UnixMilli(),
		AddedByUser: false,
	}
	wantRemoveItem := &QueueItem{
		SpaceID:   "spaceId1",
		FileID:    "fileId3",
		Timestamp: time.Now().UnixMilli(),
	}

	t.Run("with old schema", func(t *testing.T) {
		err := fx.db.Update(func(txn *badger.Txn) error {
			if err := txn.Set(uploadKey(wantUploadItem.SpaceID, wantUploadItem.FileID), binTime(wantUploadItem.Timestamp)); err != nil {
				return err
			}
			if err := txn.Set(discardedKey(wantDiscardedItem.SpaceID, wantDiscardedItem.FileID), binTime(wantDiscardedItem.Timestamp)); err != nil {
				return err
			}
			if err := txn.Set(removeKey(wantRemoveItem.SpaceID, wantRemoveItem.FileID), binTime(wantRemoveItem.Timestamp)); err != nil {
				return err
			}
			return nil
		})
		require.NoError(t, err)

		t.Run("expect errors before migration", func(t *testing.T) {
			_, err := fx.GetUpload()
			assert.Error(t, err)
			_, err = fx.GetDiscardedUpload()
			assert.Error(t, err)
			_, err = fx.GetRemove()
			assert.Error(t, err)
		})
	})

	err := fx.migrateQueue()
	require.NoError(t, err)

	got, err := fx.GetUpload()
	require.NoError(t, err)
	assert.Equal(t, wantUploadItem, got)

	got, err = fx.GetDiscardedUpload()
	require.NoError(t, err)
	assert.Equal(t, wantDiscardedItem, got)

	got, err = fx.GetRemove()
	require.NoError(t, err)
	assert.Equal(t, wantRemoveItem, got)
}

func newStoreFixture(t *testing.T) (sf *storeFixture) {
	sf = &storeFixture{
		fileSyncStore: &fileSyncStore{},
	}
	var err error
	sf.dir, err = os.MkdirTemp("", "*")
	require.NoError(t, err)
	sf.db, err = badger.Open(badger.DefaultOptions(sf.dir))
	require.NoError(t, err)
	return
}

type storeFixture struct {
	*fileSyncStore
	dir string
}

func (sf *storeFixture) Finish() {
	_ = sf.db.Close()
	_ = os.RemoveAll(sf.dir)
}
