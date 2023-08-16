package objectstore

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/uniquekey"
	"github.com/anyproto/anytype-heart/core/relation/relationutils"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/typeprovider/mock_typeprovider"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestDsObjectStore_UpdateLocalDetails(t *testing.T) {
	s := newStoreFixture(t)
	id := bson.NewObjectId()
	// bundle.RelationKeyLastOpenedDate is local relation (not stored in the changes tree)
	err := s.UpdateObjectDetails(id.String(), &types.Struct{
		Fields: map[string]*types.Value{bundle.RelationKeyLastOpenedDate.String(): pbtypes.Int64(4), "type": pbtypes.String("_otp1")},
	})
	require.NoError(t, err)

	recs, _, err := s.Query(database.Query{})
	require.NoError(t, err)
	require.Len(t, recs, 1)
	require.Equal(t, pbtypes.Int64(4), pbtypes.Get(recs[0].Details, bundle.RelationKeyLastOpenedDate.String()))

	err = s.UpdateObjectDetails(id.String(), &types.Struct{
		Fields: map[string]*types.Value{"k1": pbtypes.String("1"), "k2": pbtypes.String("2"), "type": pbtypes.String("_otp1")},
	})
	require.NoError(t, err)

	recs, _, err = s.Query(database.Query{})
	require.NoError(t, err)
	require.Len(t, recs, 1)
	require.Nil(t, pbtypes.Get(recs[0].Details, bundle.RelationKeyLastOpenedDate.String()))
	require.Equal(t, "2", pbtypes.GetString(recs[0].Details, "k2"))
}

func Test_removeByPrefix(t *testing.T) {
	s := newStoreFixture(t)
	var key = make([]byte, 32)
	for i := 0; i < 10; i++ {

		var links []string
		rand.Seed(time.Now().UnixNano())
		rand.Read(key)
		objId := fmt.Sprintf("%x", key)

		for j := 0; j < 8000; j++ {
			rand.Seed(time.Now().UnixNano())
			rand.Read(key)
			links = append(links, fmt.Sprintf("%x", key))
		}
		require.NoError(t, s.UpdateObjectDetails(objId, nil))
		require.NoError(t, s.UpdateObjectLinks(objId, links))
	}

	// Test huge transactions
	outboundRemoved, inboundRemoved, err := s.eraseLinks()
	require.Equal(t, 10*8000, outboundRemoved)
	require.Equal(t, 10*8000, inboundRemoved)
	require.NoError(t, err)
}

func TestList(t *testing.T) {
	s := newStoreFixture(t)
	typeProvider := mock_typeprovider.NewMockSmartBlockTypeProvider(t)
	s.sbtProvider = typeProvider

	obj1 := makeObjectWithName("id1", "name1")
	err := s.UpdateObjectSnippet("id1", "snippet1")
	require.NoError(t, err)
	typeProvider.EXPECT().Type("space1", "id1").Return(smartblock.SmartBlockTypePage, nil)

	obj2 := makeObjectWithName("id2", "name2")
	typeProvider.EXPECT().Type("space1", "id2").Return(smartblock.SmartBlockTypeFile, nil)

	obj3 := makeObjectWithName("id3", "date")
	obj3[bundle.RelationKeyIsDeleted] = pbtypes.Bool(true)
	typeProvider.EXPECT().Type("space1", "id3").Return(smartblock.SmartBlockTypePage, nil)

	s.addObjects(t, []testObject{obj1, obj2, obj3})

	got, err := s.List("space1")
	require.NoError(t, err)

	want := []*model.ObjectInfo{
		{
			Id:         "id1",
			Details:    makeDetails(obj1),
			Snippet:    "snippet1",
			ObjectType: model.SmartBlockType_Page,
		},
		{
			Id:         "id2",
			Details:    makeDetails(obj2),
			ObjectType: model.SmartBlockType_File,
		},
		// Skip deleted id3
	}

	assert.Equal(t, want, got)
}

func TestListIds(t *testing.T) {
	t.Run("with empty store", func(t *testing.T) {
		s := newStoreFixture(t)

		got, err := s.ListIds()
		require.NoError(t, err)
		assert.Empty(t, got)
	})
	t.Run("with not empty store", func(t *testing.T) {
		s := newStoreFixture(t)
		s.addObjects(t, []testObject{
			makeObjectWithName("id1", "name1"),
			makeObjectWithName("id2", "name2"),
		})

		got, err := s.ListIds()
		require.NoError(t, err)
		assert.Equal(t, []string{"id1", "id2"}, got)
	})
}

