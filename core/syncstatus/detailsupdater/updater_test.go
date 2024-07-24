package detailsupdater

import (
	"context"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/ocache"
	"github.com/cheggaaa/mb/v3"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/anyproto/anytype-heart/core/block/editor"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	domain "github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/syncstatus/detailsupdater/mock_detailsupdater"
	"github.com/anyproto/anytype-heart/core/syncstatus/filesyncstatus"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/space/mock_space"
	"github.com/anyproto/anytype-heart/tests/testutil"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestSyncStatusUpdater_UpdateDetails(t *testing.T) {
	t.Run("update sync status and date - no changes", func(t *testing.T) {
		// given
		fixture := newFixture(t)
		fixture.storeFixture.AddObjects(t, []objectstore.TestObject{
			{
				bundle.RelationKeyId:         pbtypes.String("id"),
				bundle.RelationKeySyncStatus: pbtypes.Int64(int64(domain.SpaceSyncStatusSynced)),
				bundle.RelationKeySyncError:  pbtypes.Int64(int64(domain.SyncErrorNull)),
			},
		})

		// when
		err := fixture.updater.updateObjectDetails(&syncStatusDetails{[]string{"id"}, domain.ObjectSyncStatusSynced, domain.SyncErrorNull, "spaceId"}, "id")

		// then
		assert.Nil(t, err)
		fixture.service.AssertNotCalled(t, "Get")
	})
	t.Run("update sync status and date - details exist in store", func(t *testing.T) {
		// given
		fixture := newFixture(t)
		space := mock_clientspace.NewMockSpace(t)
		fixture.service.EXPECT().Get(fixture.updater.ctx, "spaceId").Return(space, nil)
		fixture.storeFixture.AddObjects(t, []objectstore.TestObject{
			{
				bundle.RelationKeyId: pbtypes.String("id"),
			},
		})
		space.EXPECT().DoLockedIfNotExists("id", mock.Anything).Return(nil)

		// when
		fixture.statusUpdater.EXPECT().SendUpdate(domain.MakeSyncStatus("spaceId", domain.SpaceSyncStatusSynced, domain.SyncErrorNull))
		err := fixture.updater.updateObjectDetails(&syncStatusDetails{[]string{"id"}, domain.ObjectSyncStatusSynced, domain.SyncErrorNull, "spaceId"}, "id")

		// then
		assert.Nil(t, err)
	})
	t.Run("update sync status and date - object not exist in cache", func(t *testing.T) {
		// given
		fixture := newFixture(t)
		space := mock_clientspace.NewMockSpace(t)
		fixture.service.EXPECT().Get(fixture.updater.ctx, "spaceId").Return(space, nil)
		fixture.storeFixture.AddObjects(t, []objectstore.TestObject{
			{
				bundle.RelationKeyId:         pbtypes.String("id"),
				bundle.RelationKeySyncStatus: pbtypes.Int64(int64(domain.SpaceSyncStatusError)),
				bundle.RelationKeySyncError:  pbtypes.Int64(int64(domain.SyncErrorNetworkError)),
			},
		})
		space.EXPECT().DoLockedIfNotExists("id", mock.Anything).Return(nil)

		// when
		fixture.statusUpdater.EXPECT().SendUpdate(domain.MakeSyncStatus("spaceId", domain.SpaceSyncStatusSynced, domain.SyncErrorNull))
		err := fixture.updater.updateObjectDetails(&syncStatusDetails{[]string{"id"}, domain.ObjectSyncStatusSynced, domain.SyncErrorNull, "spaceId"}, "id")

		// then
		assert.Nil(t, err)
	})
	t.Run("update sync status and date - object exist in cache", func(t *testing.T) {
		// given
		fixture := newFixture(t)
		space := mock_clientspace.NewMockSpace(t)
		fixture.service.EXPECT().Get(fixture.updater.ctx, "spaceId").Return(space, nil)
		space.EXPECT().DoLockedIfNotExists("id", mock.Anything).Return(ocache.ErrExists)
		space.EXPECT().DoCtx(fixture.updater.ctx, "id", mock.Anything).Return(nil)

		// when
		fixture.statusUpdater.EXPECT().SendUpdate(domain.MakeSyncStatus("spaceId", domain.SpaceSyncStatusSynced, domain.SyncErrorNull))
		err := fixture.updater.updateObjectDetails(&syncStatusDetails{[]string{"id"}, domain.ObjectSyncStatusSynced, domain.SyncErrorNull, "spaceId"}, "id")

		// then
		assert.Nil(t, err)
	})

	t.Run("update sync status and date - file status", func(t *testing.T) {
		// given
		fixture := newFixture(t)
		space := mock_clientspace.NewMockSpace(t)
		fixture.service.EXPECT().Get(fixture.updater.ctx, "spaceId").Return(space, nil)
		fixture.storeFixture.AddObjects(t, []objectstore.TestObject{
			{
				bundle.RelationKeyId:               pbtypes.String("id"),
				bundle.RelationKeyFileBackupStatus: pbtypes.Int64(int64(filesyncstatus.Syncing)),
			},
		})
		space.EXPECT().DoLockedIfNotExists("id", mock.Anything).Return(nil)

		// when
		fixture.statusUpdater.EXPECT().SendUpdate(domain.MakeSyncStatus("spaceId", domain.SpaceSyncStatusSyncing, domain.SyncErrorNull))
		err := fixture.updater.updateObjectDetails(&syncStatusDetails{[]string{"id"}, domain.ObjectSyncStatusSynced, domain.SyncErrorNull, "spaceId"}, "id")

		// then
		assert.Nil(t, err)
	})
	t.Run("update sync status and date - unknown file status", func(t *testing.T) {
		// given
		fixture := newFixture(t)
		space := mock_clientspace.NewMockSpace(t)
		fixture.service.EXPECT().Get(fixture.updater.ctx, "spaceId").Return(space, nil)
		fixture.storeFixture.AddObjects(t, []objectstore.TestObject{
			{
				bundle.RelationKeyId:               pbtypes.String("id"),
				bundle.RelationKeyFileBackupStatus: pbtypes.Int64(int64(filesyncstatus.Unknown)),
			},
		})
		space.EXPECT().DoLockedIfNotExists("id", mock.Anything).Return(nil)

		// when
		fixture.statusUpdater.EXPECT().SendUpdate(domain.MakeSyncStatus("spaceId", domain.SpaceSyncStatusError, domain.SyncErrorNetworkError))
		err := fixture.updater.updateObjectDetails(&syncStatusDetails{[]string{"id"}, domain.ObjectSyncStatusSynced, domain.SyncErrorNull, "spaceId"}, "id")

		// then
		assert.Nil(t, err)
	})
	t.Run("update sync status and date - queued file status", func(t *testing.T) {
		// given
		fixture := newFixture(t)
		space := mock_clientspace.NewMockSpace(t)
		fixture.service.EXPECT().Get(fixture.updater.ctx, "spaceId").Return(space, nil)
		fixture.storeFixture.AddObjects(t, []objectstore.TestObject{
			{
				bundle.RelationKeyId:               pbtypes.String("id"),
				bundle.RelationKeyFileBackupStatus: pbtypes.Int64(int64(filesyncstatus.Queued)),
			},
		})
		space.EXPECT().DoLockedIfNotExists("id", mock.Anything).Return(nil)

		// when
		fixture.statusUpdater.EXPECT().SendUpdate(domain.MakeSyncStatus("spaceId", domain.SpaceSyncStatusSyncing, domain.SyncErrorNull))
		err := fixture.updater.updateObjectDetails(&syncStatusDetails{[]string{"id"}, domain.ObjectSyncStatusSyncing, domain.SyncErrorNull, "spaceId"}, "id")

		// then
		assert.Nil(t, err)
	})
	t.Run("update sync status and date - synced file status", func(t *testing.T) {
		// given
		fixture := newFixture(t)
		space := mock_clientspace.NewMockSpace(t)
		fixture.service.EXPECT().Get(fixture.updater.ctx, "spaceId").Return(space, nil)
		fixture.storeFixture.AddObjects(t, []objectstore.TestObject{
			{
				bundle.RelationKeyId:               pbtypes.String("id"),
				bundle.RelationKeyFileBackupStatus: pbtypes.Int64(int64(filesyncstatus.Synced)),
			},
		})
		space.EXPECT().DoLockedIfNotExists("id", mock.Anything).Return(nil)

		// when
		fixture.statusUpdater.EXPECT().SendUpdate(domain.MakeSyncStatus("spaceId", domain.SpaceSyncStatusSynced, domain.SyncErrorNull))
		err := fixture.updater.updateObjectDetails(&syncStatusDetails{[]string{"id"}, domain.ObjectSyncStatusSynced, domain.SyncErrorNull, "spaceId"}, "id")

		// then
		assert.Nil(t, err)
	})
}

func TestSyncStatusUpdater_Run(t *testing.T) {
	t.Run("run", func(t *testing.T) {
		// given
		fixture := newFixture(t)

		// when
		fixture.service.EXPECT().TechSpaceId().Return("techSpaceId")
		space := mock_clientspace.NewMockSpace(t)
		fixture.service.EXPECT().Get(mock.Anything, mock.Anything).Return(space, nil).Maybe()
		space.EXPECT().DoLockedIfNotExists(mock.Anything, mock.Anything).Return(nil).Maybe()
		space.EXPECT().DoCtx(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
		err := fixture.updater.Run(context.Background())
		fixture.statusUpdater.EXPECT().SendUpdate(mock.Anything).Return().Maybe()
		assert.Nil(t, err)
		fixture.updater.UpdateDetails([]string{"id"}, domain.ObjectSyncStatusSynced, domain.SyncErrorNull, "spaceId")

		// then
		err = fixture.updater.Close(context.Background())
		assert.Nil(t, err)
	})

	t.Run("run 2 time for 1 object", func(t *testing.T) {
		// given
		fixture := newFixture(t)

		// when
		fixture.service.EXPECT().TechSpaceId().Return("techSpaceId").Times(2)
		fixture.updater.UpdateDetails([]string{"id"}, domain.ObjectSyncStatusSynced, domain.SyncErrorNull, "spaceId")
		fixture.updater.UpdateDetails([]string{"id"}, domain.ObjectSyncStatusSyncing, domain.SyncErrorNull, "spaceId")

		// then
		assert.Equal(t, &syncStatusDetails{status: domain.ObjectSyncStatusSyncing, syncError: domain.SyncErrorNull, spaceId: "spaceId"}, fixture.updater.entries["id"])
	})
}

func TestSyncStatusUpdater_setSyncDetails(t *testing.T) {
	t.Run("set smartblock details", func(t *testing.T) {
		// given
		fixture := newFixture(t)

		// when
		err := fixture.updater.setSyncDetails(fixture.sb, domain.ObjectSyncStatusError, domain.SyncErrorNetworkError)
		assert.Nil(t, err)

		// then
		details := fixture.sb.NewState().CombinedDetails().GetFields()
		assert.NotNil(t, details)
		assert.Equal(t, pbtypes.Int64(int64(domain.SpaceSyncStatusError)), details[bundle.RelationKeySyncStatus.String()])
		assert.Equal(t, pbtypes.Int64(int64(domain.SyncErrorNetworkError)), details[bundle.RelationKeySyncError.String()])
		assert.NotNil(t, details[bundle.RelationKeySyncDate.String()])
	})
	t.Run("not set smartblock details, because it doesn't implement interface DetailsSettable", func(t *testing.T) {
		// given
		fixture := newFixture(t)

		// when
		fixture.sb.SetType(coresb.SmartBlockTypePage)
		err := fixture.updater.setSyncDetails(editor.NewMissingObject(fixture.sb), domain.ObjectSyncStatusError, domain.SyncErrorNetworkError)

		// then
		assert.Nil(t, err)
	})
	t.Run("not set smartblock details, because it doesn't need details", func(t *testing.T) {
		// given
		fixture := newFixture(t)

		// when
		fixture.sb.SetType(coresb.SmartBlockTypeHome)
		err := fixture.updater.setSyncDetails(fixture.sb, domain.ObjectSyncStatusError, domain.SyncErrorNetworkError)

		// then
		assert.Nil(t, err)
	})
}

func TestSyncStatusUpdater_isLayoutSuitableForSyncRelations(t *testing.T) {
	t.Run("isLayoutSuitableForSyncRelations - participant details", func(t *testing.T) {
		// given
		fixture := newFixture(t)

		// when
		details := &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyLayout.String(): pbtypes.Float64(float64(model.ObjectType_participant)),
		}}
		isSuitable := fixture.updater.isLayoutSuitableForSyncRelations(details)

		// then
		assert.False(t, isSuitable)
	})

	t.Run("isLayoutSuitableForSyncRelations - basic details", func(t *testing.T) {
		// given
		fixture := newFixture(t)

		// when
		details := &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyLayout.String(): pbtypes.Float64(float64(model.ObjectType_basic)),
		}}
		isSuitable := fixture.updater.isLayoutSuitableForSyncRelations(details)

		// then
		assert.True(t, isSuitable)
	})
}

func newFixture(t *testing.T) *fixture {
	smartTest := smarttest.New("id")
	storeFixture := objectstore.NewStoreFixture(t)
	service := mock_space.NewMockService(t)
	updater := &syncStatusUpdater{
		batcher: mb.New[*syncStatusDetails](0),
		finish:  make(chan struct{}),
		entries: map[string]*syncStatusDetails{},
	}
	statusUpdater := mock_detailsupdater.NewMockSpaceStatusUpdater(t)
	a := &app.App{}
	a.Register(storeFixture).
		Register(testutil.PrepareMock(context.Background(), a, service)).
		Register(testutil.PrepareMock(context.Background(), a, statusUpdater))
	err := updater.Init(a)
	assert.Nil(t, err)
	return &fixture{
		updater:       updater,
		sb:            smartTest,
		storeFixture:  storeFixture,
		service:       service,
		statusUpdater: statusUpdater,
	}
}

type fixture struct {
	sb            *smarttest.SmartTest
	updater       *syncStatusUpdater
	storeFixture  *objectstore.StoreFixture
	service       *mock_space.MockService
	statusUpdater *mock_detailsupdater.MockSpaceStatusUpdater
}
