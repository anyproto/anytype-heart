package subscription

import (
	"fmt"
	"testing"

	"github.com/anyproto/any-store/anyenc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const spaceId = "space1"

type ssFixture struct {
	store  *objectstore.StoreFixture
	sender *mock_event.MockSender
	*spaceSubscriptions
}

func newSSFixture(t *testing.T) *ssFixture {
	store := objectstore.NewStoreFixture(t)
	sender := mock_event.NewMockSender(t)

	c := newCache()
	s := &spaceSubscriptions{
		objectStore: store.SpaceIndex(spaceId),
		eventSender: sender,

		cache:            c,
		subscriptionKeys: make([]string, 0, 1),
		subscriptions:    make(map[string]subscription, 1),
		customOutput:     map[string]*internalSubOutput{},
		ctxBuf:           &opCtx{spaceId: spaceId, c: c},
		arenaPool:        &anyenc.ArenaPool{},
	}

	ds := newDependencyService(s)
	s.ds = ds

	return &ssFixture{
		store:              store,
		sender:             sender,
		spaceSubscriptions: s,
	}
}

func buildTasksReq() SubscribeRequest {
	return SubscribeRequest{
		SpaceId: spaceId,
		Filters: []database.FilterRequest{{
			RelationKey: bundle.RelationKeyType,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.String("task"),
		}},
		Sorts: []database.SortRequest{{
			RelationKey: bundle.RelationKeyAssignee,
			Type:        model.BlockContentDataviewSort_Asc,
		}},
		Keys: []string{
			bundle.RelationKeyId.String(),
			bundle.RelationKeyType.String(),
			bundle.RelationKeyName.String(),
			bundle.RelationKeyAssignee.String(),
		},
		NoDepSubscription: false,
	}
}

func TestReorderIntegration(t *testing.T) {
	t.Run("task tracker with Kirill and Sasha", func(t *testing.T) {
		// given
		f := newSSFixture(t)
		f.store.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:   domain.String("task0"),
				bundle.RelationKeyType: domain.String("task"),
				bundle.RelationKeyName: domain.String("task0"),
			},
			{
				bundle.RelationKeyId:       domain.String("task1"),
				bundle.RelationKeyType:     domain.String("task"),
				bundle.RelationKeyName:     domain.String("task1"),
				bundle.RelationKeyAssignee: domain.StringList([]string{"aleksandr"}),
			},
			{
				bundle.RelationKeyId:       domain.String("task2"),
				bundle.RelationKeyType:     domain.String("task"),
				bundle.RelationKeyName:     domain.String("task2"),
				bundle.RelationKeyAssignee: domain.StringList([]string{"kirill"}),
			},
			{
				bundle.RelationKeyId:       domain.String("task3"),
				bundle.RelationKeyType:     domain.String("task"),
				bundle.RelationKeyName:     domain.String("task3"),
				bundle.RelationKeyAssignee: domain.StringList([]string{"kirill", "aleksandr"}),
			},
			{
				bundle.RelationKeyId:   domain.String("kirill"),
				bundle.RelationKeyName: domain.String("Kirill"),
			},
			{
				bundle.RelationKeyId:   domain.String("aleksandr"),
				bundle.RelationKeyName: domain.String("Alex"),
			},
		})

		resp, err := f.Search(buildTasksReq())

		assert.NoError(t, err)
		require.NotNil(t, resp)

		require.Len(t, resp.Records, 4)
		assert.Equal(t, resp.Records[0].GetString(bundle.RelationKeyId), "task0")
		assert.Equal(t, resp.Records[1].GetString(bundle.RelationKeyId), "task1")
		assert.Equal(t, resp.Records[2].GetString(bundle.RelationKeyId), "task2")
		assert.Equal(t, resp.Records[3].GetString(bundle.RelationKeyId), "task3")

		assert.Len(t, resp.Dependencies, 2)

		var events []*pb.EventMessage
		f.sender.EXPECT().Broadcast(mock.Anything).Run(func(event *pb.Event) {
			events = event.Messages
		})

		f.onChange([]*entry{
			newEntry("aleksandr", domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:   domain.String("aleksandr"),
				bundle.RelationKeyName: domain.String("Sasha"),
			})),
		})

		order := applySubscriptionEvents(resp.SubId, resp.Records, events)
		for pos, objectNum := range []int{0, 2, 3, 1} {
			assert.Equal(t, pos, order[fmt.Sprintf("task%d", objectNum)])
		}
	})

	t.Run("task tracker with Go Team", func(t *testing.T) {
		// given
		f := newSSFixture(t)
		f.store.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:       domain.String("task0"),
				bundle.RelationKeyType:     domain.String("task"),
				bundle.RelationKeyName:     domain.String("task0"),
				bundle.RelationKeyAssignee: domain.StringList([]string{"aleksandr"}),
			},
			{
				bundle.RelationKeyId:       domain.String("task1"),
				bundle.RelationKeyType:     domain.String("task"),
				bundle.RelationKeyName:     domain.String("task1"),
				bundle.RelationKeyAssignee: domain.StringList([]string{"kirill"}),
			},
			{
				bundle.RelationKeyId:       domain.String("task2"),
				bundle.RelationKeyType:     domain.String("task"),
				bundle.RelationKeyName:     domain.String("task2"),
				bundle.RelationKeyAssignee: domain.StringList([]string{"roma"}),
			},
			{
				bundle.RelationKeyId:       domain.String("task3"),
				bundle.RelationKeyType:     domain.String("task"),
				bundle.RelationKeyName:     domain.String("task3"),
				bundle.RelationKeyAssignee: domain.StringList([]string{"sergey"}),
			},
			{
				bundle.RelationKeyId:   domain.String("kirill"),
				bundle.RelationKeyName: domain.String("Kirill"),
			},
			{
				bundle.RelationKeyId:   domain.String("aleksandr"),
				bundle.RelationKeyName: domain.String("Alex"),
			},
			{
				bundle.RelationKeyId:   domain.String("sergey"),
				bundle.RelationKeyName: domain.String("Sergey"),
			},
			{
				bundle.RelationKeyId:   domain.String("roma"),
				bundle.RelationKeyName: domain.String("Roman"),
			},
		})

		resp, err := f.Search(buildTasksReq())

		assert.NoError(t, err)
		require.NotNil(t, resp)

		require.Len(t, resp.Records, 4)
		assert.Equal(t, resp.Records[0].GetString(bundle.RelationKeyId), "task0")
		assert.Equal(t, resp.Records[1].GetString(bundle.RelationKeyId), "task1")
		assert.Equal(t, resp.Records[2].GetString(bundle.RelationKeyId), "task2")
		assert.Equal(t, resp.Records[3].GetString(bundle.RelationKeyId), "task3")

		assert.Len(t, resp.Dependencies, 4)

		var events []*pb.EventMessage
		f.sender.EXPECT().Broadcast(mock.Anything).Run(func(event *pb.Event) {
			events = event.Messages
		})

		f.onChange([]*entry{
			newEntry("aleksandr", domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:   domain.String("aleksandr"),
				bundle.RelationKeyName: domain.String("Sasha"),
			})),
		})

		order := applySubscriptionEvents(resp.SubId, resp.Records, events)
		for pos, objectNum := range []int{1, 2, 0, 3} {
			assert.Equal(t, pos, order[fmt.Sprintf("task%d", objectNum)])
		}
	})

	t.Run("task tracker sorted by task names", func(t *testing.T) {
		// given
		f := newSSFixture(t)
		f.store.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:   domain.String("task0"),
				bundle.RelationKeyType: domain.String("task"),
				bundle.RelationKeyName: domain.String("task0"),
			},
			{
				bundle.RelationKeyId:   domain.String("task1"),
				bundle.RelationKeyType: domain.String("task"),
				bundle.RelationKeyName: domain.String("task1"),
			},
			{
				bundle.RelationKeyId:   domain.String("task2"),
				bundle.RelationKeyType: domain.String("task"),
				bundle.RelationKeyName: domain.String("task2"),
			},
			{
				bundle.RelationKeyId:   domain.String("task3"),
				bundle.RelationKeyType: domain.String("task"),
				bundle.RelationKeyName: domain.String("task3"),
			},
		})

		req := buildTasksReq()
		req.Sorts = []database.SortRequest{{
			RelationKey: bundle.RelationKeyName,
			Type:        model.BlockContentDataviewSort_Asc,
		}}
		resp, err := f.Search(req)

		assert.NoError(t, err)
		require.NotNil(t, resp)

		require.Len(t, resp.Records, 4)
		assert.Equal(t, resp.Records[0].GetString(bundle.RelationKeyId), "task0")
		assert.Equal(t, resp.Records[1].GetString(bundle.RelationKeyId), "task1")
		assert.Equal(t, resp.Records[2].GetString(bundle.RelationKeyId), "task2")
		assert.Equal(t, resp.Records[3].GetString(bundle.RelationKeyId), "task3")
		assert.Empty(t, resp.Dependencies)

		var events []*pb.EventMessage
		f.sender.EXPECT().Broadcast(mock.Anything).Run(func(event *pb.Event) {
			events = event.Messages
		})

		f.onChange([]*entry{
			newEntry("task2", domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:   domain.String("task2"),
				bundle.RelationKeyType: domain.String("task"),
				bundle.RelationKeyName: domain.String("job42"),
			})),
		})

		order := applySubscriptionEvents(resp.SubId, resp.Records, events)
		for pos, objectNum := range []int{2, 0, 1, 3} {
			assert.Equal(t, pos, order[fmt.Sprintf("task%d", objectNum)])
		}
	})
}

