package subscription

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const testSpaceId = "space1"

func TestService_Search(t *testing.T) {
	var newSub = func(fx *fixture, subId string) {

		fx.store.AddObjects(t, testSpaceId, []spaceindex.TestObject{
			{
				bundle.RelationKeyId:     pbtypes.String("1"),
				bundle.RelationKeyName:   pbtypes.String("one"),
				bundle.RelationKeyAuthor: pbtypes.StringList([]string{"author1"}),
			},
			// relations
			{
				bundle.RelationKeyId:          pbtypes.String("rel1"),
				bundle.RelationKeyRelationKey: pbtypes.String(bundle.RelationKeyName.String()),
				bundle.RelationKeyLayout:      pbtypes.Int64(int64(model.ObjectType_relation)),
			},
			{
				bundle.RelationKeyId:          pbtypes.String("rel2"),
				bundle.RelationKeyRelationKey: pbtypes.String(bundle.RelationKeyAuthor.String()),
				bundle.RelationKeyLayout:      pbtypes.Int64(int64(model.ObjectType_relation)),
			},
			// dep
			{
				bundle.RelationKeyId:   pbtypes.String("author1"),
				bundle.RelationKeyName: pbtypes.String("author1"),
			},
		})

		resp, err := fx.Search(SubscribeRequest{
			SpaceId: testSpaceId,
			SubId:   subId,
			Keys:    []string{bundle.RelationKeyName.String(), bundle.RelationKeyAuthor.String()},
		})
		require.NoError(t, err)

		assert.Len(t, resp.Records, 4)
		assert.Len(t, resp.Dependencies, 1)
	}

	t.Run("dependencies", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.a.Close(context.Background())
		defer fx.ctrl.Finish()

		newSub(fx, "test")

		fx.store.AddObjects(t, testSpaceId, []spaceindex.TestObject{
			{
				bundle.RelationKeyId:   pbtypes.String("author2"),
				bundle.RelationKeyName: pbtypes.String("author2"),
			},
			{
				bundle.RelationKeyId:   pbtypes.String("author3"),
				bundle.RelationKeyName: pbtypes.String("author3"),
			},
		})

		spaceSub, err := fx.getSpaceSubscriptions(testSpaceId)
		require.NoError(t, err)

		// Wait enough time to flush pending updates to subscriptions handler
		time.Sleep(batchTime + 3*time.Millisecond)

		spaceSub.onChange([]*entry{
			newEntry("1", &types.Struct{Fields: map[string]*types.Value{
				"id":     pbtypes.String("1"),
				"name":   pbtypes.String("one"),
				"author": pbtypes.StringList([]string{"author2", "author3", "1"}),
			}}),
		})

		assert.Equal(t, []string{"test"}, spaceSub.cache.entries["1"].SubIds())
		assert.Equal(t, []string{"test", "test/dep"}, spaceSub.cache.entries["author2"].SubIds())
		assert.Equal(t, []string{"test", "test/dep"}, spaceSub.cache.entries["author3"].SubIds())

		fx.events = fx.events[:0]

		spaceSub.onChange([]*entry{
			newEntry("1", &types.Struct{Fields: map[string]*types.Value{
				"id":   pbtypes.String("1"),
				"name": pbtypes.String("one"),
			}}),
		})

		assert.NoError(t, fx.Unsubscribe("test"))
		assert.Len(t, spaceSub.cache.entries, 0)
	})
	t.Run("search with filters: one filter None", func(t *testing.T) {
		fx := newFixtureWithRealObjectStore(t)
		defer fx.a.Close(context.Background())
		defer fx.ctrl.Finish()
		source := "source"
		relationKey := "key"
		option1 := "option1"
		option2 := "option2"

		err := addTestObjects(t, source, relationKey, option1, option2, testSpaceId, fx)
		require.NoError(t, err)

		resp, err := fx.Search(SubscribeRequest{
			SpaceId: testSpaceId,
			Keys:    []string{bundle.RelationKeyId.String()},
			Filters: []*model.BlockContentDataviewFilter{
				{
					Operator:    model.BlockContentDataviewFilter_No,
					RelationKey: relationKey,
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.String(option1),
					Format:      model.RelationFormat_status,
				},
			},
			NoDepSubscription: true,
		})
		require.NoError(t, err)

		assert.Len(t, resp.Records, 1)
		assert.Equal(t, "1", resp.Records[0].Fields[bundle.RelationKeyId.String()].GetStringValue())
	})
	t.Run("search with filters: linear structure with none filters", func(t *testing.T) {
		fx := newFixtureWithRealObjectStore(t)
		defer fx.a.Close(context.Background())
		defer fx.ctrl.Finish()
		source := "source"

		relationKey := "key"
		option1 := "option1"
		option2 := "option2"

		err := addTestObjects(t, source, relationKey, option1, option2, testSpaceId, fx)
		require.NoError(t, err)

		resp, err := fx.Search(SubscribeRequest{
			SpaceId: testSpaceId,
			Keys:    []string{bundle.RelationKeyId.String()},
			Filters: []*model.BlockContentDataviewFilter{
				{
					Operator:    model.BlockContentDataviewFilter_No,
					RelationKey: relationKey,
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.String(option1),
					Format:      model.RelationFormat_status,
				},
				{
					Operator:    model.BlockContentDataviewFilter_No,
					RelationKey: relationKey,
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       pbtypes.String(option2),
					Format:      model.RelationFormat_status,
				},
			},
			NoDepSubscription: true,
		})
		require.NoError(t, err)
		assert.Len(t, resp.Records, 0)
	})
	t.Run("search with filters: tree structure with And filter in root and None filters in NesterFilters", func(t *testing.T) {
		fx := newFixtureWithRealObjectStore(t)
		defer fx.a.Close(context.Background())
		defer fx.ctrl.Finish()
		source := "source"

		relationKey := "key"
		option1 := "option1"
		option2 := "option2"

		err := addTestObjects(t, source, relationKey, option1, option2, testSpaceId, fx)
		require.NoError(t, err)

		resp, err := fx.Search(SubscribeRequest{
			SpaceId: testSpaceId,
			Keys:    []string{bundle.RelationKeyId.String()},
			Filters: []*model.BlockContentDataviewFilter{
				{
					Operator: model.BlockContentDataviewFilter_And,
					NestedFilters: []*model.BlockContentDataviewFilter{
						{
							Operator:    model.BlockContentDataviewFilter_No,
							RelationKey: relationKey,
							Condition:   model.BlockContentDataviewFilter_Equal,
							Value:       pbtypes.String(option2),
							Format:      model.RelationFormat_status,
						},
						{
							Operator:    model.BlockContentDataviewFilter_No,
							RelationKey: bundle.RelationKeyName.String(),
							Condition:   model.BlockContentDataviewFilter_Equal,
							Value:       pbtypes.String("Object 1"),
							Format:      model.RelationFormat_shorttext,
						},
					},
				},
			},
			NoDepSubscription: true,
		})
		require.NoError(t, err)
		assert.Len(t, resp.Records, 0)
	})
	t.Run("search with filters: tree structure with Or filter in root and None filters in NesterFilters", func(t *testing.T) {
		fx := newFixtureWithRealObjectStore(t)
		defer fx.a.Close(context.Background())
		defer fx.ctrl.Finish()
		source := "source"

		relationKey := "key"
		option1 := "option1"
		option2 := "option2"

		err := addTestObjects(t, source, relationKey, option1, option2, testSpaceId, fx)
		require.NoError(t, err)

		resp, err := fx.Search(SubscribeRequest{
			SpaceId: testSpaceId,
			Keys:    []string{bundle.RelationKeyId.String()},
			Filters: []*model.BlockContentDataviewFilter{
				{
					Operator: model.BlockContentDataviewFilter_Or,
					NestedFilters: []*model.BlockContentDataviewFilter{
						{
							Operator:    model.BlockContentDataviewFilter_No,
							RelationKey: relationKey,
							Condition:   model.BlockContentDataviewFilter_Equal,
							Value:       pbtypes.String(option2),
							Format:      model.RelationFormat_status,
						},
						{
							Operator:    model.BlockContentDataviewFilter_No,
							RelationKey: bundle.RelationKeyName.String(),
							Condition:   model.BlockContentDataviewFilter_Equal,
							Value:       pbtypes.String("Object 1"),
							Format:      model.RelationFormat_shorttext,
						},
					},
				},
			},
			NoDepSubscription: true,
		})
		require.NoError(t, err)
		assert.Len(t, resp.Records, 2)
		assert.Equal(t, "1", resp.Records[0].Fields[bundle.RelationKeyId.String()].GetStringValue())
		assert.Equal(t, "2", resp.Records[1].Fields[bundle.RelationKeyId.String()].GetStringValue())
	})
	t.Run("search with filters: tree structure with And filter in root and combined filters as NestedFilter", func(t *testing.T) {
		fx := newFixtureWithRealObjectStore(t)
		defer fx.a.Close(context.Background())
		defer fx.ctrl.Finish()

		option1 := "option1"
		option2 := "option2"
		option3 := "option3"

		tag1 := "work"
		tag2 := "university"

		addTestObjectsForNestedFilters(t, fx, testSpaceId, option1, option2, option3, tag1, tag2)

		resp, err := fx.Search(SubscribeRequest{
			SpaceId:           testSpaceId,
			Keys:              []string{bundle.RelationKeyId.String()},
			Filters:           prepareNestedFiltersWithOperator(model.BlockContentDataviewFilter_And, option1, option2, tag1),
			NoDepSubscription: true,
		})
		require.NoError(t, err)
		assert.Len(t, resp.Records, 1)
		assert.Equal(t, "1", resp.Records[0].Fields[bundle.RelationKeyId.String()].GetStringValue())
	})
	t.Run("search with filters: tree structure with Or filter in root and combined filters as NestedFilter", func(t *testing.T) {
		fx := newFixtureWithRealObjectStore(t)
		defer fx.a.Close(context.Background())
		defer fx.ctrl.Finish()

		option1 := "option1"
		option2 := "option2"
		option3 := "option3"

		tag1 := "work"
		tag2 := "university"

		addTestObjectsForNestedFilters(t, fx, testSpaceId, option1, option2, option3, tag1, tag2)

		resp, err := fx.Search(SubscribeRequest{
			SpaceId:           testSpaceId,
			Keys:              []string{bundle.RelationKeyId.String()},
			Filters:           prepareNestedFiltersWithOperator(model.BlockContentDataviewFilter_Or, option1, option2, tag1),
			NoDepSubscription: true,
		})
		require.NoError(t, err)
		assert.Len(t, resp.Records, 3)
		assert.Equal(t, "1", resp.Records[0].Fields[bundle.RelationKeyId.String()].GetStringValue())
		assert.Equal(t, "2", resp.Records[1].Fields[bundle.RelationKeyId.String()].GetStringValue())
		assert.Equal(t, "3", resp.Records[2].Fields[bundle.RelationKeyId.String()].GetStringValue())
	})
	t.Run("cache ref counter", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.a.Close(context.Background())
		defer fx.ctrl.Finish()

		newSub(fx, "test")

		spaceSub, err := fx.getSpaceSubscriptions(testSpaceId)
		require.NoError(t, err)

		assert.Equal(t, []string{"test"}, spaceSub.cache.entries["1"].SubIds())
		assert.Equal(t, []string{"test", "test/dep"}, spaceSub.cache.entries["author1"].SubIds())

		newSub(fx, "test1")

		assert.Equal(t, []string{"test", "test1"}, spaceSub.cache.entries["1"].SubIds())
		assert.Equal(t, []string{"test", "test/dep", "test1", "test1/dep"}, spaceSub.cache.entries["author1"].SubIds())
	})

	t.Run("filter deps", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.a.Close(context.Background())
		defer fx.ctrl.Finish()

		fx.store.AddObjects(t, testSpaceId, []spaceindex.TestObject{
			{
				bundle.RelationKeyId:     pbtypes.String("1"),
				bundle.RelationKeyName:   pbtypes.String("one"),
				bundle.RelationKeyAuthor: pbtypes.StringList([]string{"force1"}),
			},
			// relations
			{
				bundle.RelationKeyId:          pbtypes.String("rel1"),
				bundle.RelationKeyRelationKey: pbtypes.String(bundle.RelationKeyName.String()),
				bundle.RelationKeyLayout:      pbtypes.Int64(int64(model.ObjectType_relation)),
			},
			{
				bundle.RelationKeyId:          pbtypes.String("rel2"),
				bundle.RelationKeyRelationKey: pbtypes.String(bundle.RelationKeyAuthor.String()),
				bundle.RelationKeyLayout:      pbtypes.Int64(int64(model.ObjectType_relation)),
			},
			// dep
			{
				bundle.RelationKeyId:   pbtypes.String("force1"),
				bundle.RelationKeyName: pbtypes.String("force1"),
			},
			{
				bundle.RelationKeyId:   pbtypes.String("force2"),
				bundle.RelationKeyName: pbtypes.String("force2"),
			},
		})

		var resp, err = fx.Search(SubscribeRequest{
			SpaceId: testSpaceId,
			SubId:   "subId",
			Keys:    []string{bundle.RelationKeyName.String(), bundle.RelationKeyAuthor.String()},
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyAuthor.String(),
					Condition:   model.BlockContentDataviewFilter_In,
					Value:       pbtypes.StringList([]string{"force1", "force2"}),
				},
			},
		})
		require.NoError(t, err)

		assert.Len(t, resp.Records, 1)
		assert.Len(t, resp.Dependencies, 2)

	})
	t.Run("add with limit", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.a.Close(context.Background())
		defer fx.ctrl.Finish()

		fx.store.AddObjects(t, testSpaceId, []spaceindex.TestObject{
			{
				bundle.RelationKeyId:   pbtypes.String("1"),
				bundle.RelationKeyName: pbtypes.String("1"),
			},
			{
				bundle.RelationKeyId:   pbtypes.String("2"),
				bundle.RelationKeyName: pbtypes.String("2"),
			},
			{
				bundle.RelationKeyId:   pbtypes.String("3"),
				bundle.RelationKeyName: pbtypes.String("3"),
			},
			// relations
			{
				bundle.RelationKeyId:          pbtypes.String("rel1"),
				bundle.RelationKeyRelationKey: pbtypes.String(bundle.RelationKeyName.String()),
				bundle.RelationKeyLayout:      pbtypes.Int64(int64(model.ObjectType_relation)),
			},
		})
		resp, err := fx.Search(SubscribeRequest{
			SpaceId: testSpaceId,
			SubId:   "test",
			Sorts: []*model.BlockContentDataviewSort{
				{
					RelationKey: "name",
					Type:        model.BlockContentDataviewSort_Desc,
				},
			},
			Limit: 2,
			Keys:  []string{"id", "name"},
		})
		require.NoError(t, err)
		// should be 3,2 (1)
		require.Len(t, resp.Records, 2)
		assert.Equal(t, "3", pbtypes.GetString(resp.Records[0], "id"))
		assert.Equal(t, "2", pbtypes.GetString(resp.Records[1], "id"))

		spaceSub, err := fx.getSpaceSubscriptions(testSpaceId)
		require.NoError(t, err)

		spaceSub.onChange([]*entry{
			newEntry("1", &types.Struct{Fields: map[string]*types.Value{
				"id":   pbtypes.String("1"),
				"name": pbtypes.String("4"),
			}}),
		})
		// should be 1,3 (2)
		require.Len(t, fx.events[0].Messages, 3)
		assert.NotEmpty(t, fx.events[0].Messages[0].GetObjectDetailsSet())
		assert.NotEmpty(t, fx.events[0].Messages[1].GetSubscriptionAdd())
		assert.NotEmpty(t, fx.events[0].Messages[2].GetSubscriptionRemove())

		spaceSub.onChange([]*entry{
			newEntry("2", &types.Struct{Fields: map[string]*types.Value{
				"id":   pbtypes.String("2"),
				"name": pbtypes.String("6"),
			}}),
		})

		// should be 2,1 (3)
		t.Log(pbtypes.Sprint(fx.events[1]))
	})

	t.Run("delete item from list", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.a.Close(context.Background())
		defer fx.ctrl.Finish()

		fx.store.AddObjects(t, testSpaceId, []spaceindex.TestObject{
			{
				bundle.RelationKeyId:   pbtypes.String("1"),
				bundle.RelationKeyName: pbtypes.String("1"),
			},
			{
				bundle.RelationKeyId:   pbtypes.String("2"),
				bundle.RelationKeyName: pbtypes.String("2"),
			},
			{
				bundle.RelationKeyId:   pbtypes.String("3"),
				bundle.RelationKeyName: pbtypes.String("3"),
			},
			// relations
			{
				bundle.RelationKeyId:          pbtypes.String("rel1"),
				bundle.RelationKeyRelationKey: pbtypes.String(bundle.RelationKeyName.String()),
				bundle.RelationKeyLayout:      pbtypes.Int64(int64(model.ObjectType_relation)),
			},
		})

		resp, err := fx.Search(SubscribeRequest{
			SpaceId: testSpaceId,
			SubId:   "test",
			Sorts: []*model.BlockContentDataviewSort{
				{
					RelationKey:    "name",
					Type:           model.BlockContentDataviewSort_Asc,
					EmptyPlacement: model.BlockContentDataviewSort_End,
				},
			},
			Limit: 2,
			Keys:  []string{"id", "name"},
		})
		require.NoError(t, err)
		// should be 1,2 (3)
		require.Len(t, resp.Records, 2)
		assert.Equal(t, "1", pbtypes.GetString(resp.Records[0], "id"))
		assert.Equal(t, "2", pbtypes.GetString(resp.Records[1], "id"))

		spaceSub, err := fx.getSpaceSubscriptions(testSpaceId)
		require.NoError(t, err)

		spaceSub.onChange([]*entry{
			{
				id: "2",
				data: &types.Struct{
					Fields: map[string]*types.Value{
						"id":        pbtypes.String("2"),
						"isDeleted": pbtypes.Bool(true),
					}}},
		})
		// should be 1,3 (2)
		require.Len(t, fx.events[0].Messages, 4)
		assert.NotEmpty(t, fx.events[0].Messages[0].GetObjectDetailsSet())
		assert.NotEmpty(t, fx.events[0].Messages[1].GetSubscriptionAdd())
		assert.NotEmpty(t, fx.events[0].Messages[2].GetSubscriptionRemove())
		assert.NotEmpty(t, fx.events[0].Messages[3].GetSubscriptionCounters())
		assert.NotEmpty(t, fx.events[0].Messages[0].GetObjectDetailsSet().Details)
	})

	t.Run("collection: error getting collections entries - no records in response", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.a.Close(context.Background())
		defer fx.ctrl.Finish()

		collectionID := "id"
		subscriptionID := "subId"
		fx.collectionService.EXPECT().SubscribeForCollection(collectionID, subscriptionID).Return(nil, nil, fmt.Errorf("error"))
		var resp, err = fx.Search(SubscribeRequest{
			SpaceId:      testSpaceId,
			SubId:        "subId",
			CollectionId: collectionID,
		})
		require.Error(t, err)
		assert.Nil(t, resp)
	})

	t.Run("collection: collection is empty - no records in response", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.a.Close(context.Background())
		defer fx.ctrl.Finish()

		collectionID := "id"
		subscriptionID := "subId"
		fx.collectionService.EXPECT().SubscribeForCollection(collectionID, subscriptionID).Return(nil, nil, nil)
		fx.collectionService.EXPECT().UnsubscribeFromCollection(collectionID, subscriptionID).Return()
		var resp, err = fx.Search(SubscribeRequest{
			SpaceId:      testSpaceId,
			SubId:        subscriptionID,
			CollectionId: collectionID,
		})

		require.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Len(t, resp.Records, 0)
		assert.Len(t, resp.Dependencies, 0)
	})

	t.Run("collection: collection has 2 objects - return 2 objects in response", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.a.Close(context.Background())
		defer fx.ctrl.Finish()

		collectionID := "id"
		subscriptionID := "subId"

		fx.collectionService.EXPECT().SubscribeForCollection(collectionID, subscriptionID).Return([]string{"1", "2"}, nil, nil)
		fx.collectionService.EXPECT().UnsubscribeFromCollection(collectionID, subscriptionID).Return()

		fx.store.AddObjects(t, testSpaceId, []spaceindex.TestObject{
			{
				bundle.RelationKeyId:   pbtypes.String("1"),
				bundle.RelationKeyName: pbtypes.String("1"),
			},
			{
				bundle.RelationKeyId:   pbtypes.String("2"),
				bundle.RelationKeyName: pbtypes.String("2"),
			},
			// relations
			{
				bundle.RelationKeyId:          pbtypes.String("rel1"),
				bundle.RelationKeyRelationKey: pbtypes.String(bundle.RelationKeyName.String()),
				bundle.RelationKeyLayout:      pbtypes.Int64(int64(model.ObjectType_relation)),
			},
			{
				bundle.RelationKeyId:          pbtypes.String("rel2"),
				bundle.RelationKeyRelationKey: pbtypes.String(bundle.RelationKeyId.String()),
				bundle.RelationKeyLayout:      pbtypes.Int64(int64(model.ObjectType_relation)),
			},
		})
		var resp, err = fx.Search(SubscribeRequest{
			SpaceId:      testSpaceId,
			SubId:        subscriptionID,
			Keys:         []string{bundle.RelationKeyName.String(), bundle.RelationKeyId.String()},
			CollectionId: collectionID,
		})

		require.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Len(t, resp.Records, 2)
		assert.Equal(t, "1", pbtypes.GetString(resp.Records[0], bundle.RelationKeyName.String()))
		assert.Equal(t, "1", pbtypes.GetString(resp.Records[0], bundle.RelationKeyId.String()))
		assert.Equal(t, "2", pbtypes.GetString(resp.Records[1], bundle.RelationKeyName.String()))
		assert.Equal(t, "2", pbtypes.GetString(resp.Records[1], bundle.RelationKeyId.String()))
	})

	t.Run("collection: collection has 3 objects, 1 is filtered - return 2 objects in response", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.a.Close(context.Background())
		defer fx.ctrl.Finish()

		collectionID := "id"
		subscriptionID := "subId"

		fx.collectionService.EXPECT().SubscribeForCollection(collectionID, subscriptionID).Return([]string{"1", "2", "3"}, nil, nil)
		fx.collectionService.EXPECT().UnsubscribeFromCollection(collectionID, subscriptionID).Return()

		fx.store.AddObjects(t, testSpaceId, []spaceindex.TestObject{
			{
				bundle.RelationKeyId:   pbtypes.String("1"),
				bundle.RelationKeyName: pbtypes.String("1"),
			},
			{
				bundle.RelationKeyId:   pbtypes.String("2"),
				bundle.RelationKeyName: pbtypes.String("2"),
			},
			{
				bundle.RelationKeyId:   pbtypes.String("3"),
				bundle.RelationKeyName: pbtypes.String("3"),
			},
			// relations
			{
				bundle.RelationKeyId:          pbtypes.String("rel1"),
				bundle.RelationKeyRelationKey: pbtypes.String(bundle.RelationKeyName.String()),
				bundle.RelationKeyLayout:      pbtypes.Int64(int64(model.ObjectType_relation)),
			},
			{
				bundle.RelationKeyId:          pbtypes.String("rel2"),
				bundle.RelationKeyRelationKey: pbtypes.String(bundle.RelationKeyId.String()),
				bundle.RelationKeyLayout:      pbtypes.Int64(int64(model.ObjectType_relation)),
			},
		})
		var resp, err = fx.Search(SubscribeRequest{
			SpaceId:      testSpaceId,
			SubId:        subscriptionID,
			Keys:         []string{bundle.RelationKeyName.String(), bundle.RelationKeyId.String()},
			CollectionId: collectionID,
			Filters: []*model.BlockContentDataviewFilter{
				{
					Id:          "1",
					RelationKey: bundle.RelationKeyName.String(),
					Condition:   model.BlockContentDataviewFilter_NotEqual,
					Value:       pbtypes.String("3"),
				},
			},
		})

		require.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Len(t, resp.Records, 2)
		assert.Equal(t, "1", pbtypes.GetString(resp.Records[0], bundle.RelationKeyName.String()))
		assert.Equal(t, "1", pbtypes.GetString(resp.Records[0], bundle.RelationKeyId.String()))
		assert.Equal(t, "2", pbtypes.GetString(resp.Records[1], bundle.RelationKeyName.String()))
		assert.Equal(t, "2", pbtypes.GetString(resp.Records[1], bundle.RelationKeyId.String()))
	})
	t.Run("collection: collection has 3 objects, offset = 2 - return 1 object after offset", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.a.Close(context.Background())
		defer fx.ctrl.Finish()

		collectionID := "id"
		subscriptionID := "subId"

		fx.collectionService.EXPECT().SubscribeForCollection(collectionID, subscriptionID).Return([]string{"1", "2", "3"}, nil, nil)
		fx.collectionService.EXPECT().UnsubscribeFromCollection(collectionID, subscriptionID).Return()

		fx.store.AddObjects(t, testSpaceId, []spaceindex.TestObject{
			{
				bundle.RelationKeyId:   pbtypes.String("1"),
				bundle.RelationKeyName: pbtypes.String("1"),
			},
			{
				bundle.RelationKeyId:   pbtypes.String("2"),
				bundle.RelationKeyName: pbtypes.String("2"),
			},
			{
				bundle.RelationKeyId:   pbtypes.String("3"),
				bundle.RelationKeyName: pbtypes.String("3"),
			},
			// relations
			{
				bundle.RelationKeyId:          pbtypes.String("rel1"),
				bundle.RelationKeyRelationKey: pbtypes.String(bundle.RelationKeyName.String()),
				bundle.RelationKeyLayout:      pbtypes.Int64(int64(model.ObjectType_relation)),
			},
			{
				bundle.RelationKeyId:          pbtypes.String("rel2"),
				bundle.RelationKeyRelationKey: pbtypes.String(bundle.RelationKeyId.String()),
				bundle.RelationKeyLayout:      pbtypes.Int64(int64(model.ObjectType_relation)),
			},
		})

		var resp, err = fx.Search(SubscribeRequest{
			SpaceId:      testSpaceId,
			SubId:        subscriptionID,
			Keys:         []string{bundle.RelationKeyName.String(), bundle.RelationKeyId.String()},
			CollectionId: collectionID,
			Offset:       2,
		})

		require.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Len(t, resp.Records, 1)
		assert.Equal(t, "3", pbtypes.GetString(resp.Records[0], bundle.RelationKeyName.String()))
		assert.Equal(t, "3", pbtypes.GetString(resp.Records[0], bundle.RelationKeyId.String()))
	})
	t.Run("collection: collection has object with dependency, no dependency flag is set - return objects without dependency", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.a.Close(context.Background())
		defer fx.ctrl.Finish()

		collectionID := "id"
		subscriptionID := "subId"
		testRelationKey := "link_to_object"

		fx.collectionService.EXPECT().SubscribeForCollection(collectionID, subscriptionID).Return([]string{"1"}, nil, nil)
		fx.collectionService.EXPECT().UnsubscribeFromCollection(collectionID, subscriptionID).Return()

		fx.store.AddObjects(t, testSpaceId, []spaceindex.TestObject{
			{
				bundle.RelationKeyId:                pbtypes.String("1"),
				bundle.RelationKeyName:              pbtypes.String("1"),
				domain.RelationKey(testRelationKey): pbtypes.String("2"),
			},
			// relations
			{
				bundle.RelationKeyId:          pbtypes.String("rel1"),
				bundle.RelationKeyRelationKey: pbtypes.String(bundle.RelationKeyName.String()),
				bundle.RelationKeyLayout:      pbtypes.Int64(int64(model.ObjectType_relation)),
			},
			{
				bundle.RelationKeyId:             pbtypes.String("rel2"),
				bundle.RelationKeyRelationKey:    pbtypes.String(testRelationKey),
				bundle.RelationKeyLayout:         pbtypes.Int64(int64(model.ObjectType_relation)),
				bundle.RelationKeyRelationFormat: pbtypes.Int64(int64(model.RelationFormat_object)),
			},
		})

		s, err := fx.getSpaceSubscriptions(testSpaceId)
		require.NoError(t, err)

		s.ds = newDependencyService(s)

		resp, err := fx.Search(SubscribeRequest{
			SpaceId:           testSpaceId,
			SubId:             subscriptionID,
			Keys:              []string{bundle.RelationKeyName.String(), bundle.RelationKeyId.String(), testRelationKey},
			CollectionId:      collectionID,
			NoDepSubscription: true,
		})

		require.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Len(t, resp.Records, 1)
		assert.Len(t, resp.Dependencies, 0)
		assert.Equal(t, "1", pbtypes.GetString(resp.Records[0], bundle.RelationKeyName.String()))
		assert.Equal(t, "1", pbtypes.GetString(resp.Records[0], bundle.RelationKeyId.String()))
	})
	t.Run("collection: collection has object with dependency - return objects with dependency", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.a.Close(context.Background())
		defer fx.ctrl.Finish()

		collectionID := "id"
		subscriptionID := "subId"
		testRelationKey := "link_to_object"

		fx.collectionService.EXPECT().SubscribeForCollection(collectionID, subscriptionID).Return([]string{"1"}, nil, nil)
		fx.collectionService.EXPECT().UnsubscribeFromCollection(collectionID, subscriptionID).Return()

		fx.store.AddObjects(t, testSpaceId, []spaceindex.TestObject{
			{
				bundle.RelationKeyId:                pbtypes.String("1"),
				bundle.RelationKeyName:              pbtypes.String("1"),
				domain.RelationKey(testRelationKey): pbtypes.StringList([]string{"2"}),
			},
			// relations
			{
				bundle.RelationKeyId:          pbtypes.String("rel1"),
				bundle.RelationKeyRelationKey: pbtypes.String(bundle.RelationKeyName.String()),
				bundle.RelationKeyLayout:      pbtypes.Int64(int64(model.ObjectType_relation)),
			},
			{
				bundle.RelationKeyId:             pbtypes.String("rel2"),
				bundle.RelationKeyRelationKey:    pbtypes.String(testRelationKey),
				bundle.RelationKeyLayout:         pbtypes.Int64(int64(model.ObjectType_relation)),
				bundle.RelationKeyRelationFormat: pbtypes.Int64(int64(model.RelationFormat_object)),
			},
			// deps
			{
				bundle.RelationKeyId:   pbtypes.String("2"),
				bundle.RelationKeyName: pbtypes.String("2"),
			},
		})

		s, err := fx.getSpaceSubscriptions(testSpaceId)
		require.NoError(t, err)
		s.ds = newDependencyService(s)

		resp, err := fx.Search(SubscribeRequest{
			SpaceId:           testSpaceId,
			SubId:             subscriptionID,
			Keys:              []string{bundle.RelationKeyName.String(), bundle.RelationKeyId.String(), testRelationKey},
			CollectionId:      collectionID,
			NoDepSubscription: false,
		})

		require.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Len(t, resp.Records, 1)
		assert.Len(t, resp.Dependencies, 1)
		assert.Equal(t, "1", pbtypes.GetString(resp.Records[0], bundle.RelationKeyName.String()))
		assert.Equal(t, "1", pbtypes.GetString(resp.Records[0], bundle.RelationKeyId.String()))
		assert.Equal(t, "2", pbtypes.GetString(resp.Dependencies[0], bundle.RelationKeyName.String()))
		assert.Equal(t, "2", pbtypes.GetString(resp.Dependencies[0], bundle.RelationKeyId.String()))
	})

	t.Run("collection: collection has 3 objects, but limit = 2 - return 2 objects in response", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.a.Close(context.Background())
		defer fx.ctrl.Finish()

		collectionID := "id"
		subscriptionID := "subId"

		fx.collectionService.EXPECT().SubscribeForCollection(collectionID, subscriptionID).Return([]string{"1", "2", "3"}, nil, nil)
		fx.collectionService.EXPECT().UnsubscribeFromCollection(collectionID, subscriptionID).Return()

		fx.store.AddObjects(t, testSpaceId, []spaceindex.TestObject{
			{
				bundle.RelationKeyId:   pbtypes.String("1"),
				bundle.RelationKeyName: pbtypes.String("1"),
			},
			{
				bundle.RelationKeyId:   pbtypes.String("2"),
				bundle.RelationKeyName: pbtypes.String("2"),
			},
			{
				bundle.RelationKeyId:   pbtypes.String("3"),
				bundle.RelationKeyName: pbtypes.String("3"),
			},
			// relations
			{
				bundle.RelationKeyId:          pbtypes.String("rel1"),
				bundle.RelationKeyRelationKey: pbtypes.String(bundle.RelationKeyName.String()),
				bundle.RelationKeyLayout:      pbtypes.Int64(int64(model.ObjectType_relation)),
			},
			{
				bundle.RelationKeyId:          pbtypes.String("rel2"),
				bundle.RelationKeyRelationKey: pbtypes.String(bundle.RelationKeyId.String()),
				bundle.RelationKeyLayout:      pbtypes.Int64(int64(model.ObjectType_relation)),
			},
		})

		var resp, err = fx.Search(SubscribeRequest{
			SpaceId:      testSpaceId,
			SubId:        subscriptionID,
			Keys:         []string{bundle.RelationKeyName.String(), bundle.RelationKeyId.String()},
			CollectionId: collectionID,
			Limit:        1,
		})

		require.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Len(t, resp.Records, 1)
		assert.Equal(t, "1", pbtypes.GetString(resp.Records[0], bundle.RelationKeyName.String()))
		assert.Equal(t, "1", pbtypes.GetString(resp.Records[0], bundle.RelationKeyId.String()))
	})
	t.Run("Search: call onChange for records", func(t *testing.T) {
		// given
		fx := newFixture(t)
		defer fx.a.Close(context.Background())
		defer fx.ctrl.Finish()

		newSub(fx, "test")
		newSub(fx, "test2")
		spaceSub, err := fx.getSpaceSubscriptions(testSpaceId)
		assert.NoError(t, err)
		e := spaceSub.ctxBuf.c.Get("1")
		e.subFullDetailsSent["test2"] = false

		// when
		spaceSub.onChange([]*entry{
			newEntry("1", &types.Struct{Fields: map[string]*types.Value{
				"id":     pbtypes.String("1"),
				"name":   pbtypes.String("one"),
				"author": pbtypes.StringList([]string{"author2", "author3", "1"}),
			}}),
		})

		// then
		assert.NotNil(t, spaceSub.ctxBuf.outputs[defaultOutput][0].GetObjectDetailsAmend())
		assert.Contains(t, spaceSub.ctxBuf.outputs[defaultOutput][0].GetObjectDetailsAmend().GetSubIds(), "test")
		assert.Contains(t, spaceSub.ctxBuf.outputs[defaultOutput][1].GetObjectDetailsSet().GetSubIds(), "test2")
	})
	t.Run("SubscribeGroup: tag group", func(t *testing.T) {
		// given
		fx := newFixtureWithRealObjectStore(t)

		source := "source"

		relationKey := "key"
		subID := "subId"
		collectionID := "collectionId"

		fx.collectionService.EXPECT().SubscribeForCollection(collectionID, subID).Return([]string{"1"}, nil, nil).Times(1)
		fx.collectionService.EXPECT().UnsubscribeFromCollection(collectionID, subID).Return().Times(1)

		defer fx.a.Close(context.Background())
		defer fx.ctrl.Finish()
		objectTypeKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeObjectType, source)
		assert.Nil(t, err)

		relationUniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeRelation, relationKey)
		assert.Nil(t, err)

		fx.store.AddObjects(t, testSpaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:             pbtypes.String(relationKey),
				bundle.RelationKeyUniqueKey:      pbtypes.String(relationUniqueKey.Marshal()),
				bundle.RelationKeySpaceId:        pbtypes.String(testSpaceId),
				bundle.RelationKeyRelationFormat: pbtypes.Int64(int64(model.RelationFormat_tag)),
				bundle.RelationKeyLayout:         pbtypes.Int64(int64(model.ObjectType_relation)),
			},
			{
				bundle.RelationKeyId:        pbtypes.String(source),
				bundle.RelationKeyUniqueKey: pbtypes.String(objectTypeKey.Marshal()),
				bundle.RelationKeySpaceId:   pbtypes.String(testSpaceId),
				bundle.RelationKeyLayout:    pbtypes.Int64(int64(model.ObjectType_objectType)),
			},
			{
				bundle.RelationKeyId:          pbtypes.String("1"),
				bundle.RelationKeySpaceId:     pbtypes.String(testSpaceId),
				bundle.RelationKeyRelationKey: pbtypes.String(relationKey),
				bundle.RelationKeyLayout:      pbtypes.Int64(int64(model.ObjectType_relationOption)),
			},
			{
				bundle.RelationKeyId:          pbtypes.String("2"),
				bundle.RelationKeySpaceId:     pbtypes.String(testSpaceId),
				bundle.RelationKeyRelationKey: pbtypes.String(relationKey),
				bundle.RelationKeyLayout:      pbtypes.Int64(int64(model.ObjectType_relationOption)),
			},
		})

		// when
		groups, err := fx.SubscribeGroups(pb.RpcObjectGroupsSubscribeRequest{
			SpaceId:      testSpaceId,
			RelationKey:  relationKey,
			Source:       []string{source},
			SubId:        subID,
			CollectionId: collectionID,
		})

		// then
		assert.Nil(t, err)
		assert.NotNil(t, groups)
		assert.Equal(t, subID, groups.SubId)
		assert.Len(t, groups.Groups, 3)

		tagGroup := groups.Groups[0].Value.(*model.BlockContentDataviewGroupValueOfTag)
		assert.Len(t, tagGroup.Tag.Ids, 0)

		tagGroup = groups.Groups[1].Value.(*model.BlockContentDataviewGroupValueOfTag)
		assert.Len(t, tagGroup.Tag.Ids, 1)
		assert.Equal(t, "1", tagGroup.Tag.Ids[0])

		tagGroup = groups.Groups[2].Value.(*model.BlockContentDataviewGroupValueOfTag)
		assert.Len(t, tagGroup.Tag.Ids, 1)
		assert.Equal(t, "2", tagGroup.Tag.Ids[0])
	})
	t.Run("SubscribeGroup: tag group - no tags for relation", func(t *testing.T) {
		// given
		fx := newFixtureWithRealObjectStore(t)

		source := "source"

		relationKey := "key"
		subID := "subId"
		collectionID := "collectionId"

		fx.collectionService.EXPECT().SubscribeForCollection(collectionID, subID).Return([]string{"1"}, nil, nil).Times(1)
		fx.collectionService.EXPECT().UnsubscribeFromCollection(collectionID, subID).Return().Times(1)

		defer fx.a.Close(context.Background())
		defer fx.ctrl.Finish()
		objectTypeKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeObjectType, source)
		assert.Nil(t, err)

		relationUniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeRelation, relationKey)
		assert.Nil(t, err)

		fx.store.AddObjects(t, testSpaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:             pbtypes.String(relationKey),
				bundle.RelationKeyUniqueKey:      pbtypes.String(relationUniqueKey.Marshal()),
				bundle.RelationKeySpaceId:        pbtypes.String(testSpaceId),
				bundle.RelationKeyRelationFormat: pbtypes.Int64(int64(model.RelationFormat_tag)),
				bundle.RelationKeyLayout:         pbtypes.Int64(int64(model.ObjectType_relation)),
			},
			{
				bundle.RelationKeyId:        pbtypes.String(source),
				bundle.RelationKeyUniqueKey: pbtypes.String(objectTypeKey.Marshal()),
				bundle.RelationKeySpaceId:   pbtypes.String(testSpaceId),
				bundle.RelationKeyLayout:    pbtypes.Int64(int64(model.ObjectType_objectType)),
			},
		})

		// when
		groups, err := fx.SubscribeGroups(pb.RpcObjectGroupsSubscribeRequest{
			SpaceId:      testSpaceId,
			RelationKey:  relationKey,
			Source:       []string{source},
			SubId:        subID,
			CollectionId: collectionID,
		})

		// then
		assert.Nil(t, err)
		assert.NotNil(t, groups)
		assert.Equal(t, subID, groups.SubId)
		assert.Len(t, groups.Groups, 1)

		tagGroup := groups.Groups[0].Value.(*model.BlockContentDataviewGroupValueOfTag)
		assert.Len(t, tagGroup.Tag.Ids, 0)
	})
	t.Run("SubscribeGroup: status group", func(t *testing.T) {
		// given
		fx := newFixtureWithRealObjectStore(t)

		source := "source"

		relationKey := "key"

		defer fx.a.Close(context.Background())
		defer fx.ctrl.Finish()
		objectTypeKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeObjectType, source)
		assert.Nil(t, err)

		relationUniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeRelation, relationKey)
		assert.Nil(t, err)

		fx.store.AddObjects(t, testSpaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:             pbtypes.String(relationKey),
				bundle.RelationKeyUniqueKey:      pbtypes.String(relationUniqueKey.Marshal()),
				bundle.RelationKeySpaceId:        pbtypes.String(testSpaceId),
				bundle.RelationKeyRelationFormat: pbtypes.Int64(int64(model.RelationFormat_status)),
				bundle.RelationKeyLayout:         pbtypes.Int64(int64(model.ObjectType_relation)),
			},
			{
				bundle.RelationKeyId:        pbtypes.String(source),
				bundle.RelationKeyUniqueKey: pbtypes.String(objectTypeKey.Marshal()),
				bundle.RelationKeySpaceId:   pbtypes.String(testSpaceId),
				bundle.RelationKeyLayout:    pbtypes.Int64(int64(model.ObjectType_objectType)),
			},
			{
				bundle.RelationKeyId:          pbtypes.String("1"),
				bundle.RelationKeySpaceId:     pbtypes.String(testSpaceId),
				bundle.RelationKeyRelationKey: pbtypes.String(relationKey),
				bundle.RelationKeyLayout:      pbtypes.Int64(int64(model.ObjectType_relationOption)),
				bundle.RelationKeyName:        pbtypes.String("Done"),
			},
			{
				bundle.RelationKeyId:          pbtypes.String("2"),
				bundle.RelationKeySpaceId:     pbtypes.String(testSpaceId),
				bundle.RelationKeyRelationKey: pbtypes.String(relationKey),
				bundle.RelationKeyLayout:      pbtypes.Int64(int64(model.ObjectType_relationOption)),
				bundle.RelationKeyName:        pbtypes.String("Not started"),
			},
		})

		// when
		groups, err := fx.SubscribeGroups(pb.RpcObjectGroupsSubscribeRequest{
			SpaceId:     testSpaceId,
			RelationKey: relationKey,
			Source:      []string{source},
		})

		// then
		assert.Nil(t, err)
		assert.NotNil(t, groups)
		assert.Len(t, groups.Groups, 3)

		tagGroup := groups.Groups[0].Value.(*model.BlockContentDataviewGroupValueOfStatus)
		assert.Equal(t, tagGroup.Status.Id, "")

		tagGroup = groups.Groups[1].Value.(*model.BlockContentDataviewGroupValueOfStatus)
		assert.Equal(t, tagGroup.Status.Id, "1")

		tagGroup = groups.Groups[2].Value.(*model.BlockContentDataviewGroupValueOfStatus)
		assert.Equal(t, tagGroup.Status.Id, "2")
	})

	t.Run("SubscribeGroup: status group - no statuses for relation", func(t *testing.T) {
		// given
		fx := newFixtureWithRealObjectStore(t)

		source := "source"

		relationKey := "key"

		defer fx.a.Close(context.Background())
		defer fx.ctrl.Finish()
		objectTypeKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeObjectType, source)
		assert.Nil(t, err)

		relationUniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeRelation, relationKey)
		assert.Nil(t, err)

		fx.store.AddObjects(t, testSpaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:             pbtypes.String(relationKey),
				bundle.RelationKeyUniqueKey:      pbtypes.String(relationUniqueKey.Marshal()),
				bundle.RelationKeySpaceId:        pbtypes.String(testSpaceId),
				bundle.RelationKeyRelationFormat: pbtypes.Int64(int64(model.RelationFormat_status)),
				bundle.RelationKeyLayout:         pbtypes.Int64(int64(model.ObjectType_relation)),
			},
			{
				bundle.RelationKeyId:        pbtypes.String(source),
				bundle.RelationKeyUniqueKey: pbtypes.String(objectTypeKey.Marshal()),
				bundle.RelationKeySpaceId:   pbtypes.String(testSpaceId),
				bundle.RelationKeyLayout:    pbtypes.Int64(int64(model.ObjectType_objectType)),
			},
		})

		// when
		groups, err := fx.SubscribeGroups(pb.RpcObjectGroupsSubscribeRequest{
			SpaceId:     testSpaceId,
			RelationKey: relationKey,
			Source:      []string{source},
		})

		// then
		assert.Nil(t, err)
		assert.NotNil(t, groups)
		assert.Len(t, groups.Groups, 1)

		tagGroup := groups.Groups[0].Value.(*model.BlockContentDataviewGroupValueOfStatus)
		assert.Equal(t, tagGroup.Status.Id, "")
	})

	t.Run("SubscribeGroup: checkbox group", func(t *testing.T) {
		// given
		fx := newFixtureWithRealObjectStore(t)

		source := "source"

		relationKey := "key"

		defer fx.a.Close(context.Background())
		defer fx.ctrl.Finish()
		objectTypeKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeObjectType, source)
		assert.Nil(t, err)

		relationUniqueKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeRelation, relationKey)
		assert.Nil(t, err)

		fx.store.AddObjects(t, testSpaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:             pbtypes.String(relationKey),
				bundle.RelationKeyUniqueKey:      pbtypes.String(relationUniqueKey.Marshal()),
				bundle.RelationKeySpaceId:        pbtypes.String(testSpaceId),
				bundle.RelationKeyRelationFormat: pbtypes.Int64(int64(model.RelationFormat_checkbox)),
				bundle.RelationKeyLayout:         pbtypes.Int64(int64(model.ObjectType_relation)),
			},
			{
				bundle.RelationKeyId:        pbtypes.String(source),
				bundle.RelationKeyUniqueKey: pbtypes.String(objectTypeKey.Marshal()),
				bundle.RelationKeySpaceId:   pbtypes.String(testSpaceId),
				bundle.RelationKeyLayout:    pbtypes.Int64(int64(model.ObjectType_objectType)),
			},
		})

		// when
		groups, err := fx.SubscribeGroups(pb.RpcObjectGroupsSubscribeRequest{
			SpaceId:     testSpaceId,
			RelationKey: relationKey,
			Source:      []string{source},
		})

		// then
		assert.Nil(t, err)
		assert.NotNil(t, groups)
		assert.Len(t, groups.Groups, 2)

		tagGroup := groups.Groups[0].Value.(*model.BlockContentDataviewGroupValueOfCheckbox)
		assert.Equal(t, tagGroup.Checkbox.Checked, true)

		tagGroup = groups.Groups[1].Value.(*model.BlockContentDataviewGroupValueOfCheckbox)
		assert.Equal(t, tagGroup.Checkbox.Checked, false)
	})
	t.Run("SubscribeIdsReq: 1 active records", func(t *testing.T) {
		// given
		fx := newFixtureWithRealObjectStore(t)

		id := "id"
		defer fx.a.Close(context.Background())
		defer fx.ctrl.Finish()
		fx.store.AddObjects(t, testSpaceId, []objectstore.TestObject{{bundle.RelationKeyId: pbtypes.String(id), bundle.RelationKeyName: pbtypes.String("name")}})

		// when
		sub, err := fx.SubscribeIdsReq(pb.RpcObjectSubscribeIdsRequest{
			SpaceId: testSpaceId,
			Ids:     []string{id},
			SubId:   "subID",
			Keys:    []string{bundle.RelationKeyId.String()},
		})

		// then
		assert.Nil(t, err)
		assert.NotNil(t, sub)
		assert.Equal(t, "subID", sub.SubId)
		assert.Len(t, sub.Dependencies, 0)
		assert.Len(t, sub.Records, 1)
		assert.Equal(t, id, sub.Records[0].GetFields()[bundle.RelationKeyId.String()].GetStringValue())
	})
	t.Run("SubscribeIdsReq: active records with keys from request", func(t *testing.T) {
		// given
		fx := newFixtureWithRealObjectStore(t)

		id := "id"
		relationValue := "value"
		defer fx.a.Close(context.Background())
		defer fx.ctrl.Finish()

		fx.store.AddObjects(t, testSpaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:   pbtypes.String(id),
				bundle.RelationKeyName: pbtypes.String(relationValue),
			},
		})

		// when
		sub, err := fx.SubscribeIdsReq(pb.RpcObjectSubscribeIdsRequest{
			SpaceId: testSpaceId,
			Ids:     []string{id},
			Keys:    []string{bundle.RelationKeyName.String()},
		})

		// then
		assert.Nil(t, err)
		assert.NotNil(t, sub)
		assert.Len(t, sub.Dependencies, 0)
		assert.Len(t, sub.Records, 1)
		assert.Len(t, sub.Records[0].GetFields(), 1)
		assert.Equal(t, relationValue, sub.Records[0].GetFields()[bundle.RelationKeyName.String()].GetStringValue())
	})
	t.Run("SubscribeIdsReq: no active records", func(t *testing.T) {
		// given
		fx := newFixtureWithRealObjectStore(t)

		id := "id"
		defer fx.a.Close(context.Background())
		defer fx.ctrl.Finish()

		// when
		sub, err := fx.SubscribeIdsReq(pb.RpcObjectSubscribeIdsRequest{
			SpaceId: testSpaceId,
			Ids:     []string{id},
		})

		// then
		assert.Nil(t, err)
		assert.NotNil(t, sub)
		assert.Len(t, sub.Dependencies, 0)
		assert.Len(t, sub.Records, 0)
	})
}

