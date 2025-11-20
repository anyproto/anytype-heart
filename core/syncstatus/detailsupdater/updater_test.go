package detailsupdater

import (
	"context"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/ocache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/anytype/account/mock_account"
	"github.com/anyproto/anytype-heart/core/block/editor"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/syncstatus/detailsupdater/mock_detailsupdater"
	"github.com/anyproto/anytype-heart/core/syncstatus/filesyncstatus"
	"github.com/anyproto/anytype-heart/core/syncstatus/syncsubscriptions"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/clientspace/mock_clientspace"
	"github.com/anyproto/anytype-heart/space/mock_space"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

type updateTester struct {
	t              *testing.T
	waitCh         chan struct{}
	minEventsCount int
	maxEventsCount int
}

func newUpdateTester(t *testing.T, minEventsCount int, maxEventsCount int) *updateTester {
	return &updateTester{
		t:              t,
		minEventsCount: minEventsCount,
		maxEventsCount: maxEventsCount,
		waitCh:         make(chan struct{}, maxEventsCount),
	}
}

func (t *updateTester) done() {
	t.waitCh <- struct{}{}
}

// wait waits for at least one event up to t.maxEventsCount events
func (t *updateTester) wait() {
	timeout := time.After(1 * time.Second)
	minReceivedTimer := time.After(10 * time.Millisecond)
	var eventsReceived int
	for i := 0; i < t.maxEventsCount; i++ {
		select {
		case <-minReceivedTimer:
			if eventsReceived >= t.minEventsCount {
				return
			}
		case <-t.waitCh:
			eventsReceived++
		case <-timeout:
			t.t.Fatal("timeout")
		}
	}
}

func newUpdateDetailsFixture(t *testing.T) *fixture {
	fx := newFixture(t)
	err := fx.Run(context.Background())
	require.NoError(t, err)
	t.Cleanup(func() {
		err := fx.Close(context.Background())
		require.NoError(t, err)
	})
	return fx
}

func TestSyncStatusUpdater_UpdateDetails(t *testing.T) {
	t.Run("updates to the same object", func(t *testing.T) {
		fx := newUpdateDetailsFixture(t)
		updTester := newUpdateTester(t, 1, 4)

		space := mock_clientspace.NewMockSpace(t)
		fx.spaceService.EXPECT().Get(mock.Anything, "space1").Return(space, nil)
		space.EXPECT().DoLockedIfNotExists(mock.Anything, mock.Anything).Return(ocache.ErrExists).Times(0)
		space.EXPECT().DoCtx(mock.Anything, mock.Anything, mock.Anything).Run(func(ctx context.Context, objectId string, apply func(smartblock.SmartBlock) error) {
			sb := smarttest.New(objectId)
			st := sb.Doc.(*state.State)
			st.SetDetailAndBundledRelation(bundle.RelationKeyResolvedLayout, domain.Int64(int64(model.ObjectType_basic)))
			st.SetDetailAndBundledRelation(bundle.RelationKeySpaceId, domain.String("spaceId"))
			err := apply(sb)
			require.NoError(t, err)

			det := sb.Doc.LocalDetails()
			assert.True(t, det.Has(bundle.RelationKeySyncStatus))
			assert.True(t, det.Has(bundle.RelationKeySyncDate))
			assert.True(t, det.Has(bundle.RelationKeySyncError))

			fx.spaceStatusUpdater.EXPECT().Refresh("space1")

			updTester.done()
		}).Return(nil).Times(0)

		fx.UpdateDetails("id1", domain.ObjectSyncStatusSyncing, "space1")
		fx.UpdateDetails("id1", domain.ObjectSyncStatusError, "space1")
		fx.UpdateDetails("id1", domain.ObjectSyncStatusSyncing, "space1")
		fx.UpdateDetails("id1", domain.ObjectSyncStatusSynced, "space1")

		updTester.wait()
	})

	t.Run("updates to object not in cache", func(t *testing.T) {
		fx := newUpdateDetailsFixture(t)
		updTester := newUpdateTester(t, 1, 1)

		fx.subscriptionService.StoreFixture.AddObjects(t, "space1", []objectstore.TestObject{
			{
				bundle.RelationKeyId:             domain.String("id1"),
				bundle.RelationKeySpaceId:        domain.String("space1"),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
			},
		})

		space := mock_clientspace.NewMockSpace(t)
		fx.spaceService.EXPECT().Get(mock.Anything, "space1").Return(space, nil)
		space.EXPECT().DoLockedIfNotExists(mock.Anything, mock.Anything).Run(func(objectId string, proc func() error) {
			err := proc()
			require.NoError(t, err)

			details, err := fx.objectStore.SpaceIndex("space1").GetDetails(objectId)
			require.NoError(t, err)

			assert.True(t, details.GetInt64(bundle.RelationKeySyncStatus) == int64(domain.ObjectSyncStatusError))
			assert.True(t, details.GetInt64(bundle.RelationKeySyncError) == int64(domain.SyncErrorNull))
			assert.True(t, details.Has(bundle.RelationKeySyncDate))
			updTester.done()
		}).Return(nil).Times(0)

		fx.UpdateDetails("id1", domain.ObjectSyncStatusError, "space1")

		fx.spaceStatusUpdater.EXPECT().Refresh("space1")

		updTester.wait()
	})

	t.Run("updates in file object", func(t *testing.T) {
		t.Run("file backup status limited", func(t *testing.T) {
			fx := newUpdateDetailsFixture(t)
			updTester := newUpdateTester(t, 1, 1)

			space := mock_clientspace.NewMockSpace(t)
			fx.spaceService.EXPECT().Get(mock.Anything, "space1").Return(space, nil)
			space.EXPECT().DoLockedIfNotExists(mock.Anything, mock.Anything).Return(ocache.ErrExists)
			space.EXPECT().DoCtx(mock.Anything, mock.Anything, mock.Anything).Run(func(ctx context.Context, objectId string, apply func(smartblock.SmartBlock) error) {
				sb := smarttest.New(objectId)
				st := sb.Doc.(*state.State)
				st.SetDetailAndBundledRelation(bundle.RelationKeySpaceId, domain.String("spaceId"))
				st.SetDetailAndBundledRelation(bundle.RelationKeyResolvedLayout, domain.Int64(int64(model.ObjectType_file)))
				st.SetDetailAndBundledRelation(bundle.RelationKeyFileBackupStatus, domain.Int64(int64(filesyncstatus.Limited)))
				err := apply(sb)
				require.NoError(t, err)

				det := sb.Doc.LocalDetails()
				assert.True(t, det.GetInt64(bundle.RelationKeySyncStatus) == int64(domain.ObjectSyncStatusError))
				assert.True(t, det.GetInt64(bundle.RelationKeySyncError) == int64(domain.SyncErrorOversized))
				assert.True(t, det.Has(bundle.RelationKeySyncDate))

				fx.spaceStatusUpdater.EXPECT().Refresh("space1")

				updTester.done()
			}).Return(nil)

			fx.UpdateDetails("id2", domain.ObjectSyncStatusSynced, "space1")

			updTester.wait()
		})
		t.Run("prioritize object status", func(t *testing.T) {
			fx := newUpdateDetailsFixture(t)
			updTester := newUpdateTester(t, 1, 1)

			space := mock_clientspace.NewMockSpace(t)
			fx.spaceService.EXPECT().Get(mock.Anything, "space1").Return(space, nil)
			space.EXPECT().DoLockedIfNotExists(mock.Anything, mock.Anything).Return(ocache.ErrExists)
			space.EXPECT().DoCtx(mock.Anything, mock.Anything, mock.Anything).Run(func(ctx context.Context, objectId string, apply func(smartblock.SmartBlock) error) {
				sb := smarttest.New(objectId)
				st := sb.Doc.(*state.State)
				st.SetDetailAndBundledRelation(bundle.RelationKeySpaceId, domain.String("spaceId"))
				st.SetDetailAndBundledRelation(bundle.RelationKeyResolvedLayout, domain.Int64(int64(model.ObjectType_file)))
				st.SetDetailAndBundledRelation(bundle.RelationKeyFileBackupStatus, domain.Int64(int64(filesyncstatus.Synced)))
				err := apply(sb)
				require.NoError(t, err)

				det := sb.Doc.LocalDetails()
				assert.True(t, det.GetInt64(bundle.RelationKeySyncStatus) == int64(domain.ObjectSyncStatusSyncing))
				assert.True(t, det.Has(bundle.RelationKeySyncError))
				assert.True(t, det.Has(bundle.RelationKeySyncDate))

				fx.spaceStatusUpdater.EXPECT().Refresh("space1")

				updTester.done()
			}).Return(nil)

			fx.UpdateDetails("id3", domain.ObjectSyncStatusSyncing, "space1")

			updTester.wait()
		})
	})

	// TODO Test DoLockedIfNotExists
}

func TestSyncStatusUpdater_UpdateSpaceDetails(t *testing.T) {
	fx := newUpdateDetailsFixture(t)
	updTester := newUpdateTester(t, 3, 3)

	fx.subscriptionService.StoreFixture.AddObjects(t, "space1", []objectstore.TestObject{
		{
			bundle.RelationKeyId:             domain.String("id1"),
			bundle.RelationKeySpaceId:        domain.String("space1"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
			bundle.RelationKeySyncStatus:     domain.Int64(int64(domain.ObjectSyncStatusSyncing)),
		},
		{
			bundle.RelationKeyId:             domain.String("id4"),
			bundle.RelationKeySpaceId:        domain.String("space1"),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
			bundle.RelationKeySyncStatus:     domain.Int64(int64(domain.ObjectSyncStatusSyncing)),
		},
	})

	space := mock_clientspace.NewMockSpace(t)
	fx.spaceService.EXPECT().Get(mock.Anything, "space1").Return(space, nil)
	space.EXPECT().DoLockedIfNotExists(mock.Anything, mock.Anything).Return(ocache.ErrExists).Times(0)

	assertUpdate := func(objectId string, status domain.ObjectSyncStatus) {
		space.EXPECT().DoCtx(mock.Anything, objectId, mock.Anything).Run(func(ctx context.Context, objectId string, apply func(smartblock.SmartBlock) error) {
			sb := smarttest.New(objectId)
			st := sb.Doc.(*state.State)
			st.SetDetailAndBundledRelation(bundle.RelationKeySpaceId, domain.String("spaceId"))
			st.SetDetailAndBundledRelation(bundle.RelationKeyResolvedLayout, domain.Int64(int64(model.ObjectType_basic)))
			err := apply(sb)
			require.NoError(t, err)

			det := sb.Doc.LocalDetails()
			assert.True(t, det.GetInt64(bundle.RelationKeySyncStatus) == int64(status))
			assert.True(t, det.Has(bundle.RelationKeySyncDate))
			assert.True(t, det.Has(bundle.RelationKeySyncError))

			fx.spaceStatusUpdater.EXPECT().Refresh("space1")

			updTester.done()
		}).Return(nil).Times(0)
	}

	assertUpdate("id2", domain.ObjectSyncStatusSyncing)
	assertUpdate("id4", domain.ObjectSyncStatusSynced)

	fx.spaceStatusUpdater.EXPECT().UpdateMissingIds("space1", []string{"id3"})
	fx.UpdateSpaceDetails([]string{"id1", "id2"}, []string{"id3"}, "space1")

	fx.spaceStatusUpdater.EXPECT().UpdateMissingIds("space1", []string{"id3"})
	fx.spaceStatusUpdater.EXPECT().Refresh("space1")
	fx.UpdateSpaceDetails([]string{"id1", "id2"}, []string{"id3"}, "space1")

	updTester.wait()
}

func TestSyncStatusUpdater_setSyncDetails(t *testing.T) {
	t.Run("set smartblock details", func(t *testing.T) {
		// given
		fx := newFixture(t)
		sb := smarttest.New("id")
		st := sb.Doc.(*state.State)
		st.SetDetailAndBundledRelation(bundle.RelationKeySpaceId, domain.String("spaceId"))
		// when
		err := fx.setSyncDetails(sb, domain.ObjectSyncStatusError, domain.SyncErrorNetworkError)
		assert.Nil(t, err)

		// then
		details := sb.NewState().CombinedDetails()
		assert.NotNil(t, details)
		assert.Equal(t, domain.Int64(int64(domain.SpaceSyncStatusError)), details.Get(bundle.RelationKeySyncStatus))
		assert.Equal(t, domain.Int64(int64(domain.SyncErrorNetworkError)), details.Get(bundle.RelationKeySyncError))
		assert.True(t, details.Has(bundle.RelationKeySyncDate))
	})
	t.Run("not set smartblock details, because it doesn't implement interface DetailsSettable", func(t *testing.T) {
		// given
		fx := newFixture(t)
		sb := smarttest.New("id")

		// when
		sb.SetType(coresb.SmartBlockTypePage)
		err := fx.setSyncDetails(editor.NewMissingObject(sb), domain.ObjectSyncStatusError, domain.SyncErrorNetworkError)

		// then
		assert.Nil(t, err)
	})
	t.Run("not set smartblock details, because it doesn't need details", func(t *testing.T) {
		// given
		fx := newFixture(t)
		sb := smarttest.New("id")

		// when
		sb.SetType(coresb.SmartBlockTypeHome)
		err := fx.setSyncDetails(sb, domain.ObjectSyncStatusError, domain.SyncErrorNetworkError)

		// then
		assert.Nil(t, err)
	})
}

func TestSyncStatusUpdater_isLayoutSuitableForSyncRelations(t *testing.T) {
	t.Run("isLayoutSuitableForSyncRelations - participant details", func(t *testing.T) {
		// given
		fx := newFixture(t)

		// when
		details := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyResolvedLayout: domain.Float64(float64(model.ObjectType_participant)),
		})
		isSuitable := fx.isLayoutSuitableForSyncRelations(details)

		// then
		assert.False(t, isSuitable)
	})

	t.Run("isLayoutSuitableForSyncRelations - basic details", func(t *testing.T) {
		// given
		fx := newFixture(t)

		// when
		details := domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyResolvedLayout: domain.Float64(float64(model.ObjectType_basic)),
		})
		isSuitable := fx.isLayoutSuitableForSyncRelations(details)

		// then
		assert.True(t, isSuitable)
	})
}

