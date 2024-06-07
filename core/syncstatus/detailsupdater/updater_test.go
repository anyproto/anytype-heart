package detailsupdater

import (
	"context"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/ocache"
	"github.com/cheggaaa/mb/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/anyproto/anytype-heart/core/block/cache/mock_cache"
	"github.com/anyproto/anytype-heart/core/block/editor"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/syncstatus/detailsupdater/mock_detailsupdater"
	"github.com/anyproto/anytype-heart/core/syncstatus/filesyncstatus"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
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
				bundle.RelationKeySyncStatus: pbtypes.Int64(int64(domain.Synced)),
				bundle.RelationKeySyncError:  pbtypes.Int64(int64(domain.Null)),
			},
		})

		// when
		err := fixture.updater.updateDetails(&syncStatusDetails{"id", domain.Synced, domain.Null, "spaceId"})
		assert.Nil(t, err)

		// then
		fixture.service.AssertNotCalled(t, "Get")
	})
	t.Run("update sync status and date - details exist in store", func(t *testing.T) {
		// given
		fixture := newFixture(t)
		space := mock_clientspace.NewMockSpace(t)
		fixture.service.EXPECT().Get(context.Background(), "spaceId").Return(space, nil)
		fixture.storeFixture.AddObjects(t, []objectstore.TestObject{
			{
				bundle.RelationKeyId: pbtypes.String("id"),
			},
		})
		space.EXPECT().DoLockedIfNotExists("id", mock.Anything).Return(nil)

		// when
		fixture.statusUpdater.EXPECT().SendUpdate(domain.MakeSyncStatus("spaceId", domain.Synced, domain.Null, domain.Objects))
		err := fixture.updater.updateDetails(&syncStatusDetails{"id", domain.Synced, domain.Null, "spaceId"})
		assert.Nil(t, err)

		// then
		assert.Nil(t, err)
	})
	t.Run("update sync status and date - object not exist in cache", func(t *testing.T) {
		// given
		fixture := newFixture(t)
		space := mock_clientspace.NewMockSpace(t)
		fixture.service.EXPECT().Get(context.Background(), "spaceId").Return(space, nil)
		fixture.storeFixture.AddObjects(t, []objectstore.TestObject{
			{
				bundle.RelationKeyId:         pbtypes.String("id"),
				bundle.RelationKeySyncStatus: pbtypes.Int64(int64(domain.Error)),
				bundle.RelationKeySyncError:  pbtypes.Int64(int64(domain.NetworkError)),
			},
		})
		space.EXPECT().DoLockedIfNotExists("id", mock.Anything).Return(nil)

		// when
		fixture.statusUpdater.EXPECT().SendUpdate(domain.MakeSyncStatus("spaceId", domain.Synced, domain.Null, domain.Objects))
		err := fixture.updater.updateDetails(&syncStatusDetails{"id", domain.Synced, domain.Null, "spaceId"})
		assert.Nil(t, err)

		// then
		assert.Nil(t, err)
	})
	t.Run("update sync status and date - object exist in cache", func(t *testing.T) {
		// given
		fixture := newFixture(t)
		space := mock_clientspace.NewMockSpace(t)
		fixture.service.EXPECT().Get(context.Background(), "spaceId").Return(space, nil)
		space.EXPECT().DoLockedIfNotExists("id", mock.Anything).Return(ocache.ErrExists)
		space.EXPECT().Do("id", mock.Anything).Return(nil)

		// when
		fixture.statusUpdater.EXPECT().SendUpdate(domain.MakeSyncStatus("spaceId", domain.Synced, domain.Null, domain.Objects))
		err := fixture.updater.updateDetails(&syncStatusDetails{"id", domain.Synced, domain.Null, "spaceId"})
		assert.Nil(t, err)

		// then
		assert.Nil(t, err)
	})

	t.Run("update sync status and date - file status", func(t *testing.T) {
		// given
		fixture := newFixture(t)
		space := mock_clientspace.NewMockSpace(t)
		fixture.service.EXPECT().Get(context.Background(), "spaceId").Return(space, nil)
		fixture.storeFixture.AddObjects(t, []objectstore.TestObject{
			{
				bundle.RelationKeyId:               pbtypes.String("id"),
				bundle.RelationKeyFileBackupStatus: pbtypes.Int64(int64(filesyncstatus.Syncing)),
			},
		})
		space.EXPECT().DoLockedIfNotExists("id", mock.Anything).Return(nil)

		// when
		fixture.statusUpdater.EXPECT().SendUpdate(domain.MakeSyncStatus("spaceId", domain.Syncing, domain.Null, domain.Objects))
		err := fixture.updater.updateDetails(&syncStatusDetails{"id", domain.Synced, domain.Null, "spaceId"})
		assert.Nil(t, err)

		// then
		assert.Nil(t, err)
	})
}

func TestSyncStatusUpdater_Run(t *testing.T) {
	t.Run("run", func(t *testing.T) {
		// given
		fixture := newFixture(t)

		// when
		err := fixture.updater.Run(context.Background())
		assert.Nil(t, err)
		fixture.updater.UpdateDetails("id", domain.Synced, domain.Null, "spaceId")

		// then
		err = fixture.updater.Close(context.Background())
		assert.Nil(t, err)
	})
}

func TestSyncStatusUpdater_setSyncDetails(t *testing.T) {
	t.Run("set smartblock details", func(t *testing.T) {
		// given
		fixture := newFixture(t)

		// when
		err := fixture.updater.setSyncDetails(fixture.sb, domain.Error, domain.NetworkError)
		assert.Nil(t, err)

		// then
		details := fixture.sb.NewState().CombinedDetails().GetFields()
		assert.NotNil(t, details)
		assert.Equal(t, pbtypes.Int64(int64(domain.Error)), details[bundle.RelationKeySyncStatus.String()])
		assert.Equal(t, pbtypes.Int64(int64(domain.NetworkError)), details[bundle.RelationKeySyncError.String()])
		assert.NotNil(t, details[bundle.RelationKeySyncDate.String()])
	})
	t.Run("not set smartblock details, because it doesn't implement interface DetailsSettable", func(t *testing.T) {
		// given
		fixture := newFixture(t)

		// when
		fixture.sb.SetType(coresb.SmartBlockTypePage)
		err := fixture.updater.setSyncDetails(editor.NewMissingObject(fixture.sb), domain.Error, domain.NetworkError)

		// then
		assert.Nil(t, err)
	})
	t.Run("not set smartblock details, because it doesn't need details", func(t *testing.T) {
		// given
		fixture := newFixture(t)

		// when
		fixture.sb.SetType(coresb.SmartBlockTypeHome)
		err := fixture.updater.setSyncDetails(fixture.sb, domain.Error, domain.NetworkError)

		// then
		assert.Nil(t, err)
	})
}

func newFixture(t *testing.T) *fixture {
	objectGetter := mock_cache.NewMockObjectGetterComponent(t)
	smartTest := smarttest.New("id")
	storeFixture := objectstore.NewStoreFixture(t)
	service := mock_space.NewMockService(t)
	updater := &syncStatusUpdater{batcher: mb.New[*syncStatusDetails](0), finish: make(chan struct{})}
	statusUpdater := mock_detailsupdater.NewMockSpaceStatusUpdater(t)
	a := &app.App{}
	a.Register(testutil.PrepareMock(context.Background(), a, objectGetter)).
		Register(storeFixture).
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