func TestDependencyService_EnregisterObjectSorts(t *testing.T) {
	t.Run("register object sorts with tag and object relations", func(t *testing.T) {
		fx := newSSFixture(t)

		// Add relation objects to store
		fx.store.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:             domain.String("rel_status"),
				bundle.RelationKeyRelationKey:    domain.String("status"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_status)),
				bundle.RelationKeyLayout:         domain.Int64(int64(model.ObjectType_relation)),
			},
			{
				bundle.RelationKeyId:             domain.String("rel_tag"),
				bundle.RelationKeyRelationKey:    domain.String("tag"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_tag)),
				bundle.RelationKeyLayout:         domain.Int64(int64(model.ObjectType_relation)),
			},
			{
				bundle.RelationKeyId:             domain.String("rel_assignee"),
				bundle.RelationKeyRelationKey:    domain.String("assignee"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_object)),
				bundle.RelationKeyLayout:         domain.Int64(int64(model.ObjectType_relation)),
			},
		})

		sorts := []database.SortRequest{
			{RelationKey: "status", Type: model.BlockContentDataviewSort_Asc, Format: model.RelationFormat_status},
			{RelationKey: "tag", Type: model.BlockContentDataviewSort_Asc, Format: model.RelationFormat_tag},
			{RelationKey: "assignee", Type: model.BlockContentDataviewSort_Asc, Format: model.RelationFormat_object},
			{RelationKey: "name", Type: model.BlockContentDataviewSort_Asc, Format: model.RelationFormat_shorttext}, // non-object relation
		}

		fx.ds.enregisterObjectSorts("sub1", sorts)

		// Verify that only object-format relations are registered
		assert.Contains(t, fx.ds.sorts, "sub1")
		assert.Len(t, fx.ds.sorts["sub1"], 3) // status, tag, assignee

		// Verify sortKey properties
		sortKeys := fx.ds.sorts["sub1"]
		for _, key := range sortKeys {
			switch key.key {
			case "status":
				assert.True(t, key.isTag)
				assert.Equal(t, bundle.RelationKeyOrderId, key.orderKey())
			case "tag":
				assert.True(t, key.isTag)
				assert.Equal(t, bundle.RelationKeyOrderId, key.orderKey())
			case "assignee":
				assert.False(t, key.isTag)
				assert.Equal(t, bundle.RelationKeyName, key.orderKey())
			}
		}
	})

	t.Run("skip non-object relations", func(t *testing.T) {
		fx := newSSFixture(t)

		sorts := []database.SortRequest{
			{RelationKey: "name", Type: model.BlockContentDataviewSort_Asc},
			{RelationKey: "description", Type: model.BlockContentDataviewSort_Asc},
		}

		fx.ds.enregisterObjectSorts("sub1", sorts)

		// Should not register any sorts for non-object relations
		assert.NotContains(t, fx.ds.sorts, "sub1")
	})

	t.Run("handle empty sorts", func(t *testing.T) {
		fx := newSSFixture(t)

		var sorts []database.SortRequest

		fx.ds.enregisterObjectSorts("sub1", sorts)

		// Should not register empty sorts
		assert.NotContains(t, fx.ds.sorts, "sub1")
	})
}