func addTestObjects(t *testing.T, source, relationKey, option1, option2, testSpaceId string, fx *fixtureRealStore) error {
	objectTypeKey, err := domain.NewUniqueKey(smartblock.SmartBlockTypeObjectType, source)
	assert.Nil(t, err)
	fx.store.AddObjects(t, testSpaceId, []objectstore.TestObject{
		{
			bundle.RelationKeyId:            pbtypes.String("1"),
			bundle.RelationKeySpaceId:       pbtypes.String(testSpaceId),
			domain.RelationKey(relationKey): pbtypes.String(option1),
			bundle.RelationKeyLayout:        pbtypes.Int64(int64(model.ObjectType_basic)),
			bundle.RelationKeyName:          pbtypes.String("Object 1"),
			bundle.RelationKeyType:          pbtypes.String(objectTypeKey.Marshal()),
		},
		{
			bundle.RelationKeyId:            pbtypes.String("2"),
			bundle.RelationKeySpaceId:       pbtypes.String(testSpaceId),
			domain.RelationKey(relationKey): pbtypes.String(option2),
			bundle.RelationKeyLayout:        pbtypes.Int64(int64(model.ObjectType_basic)),
			bundle.RelationKeyName:          pbtypes.String("Object 2"),
			bundle.RelationKeyType:          pbtypes.String(objectTypeKey.Marshal()),
		},
	})
	return err
}

