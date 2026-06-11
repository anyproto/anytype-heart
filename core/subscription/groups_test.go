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
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const statusKey = "taskStatus"

func givenStatusRelation() objectstore.TestObject {
	return objectstore.TestObject{
		bundle.RelationKeyId:             domain.String("rel-taskStatus"),
		bundle.RelationKeyRelationKey:    domain.String(statusKey),
		bundle.RelationKeyRelationFormat: domain.Int64(int64(model.RelationFormat_status)),
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relation)),
		bundle.RelationKeyUniqueKey:      domain.String("rel-" + statusKey),
	}
}

func givenStatusOption(id, text string) objectstore.TestObject {
	return objectstore.TestObject{
		bundle.RelationKeyId:             domain.String(id),
		bundle.RelationKeyName:           domain.String(text),
		bundle.RelationKeyRelationKey:    domain.String(statusKey),
		bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_relationOption)),
	}
}

func waitGroupEvents(t *testing.T, fx *engineFixture, min int) []*pb.EventObjectSubscriptionGroups {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var res []*pb.EventObjectSubscriptionGroups
	for len(res) < min {
		events, err := fx.broadcastEvents.NewCond().WithMin(1).Wait(ctx)
		require.NoError(t, err)
		for _, e := range events {
			for _, msg := range e.Messages {
				if g := msg.GetSubscriptionGroups(); g != nil {
					res = append(res, g)
				}
			}
		}
	}
	return res
}

func TestSubscribeGroups(t *testing.T) {
	t.Run("status groups come from relation options", func(t *testing.T) {
		fx := newEngineFixture(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			givenStatusRelation(),
			givenStatusOption("opt-todo", "Todo"),
			givenStatusOption("opt-done", "Done"),
		})

		resp, err := fx.SubscribeGroups(SubscribeGroupsRequest{
			SpaceId:     testSpaceId,
			SubId:       "groups-sub",
			RelationKey: statusKey,
		})
		require.NoError(t, err)
		assert.Equal(t, "groups-sub", resp.SubId)

		ids := make([]string, 0, len(resp.Groups))
		for _, g := range resp.Groups {
			ids = append(ids, g.Id)
		}
		// the kanban grouper always adds the no-value column
		assert.ElementsMatch(t, []string{"opt-todo", "opt-done", "empty"}, ids)
	})

	t.Run("a new option emits a group add event", func(t *testing.T) {
		fx := newEngineFixture(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			givenStatusRelation(),
			givenStatusOption("opt-todo", "Todo"),
		})

		_, err := fx.SubscribeGroups(SubscribeGroupsRequest{
			SpaceId:     testSpaceId,
			SubId:       "groups-sub",
			RelationKey: statusKey,
		})
		require.NoError(t, err)

		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			givenStatusOption("opt-doing", "Doing"),
		})

		events := waitGroupEvents(t, fx, 1)
		require.Len(t, events, 1)
		assert.Equal(t, "groups-sub", events[0].SubId)
		assert.False(t, events[0].Remove)
		require.NotNil(t, events[0].Group)
		assert.Equal(t, "opt-doing", events[0].Group.Id)
	})

	t.Run("a deleted option emits a group remove event", func(t *testing.T) {
		fx := newEngineFixture(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			givenStatusRelation(),
			givenStatusOption("opt-todo", "Todo"),
			givenStatusOption("opt-done", "Done"),
		})

		_, err := fx.SubscribeGroups(SubscribeGroupsRequest{
			SpaceId:     testSpaceId,
			SubId:       "groups-sub",
			RelationKey: statusKey,
		})
		require.NoError(t, err)

		gone := givenStatusOption("opt-done", "Done")
		gone[bundle.RelationKeyIsDeleted] = domain.Bool(true)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{gone})

		events := waitGroupEvents(t, fx, 1)
		require.Len(t, events, 1)
		assert.True(t, events[0].Remove)
		require.NotNil(t, events[0].Group)
		assert.Equal(t, "opt-done", events[0].Group.Id)
	})

	t.Run("a hard-deleted option emits a group remove event", func(t *testing.T) {
		fx := newEngineFixture(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			givenStatusRelation(),
			givenStatusOption("opt-todo", "Todo"),
			givenStatusOption("opt-done", "Done"),
		})

		// real kanban requests filter by layout, so option objects are never
		// match-set members; a hard delete tombstones the option down to
		// {id, isDeleted} — only the tracked option set can identify it
		_, err := fx.SubscribeGroups(SubscribeGroupsRequest{
			SpaceId:     testSpaceId,
			SubId:       "groups-sub",
			RelationKey: statusKey,
			Filters: []database.FilterRequest{
				{
					RelationKey: bundle.RelationKeyResolvedLayout,
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       domain.Int64(int64(model.ObjectType_todo)),
				},
			},
		})
		require.NoError(t, err)

		require.NoError(t, fx.objectStore.SpaceIndex(testSpaceId).DeleteObject("opt-done"))

		events := waitGroupEvents(t, fx, 1)
		require.Len(t, events, 1)
		assert.True(t, events[0].Remove)
		require.NotNil(t, events[0].Group)
		assert.Equal(t, "opt-done", events[0].Group.Id)
	})

	t.Run("unsubscribe stops group events", func(t *testing.T) {
		fx := newEngineFixture(t)
		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			givenStatusRelation(),
			givenStatusOption("opt-todo", "Todo"),
		})

		_, err := fx.SubscribeGroups(SubscribeGroupsRequest{
			SpaceId:     testSpaceId,
			SubId:       "groups-sub",
			RelationKey: statusKey,
		})
		require.NoError(t, err)
		assert.Equal(t, []string{"groups-sub"}, fx.SubscriptionIDs())

		require.NoError(t, fx.Unsubscribe("groups-sub"))
		assert.Empty(t, fx.SubscriptionIDs())

		fx.objectStore.AddObjects(t, testSpaceId, []objectstore.TestObject{
			givenStatusOption("opt-doing", "Doing"),
		})

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		_, err = fx.broadcastEvents.NewCond().WithMin(1).Wait(ctx)
		require.ErrorIs(t, err, context.DeadlineExceeded)
	})
}