func TestHasIDs(t *testing.T) {
	s := newStoreFixture(t)
	s.addObjects(t, []testObject{
		makeObjectWithName("id1", "name1"),
		makeObjectWithName("id2", "name2"),
		makeObjectWithName("id3", "name3"),
	})

	t.Run("none found", func(t *testing.T) {
		got, err := s.HasIDs("id4", "id5")
		require.NoError(t, err)
		assert.Empty(t, got)
	})
	t.Run("some found", func(t *testing.T) {
		got, err := s.HasIDs("id2", "id3", "id4")
		require.NoError(t, err)
		assert.Equal(t, []string{"id2", "id3"}, got)
	})
	t.Run("all found", func(t *testing.T) {
		got, err := s.HasIDs("id1", "id3")
		require.NoError(t, err)
		assert.Equal(t, []string{"id1", "id3"}, got)
	})
	t.Run("all found, check that input and output orders are equal by reversing arguments", func(t *testing.T) {
		got, err := s.HasIDs("id3", "id1")
		require.NoError(t, err)
		assert.Equal(t, []string{"id3", "id1"}, got)
	})
}

func TestGetObjectType(t *testing.T) {
	t.Run("get bundled type", func(t *testing.T) {
		s := newStoreFixture(t)

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
		s := newStoreFixture(t)

		id := "id1"
		obj := testObject{
			bundle.RelationKeyId:   pbtypes.String(id),
			bundle.RelationKeyType: pbtypes.String(bundle.TypeKeyNote.URL()),
		}
		s.addObjects(t, []testObject{obj})

		_, err := s.GetObjectType(id)
		require.Error(t, err)
		ok, err := s.HasObjectType(id)
		require.NoError(t, err)
		assert.False(t, ok)
	})

	t.Run("with object is type", func(t *testing.T) {
		// Given
		s := newStoreFixture(t)

		id := "id1"
		relationID := "derivedFrom(assignee)"
		uniqueKey, err := uniquekey.New(model.SmartBlockType_STType, "note")
		require.NoError(t, err)
		obj := testObject{
			bundle.RelationKeyId:                   pbtypes.String(id),
			bundle.RelationKeyType:                 pbtypes.String(bundle.TypeKeyObjectType.URL()),
			bundle.RelationKeyName:                 pbtypes.String("my note"),
			bundle.RelationKeyRecommendedRelations: pbtypes.StringList([]string{relationID}),
			bundle.RelationKeyRecommendedLayout:    pbtypes.Int64(int64(model.ObjectType_note)),
			bundle.RelationKeyIconEmoji:            pbtypes.String("📝"),
			bundle.RelationKeyIsArchived:           pbtypes.Bool(true),
			bundle.RelationKeyUniqueKey:            pbtypes.String(uniqueKey.Marshal()),
		}
		relObj := testObject{
			bundle.RelationKeyId:          pbtypes.String(relationID),
			bundle.RelationKeyRelationKey: pbtypes.String(bundle.RelationKeyAssignee.String()),
			bundle.RelationKeyType:        pbtypes.String(bundle.TypeKeyRelation.URL()),
		}
		s.addObjects(t, []testObject{obj, relObj})

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

func TestGetAggregatedOptions(t *testing.T) {
	t.Run("with no options", func(t *testing.T) {
		s := newStoreFixture(t)

		got, err := s.GetAggregatedOptions(bundle.RelationKeyTag.String())
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("with options", func(t *testing.T) {
		s := newStoreFixture(t)
		opt1 := makeRelationOptionObject("id1", "name1", "color1", bundle.RelationKeyTag.String())
		opt2 := makeRelationOptionObject("id2", "name2", "color2", bundle.RelationKeyStatus.String())
		opt3 := makeRelationOptionObject("id3", "name3", "color3", bundle.RelationKeyTag.String())
		s.addObjects(t, []testObject{opt1, opt2, opt3})

		got, err := s.GetAggregatedOptions(bundle.RelationKeyTag.String())
		require.NoError(t, err)
		want := []*model.RelationOption{
			{
				Id:          "id1",
				Text:        "name1",
				Color:       "color1",
				RelationKey: bundle.RelationKeyTag.String(),
			},
			{
				Id:          "id3",
				Text:        "name3",
				Color:       "color3",
				RelationKey: bundle.RelationKeyTag.String(),
			},
		}
		assert.Equal(t, want, got)
	})
}

func makeRelationOptionObject(id, name, color, relationKey string) testObject {
	return testObject{
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
		s := newStoreFixture(t)

		_, err := s.GetRelationByID("relationID")
		require.Error(t, err)
	})

	t.Run("requested object is not relation", func(t *testing.T) {
		s := newStoreFixture(t)

		s.addObjects(t, []testObject{makeObjectWithName("id1", "name1")})

		_, err := s.GetRelationByID("id1")
		require.Error(t, err)
	})

	t.Run("relation is found", func(t *testing.T) {
		s := newStoreFixture(t)

		relation := &relationutils.Relation{Relation: bundle.MustGetRelation(bundle.RelationKeyName)}
		relationID := "derivedFrom(name)"
		relation.Id = relationID
		relObject := relation.ToStruct()
		err := s.UpdateObjectDetails(relation.Id, relObject)
		require.NoError(t, err)

		got, err := s.GetRelationByID(relationID)
		require.NoError(t, err)
		assert.Equal(t, relationutils.RelationFromStruct(relObject).Relation, got)
	})
}

func TestGetWithLinksInfoByID(t *testing.T) {
	s := newStoreFixture(t)
	obj1 := makeObjectWithName("id1", "name1")
	obj2 := makeObjectWithName("id2", "name2")
	obj3 := makeObjectWithName("id3", "name3")
	s.addObjects(t, []testObject{obj1, obj2, obj3})

	err := s.UpdateObjectLinks("id1", []string{"id2", "id3"})
	require.NoError(t, err)

	t.Run("links of first object", func(t *testing.T) {
		got, err := s.GetWithLinksInfoByID("space1", "id1")
		require.NoError(t, err)

		assert.Equal(t, makeDetails(obj1), got.Info.Details)
		require.Len(t, got.Links.Outbound, 2)
		assert.Equal(t, makeDetails(obj2), got.Links.Outbound[0].Details)
		assert.Equal(t, makeDetails(obj3), got.Links.Outbound[1].Details)
	})

	t.Run("links of second object", func(t *testing.T) {
		got, err := s.GetWithLinksInfoByID("space1", "id2")
		require.NoError(t, err)

		assert.Equal(t, makeDetails(obj2), got.Info.Details)
		require.Len(t, got.Links.Inbound, 1)
		assert.Equal(t, makeDetails(obj1), got.Links.Inbound[0].Details)
	})

	t.Run("links of third object", func(t *testing.T) {
		got, err := s.GetWithLinksInfoByID("space1", "id3")
		require.NoError(t, err)

		assert.Equal(t, makeDetails(obj3), got.Info.Details)
		require.Len(t, got.Links.Inbound, 1)
		assert.Equal(t, makeDetails(obj1), got.Links.Inbound[0].Details)
	})
}

func TestDeleteObject(t *testing.T) {
	t.Run("object is not found", func(t *testing.T) {
		s := newStoreFixture(t)

		err := s.DeleteObject("id1")
		require.NoError(t, err)

		got, err := s.GetDetails("id1")
		require.NoError(t, err)
		assert.Equal(t, &model.ObjectDetails{
			Details: makeDetails(testObject{
				bundle.RelationKeyId:        pbtypes.String("id1"),
				bundle.RelationKeyIsDeleted: pbtypes.Bool(true),
			}),
		}, got)
	})

	t.Run("object is already deleted", func(t *testing.T) {
		s := newStoreFixture(t)
		err := s.DeleteObject("id1")
		require.NoError(t, err)

		err = s.DeleteObject("id1")
		require.NoError(t, err)

		got, err := s.GetDetails("id1")
		require.NoError(t, err)
		assert.Equal(t, &model.ObjectDetails{
			Details: makeDetails(testObject{
				bundle.RelationKeyId:        pbtypes.String("id1"),
				bundle.RelationKeyIsDeleted: pbtypes.Bool(true),
			}),
		}, got)
	})

	t.Run("delete object", func(t *testing.T) {
		// Arrange
		s := newStoreFixture(t)
		obj := makeObjectWithName("id1", "name1")
		s.addObjects(t, []testObject{obj})

		err := s.UpdateObjectSnippet("id1", "snippet1")
		require.NoError(t, err)

		err = s.UpdateObjectLinks("id2", []string{"id1"})
		require.NoError(t, err)

		err = s.SaveLastIndexedHeadsHash("id1", "hash1")
		require.NoError(t, err)

		err = s.AddToIndexQueue("id1")
		require.NoError(t, err)

		// Act
		err = s.DeleteObject("id1")
		require.NoError(t, err)

		// Assert
		got, err := s.GetDetails("id1")
		require.NoError(t, err)
		assert.Equal(t, &model.ObjectDetails{
			Details: makeDetails(testObject{
				bundle.RelationKeyId:        pbtypes.String("id1"),
				bundle.RelationKeyIsDeleted: pbtypes.Bool(true),
			}),
		}, got)

		objects, err := s.GetByIDs("space1", []string{"id1"})
		require.NoError(t, err)
		assert.Empty(t, objects)

		outbound, err := s.GetOutboundLinksByID("id1")
		require.NoError(t, err)
		assert.Empty(t, outbound)

		inbound, err := s.GetInboundLinksByID("id2")
		require.NoError(t, err)
		assert.Empty(t, inbound)

		hash, err := s.GetLastIndexedHeadsHash("id1")
		require.NoError(t, err)
		assert.Empty(t, hash)

		ids, err := s.ListIDsFromFullTextQueue()
		require.NoError(t, err)
		assert.Empty(t, ids)
	})
}

func TestDeleteDetails(t *testing.T) {
	s := newStoreFixture(t)
	s.addObjects(t, []testObject{makeObjectWithName("id1", "name1")})

	err := s.DeleteDetails("id1")
	require.NoError(t, err)

	got, err := s.GetDetails("id1")
	require.NoError(t, err)
	assert.Equal(t, &model.ObjectDetails{Details: &types.Struct{
		Fields: map[string]*types.Value{},
	}}, got)
}
