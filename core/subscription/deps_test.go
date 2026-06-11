package subscription

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const assigneeKey = "assignee"

func givenAssigneeRelation() objectstore.TestObject {
	return objectstore.TestObject{
		bundle.RelationKeyId:             domain.String("rel-assignee"),
		bundle.RelationKeyRelationKey:    domain.String(assigneeKey),
		bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_object)),
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
		bundle.RelationKeyUniqueKey:      domain.String("rel-" + assigneeKey),
	}
}

func givenPerson(id, name string) objectstore.TestObject {
	return objectstore.TestObject{
		bundle.RelationKeyId:             domain.String(id),
		bundle.RelationKeyName:           domain.String(name),
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_profile)),
	}
}

func givenTask(id, name, assignee string) objectstore.TestObject {
	obj := objectstore.TestObject{
		bundle.RelationKeyId:             domain.String(id),
		bundle.RelationKeyName:           domain.String(name),
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_todo)),
	}
	if assignee != "" {
		obj[assigneeKey] = domain.StringList([]string{assignee})
	}
	return obj
}

func givenDepRequest() SubscribeRequest {
	return SubscribeRequest{
		SpaceId:  testSpaceId,
		SubId:    "dep-parent",
		Internal: true, // queue delivery makes event sequences assertable
		Keys:     []string{bundle.RelationKeyId.String(), bundle.RelationKeyName.String(), assigneeKey},
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(int64(model.ObjectType_todo)),
			},
		},
	}
}

