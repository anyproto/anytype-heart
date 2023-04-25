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
	require.NoError(t, fx.QueueUpload("spaceId1", "fileId1"))
	l, err := fx.QueueLen()
	require.NoError(t, err)
	assert.Equal(t, 1, l)
	spaceId, fileId, err := fx.GetUpload()
	require.NoError(t, err)
	assert.Equal(t, "spaceId1", spaceId)
	assert.Equal(t, "fileId1", fileId)
}

func TestFileSyncStore_QueueRemove(t *testing.T) {
	fx := newStoreFixture(t)
	defer fx.Finish()
	require.NoError(t, fx.QueueRemove("spaceId1", "fileId1"))
	l, err := fx.QueueLen()
	require.NoError(t, err)
	assert.Equal(t, 1, l)
	spaceId, fileId, err := fx.GetRemove()
	require.NoError(t, err)
	assert.Equal(t, "spaceId1", spaceId)
	assert.Equal(t, "fileId1", fileId)
}

func TestFileSyncStore_DoneUpload(t *testing.T) {
	fx := newStoreFixture(t)
	defer fx.Finish()
	require.NoError(t, fx.QueueUpload("spaceId1", "fileId1"))
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
	require.NoError(t, fx.QueueUpload("spaceId1", "fileId1"))
	spaceId, fileId, err := fx.GetUpload()
	require.NoError(t, err)
	assert.Equal(t, "spaceId1", spaceId)
	assert.Equal(t, "fileId1", fileId)
	require.NoError(t, fx.DoneUpload(spaceId, fileId))
	_, _, err = fx.GetUpload()
	assert.EqualError(t, err, errQueueIsEmpty.Error())
}

func TestFileSyncStore_PushBackToQueue(t *testing.T) {
	fx := newStoreFixture(t)
	defer fx.Finish()

	require.NoError(t, fx.QueueUpload("spaceId1", "fileId1"))
	time.Sleep(2 * time.Millisecond)
	require.NoError(t, fx.QueueUpload("spaceId2", "fileId2"))

	spaceId, fileId, err := fx.GetUpload()
	require.NoError(t, err)
	assert.Equal(t, "spaceId1", spaceId)
	assert.Equal(t, "fileId1", fileId)

	time.Sleep(2 * time.Millisecond)
	require.NoError(t, fx.QueueUpload("spaceId1", "fileId1"))

	spaceId, fileId, err = fx.GetUpload()
	require.NoError(t, err)
	assert.Equal(t, "spaceId2", spaceId)
	assert.Equal(t, "fileId2", fileId)
}

func TestFileSyncStore_IsDone(t *testing.T) {
	fx := newStoreFixture(t)
	defer fx.Finish()
	require.NoError(t, fx.QueueUpload("spaceId1", "fileId1"))
	done, err := fx.IsAlreadyUploaded("spaceId1", "fileId1")
	require.NoError(t, err)
	assert.False(t, done)
	require.NoError(t, fx.DoneUpload("spaceId1", "fileId1"))
	done, err = fx.IsAlreadyUploaded("spaceId1", "fileId1")
	require.NoError(t, err)
	assert.True(t, done)
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
