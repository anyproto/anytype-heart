package subscription

import (
	"context"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestService_Search(t *testing.T) {
	var newSub = func(fx *fixture, subId string) {
		fx.store.EXPECT().QueryRaw(gomock.Any(), 0, 0).Return(
			[]database.Record{
				{Details: &types.Struct{Fields: map[string]*types.Value{
					"id":     pbtypes.String("1"),
					"name":   pbtypes.String("one"),
					"author": pbtypes.StringList([]string{"author1"}),
				}}},
			},
			nil,
		)
		fx.store.EXPECT().GetRelationByKey(bundle.RelationKeyName.String()).Return(&model.Relation{
			Key:    bundle.RelationKeyName.String(),
			Format: model.RelationFormat_shorttext,
		}, nil).AnyTimes()
		fx.store.EXPECT().GetRelationByKey(bundle.RelationKeyAuthor.String()).Return(&model.Relation{
			Key:    bundle.RelationKeyAuthor.String(),
			Format: model.RelationFormat_object,
		}, nil).AnyTimes()

		fx.store.EXPECT().QueryByID([]string{"author1"}).Return([]database.Record{
			{Details: &types.Struct{Fields: map[string]*types.Value{
				"id":   pbtypes.String("author1"),
				"name": pbtypes.String("author1"),
			}}},
		}, nil).AnyTimes()

		resp, err := fx.Search(pb.RpcObjectSearchSubscribeRequest{
			SubId: subId,
			Keys:  []string{bundle.RelationKeyName.String(), bundle.RelationKeyAuthor.String()},
		})
		require.NoError(t, err)

		assert.Len(t, resp.Records, 1)
		assert.Len(t, resp.Dependencies, 1)
	}

	t.Run("dependencies", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.a.Close(context.Background())
		defer fx.ctrl.Finish()

		newSub(fx, "test")

		fx.store.EXPECT().QueryByID([]string{"author2", "author3"}).Return([]database.Record{
			{Details: &types.Struct{Fields: map[string]*types.Value{
				"id":   pbtypes.String("author2"),
				"name": pbtypes.String("author2"),
			}}},
			{Details: &types.Struct{Fields: map[string]*types.Value{
				"id":   pbtypes.String("author3"),
				"name": pbtypes.String("author3"),
			}}},
		}, nil)

		fx.Service.(*service).onChange([]*entry{
			{id: "1", data: &types.Struct{Fields: map[string]*types.Value{
				"id":     pbtypes.String("1"),
				"name":   pbtypes.String("one"),
				"author": pbtypes.StringList([]string{"author2", "author3", "1"}),
			}}},
		})

		require.Len(t, fx.Service.(*service).cache.entries, 3)
		assert.Len(t, fx.Service.(*service).cache.entries["1"].SubIds(), 2)
		assert.Len(t, fx.Service.(*service).cache.entries["author2"].SubIds(), 1)
		assert.Len(t, fx.Service.(*service).cache.entries["author3"].SubIds(), 1)

		fx.events = fx.events[:0]

		fx.Service.(*service).onChange([]*entry{
			{id: "1", data: &types.Struct{Fields: map[string]*types.Value{
				"id":   pbtypes.String("1"),
				"name": pbtypes.String("one"),
			}}},
		})

		assert.Len(t, fx.Service.(*service).cache.entries, 1)

		assert.NoError(t, fx.Unsubscribe("test"))
		assert.Len(t, fx.Service.(*service).cache.entries, 0)
	})
	t.Run("cache ref counter", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.a.Close(context.Background())
		defer fx.ctrl.Finish()

		newSub(fx, "test")

		require.Len(t, fx.Service.(*service).cache.entries, 2)
		assert.Equal(t, []string{"test"}, fx.Service.(*service).cache.entries["1"].SubIds())
		assert.Equal(t, []string{"test/dep"}, fx.Service.(*service).cache.entries["author1"].SubIds())

		newSub(fx, "test1")

		require.Len(t, fx.Service.(*service).cache.entries, 2)
		assert.Len(t, fx.Service.(*service).cache.entries["1"].SubIds(), 2)
		assert.Len(t, fx.Service.(*service).cache.entries["author1"].SubIds(), 2)
	})

	t.Run("filter deps", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.a.Close(context.Background())
		defer fx.ctrl.Finish()

		fx.store.EXPECT().QueryRaw(gomock.Any(), 0, 0).Return(
			[]database.Record{
				{Details: &types.Struct{Fields: map[string]*types.Value{
					"id":   pbtypes.String("1"),
					"name": pbtypes.String("one"),
				}}},
			},
			nil,
		)
		fx.store.EXPECT().GetRelationByKey(bundle.RelationKeyName.String()).Return(&model.Relation{
			Key:    bundle.RelationKeyName.String(),
			Format: model.RelationFormat_shorttext,
		}, nil).AnyTimes()
		fx.store.EXPECT().GetRelationByKey(bundle.RelationKeyAuthor.String()).Return(&model.Relation{
			Key:    bundle.RelationKeyAuthor.String(),
			Format: model.RelationFormat_object,
		}, nil).AnyTimes()

		fx.store.EXPECT().QueryByID([]string{"force1", "force2"}).Return([]database.Record{
			{Details: &types.Struct{Fields: map[string]*types.Value{
				"id":   pbtypes.String("force1"),
				"name": pbtypes.String("force1"),
			}}},
			{Details: &types.Struct{Fields: map[string]*types.Value{
				"id":   pbtypes.String("force2"),
				"name": pbtypes.String("force2"),
			}}},
		}, nil)

		var resp, err = fx.Search(pb.RpcObjectSearchSubscribeRequest{
			SubId: "subId",
			Keys:  []string{bundle.RelationKeyName.String(), bundle.RelationKeyAuthor.String()},
			Filters: []*model.BlockContentDataviewFilter{
				{
					RelationKey: bundle.RelationKeyAuthor.String(),
					Condition:   model.BlockContentDataviewFilter_Equal,
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

		fx.store.EXPECT().QueryRaw(gomock.Any(), 0, 0).Return(
			[]database.Record{
				{Details: &types.Struct{Fields: map[string]*types.Value{
					"id":   pbtypes.String("1"),
					"name": pbtypes.String("1"),
				}}},
				{Details: &types.Struct{Fields: map[string]*types.Value{
					"id":   pbtypes.String("2"),
					"name": pbtypes.String("2"),
				}}},
				{Details: &types.Struct{Fields: map[string]*types.Value{
					"id":   pbtypes.String("3"),
					"name": pbtypes.String("3"),
				}}},
			},
			nil,
		)
		fx.store.EXPECT().GetRelationByKey(bundle.RelationKeyName.String()).Return(&model.Relation{
			Key:    bundle.RelationKeyName.String(),
			Format: model.RelationFormat_shorttext,
		}, nil).AnyTimes()

		resp, err := fx.Search(pb.RpcObjectSearchSubscribeRequest{
			SubId: "test",
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

		fx.Service.(*service).onChange([]*entry{
			{id: "1", data: &types.Struct{Fields: map[string]*types.Value{
				"id":   pbtypes.String("1"),
				"name": pbtypes.String("4"),
			}}},
		})
		// should be 1,3 (2)
		require.Len(t, fx.events[0].Messages, 3)
		assert.NotEmpty(t, fx.events[0].Messages[0].GetObjectDetailsSet())
		assert.NotEmpty(t, fx.events[0].Messages[1].GetSubscriptionAdd())
		assert.NotEmpty(t, fx.events[0].Messages[2].GetSubscriptionRemove())

		fx.Service.(*service).onChange([]*entry{
			{id: "2", data: &types.Struct{Fields: map[string]*types.Value{
				"id":   pbtypes.String("2"),
				"name": pbtypes.String("6"),
			}}},
		})

		// should be 2,1 (3)
		t.Log(pbtypes.Sprint(fx.events[1]))
	})

	t.Run("delete item from list", func(t *testing.T) {
		fx := newFixture(t)
		defer fx.a.Close(context.Background())
		defer fx.ctrl.Finish()

		fx.store.EXPECT().QueryRaw(gomock.Any(), 0, 0).Return(
			[]database.Record{
				{Details: &types.Struct{Fields: map[string]*types.Value{
					"id":   pbtypes.String("1"),
					"name": pbtypes.String("1"),
				}}},
				{Details: &types.Struct{Fields: map[string]*types.Value{
					"id":   pbtypes.String("2"),
					"name": pbtypes.String("2"),
				}}},
				{Details: &types.Struct{Fields: map[string]*types.Value{
					"id":   pbtypes.String("3"),
					"name": pbtypes.String("3"),
				}}},
			},
			nil,
		)
		fx.store.EXPECT().GetRelationByKey(bundle.RelationKeyName.String()).Return(&model.Relation{
			Key:    bundle.RelationKeyName.String(),
			Format: model.RelationFormat_shorttext,
		}, nil).AnyTimes()

		resp, err := fx.Search(pb.RpcObjectSearchSubscribeRequest{
			SubId: "test",
			Sorts: []*model.BlockContentDataviewSort{
				{
					RelationKey: "name",
					Type:        model.BlockContentDataviewSort_Asc,
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

		fx.Service.(*service).onChange([]*entry{
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
}

func TestNestedSubscription(t *testing.T) {
	t.Run("update nested object, so it's not satisfying filter anymore", func(t *testing.T) {
		fx := testCreateSubscriptionWithNestedFilter(t)

		err := fx.store.UpdateObjectDetails("assignee1", &types.Struct{
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

		err := fx.store.UpdateObjectDetails("task1", &types.Struct{
			Fields: map[string]*types.Value{
				"id":       pbtypes.String("task1"),
				"assignee": pbtypes.String("assignee2"),
			},
		})
		require.NoError(t, err)
	})
}

func (c *collectionServiceMock) Name() string {
	return "collectionService"
}

func (c *collectionServiceMock) Init(a *app.App) error { return nil }

func testCreateSubscriptionWithNestedFilter(t *testing.T) *fixtureRealStore {
	fx := newFixtureWithRealObjectStore(t)
	resp, err := fx.Search(pb.RpcObjectSearchSubscribeRequest{
		SubId: "test",
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
		fx.store.AddObjects(t, []objectstore.TestObject{
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
		fx.store.AddObjects(t, []objectstore.TestObject{
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
