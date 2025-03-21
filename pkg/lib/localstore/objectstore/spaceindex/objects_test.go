package spaceindex

import (
	context2 "context"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/globalsign/mgo/bson"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
)

func TestDsObjectStore_UpdateLocalDetails(t *testing.T) {
	s := NewStoreFixture(t)
	id := bson.NewObjectId()
	// bundle.RelationKeyLastOpenedDate is local relation (not stored in the changes tree)
	err := s.UpdateObjectDetails(context2.Background(), id.String(),
		domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyLastOpenedDate: domain.Int64(4),
			bundle.RelationKeyType:           domain.String("_otp1"),
		}))
	require.NoError(t, err)

	recs, err := s.Query(database.Query{})
	require.NoError(t, err)
	require.Len(t, recs, 1)
	require.Equal(t, domain.Int64(4), recs[0].Details.Get(bundle.RelationKeyLastOpenedDate))

	err = s.UpdateObjectDetails(context2.Background(), id.String(),
		domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			"k1":   domain.String("1"),
			"k2":   domain.String("2"),
			"type": domain.String("_otp1"),
		}))
	require.NoError(t, err)

	recs, err = s.Query(database.Query{})
	require.NoError(t, err)
	require.Len(t, recs, 1)
	require.False(t, recs[0].Details.Get(bundle.RelationKeyLastOpenedDate).Ok())
	require.Equal(t, "2", recs[0].Details.GetString("k2"))
}

func Test_removeByPrefix(t *testing.T) {
	s := NewStoreFixture(t)
	ctx := context2.Background()

	var key = make([]byte, 32)
	spaceId := "space1"
	objectsCount := 10
	objectIds := make([]string, 0, objectsCount)
	for i := 0; i < objectsCount; i++ {
		var links []string
		rand.Seed(time.Now().UnixNano())
		rand.Read(key)
		objId := fmt.Sprintf("%x", key)
		objectIds = append(objectIds, objId)
		for j := 0; j < 8000; j++ {
			rand.Seed(time.Now().UnixNano())
			rand.Read(key)
			links = append(links, fmt.Sprintf("%x", key))
		}
		details := makeDetails(TestObject{
			bundle.RelationKeyId:      domain.String(objId),
			bundle.RelationKeySpaceId: domain.String(spaceId),
		})
		require.NoError(t, s.UpdateObjectDetails(context2.Background(), objId, details))
		require.NoError(t, s.UpdateObjectLinks(ctx, objId, links))
	}

	// Test huge transaction
	err := s.DeleteLinks(objectIds)
	require.NoError(t, err)

	for _, id := range objectIds {
		links, err := s.GetInboundLinksById(id)
		require.NoError(t, err)
		require.Empty(t, links)

		links, err = s.GetOutboundLinksById(id)
		require.NoError(t, err)
		require.Empty(t, links)
	}
}

func TestList(t *testing.T) {
	s := NewStoreFixture(t)

	obj1 := makeObjectWithName("id1", "name1")
	obj1[bundle.RelationKeySnippet] = domain.String("snippet1")

	obj2 := makeObjectWithName("id2", "name2")

	obj3 := makeObjectWithName("id3", "date")
	obj3[bundle.RelationKeyIsDeleted] = domain.Bool(true)

	s.AddObjects(t, []TestObject{obj1, obj2, obj3})

	got, err := s.List(false)
	require.NoError(t, err)

	want := []*database.ObjectInfo{
		{
			Id:      "id1",
			Details: makeDetails(obj1),
			Snippet: "snippet1",
		},
		{
			Id:      "id2",
			Details: makeDetails(obj2),
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
		got, err := s.HasIds([]string{"id4", "id5"})
		require.NoError(t, err)
		assert.Empty(t, got)
	})
	t.Run("some found", func(t *testing.T) {
		got, err := s.HasIds([]string{"id2", "id3", "id4"})
		require.NoError(t, err)
		assert.Equal(t, []string{"id2", "id3"}, got)
	})
	t.Run("all found", func(t *testing.T) {
		got, err := s.HasIds([]string{"id1", "id3"})
		require.NoError(t, err)
		assert.Equal(t, []string{"id1", "id3"}, got)
	})
	t.Run("all found, check that input and output orders are equal by reversing arguments", func(t *testing.T) {
		got, err := s.HasIds([]string{"id3", "id1"})
		require.NoError(t, err)
		assert.Equal(t, []string{"id3", "id1"}, got)
	})
}

