package objectcreator

import (
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/mock"

	"github.com/anyproto/anytype-heart/core/relationutils"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/mock_space"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestUpdateSystemRelation(t *testing.T) {
	marketRels := relationutils.Relations{
		{&model.Relation{Key: "id", Revision: 1}},
		{&model.Relation{Key: "description", Revision: 2}},
		{&model.Relation{Key: "lyrics", Revision: 1}},
		{&model.Relation{Key: "isReadonly", Revision: 3}},
	}

	t.Run("system relation is updated if revision is higher", func(t *testing.T) {
		rel := &relationutils.Relation{Relation: &model.Relation{Id: "1", Key: "description", Revision: 1}}
		space := mock_space.NewMockSpace(t)
		space.EXPECT().Do(mock.Anything, mock.Anything).Times(1).Return(nil)

		reviseSystemRelation(space, rel, marketRels)
	})

	t.Run("system relation is updated if no revision is set", func(t *testing.T) {
		rel := &relationutils.Relation{Relation: &model.Relation{Id: "id", Key: "id"}}
		space := mock_space.NewMockSpace(t)
		space.EXPECT().Do(mock.Anything, mock.Anything).Times(1).Return(nil)

		reviseSystemRelation(space, rel, marketRels)
	})

	t.Run("custom relation is not updated", func(t *testing.T) {
		rel := &relationutils.Relation{Relation: &model.Relation{Key: "custom"}}
		space := mock_space.NewMockSpace(t) // if unexpected space.Do will be called, test will fail

		reviseSystemRelation(space, rel, marketRels)
	})

	t.Run("non system relation is not updated", func(t *testing.T) {
		rel := &relationutils.Relation{Relation: &model.Relation{Key: "lyrics"}}
		space := mock_space.NewMockSpace(t) // if unexpected space.Do will be called, test will fail

		reviseSystemRelation(space, rel, marketRels)
	})

	t.Run("system relation with same revision is not updated", func(t *testing.T) {
		rel := &relationutils.Relation{Relation: &model.Relation{Key: "isReadonly", Revision: 3}}
		space := mock_space.NewMockSpace(t) // if unexpected space.Do will be called, test will fail

		reviseSystemRelation(space, rel, marketRels)
	})
}

func TestUpdateSystemObjectType(t *testing.T) {
	marketTypes := map[string]*types.Struct{
		"_otnote":    {Fields: map[string]*types.Value{revisionKey: pbtypes.Int64(3)}},
		"_otpage":    {Fields: map[string]*types.Value{revisionKey: pbtypes.Int64(2)}},
		"_otcontact": {Fields: map[string]*types.Value{revisionKey: pbtypes.Int64(1)}},
	}

	t.Run("system object type is updated if revision is higher", func(t *testing.T) {
		objectType := &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyRevision.String():     pbtypes.Int64(1),
			bundle.RelationKeySourceObject.String(): pbtypes.String("_otnote"),
			bundle.RelationKeyUniqueKey.String():    pbtypes.String("ot-note"),
		}}
		space := mock_space.NewMockSpace(t)
		space.EXPECT().Do(mock.Anything, mock.Anything).Times(1).Return(nil)

		reviseSystemObjectType(space, objectType, marketTypes)
	})

	t.Run("system object type is updated if no revision is set", func(t *testing.T) {
		objectType := &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeySourceObject.String(): pbtypes.String("_otpage"),
			bundle.RelationKeyUniqueKey.String():    pbtypes.String("ot-page"),
		}}
		space := mock_space.NewMockSpace(t)
		space.EXPECT().Do(mock.Anything, mock.Anything).Times(1).Return(nil)

		reviseSystemObjectType(space, objectType, marketTypes)
	})

	t.Run("custom object type is not updated", func(t *testing.T) {
		objectType := &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyUniqueKey.String(): pbtypes.String("ot-kitty"),
		}}
		space := mock_space.NewMockSpace(t) // if unexpected space.Do will be called, test will fail

		reviseSystemObjectType(space, objectType, marketTypes)
	})

	t.Run("non system object type is not updated", func(t *testing.T) {
		objectType := &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeySourceObject.String(): pbtypes.String("_otcontact"),
			bundle.RelationKeyUniqueKey.String():    pbtypes.String("ot-contact"),
		}}
		space := mock_space.NewMockSpace(t) // if unexpected space.Do will be called, test will fail

		reviseSystemObjectType(space, objectType, marketTypes)
	})

	t.Run("system object type with same revision is not updated", func(t *testing.T) {
		objectType := &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyRevision.String():     pbtypes.Int64(3),
			bundle.RelationKeySourceObject.String(): pbtypes.String("_otnote"),
			bundle.RelationKeyUniqueKey.String():    pbtypes.String("ot-note"),
		}}
		space := mock_space.NewMockSpace(t) // if unexpected space.Do will be called, test will fail

		reviseSystemObjectType(space, objectType, marketTypes)
	})
}