func TestDependencies(t *testing.T) {
	t.Run("response carries dependencies of visible members", func(t *testing.T) {
		fx := newEngineFixture(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			givenAssigneeRelation(),
			givenPerson("alice", "Alice"),
			givenTask("task1", "fix bug", "alice"),
		})

		resp, err := fx.Search(givenDepRequest())
		require.NoError(t, err)

		require.Len(t, resp.Records, 1)
		require.Len(t, resp.Dependencies, 1)
		assert.Equal(t, "alice", resp.Dependencies[0].GetString(bundle.RelationKeyId))
	})

	t.Run("dep detail change arrives under the dep subId, detail events only", func(t *testing.T) {
		fx := newEngineFixture(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			givenAssigneeRelation(),
			givenPerson("alice", "Alice"),
			givenTask("task1", "fix bug", "alice"),
		})

		resp, err := fx.Search(givenDepRequest())
		require.NoError(t, err)

		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			givenPerson("alice", "Alice Renamed"),
		})

		msgs := waitMessages(t, resp.Output, 1)
		require.Len(t, msgs, 1)
		amend := msgs[0].GetObjectDetailsAmend()
		require.NotNil(t, amend)
		assert.Equal(t, "alice", amend.Id)
		assert.Equal(t, []string{"dep-parent/dep"}, amend.SubIds)
		require.Len(t, amend.Details, 1)
		assert.Equal(t, bundle.RelationKeyName.String(), amend.Details[0].Key)
	})

	t.Run("changing a member's dep value swaps the tracked dependency", func(t *testing.T) {
		fx := newEngineFixture(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			givenAssigneeRelation(),
			givenPerson("alice", "Alice"),
			givenPerson("bob", "Bob"),
			givenTask("task1", "fix bug", "alice"),
		})

		resp, err := fx.Search(givenDepRequest())
		require.NoError(t, err)

		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			givenTask("task1", "fix bug", "bob"),
		})

		// the parent's Amend plus the new dependency's DetailsSet under /dep,
		// in one batch; the dropped dep emits nothing (clients ignore dep
		// membership)
		msgs := waitMessages(t, resp.Output, 2)
		require.Len(t, msgs, 2)
		parentAmend := msgs[0].GetObjectDetailsAmend()
		require.NotNil(t, parentAmend)
		assert.Equal(t, "task1", parentAmend.Id)
		assert.Equal(t, []string{"dep-parent"}, parentAmend.SubIds)
		depSet := msgs[1].GetObjectDetailsSet()
		require.NotNil(t, depSet)
		assert.Equal(t, "bob", depSet.Id)
		assert.Equal(t, []string{"dep-parent/dep"}, depSet.SubIds)

		// alice is no longer tracked: her changes stay silent
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			givenPerson("alice", "Alice Again"),
		})
		assertNoMessages(t, resp.Output)
	})

	t.Run("filter-value deps are tracked", func(t *testing.T) {
		fx := newEngineFixture(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			givenAssigneeRelation(),
			givenPerson("alice", "Alice"),
		})

		req := givenDepRequest()
		req.Filters = append(req.Filters, database.FilterRequest{
			RelationKey: assigneeKey,
			Condition:   model.BlockContentDataviewFilter_In,
			Value:       domain.StringList([]string{"alice"}),
		})
		resp, err := fx.Search(req)
		require.NoError(t, err)

		require.Len(t, resp.Dependencies, 1)
		assert.Equal(t, "alice", resp.Dependencies[0].GetString(bundle.RelationKeyId))

		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			givenPerson("alice", "Alice Renamed"),
		})
		msgs := waitMessages(t, resp.Output, 1)
		amend := msgs[0].GetObjectDetailsAmend()
		require.NotNil(t, amend)
		assert.Equal(t, []string{"dep-parent/dep"}, amend.SubIds)
	})

	t.Run("collection scope change delivers new deps in the same batch", func(t *testing.T) {
		fx := newEngineFixture(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			givenAssigneeRelation(),
			givenPerson("alice", "Alice"),
			givenPerson("bob", "Bob"),
			givenTask("task1", "task one", "alice"),
			givenTask("task2", "task two", "bob"),
		})
		ch := make(chan []string, 4)
		fx.collectionService.EXPECT().SubscribeForCollection("coll1", "dep-parent").Return([]string{"task1"}, ch, nil)
		fx.collectionService.EXPECT().UnsubscribeFromCollection("coll1", "dep-parent").Return(nil)

		req := givenDepRequest()
		req.CollectionId = "coll1"
		resp, err := fx.Search(req)
		require.NoError(t, err)
		require.Len(t, resp.Dependencies, 1)
		require.Equal(t, "alice", resp.Dependencies[0].GetString(bundle.RelationKeyId))

		// task2 joins the collection: bob's /dep DetailsSet must arrive with
		// the membership events, not wait for unrelated feed activity
		ch <- []string{"task1", "task2"}

		var sawDepSet bool
		for !sawDepSet {
			msgs := waitMessages(t, resp.Output, 1)
			for _, msg := range msgs {
				if set := msg.GetObjectDetailsSet(); set != nil && set.Id == "bob" {
					assert.Equal(t, []string{"dep-parent/dep"}, set.SubIds)
					sawDepSet = true
				}
			}
		}
		require.NoError(t, fx.Unsubscribe("dep-parent"))
	})

	t.Run("renaming the dep you sort by reorders the parent", func(t *testing.T) {
		fx := newEngineFixture(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			givenAssigneeRelation(),
			givenPerson("alice", "Alice"),
			givenPerson("bob", "Bob"),
			givenTask("task1", "task one", "alice"),
			givenTask("task2", "task two", "bob"),
		})

		req := givenDepRequest()
		req.SubId = "sorted-parent"
		req.Sorts = []database.SortRequest{
			{RelationKey: assigneeKey, Type: model.BlockContentDataviewSort_Asc, Format: model.RelationFormat_object},
		}
		resp, err := fx.Search(req)
		require.NoError(t, err)
		require.Equal(t, []string{"task1", "task2"}, recordIds(resp.Records))

		// Alice → Zara: the visible order must become task2, task1. Replay
		// the emitted membership events the way the client applies them.
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			givenPerson("alice", "Zara"),
		})

		client := newClientList([]string{"task1", "task2"})
		client.waitConverge(t, resp.Output, []string{"task2", "task1"}, "dep-driven reorder")
	})
}
