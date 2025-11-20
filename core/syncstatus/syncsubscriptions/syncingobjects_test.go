package syncsubscriptions

import (
	"context"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/anytype/account/mock_account"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/syncstatus/filesyncstatus"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

const testAccountId = "account1"

type fixture struct {
	ctx context.Context
	SyncSubscriptions
	subService *subscription.InternalTestService
}

func newFixture(t *testing.T) *fixture {
	a := new(app.App)
	subService := subscription.NewInternalTestService(t)
	accountService := mock_account.NewMockService(t)
	accountService.EXPECT().AccountID().Return(testAccountId)

	ctx := context.Background()
	a.Register(subService)
	a.Register(testutil.PrepareMock(ctx, a, accountService))

	s := New()
	err := s.Init(a)
	require.NoError(t, err)

	return &fixture{ctx: ctx, SyncSubscriptions: s, subService: subService}
}

func TestCount(t *testing.T) {
	t.Run("syncing objects count", func(t *testing.T) {
		spaceId := "space1"
		fx := newFixture(t)
		fx.subService.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:         domain.String("1"),
				bundle.RelationKeyName:       domain.String("1"),
				bundle.RelationKeySyncStatus: domain.Int64(domain.ObjectSyncStatusSyncing),
			},
			{
				bundle.RelationKeyId:         domain.String("2"),
				bundle.RelationKeyName:       domain.String("2"),
				bundle.RelationKeySyncStatus: domain.Int64(domain.ObjectSyncStatusError),
			},
			{
				bundle.RelationKeyId:         domain.String("4"),
				bundle.RelationKeyName:       domain.String("4"),
				bundle.RelationKeySyncStatus: domain.Int64(domain.ObjectSyncStatusQueued),
			},
		})
		err := fx.Run(fx.ctx)
		require.NoError(t, err)

		syncing, err := fx.GetSubscription(spaceId)
		require.NoError(t, err)

		cnt := syncing.SyncingObjectsCount([]string{"1", "2", "3"})
		require.Equal(t, 4, cnt)
	})

	t.Run("uploading files count", func(t *testing.T) {
		spaceId := "space1"
		fx := newFixture(t)
		fx.subService.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:               domain.String("1"),
				bundle.RelationKeyName:             domain.String("1"),
				bundle.RelationKeyResolvedLayout:   domain.Int64(model.ObjectType_image),
				bundle.RelationKeyFileBackupStatus: domain.Int64(filesyncstatus.Synced),
				bundle.RelationKeyCreator:          domain.String(testAccountId),
			},
			{
				bundle.RelationKeyId:               domain.String("2"),
				bundle.RelationKeyName:             domain.String("2"),
				bundle.RelationKeyResolvedLayout:   domain.Int64(model.ObjectType_file),
				bundle.RelationKeyFileBackupStatus: domain.Int64(filesyncstatus.Syncing),
				bundle.RelationKeyCreator:          domain.String(testAccountId),
			},
			{
				bundle.RelationKeyId:               domain.String("3"),
				bundle.RelationKeyName:             domain.String("3"),
				bundle.RelationKeyResolvedLayout:   domain.Int64(model.ObjectType_pdf),
				bundle.RelationKeyFileBackupStatus: domain.Int64(filesyncstatus.Queued),
				bundle.RelationKeyCreator:          domain.String(testAccountId),
			},
			{
				bundle.RelationKeyId:               domain.String("4"),
				bundle.RelationKeyName:             domain.String("4"),
				bundle.RelationKeyResolvedLayout:   domain.Int64(model.ObjectType_audio),
				bundle.RelationKeyFileBackupStatus: domain.Int64(filesyncstatus.Syncing),
				bundle.RelationKeyCreator:          domain.String("account2"),
			},
		})

		err := fx.Run(fx.ctx)
		require.NoError(t, err)

		syncing, err := fx.GetSubscription(spaceId)
		require.NoError(t, err)

		cnt := syncing.UploadingFilesCount()
		require.Equal(t, 2, cnt)
	})
}