func addTestObjectsForNestedFilters(t *testing.T, fx *fixtureRealStore, testSpaceId, option1, option2, option3, tag1, tag2 string) {
	fx.store.AddObjects(t, testSpaceId, []objectstore.TestObject{
		{
			bundle.RelationKeyId:      pbtypes.String("1"),
			bundle.RelationKeySpaceId: pbtypes.String(testSpaceId),
			bundle.RelationKeyStatus:  pbtypes.String(option1),
			bundle.RelationKeyLayout:  pbtypes.Int64(int64(model.ObjectType_basic)),
			bundle.RelationKeyName:    pbtypes.String("Object 1"),
			bundle.RelationKeyType:    pbtypes.String(bundle.TypeKeyPage.String()),
			bundle.RelationKeyTag:     pbtypes.StringList([]string{tag1}),
			bundle.RelationKeyDueDate: pbtypes.Int64(1704070917),
		},
		{
			bundle.RelationKeyId:      pbtypes.String("2"),
			bundle.RelationKeySpaceId: pbtypes.String(testSpaceId),
			bundle.RelationKeyStatus:  pbtypes.String(option3),
			bundle.RelationKeyLayout:  pbtypes.Int64(int64(model.ObjectType_basic)),
			bundle.RelationKeyName:    pbtypes.String("Object 2"),
			bundle.RelationKeyType:    pbtypes.String(bundle.TypeKeyPage.String()),
			bundle.RelationKeyTag:     pbtypes.StringList([]string{tag2}),
			bundle.RelationKeyDueDate: pbtypes.Int64(1709254917),
		},
		{
			bundle.RelationKeyId:      pbtypes.String("3"),
			bundle.RelationKeySpaceId: pbtypes.String(testSpaceId),
			bundle.RelationKeyStatus:  pbtypes.String(option2),
			bundle.RelationKeyLayout:  pbtypes.Int64(int64(model.ObjectType_basic)),
			bundle.RelationKeyName:    pbtypes.String("Object 3"),
			bundle.RelationKeyType:    pbtypes.String(bundle.TypeKeyPage.String()),
			bundle.RelationKeyTag:     pbtypes.StringList([]string{tag1, tag2}),
			bundle.RelationKeyDueDate: pbtypes.Int64(1711933317),
		},
		{
			bundle.RelationKeyId:      pbtypes.String("4"),
			bundle.RelationKeySpaceId: pbtypes.String(testSpaceId),
			bundle.RelationKeyStatus:  pbtypes.String(option1),
			bundle.RelationKeyLayout:  pbtypes.Int64(int64(model.ObjectType_basic)),
			bundle.RelationKeyName:    pbtypes.String("Object 4"),
			bundle.RelationKeyType:    pbtypes.String(bundle.TypeKeyPage.String()),
			bundle.RelationKeyDueDate: pbtypes.Int64(1714525317),
		},
	})
}

