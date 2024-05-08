package reconciler

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/cache/mock_cache"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/filestorage/filesync/mock_filesync"
	"github.com/anyproto/anytype-heart/core/filestorage/mock_filestorage"
	"github.com/anyproto/anytype-heart/core/syncstatus/filesyncstatus"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/tests/testutil"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const testFileId = domain.FileId("bafybeihdwdcefgh4dqkjv67uzcmw7ojee6xedzdetojuzjevtenxquvyku")

var testFullFileId = domain.FullFileId{SpaceId: "spaceId", FileId: testFileId}

func TestQueueItemMarshalUnmarshal(t *testing.T) {
	item := queueItem{
		ObjectId: "objectId",
		FileId:   domain.FullFileId{SpaceId: "spaceId", FileId: "fileId"},
	}

	raw, err := json.Marshal(item)
	require.NoError(t, err)

	var got queueItem
	err = json.Unmarshal(raw, &got)
	require.NoError(t, err)

	assert.Equal(t, item, got)
}

type fixture struct {
	*reconciler

	fileSync     *mock_filesync.MockFileSync
	objectStore  *objectstore.StoreFixture
	objectGetter *mock_cache.MockObjectGetterComponent
}

func newFixture(t *testing.T) *fixture {
	r := New()
	objectStore := objectstore.NewStoreFixture(t)
	fileSync := mock_filesync.NewMockFileSync(t)
	fileSync.EXPECT().OnUploaded(mock.Anything)

	fileStorage := mock_filestorage.NewMockFileStorage(t)
	objectGetter := mock_cache.NewMockObjectGetterComponent(t)

	dataStore, err := datastore.NewInMemory()
	require.NoError(t, err)

	ctx := context.Background()
	a := new(app.App)
	a.Register(objectStore)
	a.Register(dataStore)
	a.Register(testutil.PrepareMock(ctx, a, fileSync))
	a.Register(testutil.PrepareMock(ctx, a, fileStorage))
	a.Register(testutil.PrepareMock(ctx, a, objectGetter))
	a.Register(r)

	err = r.Init(a)
	require.NoError(t, err)

	return &fixture{
		reconciler:   r.(*reconciler),
		fileSync:     fileSync,
		objectStore:  objectStore,
		objectGetter: objectGetter,
	}
}

func TestReconciler(t *testing.T) {
	t.Run("start", func(t *testing.T) {
		t.Run("file object found, mark it as reconciled", func(t *testing.T) {

		})
		t.Run("no file object found", func(t *testing.T) {
			t.Run("this file has disappeared in pre-FilesAsObject version, delete it for good", func(t *testing.T) {

			})
			t.Run("this file eventually will be loaded from remote peer, push it to rebinding queue", func(t *testing.T) {
				// fullFileId := domain.FullFileId{
				// 	SpaceId: "spaceId",
				// 	FileId:  testFileId,
				// }
				//
				// fx.fileSync.EXPECT().CancelDeletion(fullId.ObjectID, fullFileId).Return(nil)

			})
		})
	})

	t.Run("app restarted", func(t *testing.T) {
		t.Run("reconcilation has not been started", func(t *testing.T) {

		})
		t.Run("reconcilation has been started", func(t *testing.T) {

		})
	})
}

func TestFileObjectHook(t *testing.T) {
	t.Run("reconcilation not started: do nothing", func(t *testing.T) {
		fx := newFixture(t)
		err := fx.deletedFiles.Set(testFileId.String(), struct{}{})
		require.NoError(t, err)

		fullId := domain.FullID{
			SpaceID:  "spaceId",
			ObjectID: "fileObjectId",
		}

		hook := fx.FileObjectHook(fullId)

		st := state.NewDoc(fullId.ObjectID, nil).(*state.State)
		st.SetDetailAndBundledRelation(bundle.RelationKeyFileBackupStatus, pbtypes.Int64(int64(filesyncstatus.Synced)))
		st.SetDetailAndBundledRelation(bundle.RelationKeyFileId, pbtypes.String(testFileId.String()))

		err = hook(smartblock.ApplyInfo{
			State: st,
		})

		require.NoError(t, err)

		ok := fx.rebindQueue.Has(fullId.ObjectID)
		assert.False(t, ok)
	})
	t.Run("reconcilation started", func(t *testing.T) {
		t.Run("file hasn't been deleted: do nothing", func(t *testing.T) {
			fx := newFixture(t)
			fx.isStarted = true

			fullId := domain.FullID{
				SpaceID:  "spaceId",
				ObjectID: "fileObjectId",
			}

			hook := fx.FileObjectHook(fullId)

			st := state.NewDoc(fullId.ObjectID, nil).(*state.State)
			st.SetDetailAndBundledRelation(bundle.RelationKeyFileBackupStatus, pbtypes.Int64(int64(filesyncstatus.Synced)))
			st.SetDetailAndBundledRelation(bundle.RelationKeyFileId, pbtypes.String(testFileId.String()))

			err := hook(smartblock.ApplyInfo{
				State: st,
			})
			require.NoError(t, err)

			ok := fx.rebindQueue.Has(fullId.ObjectID)
			assert.False(t, ok)
		})
		t.Run("file has been deleted: push it to rebinding queue", func(t *testing.T) {
			fx := newFixture(t)
			fx.isStarted = true
			err := fx.deletedFiles.Set(testFileId.String(), struct{}{})
			require.NoError(t, err)

			fullId := domain.FullID{
				SpaceID:  "spaceId",
				ObjectID: "fileObjectId",
			}

			hook := fx.FileObjectHook(fullId)

			st := state.NewDoc(fullId.ObjectID, nil).(*state.State)
			st.SetDetailAndBundledRelation(bundle.RelationKeyFileBackupStatus, pbtypes.Int64(int64(filesyncstatus.Synced)))
			st.SetDetailAndBundledRelation(bundle.RelationKeyFileId, pbtypes.String(testFileId.String()))

			err = hook(smartblock.ApplyInfo{
				State: st,
			})

			require.NoError(t, err)

			ok := fx.rebindQueue.Has(fullId.ObjectID)
			assert.True(t, ok)
		})
	})
}
