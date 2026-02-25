package filesync

import (
	"context"
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

const techSpaceId = "tech-space-id"
const regularSpaceId = "regular-space-id"

func newManagerFixture(t *testing.T, initialSpaceViewDetails, initialDeletedSpacesDetails []*domain.Details) *spaceUsageManager {
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

	spaceViews := objectsubscription.NewFromQueue[*spaceUsage](nil, makeSpaceViewParams, initialSpaceViewDetails)
	deletedSpaceViews := objectsubscription.NewFromQueue[struct{}](nil, deletedParams, initialDeletedSpacesDetails)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	rpcStore := mock_rpcstore.NewMockRpcStore(t)
	rpcStore.EXPECT().SpaceInfo(mock.Anything, mock.Anything).Return(nil, fileproto.ErrForbidden).Maybe()

	return &spaceUsageManager{
		ctx:               ctx,
		ctxCancel:         cancel,
		techSpaceId:       techSpaceId,
		rpcStore:          rpcStore,
		spaceViews:        spaceViews,
		deletedSpaceViews: deletedSpaceViews,
		updateCh:          make(chan updateMessage, 1),
	}
}

func TestGetSpace(t *testing.T) {
	t.Run("tech space creates dedicated spaceUsage", func(t *testing.T) {
		// given
		m := newManagerFixture(t, nil, nil)

		// when
		got, err := m.getSpace(techSpaceId)

		// then
		require.NoError(t, err)
		assert.Equal(t, techSpaceId, got.spaceId)
	})

	t.Run("tech space returns same instance on second call", func(t *testing.T) {
		// given
		m := newManagerFixture(t, nil, nil)

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
		m := newManagerFixture(t, nil, nil)

		// when
		_, err := m.getSpace("unknownSpace")

		// then
		require.EqualError(t, err, "spaceView not found")
	})

	t.Run("deleted space returns errSpaceDeleted", func(t *testing.T) {
		// given
		m := newManagerFixture(t, nil, []*domain.Details{
			domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyTargetSpaceId: domain.String("deletedSpace"),
			}),
		})

		// when
		_, err := m.getSpace("deletedSpace")

		// then
		require.ErrorIs(t, err, errSpaceDeleted)
	})

	t.Run("regular space found directly", func(t *testing.T) {
		// given
		m := newManagerFixture(t, []*domain.Details{
			domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyTargetSpaceId: domain.String(regularSpaceId),
			}),
		}, nil)

		// when
		got, err := m.getSpace(regularSpaceId)

		// then
		require.NoError(t, err)
		assert.Equal(t, regularSpaceId, got.spaceId)
	})
}
