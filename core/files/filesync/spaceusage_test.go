package filesync

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	fileproto "github.com/anyproto/any-sync/commonfile/fileproto/fileprotoerr"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files/filestorage/rpcstore/mock_rpcstore"
	"github.com/anyproto/anytype-heart/core/subscription/objectsubscription"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

func TestGetSpace(t *testing.T) {
	const techSpaceId = "tech-space-id"
	const regularSpaceId = "regular-space-id"

	makeSpaceViewParams := objectsubscription.SubscriptionParams[*spaceUsage]{
		SetDetails: func(details *domain.Details) (string, *spaceUsage) {
			spaceId := details.GetString(bundle.RelationKeyTargetSpaceId)
			return spaceId, &spaceUsage{spaceId: spaceId}
		},
		UpdateKeys: func(_ []objectsubscription.RelationKeyValue, cur *spaceUsage) *spaceUsage {
			return cur
		},
		RemoveKeys: func(_ []string, cur *spaceUsage) *spaceUsage {
			return cur
		},
	}

	deletedParams := objectsubscription.SubscriptionParams[struct{}]{
		SetDetails: func(details *domain.Details) (string, struct{}) {
			spaceId := details.GetString(bundle.RelationKeyTargetSpaceId)
			return spaceId, struct{}{}
		},
		UpdateKeys: func(_ []objectsubscription.RelationKeyValue, cur struct{}) struct{} {
			return cur
		},
		RemoveKeys: func(_ []string, cur struct{}) struct{} {
			return cur
		},
	}

	newManager := func(t *testing.T, spaceViews *objectsubscription.ObjectSubscription[*spaceUsage], deletedSpaceViews *objectsubscription.ObjectSubscription[struct{}]) *spaceUsageManager {
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)

		rpcStore := mock_rpcstore.NewMockRpcStore(t)
		rpcStore.EXPECT().SpaceInfo(mock.Anything, mock.Anything).Return(nil, fileproto.ErrForbidden).Maybe()

		updateCh := make(chan updateMessage, 1)
		m := &spaceUsageManager{
			ctx:               ctx,
			ctxCancel:         cancel,
			techSpaceId:       techSpaceId,
			rpcStore:          rpcStore,
			spaceViews:        spaceViews,
			deletedSpaceViews: deletedSpaceViews,
			updateCh:          updateCh,
		}
		m.getTechSpaceUsage = sync.OnceValue(func() *spaceUsage {
			ch := m.setupUpdateCh()
			return newSpaceUsage(ctx, techSpaceId, rpcStore, ch)
		})
		return m
	}

	t.Run("tech space creates dedicated spaceUsage", func(t *testing.T) {
		// given
		spaceViews := objectsubscription.NewFromQueue[*spaceUsage](nil, makeSpaceViewParams, nil)
		deletedSpaceViews := objectsubscription.NewFromQueue[struct{}](nil, deletedParams, nil)
		m := newManager(t, spaceViews, deletedSpaceViews)

		// when
		got, err := m.getSpace(techSpaceId)

		// then
		require.NoError(t, err)
		assert.Equal(t, techSpaceId, got.spaceId)
	})

	t.Run("tech space returns same instance on second call", func(t *testing.T) {
		// given
		spaceViews := objectsubscription.NewFromQueue[*spaceUsage](nil, makeSpaceViewParams, nil)
		deletedSpaceViews := objectsubscription.NewFromQueue[struct{}](nil, deletedParams, nil)
		m := newManager(t, spaceViews, deletedSpaceViews)

		// when
		first, err := m.getSpace(techSpaceId)
		require.NoError(t, err)
		second, err := m.getSpace(techSpaceId)
		require.NoError(t, err)

		// then
		assert.Same(t, first, second)
	})

	t.Run("unknown space returns spaceView not found", func(t *testing.T) {
		// given
		spaceViews := objectsubscription.NewFromQueue[*spaceUsage](nil, makeSpaceViewParams, nil)
		deletedSpaceViews := objectsubscription.NewFromQueue[struct{}](nil, deletedParams, nil)
		m := newManager(t, spaceViews, deletedSpaceViews)

		// when
		_, err := m.getSpace("unknownSpace")

		// then
		require.EqualError(t, err, "spaceView not found")
	})

	t.Run("deleted space returns errSpaceDeleted", func(t *testing.T) {
		// given
		spaceViews := objectsubscription.NewFromQueue[*spaceUsage](nil, makeSpaceViewParams, nil)
		deletedSpaceViews := objectsubscription.NewFromQueue[struct{}](nil, deletedParams, []*domain.Details{
			domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyTargetSpaceId: domain.String("deletedSpace"),
			}),
		})
		m := newManager(t, spaceViews, deletedSpaceViews)

		// when
		_, err := m.getSpace("deletedSpace")

		// then
		require.ErrorIs(t, err, errSpaceDeleted)
	})

	t.Run("regular space found directly", func(t *testing.T) {
		// given
		spaceViews := objectsubscription.NewFromQueue[*spaceUsage](nil, makeSpaceViewParams, []*domain.Details{
			domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyTargetSpaceId: domain.String(regularSpaceId),
			}),
		})
		deletedSpaceViews := objectsubscription.NewFromQueue[struct{}](nil, deletedParams, nil)
		m := newManager(t, spaceViews, deletedSpaceViews)

		// when
		got, err := m.getSpace(regularSpaceId)

		// then
		require.NoError(t, err)
		assert.Equal(t, regularSpaceId, got.spaceId)
	})
}
