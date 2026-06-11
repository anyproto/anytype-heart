package subscription

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

// drainBroadcast collects every message the engine broadcasts until min
// messages arrived
func drainBroadcast(t *testing.T, fx *engineFixture, min int) []*pb.EventMessage {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var msgs []*pb.EventMessage
	for len(msgs) < min {
		events, err := fx.broadcastEvents.NewCond().WithMin(1).Wait(ctx)
		require.NoError(t, err)
		for _, e := range events {
			msgs = append(msgs, e.Messages...)
		}
	}
	return msgs
}

func TestSubscribeIds(t *testing.T) {
	t.Run("records in request order, missing ids tolerated", func(t *testing.T) {
		fx := newEngineFixture(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			givenNamedParticipant("p1", "alice"),
			givenNamedParticipant("p2", "bob"),
		})

		resp, err := fx.SubscribeIdsReq(pb.RpcObjectSubscribeIdsRequest{
			SpaceId: testSpaceId,
			SubId:   "ids-sub",
			Ids:     []string{"p2", "missing", "p1"},
			Keys:    []string{bundle.RelationKeyId.String(), bundle.RelationKeyName.String()},
		})
		require.NoError(t, err)

		require.Len(t, resp.Records, 2)
		assert.Equal(t, "p2", pbtypes.GetString(resp.Records[0], bundle.RelationKeyId.String()))
		assert.Equal(t, "p1", pbtypes.GetString(resp.Records[1], bundle.RelationKeyId.String()))
	})

	t.Run("missing id appears later with request-order afterId", func(t *testing.T) {
		fx := newEngineFixture(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			givenNamedParticipant("p1", "alice"),
			givenNamedParticipant("p3", "cara"),
		})

		_, err := fx.SubscribeIdsReq(pb.RpcObjectSubscribeIdsRequest{
			SpaceId: testSpaceId,
			SubId:   "ids-sub",
			Ids:     []string{"p1", "p2", "p3"},
			Keys:    []string{bundle.RelationKeyId.String(), bundle.RelationKeyName.String()},
		})
		require.NoError(t, err)

		// p2 gets indexed later: the subscription catches it, Add is placed
		// after its preceding requested id that is present (p1)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			givenNamedParticipant("p2", "bob"),
		})

		msgs := drainBroadcast(t, fx, 2)
		require.Len(t, msgs, 2)
		set := msgs[0].GetObjectDetailsSet()
		require.NotNil(t, set)
		assert.Equal(t, "p2", set.Id)
		add := msgs[1].GetSubscriptionAdd()
		require.NotNil(t, add)
		assert.Equal(t, "p2", add.Id)
		assert.Equal(t, "p1", add.AfterId)
	})

	t.Run("explicit ids stay tracked through soft deletion", func(t *testing.T) {
		fx := newEngineFixture(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			givenNamedParticipant("p1", "alice"),
		})

		_, err := fx.SubscribeIdsReq(pb.RpcObjectSubscribeIdsRequest{
			SpaceId: testSpaceId,
			SubId:   "ids-sub",
			Ids:     []string{"p1"},
			Keys:    []string{bundle.RelationKeyId.String(), bundle.RelationKeyIsDeleted.String()},
		})
		require.NoError(t, err)

		obj := givenNamedParticipant("p1", "alice")
		obj[bundle.RelationKeyIsDeleted] = domain.Bool(true)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{obj})

		// no Remove: the deletion arrives as a detail change
		msgs := drainBroadcast(t, fx, 1)
		require.Len(t, msgs, 1)
		amend := msgs[0].GetObjectDetailsAmend()
		require.NotNil(t, amend)
		require.Len(t, amend.Details, 1)
		assert.Equal(t, bundle.RelationKeyIsDeleted.String(), amend.Details[0].Key)
	})
}

func TestCollectionScope(t *testing.T) {
	t.Run("membership is collection ids intersected with filters", func(t *testing.T) {
		fx := newEngineFixture(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			givenNamedParticipant("p1", "alice"),
			givenNamedParticipant("p2", "bob"),
			givenNamedParticipant("p3", "cara"),
		})
		ch := make(chan []string, 4)
		fx.collectionService.EXPECT().SubscribeForCollection("coll1", "coll-sub").Return([]string{"p3", "p1"}, ch, nil)
		fx.collectionService.EXPECT().UnsubscribeFromCollection("coll1", "coll-sub").Return(nil)

		req := givenParticipantRequest()
		req.SubId = "coll-sub"
		req.CollectionId = "coll1"
		resp, err := fx.Search(req)
		require.NoError(t, err)

		assert.ElementsMatch(t, []string{"p1", "p3"}, recordIds(resp.Records))
		assert.Equal(t, int64(2), resp.Counters.Total)

		t.Run("collection update produces enter and leave events", func(t *testing.T) {
			// p3 leaves the collection, p2 joins; the total is unchanged so
			// no Counters event accompanies the membership script
			ch <- []string{"p1", "p2"}

			msgs := waitMessages(t, resp.Output, 3)
			var removed, added []string
			for _, msg := range msgs {
				if r := msg.GetSubscriptionRemove(); r != nil {
					removed = append(removed, r.Id)
				}
				if a := msg.GetSubscriptionAdd(); a != nil {
					added = append(added, a.Id)
				}
			}
			assert.Equal(t, []string{"p3"}, removed)
			assert.Equal(t, []string{"p2"}, added)
		})

		t.Run("unsubscribe releases the collection subscription", func(t *testing.T) {
			require.NoError(t, fx.Unsubscribe("coll-sub"))
		})
	})

	t.Run("non-matching collection members stay excluded", func(t *testing.T) {
		fx := newEngineFixture(t)
		obj := givenNamedParticipant("p2", "bob")
		obj[bundle.RelationKeyResolvedLayout] = domain.Int64(int64(model.ObjectType_basic))
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			givenNamedParticipant("p1", "alice"),
			obj,
		})
		ch := make(chan []string, 4)
		fx.collectionService.EXPECT().SubscribeForCollection("coll1", "coll-sub").Return([]string{"p1", "p2"}, ch, nil)
		fx.collectionService.EXPECT().UnsubscribeFromCollection("coll1", "coll-sub").Return(nil)

		req := givenParticipantRequest()
		req.SubId = "coll-sub"
		req.CollectionId = "coll1"
		resp, err := fx.Search(req)
		require.NoError(t, err)

		// p2 is in the collection but filtered out by layout
		assert.Equal(t, []string{"p1"}, recordIds(resp.Records))

		require.NoError(t, fx.Unsubscribe("coll-sub"))
	})
}