func prepareNestedFiltersWithOperator(operator model.BlockContentDataviewFilterOperator, option1 string, option2 string, tag1 string) []*model.BlockContentDataviewFilter {
	return []*model.BlockContentDataviewFilter{
		{
			Operator: operator,
			NestedFilters: []*model.BlockContentDataviewFilter{
				{
					Operator: model.BlockContentDataviewFilter_Or,
					NestedFilters: []*model.BlockContentDataviewFilter{
						{
							Operator:    model.BlockContentDataviewFilter_No,
							RelationKey: bundle.RelationKeyName.String(),
							Condition:   model.BlockContentDataviewFilter_Equal,
							Value:       pbtypes.String("Object 1"),
							Format:      model.RelationFormat_shorttext,
						},
						{
							Operator:    model.BlockContentDataviewFilter_No,
							RelationKey: bundle.RelationKeyName.String(),
							Condition:   model.BlockContentDataviewFilter_Equal,
							Value:       pbtypes.String("Object 2"),
							Format:      model.RelationFormat_shorttext,
						},
						{
							Operator:    model.BlockContentDataviewFilter_No,
							RelationKey: bundle.RelationKeyName.String(),
							Condition:   model.BlockContentDataviewFilter_Equal,
							Value:       pbtypes.String("Object 3"),
							Format:      model.RelationFormat_shorttext,
						},
					},
				},
				{
					Operator: model.BlockContentDataviewFilter_And,
					NestedFilters: []*model.BlockContentDataviewFilter{
						{
							Operator:    model.BlockContentDataviewFilter_No,
							RelationKey: bundle.RelationKeyTag.String(),
							Condition:   model.BlockContentDataviewFilter_In,
							Value:       pbtypes.StringList([]string{tag1}),
							Format:      model.RelationFormat_tag,
						},
						{
							Operator: model.BlockContentDataviewFilter_Or,
							NestedFilters: []*model.BlockContentDataviewFilter{
								{
									Operator:    model.BlockContentDataviewFilter_No,
									RelationKey: bundle.RelationKeyStatus.String(),
									Condition:   model.BlockContentDataviewFilter_Equal,
									Value:       pbtypes.String(option1),
									Format:      model.RelationFormat_shorttext,
								},
								{
									Operator:    model.BlockContentDataviewFilter_No,
									RelationKey: bundle.RelationKeyName.String(),
									Condition:   model.BlockContentDataviewFilter_Equal,
									Value:       pbtypes.String(option2),
									Format:      model.RelationFormat_shorttext,
								},
							},
						},
					},
				},
				{
					Operator:    model.BlockContentDataviewFilter_No,
					RelationKey: bundle.RelationKeyDueDate.String(),
					Condition:   model.BlockContentDataviewFilter_Less,
					Value:       pbtypes.Int64(1709254917),
					Format:      model.RelationFormat_shorttext,
				},
			},
		},
	}
}

