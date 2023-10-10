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

	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spacecore/typeprovider/mock_typeprovider"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestDsObjectStore_UpdateLocalDetails(t *testing.T) {
	s := NewStoreFixture(t)
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
	s := NewStoreFixture(t)
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
	s := NewStoreFixture(t)
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

	s.AddObjects(t, []TestObject{obj1, obj2, obj3})

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
		s := NewStoreFixture(t)

		got, err := s.ListIds()
		require.NoError(t, err)
		assert.Empty(t, got)
	})
	t.Run("with not empty store", func(t *testing.T) {
		s := NewStoreFixture(t)
		s.AddObjects(t, []TestObject{
			makeObjectWithName("id1", "name1"),
			makeObjectWithName("id2", "name2"),
		})

		got, err := s.ListIds()
		require.NoError(t, err)
		assert.Equal(t, []string{"id1", "id2"}, got)
	})
}

func TestHasIDs(t *testing.T) {
	s := NewStoreFixture(t)
	s.AddObjects(t, []TestObject{
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

func TestGetWithLinksInfoByID(t *testing.T) {
	s := NewStoreFixture(t)
	obj1 := makeObjectWithName("id1", "name1")
	obj2 := makeObjectWithName("id2", "name2")
	obj3 := makeObjectWithName("id3", "name3")
	s.AddObjects(t, []TestObject{obj1, obj2, obj3})

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
		s := NewStoreFixture(t)

		err := s.DeleteObject("id1")
		require.NoError(t, err)

		got, err := s.GetDetails("id1")
		require.NoError(t, err)
		assert.Equal(t, &model.ObjectDetails{
			Details: makeDetails(TestObject{
				bundle.RelationKeyId:        pbtypes.String("id1"),
				bundle.RelationKeyIsDeleted: pbtypes.Bool(true),
			}),
		}, got)
	})

	t.Run("object is already deleted", func(t *testing.T) {
		s := NewStoreFixture(t)
		err := s.DeleteObject("id1")
		require.NoError(t, err)

		err = s.DeleteObject("id1")
		require.NoError(t, err)

		got, err := s.GetDetails("id1")
		require.NoError(t, err)
		assert.Equal(t, &model.ObjectDetails{
			Details: makeDetails(TestObject{
				bundle.RelationKeyId:        pbtypes.String("id1"),
				bundle.RelationKeyIsDeleted: pbtypes.Bool(true),
			}),
		}, got)
	})

	t.Run("delete object", func(t *testing.T) {
		// Arrange
		s := NewStoreFixture(t)
		obj := makeObjectWithName("id1", "name1")
		s.AddObjects(t, []TestObject{obj})

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
			Details: makeDetails(TestObject{
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
	s := NewStoreFixture(t)
	s.AddObjects(t, []TestObject{makeObjectWithName("id1", "name1")})

	err := s.DeleteDetails("id1")
	require.NoError(t, err)

	got, err := s.GetDetails("id1")
	require.NoError(t, err)
	assert.Equal(t, &model.ObjectDetails{Details: &types.Struct{
		Fields: map[string]*types.Value{},
	}}, got)
}
