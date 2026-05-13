package objectstore

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestGetSpaceViewDetails(t *testing.T) {
	spaceId := "id"
	spaceViewId := "spaceViewId"

	t.Run("no space found returns error", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)

		// when
		got, err := s.GetSpaceViewDetails(spaceId)

		// then
		assert.Error(t, err)
		assert.Nil(t, got)
	})

	t.Run("space with name and ux type", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)

		err := s.SpaceIndex(s.techSpaceId).UpdateObjectDetails(context.Background(), spaceViewId, domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:             domain.String(spaceViewId),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_spaceView)),
			bundle.RelationKeyTargetSpaceId:  domain.String(spaceId),
			bundle.RelationKeyName:           domain.String("Test Space"),
			bundle.RelationKeySpaceUxType:    domain.Int64(int64(model.SpaceUxType_OneToOne)),
		}))
		assert.Nil(t, err)

		// when
		got, err := s.GetSpaceViewDetails(spaceId)

		// then
		assert.NoError(t, err)
		assert.Equal(t, "Test Space", got.GetString(bundle.RelationKeyName))
		assert.Equal(t, int64(model.SpaceUxType_OneToOne), got.GetInt64(bundle.RelationKeySpaceUxType))
	})

	t.Run("wrong spaceId returns error", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)

		err := s.SpaceIndex(s.techSpaceId).UpdateObjectDetails(context.Background(), spaceViewId, domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId:             domain.String(spaceViewId),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_spaceView)),
			bundle.RelationKeyTargetSpaceId:  domain.String(spaceId),
			bundle.RelationKeyName:           domain.String("Test Space"),
		}))
		assert.Nil(t, err)

		// when
		got, err := s.GetSpaceViewDetails("not exist")

		// then
		assert.Error(t, err)
		assert.Nil(t, got)
	})
}

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
			bundle.RelationKeyId:             domain.String(spaceViewId),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_spaceView)),
			bundle.RelationKeyTargetSpaceId:  domain.String(spaceId),
			bundle.RelationKeyName:           domain.String(spaceName),
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
			bundle.RelationKeyId:             domain.String(spaceViewId),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_spaceView)),
			bundle.RelationKeyTargetSpaceId:  domain.String(spaceId),
		}))
		assert.Nil(t, err)

		// when
		name := s.GetSpaceName("not exist")

		// then
		assert.Equal(t, "", name)
	})
}
