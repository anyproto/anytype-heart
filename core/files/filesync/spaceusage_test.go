package filesync

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestGetSpace(t *testing.T) {
	t.Run("regular space found directly", func(t *testing.T) {
		// given
		fx := newFixture(t, 1024*1024)
		defer fx.Finish(t)

		// when
		got, err := fx.limitManager.getSpace("space1")

		// then
		require.NoError(t, err)
		assert.Equal(t, "space1", got.spaceId)
	})

	t.Run("tech space creates dedicated spaceUsage", func(t *testing.T) {
		// given
		fx := newFixture(t, 1024*1024)
		defer fx.Finish(t)

		// when
		got, err := fx.limitManager.getSpace(techSpaceId)

		// then
		require.NoError(t, err)
		assert.Equal(t, techSpaceId, got.spaceId)
	})

	t.Run("tech space returns same instance on second call", func(t *testing.T) {
		// given
		fx := newFixture(t, 1024*1024)
		defer fx.Finish(t)

		// when
		first, err := fx.limitManager.getSpace(techSpaceId)
		require.NoError(t, err)
		second, err := fx.limitManager.getSpace(techSpaceId)
		require.NoError(t, err)

		// then
		assert.Same(t, first, second)
	})

	t.Run("unknown space returns spaceView not found", func(t *testing.T) {
		// given
		fx := newFixture(t, 1024*1024)
		defer fx.Finish(t)

		// when
		_, err := fx.limitManager.getSpace("unknownSpace")

		// then
		require.EqualError(t, err, "spaceView not found")
	})

	t.Run("deleted space returns errSpaceDeleted", func(t *testing.T) {
		// given
		fx := newFixtureNotStarted(t, 1024*1024)
		fx.subService.AddObjects(t, techSpaceId, []spaceindex.TestObject{
			{
				bundle.RelationKeyId:                 domain.String("spaceViewDeleted"),
				bundle.RelationKeyTargetSpaceId:      domain.String("deletedSpace"),
				bundle.RelationKeyResolvedLayout:     domain.Int64(model.ObjectType_spaceView),
				bundle.RelationKeySpaceAccountStatus: domain.Int64(int64(model.SpaceStatus_SpaceDeleted)),
			},
		})
		require.NoError(t, fx.a.Start(ctx))
		defer fx.Finish(t)

		// when
		_, err := fx.limitManager.getSpace("deletedSpace")

		// then
		require.ErrorIs(t, err, errSpaceDeleted)
	})
}