func TestDependencyService_DepIdsByEntries(t *testing.T) {
	t.Run("extract dependency IDs from entries", func(t *testing.T) {
		fx := newSSFixture(t)

		// Setup sorts to track dependencies
		fx.ds.sorts["sub1"] = []sortKey{
			{key: "assignee", isTag: false},
			{key: "status", isTag: true},
		}

		entries := []*entry{
			{
				id: "obj1",
				data: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
					"assignee": domain.StringList([]string{"user1", "user2"}),
					"status":   domain.StringList([]string{"status1"}),
					"tag":      domain.StringList([]string{"tag1"}), // not a sort key
				}),
			},
			{
				id: "obj2",
				data: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
					"assignee": domain.StringList([]string{"user3"}),
					"status":   domain.StringList([]string{""}), // empty value should be ignored
				}),
			},
		}

		depKeys := []domain.RelationKey{"assignee", "status", "tag"}
		forceIds := []string{"force1", "force2"}

		depIds := fx.ds.depIdsByEntries("sub1", entries, depKeys, forceIds)

		// Should include force IDs and unique dependency IDs
		expectedIds := []string{"force1", "force2", "user1", "user2", "status1", "tag1", "user3"}
		assert.ElementsMatch(t, expectedIds, depIds)

		// Verify dependency tracking for sort keys
		assert.Contains(t, fx.ds.depOrderObjects, "user1")
		assert.Contains(t, fx.ds.depOrderObjects, "user2")
		assert.Contains(t, fx.ds.depOrderObjects, "user3")
		assert.Contains(t, fx.ds.depOrderObjects, "status1")
		assert.NotContains(t, fx.ds.depOrderObjects, "tag1") // not a sort key

		// Verify subscription tracking
		assert.Contains(t, fx.ds.depOrderObjects["user1"], "sub1")
		assert.Contains(t, fx.ds.depOrderObjects["status1"], "sub1")
	})

	t.Run("ignore self-references and duplicates", func(t *testing.T) {
		fx := newSSFixture(t)

		fx.ds.sorts["sub1"] = []sortKey{{key: "assignee", isTag: false}}

		entries := []*entry{
			{
				id: "obj1",
				data: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
					"assignee": domain.StringList([]string{"user1", "user1", "obj1"}), // duplicate and self-ref
				}),
			},
		}

		depKeys := []domain.RelationKey{"assignee"}
		forceIds := []string{"user1"} // duplicate with entry

		depIds := fx.ds.depIdsByEntries("sub1", entries, depKeys, forceIds)

		// Should only contain unique IDs and exclude self-references
		expectedIds := []string{"user1"}
		assert.ElementsMatch(t, expectedIds, depIds)
	})

	t.Run("handle empty entries", func(t *testing.T) {
		fx := newSSFixture(t)

		var entries []*entry
		depKeys := []domain.RelationKey{"assignee"}
		forceIds := []string{"force1"}

		depIds := fx.ds.depIdsByEntries("sub1", entries, depKeys, forceIds)

		// Should only return force IDs
		assert.Equal(t, []string{"force1"}, depIds)
	})
}
