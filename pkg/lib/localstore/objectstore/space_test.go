package objectstore

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestGetSpaceName(t *testing.T) {
	spaceId := "id"
	spaceViewId := "spaceViewId"
	spaceName := "Test"

	t.Run("no space find", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)

		// when
		name := s.GetSpaceName(spaceId)

		// then
		assert.Equal(t, "", name)
	})

	t.Run("find space with given name", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)

		err := s.SpaceIndex(s.techSpaceId).UpdateObjectDetails(context.Background(), spaceViewId, domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:            domain.String(spaceViewId),
			bundle.RelationKeyLayout:        domain.Int64(int64(model.ObjectType_spaceView)),
			bundle.RelationKeyTargetSpaceId: domain.String(spaceId),
			bundle.RelationKeyName:          domain.String(spaceName),
		}))
		assert.Nil(t, err)

		// when
		name := s.GetSpaceName(spaceId)

		// then
		assert.Equal(t, spaceName, name)
	})

	t.Run("don't have searched space in store", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)

		err := s.SpaceIndex(s.techSpaceId).UpdateObjectDetails(context.Background(), spaceViewId, domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:            domain.String(spaceViewId),
			bundle.RelationKeyLayout:        domain.Int64(int64(model.ObjectType_spaceView)),
			bundle.RelationKeyTargetSpaceId: domain.String(spaceId),
		}))
		assert.Nil(t, err)

		// when
		name := s.GetSpaceName("not exist")

		// then
		assert.Equal(t, "", name)
	})
}
