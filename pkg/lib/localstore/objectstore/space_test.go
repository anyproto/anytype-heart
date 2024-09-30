package objectstore

import (
	"context"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
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

		err := s.UpdateObjectDetails(context.Background(), spaceViewId, &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyId.String():            pbtypes.String(spaceViewId),
			bundle.RelationKeyLayout.String():        pbtypes.Int64(int64(model.ObjectType_spaceView)),
			bundle.RelationKeyTargetSpaceId.String(): pbtypes.String(spaceId),
			bundle.RelationKeyName.String():          pbtypes.String(spaceName),
		}})
		assert.Nil(t, err)

		// when
		name := s.GetSpaceName(spaceId)

		// then
		assert.Equal(t, spaceName, name)
	})

	t.Run("don't have searched space in store", func(t *testing.T) {
		// given
		s := NewStoreFixture(t)

		err := s.UpdateObjectDetails(context.Background(), spaceViewId, &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyId.String():            pbtypes.String(spaceViewId),
			bundle.RelationKeyLayout.String():        pbtypes.Int64(int64(model.ObjectType_spaceView)),
			bundle.RelationKeyTargetSpaceId.String(): pbtypes.String(spaceId),
		}})
		assert.Nil(t, err)

		// when
		name := s.GetSpaceName("not exist")

		// then
		assert.Equal(t, "", name)
	})
}