func xTestNestedSubscription(t *testing.T) {
	t.Run("update nested object, so it's not satisfying filter anymore", func(t *testing.T) {
		fx := testCreateSubscriptionWithNestedFilter(t)

		err := fx.store.SpaceIndex(testSpaceId).UpdateObjectDetails(context.Background(), "assignee1", &types.Struct{
			Fields: map[string]*types.Value{
				"id":   pbtypes.String("assignee1"),
				"name": pbtypes.String("John Doe"),
			},
		})
		require.NoError(t, err)

		fx.waitEvents(t,
			&pb.EventMessageValueOfSubscriptionRemove{
				SubscriptionRemove: &pb.EventObjectSubscriptionRemove{
					SubId: "test-nested-1",
					Id:    "assignee1",
				},
			},
			&pb.EventMessageValueOfSubscriptionRemove{
				SubscriptionRemove: &pb.EventObjectSubscriptionRemove{
					SubId: "test",
					Id:    "task1",
				},
			},
			&pb.EventMessageValueOfSubscriptionCounters{
				SubscriptionCounters: &pb.EventObjectSubscriptionCounters{
					SubId: "test-nested-1",
					Total: 0,
				},
			},
			&pb.EventMessageValueOfSubscriptionCounters{
				SubscriptionCounters: &pb.EventObjectSubscriptionCounters{
					SubId: "test",
					Total: 0,
				},
			})
	})

	t.Run("update parent object relation so no nested objects satisfy filter anymore", func(t *testing.T) {
		fx := testCreateSubscriptionWithNestedFilter(t)

		err := fx.store.SpaceIndex(testSpaceId).UpdateObjectDetails(context.Background(), "task1", &types.Struct{
			Fields: map[string]*types.Value{
				"id":       pbtypes.String("task1"),
				"assignee": pbtypes.String("assignee2"),
			},
		})
		require.NoError(t, err)
	})
}