func TestGetWithLinksInfoByID(t *testing.T) {
	s := NewStoreFixture(t)
	ctx := context2.Background()
	obj1 := makeObjectWithName("id1", "name1")
	obj2 := makeObjectWithName("id2", "name2")
	obj3 := makeObjectWithName("id3", "name3")
	s.AddObjects(t, []TestObject{obj1, obj2, obj3})

	err := s.UpdateObjectLinks(ctx, "id1", []string{"id2", "id3"})
	require.NoError(t, err)

	t.Run("links of first object", func(t *testing.T) {
		got, err := s.GetWithLinksInfoById("id1")
		require.NoError(t, err)

		assert.Equal(t, makeDetails(obj1).ToProto(), got.Info.Details)
		require.Len(t, got.Links.Outbound, 2)
		assert.Equal(t, makeDetails(obj2).ToProto(), got.Links.Outbound[0].Details)
		assert.Equal(t, makeDetails(obj3).ToProto(), got.Links.Outbound[1].Details)
	})

	t.Run("links of second object", func(t *testing.T) {
		got, err := s.GetWithLinksInfoById("id2")
		require.NoError(t, err)

		assert.Equal(t, makeDetails(obj2).ToProto(), got.Info.Details)
		require.Len(t, got.Links.Inbound, 1)
		assert.Equal(t, makeDetails(obj1).ToProto(), got.Links.Inbound[0].Details)
	})

	t.Run("links of third object", func(t *testing.T) {
		got, err := s.GetWithLinksInfoById("id3")
		require.NoError(t, err)

		assert.Equal(t, makeDetails(obj3).ToProto(), got.Info.Details)
		require.Len(t, got.Links.Inbound, 1)
		assert.Equal(t, makeDetails(obj1).ToProto(), got.Links.Inbound[0].Details)
	})
}

func TestDeleteObject(t *testing.T) {
	t.Run("on deleting object: details of deleted object are updated, but object is still in store", func(t *testing.T) {
		s := NewStoreFixture(t)

		err := s.DeleteObject("id1")
		require.NoError(t, err)

		got, err := s.GetDetails("id1")
		require.NoError(t, err)
		assert.Equal(t, makeDetails(TestObject{
			bundle.RelationKeyId:        domain.String("id1"),
			bundle.RelationKeySpaceId:   domain.String("test"),
			bundle.RelationKeyIsDeleted: domain.Bool(true),
		}), got)
	})

	t.Run("object is already deleted", func(t *testing.T) {
		s := NewStoreFixture(t)
		err := s.DeleteObject("id1")
		require.NoError(t, err)

		err = s.DeleteObject("id1")
		require.NoError(t, err)

		got, err := s.GetDetails("id1")
		require.NoError(t, err)
		assert.Equal(t, makeDetails(TestObject{
			bundle.RelationKeyId:        domain.String("id1"),
			bundle.RelationKeySpaceId:   domain.String("test"),
			bundle.RelationKeyIsDeleted: domain.Bool(true),
		}), got)
	})

	t.Run("delete object", func(t *testing.T) {
		// Arrange
		s := NewStoreFixture(t)
		ctx := context2.Background()
		obj := makeObjectWithName("id1", "name1")
		s.AddObjects(t, []TestObject{obj})

		err := s.UpdateObjectLinks(ctx, "id2", []string{"id1"})
		require.NoError(t, err)

		err = s.SaveLastIndexedHeadsHash(ctx, "id1", "hash1")
		require.NoError(t, err)

		err = s.fulltextQueue.AddToIndexQueue(ctx, domain.FullID{ObjectID: "id1", SpaceID: spaceName})
		require.NoError(t, err)

		// Act
		err = s.DeleteObject("id1")
		require.NoError(t, err)

		// Assert
		got, err := s.GetDetails("id1")
		require.NoError(t, err)
		assert.Equal(t, makeDetails(TestObject{
			bundle.RelationKeyId:        domain.String("id1"),
			bundle.RelationKeySpaceId:   domain.String("test"),
			bundle.RelationKeyIsDeleted: domain.Bool(true),
		}), got)

		objects, err := s.GetInfosByIds([]string{"id1"})
		require.NoError(t, err)
		assert.Empty(t, objects)

		outbound, err := s.GetOutboundLinksById("id1")
		require.NoError(t, err)
		assert.Empty(t, outbound)

		inbound, err := s.GetInboundLinksById("id2")
		require.NoError(t, err)
		assert.Empty(t, inbound)

		hash, err := s.GetLastIndexedHeadsHash(ctx, "id1")
		require.NoError(t, err)
		assert.Empty(t, hash)

		ids, err := s.fulltextQueue.ListIdsFromFullTextQueue([]string{spaceName}, 0)
		require.NoError(t, err)
		assert.Empty(t, ids)
	})
}

func TestDeleteDetails(t *testing.T) {
	s := NewStoreFixture(t)
	ctx := context2.Background()
	s.AddObjects(t, []TestObject{makeObjectWithName("id1", "name1")})

	err := s.DeleteDetails(ctx, []string{"id1"})
	require.NoError(t, err)

	got, err := s.GetDetails("id1")
	require.NoError(t, err)
	assert.Equal(t, domain.NewDetails(), got)
}