func newFixture(t *testing.T) *fixture {
	service := mock_space.NewMockService(t)
	updater := New()
	statusUpdater := mock_detailsupdater.NewMockSpaceStatusUpdater(t)
	accountService := mock_account.NewMockService(t)
	accountService.EXPECT().AccountID().Return("account1").Maybe()

	syncSub := syncsubscriptions.New()

	ctx := context.Background()

	a := &app.App{}
	subscriptionService := subscription.RegisterSubscriptionService(t, a)
	a.Register(syncSub)
	a.Register(testutil.PrepareMock(ctx, a, service))
	a.Register(testutil.PrepareMock(ctx, a, statusUpdater))
	a.Register(testutil.PrepareMock(ctx, a, accountService))
	err := updater.Init(a)
	require.NoError(t, err)

	err = a.Start(ctx)
	require.NoError(t, err)

	return &fixture{
		syncStatusUpdater:   updater.(*syncStatusUpdater),
		spaceService:        service,
		spaceStatusUpdater:  statusUpdater,
		subscriptionService: subscriptionService,
	}
}

type fixture struct {
	*syncStatusUpdater
	spaceService        *mock_space.MockService
	spaceStatusUpdater  *mock_detailsupdater.MockSpaceStatusUpdater
	subscriptionService *subscription.InternalTestService
}
