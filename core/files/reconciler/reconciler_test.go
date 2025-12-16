package reconciler

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/cache/mock_cache"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files/filestorage/mock_filestorage"
	"github.com/anyproto/anytype-heart/core/files/filesync"
	"github.com/anyproto/anytype-heart/core/files/filesync/mock_filesync"
	"github.com/anyproto/anytype-heart/core/syncstatus/filesyncstatus"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore/anystoreprovider"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/tests/testutil"
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
	fileStorage  *mock_filestorage.MockFileStorage
}

func newFixture(t *testing.T) *fixture {
	r := New()
	objectStore := objectstore.NewStoreFixture(t)
	fileSync := mock_filesync.NewMockFileSync(t)
	fileSync.EXPECT().OnStatusUpdated(mock.Anything).Maybe()

	fileStorage := mock_filestorage.NewMockFileStorage(t)
	objectGetter := mock_cache.NewMockObjectGetterComponent(t)

	dbProvider, err := anystoreprovider.NewInPath(t.TempDir())

	ctx := context.Background()
	a := new(app.App)
	a.Register(objectStore)
	a.Register(testutil.PrepareMock(ctx, a, fileSync))
	a.Register(testutil.PrepareMock(ctx, a, fileStorage))
	a.Register(testutil.PrepareMock(ctx, a, objectGetter))
	a.Register(r)
	a.Register(dbProvider)

	err = r.Init(a)
	require.NoError(t, err)

	return &fixture{
		reconciler:   r.(*reconciler),
		fileSync:     fileSync,
		objectStore:  objectStore,
		objectGetter: objectGetter,
		fileStorage:  fileStorage,
	}
}

func TestReconcileRemoteStorage(t *testing.T) {
	fx := newFixture(t)
	fx.objectStore.AddObjects(t, "space1", []objectstore.TestObject{
		{
			bundle.RelationKeyId:               domain.String("objectId1"),
			bundle.RelationKeyFileId:           domain.String(testFileId.String()),
			bundle.RelationKeyFileBackupStatus: domain.Int64(int64(filesyncstatus.Synced)),
		},
		{
			bundle.RelationKeyId:               domain.String("objectId2"),
			bundle.RelationKeyFileId:           domain.String("deletedFileId"),
			bundle.RelationKeyFileBackupStatus: domain.Int64(int64(filesyncstatus.Synced)),
			bundle.RelationKeyIsDeleted:        domain.Bool(true),
		},
	})

	fx.fileStorage.EXPECT().IterateFiles(mock.Anything, mock.Anything).
		Run(func(ctx context.Context, iterFunc func(domain.FullFileId)) {
			iterFunc(domain.FullFileId{SpaceId: "spaceId", FileId: testFileId})
			iterFunc(domain.FullFileId{SpaceId: "spaceId", FileId: "deletedFileId"})
			iterFunc(domain.FullFileId{SpaceId: "spaceId", FileId: "anotherFileId"})
		}).
		Return(nil)

	wantDeletedFiles := []domain.FileId{
		"deletedFileId",
		"anotherFileId",
	}
	for _, fileId := range wantDeletedFiles {
		fx.fileSync.EXPECT().DeleteFile("", domain.FullFileId{SpaceId: "spaceId", FileId: fileId}).Return(nil)
		ok, err := fx.deletedFiles.Has(context.Background(), fileId.String())
		require.NoError(t, err)
		assert.False(t, ok)
	}

	err := fx.reconcileRemoteStorage(context.Background())

	require.NoError(t, err)
}

func TestFileObjectHook(t *testing.T) {
	t.Run("reconcilation not started: do nothing", func(t *testing.T) {
		fx := newFixture(t)
		err := fx.deletedFiles.Set(context.Background(), testFileId.String(), struct{}{})
		require.NoError(t, err)

		fullId := domain.FullID{
			SpaceID:  "spaceId",
			ObjectID: "fileObjectId",
		}

		hook := fx.FileObjectHook(fullId)

		st := state.NewDoc(fullId.ObjectID, nil).(*state.State)
		st.SetDetailAndBundledRelation(bundle.RelationKeyFileBackupStatus, domain.Int64(int64(filesyncstatus.Synced)))
		st.SetDetailAndBundledRelation(bundle.RelationKeyFileId, domain.String(testFileId.String()))

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
			st.SetDetailAndBundledRelation(bundle.RelationKeyFileBackupStatus, domain.Int64(int64(filesyncstatus.Synced)))
			st.SetDetailAndBundledRelation(bundle.RelationKeyFileId, domain.String(testFileId.String()))

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
			err := fx.deletedFiles.Set(context.Background(), testFileId.String(), struct{}{})
			require.NoError(t, err)

			fullId := domain.FullID{
				SpaceID:  "spaceId",
				ObjectID: "fileObjectId",
			}

			hook := fx.FileObjectHook(fullId)

			st := state.NewDoc(fullId.ObjectID, nil).(*state.State)
			st.SetDetailAndBundledRelation(bundle.RelationKeyFileBackupStatus, domain.Int64(int64(filesyncstatus.Synced)))
			st.SetDetailAndBundledRelation(bundle.RelationKeyFileId, domain.String(testFileId.String()))

			err = hook(smartblock.ApplyInfo{
				State: st,
			})

			require.NoError(t, err)

			ok := fx.rebindQueue.Has(fullId.ObjectID)
			assert.True(t, ok)
		})
	})
}

func TestRebindQueue(t *testing.T) {
	fx := newFixture(t)

	fx.fileSync.EXPECT().AddFile(filesync.AddFileRequest{
		FileObjectId:   "objectId1",
		FileId:         testFullFileId,
		UploadedByUser: false,
		Imported:       false,
	}).Return(nil)

	err := fx.rebindQueue.Add(&queueItem{
		ObjectId: "objectId1",
		FileId:   testFullFileId,
	})
	require.NoError(t, err)

	fx.rebindQueue.Run()

	timeout := time.NewTimer(50 * time.Millisecond)
	defer timeout.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if fx.rebindQueue.NumProcessedItems() == 1 {
				return
			}
		case <-timeout.C:
			t.Fatal("timeout")
		}
	}
}