func testCreateSubscriptionWithNestedFilter(t *testing.T) *fixtureRealStore {
	fx := newFixtureWithRealObjectStore(t)
	// fx.store.EXPECT().GetRelationFormatByKey(mock.Anything).Return(&model.Relation{}, nil)
	resp, err := fx.Search(SubscribeRequest{
		SpaceId: testSpaceId,
		SubId:   "test",
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: "assignee.name",
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String("Joe Doe"),
			},
		},
		Keys: []string{"id", "name"},
	})

	require.NoError(t, err)
	require.Empty(t, resp.Records)

	t.Run("add nested object", func(t *testing.T) {
		fx.store.AddObjects(t, testSpaceId, []objectstore.TestObject{
			{
				"id":   pbtypes.String("assignee1"),
				"name": pbtypes.String("Joe Doe"),
			},
		})
		fx.waitEvents(t,
			&pb.EventMessageValueOfObjectDetailsSet{
				ObjectDetailsSet: &pb.EventObjectDetailsSet{
					Id: "assignee1",
					SubIds: []string{
						"test-nested-1",
					},
					Details: &types.Struct{
						Fields: map[string]*types.Value{
							"id": pbtypes.String("assignee1"),
						},
					},
				},
			},
			&pb.EventMessageValueOfSubscriptionAdd{
				SubscriptionAdd: &pb.EventObjectSubscriptionAdd{
					SubId: "test-nested-1",
					Id:    "assignee1",
				},
			},
			&pb.EventMessageValueOfSubscriptionCounters{
				SubscriptionCounters: &pb.EventObjectSubscriptionCounters{
					SubId: "test-nested-1",
					Total: 1,
				},
			})
	})

	t.Run("add object satisfying nested filter", func(t *testing.T) {
		fx.store.AddObjects(t, testSpaceId, []objectstore.TestObject{
			{
				"id":       pbtypes.String("task1"),
				"assignee": pbtypes.String("assignee1"),
			},
		})
		fx.waitEvents(t,
			&pb.EventMessageValueOfObjectDetailsSet{
				ObjectDetailsSet: &pb.EventObjectDetailsSet{
					Id: "task1",
					SubIds: []string{
						"test",
					},
					Details: &types.Struct{
						Fields: map[string]*types.Value{
							"id": pbtypes.String("task1"),
						},
					},
				},
			},
			&pb.EventMessageValueOfSubscriptionAdd{
				SubscriptionAdd: &pb.EventObjectSubscriptionAdd{
					SubId: "test",
					Id:    "task1",
				},
			},
			&pb.EventMessageValueOfSubscriptionCounters{
				SubscriptionCounters: &pb.EventObjectSubscriptionCounters{
					SubId: "test",
					Total: 1,
				},
			})
	})
	return fx
}
