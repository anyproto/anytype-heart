package objectcreator

import (
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/mock"

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/space/mock_space"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestUpdateSystemObject(t *testing.T) {
	marketObjects := map[string]*types.Struct{
		"_otnote":        {Fields: map[string]*types.Value{revisionKey: pbtypes.Int64(3)}},
		"_otpage":        {Fields: map[string]*types.Value{revisionKey: pbtypes.Int64(2)}},
		"_otcontact":     {Fields: map[string]*types.Value{revisionKey: pbtypes.Int64(1)}},
		"_brid":          {Fields: map[string]*types.Value{revisionKey: pbtypes.Int64(1)}},
		"_brdescription": {Fields: map[string]*types.Value{revisionKey: pbtypes.Int64(2)}},
		"_brlyrics":      {Fields: map[string]*types.Value{revisionKey: pbtypes.Int64(1)}},
		"_brisReadonly":  {Fields: map[string]*types.Value{revisionKey: pbtypes.Int64(3)}},
	}

	t.Run("system object type is updated if revision is higher", func(t *testing.T) {
		objectType := &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyRevision.String():     pbtypes.Int64(1),
			bundle.RelationKeySourceObject.String(): pbtypes.String("_otnote"),
			bundle.RelationKeyUniqueKey.String():    pbtypes.String("ot-note"),
		}}
		space := mock_space.NewMockSpace(t)
		space.EXPECT().Do(mock.Anything, mock.Anything).Times(1).Return(nil)

		reviseSystemObject(space, objectType, marketObjects)
	})

	t.Run("system object type is updated if no revision is set", func(t *testing.T) {
		objectType := &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeySourceObject.String(): pbtypes.String("_otpage"),
			bundle.RelationKeyUniqueKey.String():    pbtypes.String("ot-page"),
		}}
		space := mock_space.NewMockSpace(t)
		space.EXPECT().Do(mock.Anything, mock.Anything).Times(1).Return(nil)

		reviseSystemObject(space, objectType, marketObjects)
	})

	t.Run("custom object type is not updated", func(t *testing.T) {
		objectType := &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyUniqueKey.String(): pbtypes.String("ot-kitty"),
		}}
		space := mock_space.NewMockSpace(t) // if unexpected space.Do will be called, test will fail

		reviseSystemObject(space, objectType, marketObjects)
	})

	t.Run("non system object type is not updated", func(t *testing.T) {
		objectType := &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeySourceObject.String(): pbtypes.String("_otcontact"),
			bundle.RelationKeyUniqueKey.String():    pbtypes.String("ot-contact"),
		}}
		space := mock_space.NewMockSpace(t) // if unexpected space.Do will be called, test will fail

		reviseSystemObject(space, objectType, marketObjects)
	})

	t.Run("system object type with same revision is not updated", func(t *testing.T) {
		objectType := &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyRevision.String():     pbtypes.Int64(3),
			bundle.RelationKeySourceObject.String(): pbtypes.String("_otnote"),
			bundle.RelationKeyUniqueKey.String():    pbtypes.String("ot-note"),
		}}
		space := mock_space.NewMockSpace(t) // if unexpected space.Do will be called, test will fail

		reviseSystemObject(space, objectType, marketObjects)
	})

	t.Run("system relation is updated if revision is higher", func(t *testing.T) {
		rel := &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyRevision.String():     pbtypes.Int64(1),
			bundle.RelationKeySourceObject.String(): pbtypes.String("_brdescription"),
			bundle.RelationKeyUniqueKey.String():    pbtypes.String("rel-description"),
		}}
		space := mock_space.NewMockSpace(t)
		space.EXPECT().Do(mock.Anything, mock.Anything).Times(1).Return(nil)

		reviseSystemObject(space, rel, marketObjects)
	})

	t.Run("system relation is updated if no revision is set", func(t *testing.T) {
		rel := &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeySourceObject.String(): pbtypes.String("_brid"),
			bundle.RelationKeyUniqueKey.String():    pbtypes.String("rel-id"),
		}}
		space := mock_space.NewMockSpace(t)
		space.EXPECT().Do(mock.Anything, mock.Anything).Times(1).Return(nil)

		reviseSystemObject(space, rel, marketObjects)
	})

	t.Run("custom relation is not updated", func(t *testing.T) {
		rel := &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyUniqueKey.String(): pbtypes.String("rel-custom"),
		}}
		space := mock_space.NewMockSpace(t) // if unexpected space.Do will be called, test will fail

		reviseSystemObject(space, rel, marketObjects)
	})

	t.Run("non system relation is not updated", func(t *testing.T) {
		rel := &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyRevision.String():     pbtypes.Int64(1),
			bundle.RelationKeySourceObject.String(): pbtypes.String("_brlyrics"),
			bundle.RelationKeyUniqueKey.String():    pbtypes.String("rel-lyrics"),
		}}
		space := mock_space.NewMockSpace(t) // if unexpected space.Do will be called, test will fail

		reviseSystemObject(space, rel, marketObjects)
	})

	t.Run("system relation with same revision is not updated", func(t *testing.T) {
		rel := &types.Struct{Fields: map[string]*types.Value{
			bundle.RelationKeyRevision.String():     pbtypes.Int64(3),
			bundle.RelationKeySourceObject.String(): pbtypes.String("_brisReadonly"),
			bundle.RelationKeyUniqueKey.String():    pbtypes.String("rel-isReadonly"),
		}}
		space := mock_space.NewMockSpace(t) // if unexpected space.Do will be called, test will fail

		reviseSystemObject(space, rel, marketObjects)
	})
}
