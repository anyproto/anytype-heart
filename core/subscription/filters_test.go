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

// nested And/Or filter trees must hold for both the snapshot query and live
// feed evaluation — the client sends them for dataview view filters

func nameEq(name string) database.FilterRequest {
	return database.FilterRequest{
		RelationKey: bundle.RelationKeyName,
		Condition:   model.BlockContentDataviewFilter_Equal,
		Value:       domain.String(name),
	}
}

func descEq(desc string) database.FilterRequest {
	return database.FilterRequest{
		RelationKey: bundle.RelationKeyDescription,
		Condition:   model.BlockContentDataviewFilter_Equal,
		Value:       domain.String(desc),
	}
}

// Source entries (setOf semantics) resolve to type/relation membership
// filters at the request boundary
func TestSourceResolution(t *testing.T) {
	givenObjects := func(fx *engineFixture) {
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:             domain.String("type-task"),
				bundle.RelationKeyUniqueKey:      domain.String("ot-task"),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
			},
			{
				bundle.RelationKeyId:             domain.String("rel-assignee"),
				bundle.RelationKeyRelationKey:    domain.String("assignee"),
				bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_object)),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
				bundle.RelationKeyUniqueKey:      domain.String("rel-assignee"),
			},
			{
				bundle.RelationKeyId:             domain.String("o1"),
				bundle.RelationKeyType:           domain.String("type-task"),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
			},
			{
				bundle.RelationKeyId:             domain.String("o2"),
				"assignee":                       domain.StringList([]string{"alice"}),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
			},
			{
				bundle.RelationKeyId:             domain.String("o3"),
				bundle.RelationKeyName:           domain.String("plain"),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
			},
		})
	}

	t.Run("type unique key and relation id sources OR-combine", func(t *testing.T) {
		fx := newEngineFixture(t)
		givenObjects(fx)

		resp, err := fx.Search(SubscribeRequest{
			SpaceId:           testSpaceId,
			SubId:             "source-sub",
			Internal:          true,
			NoDepSubscription: true,
			Keys:              []string{bundle.RelationKeyId.String()},
			Source:            []string{"ot-task", "rel-assignee"},
		})
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"o1", "o2"}, recordIds(resp.Records))
	})

	// GO-7404: a relation source means "has this property", not "has a value in
	// it" — filtering the empty ones out breaks `Property → is empty` views
	t.Run("relation source keeps objects holding the property with an empty value", func(t *testing.T) {
		fx := newEngineFixture(t)
		givenObjects(fx)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:             domain.String("o4"),
				"assignee":                       domain.StringList([]string{}),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
			},
			{
				bundle.RelationKeyId:             domain.String("o5"),
				"assignee":                       domain.String(""),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_basic)),
			},
		})

		resp, err := fx.Search(SubscribeRequest{
			SpaceId:           testSpaceId,
			SubId:             "source-empty",
			Internal:          true,
			NoDepSubscription: true,
			Keys:              []string{bundle.RelationKeyId.String()},
			Source:            []string{"rel-assignee"},
		})
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"o2", "o4", "o5"}, recordIds(resp.Records))
	})

	t.Run("unresolvable source errors instead of widening to the space", func(t *testing.T) {
		fx := newEngineFixture(t)
		givenObjects(fx)

		_, err := fx.Search(SubscribeRequest{
			SpaceId:           testSpaceId,
			SubId:             "source-bad",
			Internal:          true,
			NoDepSubscription: true,
			Keys:              []string{bundle.RelationKeyId.String()},
			Source:            []string{"does-not-exist"},
		})
		require.Error(t, err)
	})
}

func TestNestedFilterTrees(t *testing.T) {
	givenObjects := func(fx *engineFixture) {
		mk := func(id, name, desc string) objectstore.TestObject {
			obj := givenNamedParticipant(id, name)
			obj[bundle.RelationKeyDescription] = domain.String(desc)
			return obj
		}
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			mk("a", "Alice", "open"),
			mk("b", "Bob", "done"),
			mk("c", "Carol", "open"),
		})
	}
	givenRequest := func(filters []database.FilterRequest) SubscribeRequest {
		req := givenParticipantRequest()
		req.Keys = append(req.Keys, bundle.RelationKeyDescription.String())
		req.Filters = append(req.Filters, filters...)
		return req
	}

	t.Run("contradictory leaves under the implicit And match nothing", func(t *testing.T) {
		fx := newEngineFixture(t)
		givenObjects(fx)

		resp, err := fx.Search(givenRequest([]database.FilterRequest{nameEq("Alice"), nameEq("Bob")}))
		require.NoError(t, err)
		assert.Empty(t, resp.Records)
		assert.Zero(t, resp.Counters.Total)
	})

	t.Run("Or root with nested leaves", func(t *testing.T) {
		fx := newEngineFixture(t)
		givenObjects(fx)

		resp, err := fx.Search(givenRequest([]database.FilterRequest{{
			Operator:      model.BlockContentDataviewFilter_Or,
			NestedFilters: []database.FilterRequest{nameEq("Alice"), nameEq("Bob")},
		}}))
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"a", "b"}, recordIds(resp.Records))
	})

	t.Run("Or of leaf and And-subtree, live transitions included", func(t *testing.T) {
		fx := newEngineFixture(t)
		givenObjects(fx)

		// Alice, or anyone named Carol whose description is open
		resp, err := fx.Search(givenRequest([]database.FilterRequest{{
			Operator: model.BlockContentDataviewFilter_Or,
			NestedFilters: []database.FilterRequest{
				nameEq("Alice"),
				{
					Operator:      model.BlockContentDataviewFilter_And,
					NestedFilters: []database.FilterRequest{nameEq("Carol"), descEq("open")},
				},
			},
		}}))
		require.NoError(t, err)
		require.ElementsMatch(t, []string{"a", "c"}, recordIds(resp.Records))

		t.Run("an object satisfying an Or leaf enters live", func(t *testing.T) {
			obj := givenNamedParticipant("b", "Alice")
			obj[bundle.RelationKeyDescription] = domain.String("done")
			fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{obj})

			msgs := waitMessages(t, resp.Output, 3)
			require.NotNil(t, msgs[1].GetSubscriptionAdd())
			assert.Equal(t, "b", msgs[1].GetSubscriptionAdd().Id)
		})

		t.Run("breaking the And subtree leaves live", func(t *testing.T) {
			obj := givenNamedParticipant("c", "Carol")
			obj[bundle.RelationKeyDescription] = domain.String("done")
			fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{obj})

			msgs := waitMessages(t, resp.Output, 2)
			require.NotNil(t, msgs[0].GetSubscriptionRemove())
			assert.Equal(t, "c", msgs[0].GetSubscriptionRemove().Id)
		})
	})
}
