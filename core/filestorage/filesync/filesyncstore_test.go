package filesync

import (
	"os"
	"testing"

	"github.com/dgraph-io/badger/v4"
	"github.com/stretchr/testify/require"
)

//
// func TestMigration(t *testing.T) {
// 	fx := newStoreFixture(t)
// 	defer fx.Finish()
//
// 	wantUploadItem := &QueueItem{
// 		SpaceId:     "spaceId1",
// 		FileId:      "fileId1",
// 		Timestamp:   time.Now().UnixMilli(),
// 		AddedByUser: false,
// 	}
// 	wantDiscardedItem := &QueueItem{
// 		SpaceId:     "spaceId1",
// 		FileId:      "fileId2",
// 		Timestamp:   time.Now().UnixMilli(),
// 		AddedByUser: false,
// 	}
// 	wantRemoveItem := &QueueItem{
// 		SpaceId:   "spaceId1",
// 		FileId:    "fileId3",
// 		Timestamp: time.Now().UnixMilli(),
// 	}
//
// 	t.Run("with old schema", func(t *testing.T) {
// 		err := fx.db.Update(func(txn *badger.Txn) error {
// 			if err := txn.Set(uploadKey(wantUploadItem.SpaceId, wantUploadItem.FileId), binTime(wantUploadItem.Timestamp)); err != nil {
// 				return err
// 			}
// 			if err := txn.Set(discardedKey(wantDiscardedItem.SpaceId, wantDiscardedItem.FileId), binTime(wantDiscardedItem.Timestamp)); err != nil {
// 				return err
// 			}
// 			if err := txn.Set(removeKey(wantRemoveItem.SpaceId, wantRemoveItem.FileId), binTime(wantRemoveItem.Timestamp)); err != nil {
// 				return err
// 			}
// 			return nil
// 		})
// 		require.NoError(t, err)
//
// 		t.Run("expect errors before migration", func(t *testing.T) {
// 			_, err := fx.GetUpload()
// 			assert.Error(t, err)
// 			_, err = fx.GetDiscardedUpload()
// 			assert.Error(t, err)
// 			_, err = fx.GetRemove()
// 			assert.Error(t, err)
// 		})
// 	})
//
// 	err := fx.migrateQueue()
// 	require.NoError(t, err)
//
// 	got, err := fx.GetUpload()
// 	require.NoError(t, err)
// 	assert.Equal(t, wantUploadItem, got)
//
// 	got, err = fx.GetDiscardedUpload()
// 	require.NoError(t, err)
// 	assert.Equal(t, wantDiscardedItem, got)
//
// 	got, err = fx.GetRemove()
// 	require.NoError(t, err)
// 	assert.Equal(t, wantRemoveItem, got)
// }

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
