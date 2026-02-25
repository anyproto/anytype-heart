package filesync

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
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

	t.Run("tech space returns any available space", func(t *testing.T) {
		// given
		spaceViews := objectsubscription.NewFromQueue[*spaceUsage](nil, makeSpaceViewParams, []*domain.Details{
			domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyTargetSpaceId: domain.String(regularSpaceId),
			}),
		})
		deletedSpaceViews := objectsubscription.NewFromQueue[struct{}](nil, deletedParams, nil)

		m := &spaceUsageManager{
			techSpaceId:       techSpaceId,
			spaceViews:        spaceViews,
			deletedSpaceViews: deletedSpaceViews,
		}

		// when
		got, err := m.getSpace(techSpaceId)

		// then
		require.NoError(t, err)
		assert.Equal(t, regularSpaceId, got.spaceId)
	})

	t.Run("tech space returns error when no spaces available", func(t *testing.T) {
		// given
		spaceViews := objectsubscription.NewFromQueue[*spaceUsage](nil, makeSpaceViewParams, nil)
		deletedSpaceViews := objectsubscription.NewFromQueue[struct{}](nil, deletedParams, nil)

		m := &spaceUsageManager{
			techSpaceId:       techSpaceId,
			spaceViews:        spaceViews,
			deletedSpaceViews: deletedSpaceViews,
		}

		// when
		_, err := m.getSpace(techSpaceId)

		// then
		require.EqualError(t, err, "no spaces available")
	})

	t.Run("unknown space returns spaceView not found", func(t *testing.T) {
		// given
		spaceViews := objectsubscription.NewFromQueue[*spaceUsage](nil, makeSpaceViewParams, nil)
		deletedSpaceViews := objectsubscription.NewFromQueue[struct{}](nil, deletedParams, nil)

		m := &spaceUsageManager{
			techSpaceId:       techSpaceId,
			spaceViews:        spaceViews,
			deletedSpaceViews: deletedSpaceViews,
		}

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

		m := &spaceUsageManager{
			techSpaceId:       techSpaceId,
			spaceViews:        spaceViews,
			deletedSpaceViews: deletedSpaceViews,
		}

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

		m := &spaceUsageManager{
			techSpaceId:       techSpaceId,
			spaceViews:        spaceViews,
			deletedSpaceViews: deletedSpaceViews,
		}

		// when
		got, err := m.getSpace(regularSpaceId)

		// then
		require.NoError(t, err)
		assert.Equal(t, regularSpaceId, got.spaceId)
	})
}
