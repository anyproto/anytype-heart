package relation

import (
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/relation/relationutils"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/mock_core"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/tests/testutil"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type fixture struct {
	objectStore *objectstore.StoreFixture

	Service
}

func newFixture(t *testing.T) *fixture {
	objectStore := objectstore.NewStoreFixture(t)
	coreService := mock_core.NewMockService(t)

	a := new(app.App)
	a.Register(objectStore)
	a.Register(testutil.PrepareMock(a, coreService))

	s := New()
	err := s.Init(a)
	require.NoError(t, err)
	return &fixture{
		Service:     s,
		objectStore: objectStore,
	}
}

func TestGetObjectType(t *testing.T) {
	t.Run("get bundled type", func(t *testing.T) {
		s := newFixture(t)

		id := bundle.TypeKeyTask.BundledURL()
		got, err := s.GetObjectType(bundle.TypeKeyTask.BundledURL())
		require.NoError(t, err)

		want := bundle.MustGetType(bundle.TypeKeyTask)
		assert.Equal(t, want, got)
		ok, err := s.HasObjectType(id)
		require.NoError(t, err)
		assert.True(t, ok)
	})

	t.Run("with object is not type expect error", func(t *testing.T) {
		s := newFixture(t)

		id := "id1"
		obj := objectstore.TestObject{
			bundle.RelationKeyId:   pbtypes.String(id),
			bundle.RelationKeyType: pbtypes.String(bundle.TypeKeyNote.URL()),
		}
		s.objectStore.AddObjects(t, []objectstore.TestObject{obj})

		_, err := s.GetObjectType(id)
		require.Error(t, err)
		ok, err := s.HasObjectType(id)
		require.NoError(t, err)
		assert.False(t, ok)
	})

	t.Run("with object is type", func(t *testing.T) {
		// Given
		s := newFixture(t)

		id := "id1"
		relationID := "derivedFrom(assignee)"
		uniqueKey, err := domain.NewUniqueKey(model.SmartBlockType_STType, "note")
		require.NoError(t, err)
		obj := objectstore.TestObject{
			bundle.RelationKeyId:                   pbtypes.String(id),
			bundle.RelationKeyType:                 pbtypes.String(bundle.TypeKeyObjectType.URL()),
			bundle.RelationKeyName:                 pbtypes.String("my note"),
			bundle.RelationKeyRecommendedRelations: pbtypes.StringList([]string{relationID}),
			bundle.RelationKeyRecommendedLayout:    pbtypes.Int64(int64(model.ObjectType_note)),
			bundle.RelationKeyIconEmoji:            pbtypes.String("📝"),
			bundle.RelationKeyIsArchived:           pbtypes.Bool(true),
			bundle.RelationKeyUniqueKey:            pbtypes.String(uniqueKey.Marshal()),
		}
		relObj := objectstore.TestObject{
			bundle.RelationKeyId:          pbtypes.String(relationID),
			bundle.RelationKeyRelationKey: pbtypes.String(bundle.RelationKeyAssignee.String()),
			bundle.RelationKeyType:        pbtypes.String(bundle.TypeKeyRelation.URL()),
		}
		s.objectStore.AddObjects(t, []objectstore.TestObject{obj, relObj})

		// When
		got, err := s.GetObjectType(id)
		require.NoError(t, err)

		// Then
		want := &model.ObjectType{
			Url:        id,
			Name:       "my note",
			Layout:     model.ObjectType_note,
			IconEmoji:  "📝",
			IsArchived: true,
			Types:      []model.SmartBlockType{model.SmartBlockType_Page},
			Key:        "note",
			RelationLinks: []*model.RelationLink{
				{
					Key:    bundle.RelationKeyAssignee.String(),
					Format: model.RelationFormat_longtext,
				},
			},
		}

		assert.Equal(t, want, got)
		ok, err := s.HasObjectType(id)
		require.NoError(t, err)
		assert.True(t, ok)
	})
}

// TODO Decide what to do with it
// func TestGetAggregatedOptions(t *testing.T) {
// 	t.Run("with no options", func(t *testing.T) {
// 		s := newFixture(t)
//
// 		got, err := s.GetAggregatedOptions(bundle.RelationKeyTag.String())
// 		require.NoError(t, err)
// 		assert.Empty(t, got)
// 	})
//
// 	t.Run("with options", func(t *testing.T) {
// 		s := newFixture(t)
// 		opt1 := makeRelationOptionObject("id1", "name1", "color1", bundle.RelationKeyTag.String())
// 		opt2 := makeRelationOptionObject("id2", "name2", "color2", bundle.RelationKeyStatus.String())
// 		opt3 := makeRelationOptionObject("id3", "name3", "color3", bundle.RelationKeyTag.String())
// 		s.AddObjects(t, []objectstore.TestObject{opt1, opt2, opt3})
//
// 		got, err := s.GetAggregatedOptions(bundle.RelationKeyTag.String())
// 		require.NoError(t, err)
// 		want := []*model.RelationOption{
// 			{
// 				Id:          "id1",
// 				Text:        "name1",
// 				Color:       "color1",
// 				RelationKey: bundle.RelationKeyTag.String(),
// 			},
// 			{
// 				Id:          "id3",
// 				Text:        "name3",
// 				Color:       "color3",
// 				RelationKey: bundle.RelationKeyTag.String(),
// 			},
// 		}
// 		assert.Equal(t, want, got)
// 	})
// }

func makeRelationOptionObject(id, name, color, relationKey string) objectstore.TestObject {
	return objectstore.TestObject{
		bundle.RelationKeyId:                  pbtypes.String(id),
		bundle.RelationKeyType:                pbtypes.String(bundle.TypeKeyRelationOption.URL()),
		bundle.RelationKeyName:                pbtypes.String(name),
		bundle.RelationKeyRelationOptionColor: pbtypes.String(color),
		bundle.RelationKeyRelationKey:         pbtypes.String(relationKey),
		bundle.RelationKeyLayout:              pbtypes.Int64(int64(model.ObjectType_relationOption)),
	}
}

func TestGetRelationById(t *testing.T) {
	t.Run("relation is not found", func(t *testing.T) {
		s := newFixture(t)

		_, err := s.GetRelationByID("relationID")
		require.Error(t, err)
	})

	t.Run("requested object is not relation", func(t *testing.T) {
		s := newFixture(t)

		obj := objectstore.TestObject{
			bundle.RelationKeyId:      pbtypes.String("id1"),
			bundle.RelationKeyName:    pbtypes.String("name1"),
			bundle.RelationKeySpaceId: pbtypes.String("space1"),
		}
		s.objectStore.AddObjects(t, []objectstore.TestObject{obj})

		_, err := s.GetRelationByID("id1")
		require.Error(t, err)
	})

	t.Run("relation is found", func(t *testing.T) {
		s := newFixture(t)

		relation := &relationutils.Relation{Relation: bundle.MustGetRelation(bundle.RelationKeyName)}
		relationID := "derivedFrom(name)"
		relation.Id = relationID
		relObject := relation.ToStruct()
		err := s.objectStore.UpdateObjectDetails(relation.Id, relObject)
		require.NoError(t, err)

		got, err := s.GetRelationByID(relationID)
		require.NoError(t, err)
		assert.Equal(t, relationutils.RelationFromStruct(relObject).Relation, got)
	})
}
