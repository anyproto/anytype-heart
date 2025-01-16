package subscription

import (
	"context"
	"errors"
	"testing"
	"time"

	mb2 "github.com/cheggaaa/mb/v3"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func wrapToEventMessages(spaceId string, vals []pb.IsEventMessageValue) []*pb.EventMessage {
	msgs := make([]*pb.EventMessage, len(vals))
	for i, v := range vals {
		msgs[i] = &pb.EventMessage{SpaceId: spaceId, Value: v}
	}
	return msgs
}

func TestInternalSubscriptionSingle(t *testing.T) {
	fx := NewInternalTestService(t)
	resp, err := fx.Search(SubscribeRequest{
		SpaceId: testSpaceId,
		SubId:   "test",
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyPriority,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(10),
			},
		},
		Keys:     []string{bundle.RelationKeyId.String(), bundle.RelationKeyName.String(), bundle.RelationKeyPriority.String()},
		Internal: true,
	})

	require.NoError(t, err)
	require.Empty(t, resp.Records)

	t.Run("amend details not related to filter", func(t *testing.T) {
		fx.AddObjects(t, testSpaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:             domain.String("id1"),
				bundle.RelationKeyName:           domain.String("task1"),
				bundle.RelationKeyPriority:       domain.Int64(10),
				bundle.RelationKeyLinkedProjects: domain.StringList([]string{"project1", "project2"}), // Should be ignored as not listed in keys
			},
		})
		time.Sleep(batchTime)
		fx.AddObjects(t, testSpaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:       domain.String("id1"),
				bundle.RelationKeyName:     domain.String("task1 renamed"),
				bundle.RelationKeyPriority: domain.Int64(10),
			},
		})
		time.Sleep(batchTime)

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		want := givenMessagesForFirstObject("test")

		msgs, err := resp.Output.NewCond().WithMin(len(want)).Wait(ctx)
		require.NoError(t, err)

		require.Equal(t, wrapToEventMessages(testSpaceId, want), msgs)
	})

	t.Run("amend details related to filter -- remove from subscription", func(t *testing.T) {
		fx.AddObjects(t, testSpaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:       domain.String("id2"),
				bundle.RelationKeyName:     domain.String("task2"),
				bundle.RelationKeyPriority: domain.Int64(10),
			},
		})
		time.Sleep(batchTime)

		fx.AddObjects(t, testSpaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:       domain.String("id2"),
				bundle.RelationKeyName:     domain.String("task2"),
				bundle.RelationKeyPriority: domain.Int64(9),
			},
		})
		time.Sleep(batchTime)

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		want := givenMessagesForSecondObject("test")
		msgs, err := resp.Output.NewCond().WithMin(len(want)).Wait(ctx)
		require.NoError(t, err)

		require.Equal(t, wrapToEventMessages(testSpaceId, want), msgs)
	})

	t.Run("unsubscribe", func(t *testing.T) {
		err = fx.Unsubscribe("test")
		require.NoError(t, err)

		err = resp.Output.Add(context.Background(), event.NewMessage("", nil))
		require.True(t, errors.Is(err, mb2.ErrClosed))
	})

	t.Run("try to add after close", func(t *testing.T) {
		time.Sleep(batchTime)
		fx.AddObjects(t, testSpaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:       domain.String("id3"),
				bundle.RelationKeyName:     domain.String("task2"),
				bundle.RelationKeyPriority: domain.Int64(10),
			},
		})
	})
}

func TestInternalSubscriptionMultiple(t *testing.T) {
	fx := newFixtureWithRealObjectStore(t)
	resp1, err := fx.Search(SubscribeRequest{
		SpaceId: testSpaceId,
		SubId:   "internal1",
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyPriority,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(10),
			},
		},
		Keys:     []string{bundle.RelationKeyId.String(), bundle.RelationKeyName.String(), bundle.RelationKeyPriority.String()},
		Internal: true,
	})
	_, err = fx.Search(SubscribeRequest{
		SpaceId: testSpaceId,
		SubId:   "client1",
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyPriority,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(10),
			},
		},
		Keys: []string{bundle.RelationKeyId.String(), bundle.RelationKeyName.String(), bundle.RelationKeyPriority.String()},
	})
	_, err = fx.Search(SubscribeRequest{
		SpaceId: testSpaceId,
		SubId:   "client2",
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyPriority,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(10),
			},
		},
		Keys: []string{bundle.RelationKeyId.String(), bundle.RelationKeyName.String(), bundle.RelationKeyPriority.String()},
	})
	resp4, err := fx.Search(SubscribeRequest{
		SpaceId: testSpaceId,
		SubId:   "internal2",
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyName,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.String("Jane Doe"),
			},
		},
		Keys:     []string{bundle.RelationKeyId.String(), bundle.RelationKeyName.String(), bundle.RelationKeyPriority.String()},
		Internal: true,
	})

	require.NoError(t, err)
	require.Empty(t, resp1.Records)

	t.Run("amend details not related to filter", func(t *testing.T) {
		fx.store.AddObjects(t, testSpaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:             domain.String("id1"),
				bundle.RelationKeyName:           domain.String("task1"),
				bundle.RelationKeyPriority:       domain.Int64(10),
				bundle.RelationKeyLinkedProjects: domain.StringList([]string{"project1", "project2"}), // Should be ignored as not listed in keys
			},
		})
		time.Sleep(batchTime)
		fx.store.AddObjects(t, testSpaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:       domain.String("id1"),
				bundle.RelationKeyName:     domain.String("task1 renamed"),
				bundle.RelationKeyPriority: domain.Int64(10),
			},
		})
		time.Sleep(batchTime)

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		want := givenMessagesForFirstObject("internal1")

		msgs, err := resp1.Output.NewCond().WithMin(len(want)).Wait(ctx)
		require.NoError(t, err)

		require.Equal(t, wrapToEventMessages(testSpaceId, want), msgs)

		want = givenMessagesForFirstObject("client1", "client2")
		fx.waitEvents(t, want...)
	})

	t.Run("amend details related to filter -- remove from subscription", func(t *testing.T) {
		fx.store.AddObjects(t, testSpaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:       domain.String("id2"),
				bundle.RelationKeyName:     domain.String("task2"),
				bundle.RelationKeyPriority: domain.Int64(10),
			},
		})
		time.Sleep(batchTime)

		fx.store.AddObjects(t, testSpaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:       domain.String("id2"),
				bundle.RelationKeyName:     domain.String("task2"),
				bundle.RelationKeyPriority: domain.Int64(9),
			},
		})
		time.Sleep(batchTime)

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		want := givenMessagesForSecondObject("internal1")
		msgs, err := resp1.Output.NewCond().WithMin(len(want)).Wait(ctx)
		require.NoError(t, err)

		require.Equal(t, wrapToEventMessages(testSpaceId, want), msgs)

		want = givenMessagesForSecondObject("client1", "client2")
		fx.waitEvents(t, want...)
	})

	t.Run("add item satisfying filters from all subscription", func(t *testing.T) {
		fx.store.AddObjects(t, testSpaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:       domain.String("id3"),
				bundle.RelationKeyName:     domain.String("Jane Doe"),
				bundle.RelationKeyPriority: domain.Int64(10),
			},
		})
		time.Sleep(batchTime)

		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		want := givenMessagesForThirdObject(2, "id1", "internal1")
		msgs, err := resp1.Output.NewCond().WithMin(len(want)).Wait(ctx)
		require.NoError(t, err)
		require.Equal(t, wrapToEventMessages(testSpaceId, want), msgs)

		want = givenMessagesForThirdObject(1, "", "internal2")
		msgs, err = resp4.Output.NewCond().WithMin(len(want)).Wait(ctx)
		require.NoError(t, err)
		require.Equal(t, wrapToEventMessages(testSpaceId, want), msgs)

		want = givenMessagesForThirdObject(2, "id1", "client1", "client2")
		fx.waitEvents(t, want...)
	})
}

func TestInternalSubCustomQueue(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	subId := "test"
	fx := newFixtureWithRealObjectStore(t)

	queue := mb2.New[*pb.EventMessage](0)

	resp, err := fx.Search(SubscribeRequest{
		SpaceId: testSpaceId,
		SubId:   subId,
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyPriority,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(10),
			},
		},
		Keys:          []string{bundle.RelationKeyId.String(), bundle.RelationKeyName.String(), bundle.RelationKeyPriority.String()},
		Internal:      true,
		InternalQueue: queue,
	})
	require.NoError(t, err)
	require.Same(t, resp.Output, queue)

	obj := objectstore.TestObject{
		bundle.RelationKeyId:       domain.String("id1"),
		bundle.RelationKeyName:     domain.String("Jane Doe"),
		bundle.RelationKeyPriority: domain.Int64(10),
	}
	fx.store.AddObjects(t, testSpaceId, []objectstore.TestObject{obj})

	want := []pb.IsEventMessageValue{
		&pb.EventMessageValueOfObjectDetailsSet{
			ObjectDetailsSet: &pb.EventObjectDetailsSet{
				Id:      "id1",
				SubIds:  []string{subId},
				Details: obj.Details().ToProto(),
			},
		},
		&pb.EventMessageValueOfSubscriptionAdd{
			SubscriptionAdd: &pb.EventObjectSubscriptionAdd{
				SubId: subId,
				Id:    "id1",
			},
		},
		&pb.EventMessageValueOfSubscriptionCounters{
			SubscriptionCounters: &pb.EventObjectSubscriptionCounters{
				SubId: subId,
				Total: 1,
			},
		},
	}

	msgs, err := queue.NewCond().WithMin(len(want)).Wait(ctx)
	require.NoError(t, err)
	require.Equal(t, wrapToEventMessages(testSpaceId, want), msgs)
}

func TestInternalSubAsyncInit(t *testing.T) {
	ctx := context.Background()
	subId := "test"
	fx := newFixtureWithRealObjectStore(t)
	obj := objectstore.TestObject{
		bundle.RelationKeyId:       domain.String("id1"),
		bundle.RelationKeyName:     domain.String("Jane Doe"),
		bundle.RelationKeyPriority: domain.Int64(10),
	}

	fx.store.AddObjects(t, testSpaceId, []objectstore.TestObject{
		obj,
	})

	resp, err := fx.Search(SubscribeRequest{
		SpaceId: testSpaceId,
		SubId:   subId,
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyPriority,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(10),
			},
		},
		Keys:      []string{bundle.RelationKeyId.String(), bundle.RelationKeyName.String(), bundle.RelationKeyPriority.String()},
		Internal:  true,
		AsyncInit: true,
	})

	require.NoError(t, err)
	require.Empty(t, resp.Records)

	want := []pb.IsEventMessageValue{
		&pb.EventMessageValueOfObjectDetailsSet{
			ObjectDetailsSet: &pb.EventObjectDetailsSet{
				Id:      "id1",
				SubIds:  []string{subId},
				Details: obj.Details().ToProto(),
			},
		},
		&pb.EventMessageValueOfSubscriptionAdd{
			SubscriptionAdd: &pb.EventObjectSubscriptionAdd{
				SubId: subId,
				Id:    "id1",
			},
		},
		&pb.EventMessageValueOfSubscriptionCounters{
			SubscriptionCounters: &pb.EventObjectSubscriptionCounters{
				SubId: subId,
				Total: 1,
			},
		},
	}

	msgs, err := resp.Output.NewCond().WithMin(len(want)).Wait(ctx)
	require.NoError(t, err)
	require.Equal(t, wrapToEventMessages(testSpaceId, want), msgs)
}

func givenMessagesForFirstObject(subIds ...string) []pb.IsEventMessageValue {
	var msgs []pb.IsEventMessageValue
	msgs = append(msgs, &pb.EventMessageValueOfObjectDetailsSet{
		ObjectDetailsSet: &pb.EventObjectDetailsSet{
			Id:     "id1",
			SubIds: subIds,
			Details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:       domain.String("id1"),
				bundle.RelationKeyName:     domain.String("task1"),
				bundle.RelationKeyPriority: domain.Int64(10),
			}).ToProto(),
		},
	})

	for _, subId := range subIds {
		msgs = append(msgs, &pb.EventMessageValueOfSubscriptionAdd{
			SubscriptionAdd: &pb.EventObjectSubscriptionAdd{
				SubId: subId,
				Id:    "id1",
			},
		})
	}
	for _, subId := range subIds {
		msgs = append(msgs, &pb.EventMessageValueOfSubscriptionCounters{
			SubscriptionCounters: &pb.EventObjectSubscriptionCounters{
				SubId: subId,
				Total: 1,
			},
		})
	}
	msgs = append(msgs, &pb.EventMessageValueOfObjectDetailsAmend{
		ObjectDetailsAmend: &pb.EventObjectDetailsAmend{
			Id:     "id1",
			SubIds: subIds,
			Details: []*pb.EventObjectDetailsAmendKeyValue{
				{
					Key:   bundle.RelationKeyName.String(),
					Value: domain.String("task1 renamed").ToProto(),
				},
			},
		},
	})
	return msgs
}

func givenMessagesForSecondObject(subIds ...string) []pb.IsEventMessageValue {
	var msgs []pb.IsEventMessageValue
	msgs = append(msgs, &pb.EventMessageValueOfObjectDetailsSet{
		ObjectDetailsSet: &pb.EventObjectDetailsSet{
			Id:     "id2",
			SubIds: subIds,
			Details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:       domain.String("id2"),
				bundle.RelationKeyName:     domain.String("task2"),
				bundle.RelationKeyPriority: domain.Int64(10),
			}).ToProto(),
		},
	})

	for _, subId := range subIds {
		msgs = append(msgs, &pb.EventMessageValueOfSubscriptionAdd{
			SubscriptionAdd: &pb.EventObjectSubscriptionAdd{
				SubId:   subId,
				AfterId: "id1",
				Id:      "id2",
			},
		})
	}
	for _, subId := range subIds {
		msgs = append(msgs, &pb.EventMessageValueOfSubscriptionCounters{
			SubscriptionCounters: &pb.EventObjectSubscriptionCounters{
				SubId: subId,
				Total: 2,
			},
		})
	}
	for _, subId := range subIds {
		msgs = append(msgs, &pb.EventMessageValueOfSubscriptionRemove{
			SubscriptionRemove: &pb.EventObjectSubscriptionRemove{
				Id:    "id2",
				SubId: subId,
			},
		})
	}
	for _, subId := range subIds {
		msgs = append(msgs, &pb.EventMessageValueOfSubscriptionCounters{
			SubscriptionCounters: &pb.EventObjectSubscriptionCounters{
				SubId: subId,
				Total: 1,
			},
		})
	}
	return msgs
}

func givenMessagesForThirdObject(total int, afterId string, subIds ...string) []pb.IsEventMessageValue {
	var msgs []pb.IsEventMessageValue
	msgs = append(msgs, &pb.EventMessageValueOfObjectDetailsSet{
		ObjectDetailsSet: &pb.EventObjectDetailsSet{
			Id:     "id3",
			SubIds: subIds,
			Details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId:       domain.String("id3"),
				bundle.RelationKeyName:     domain.String("Jane Doe"),
				bundle.RelationKeyPriority: domain.Int64(10),
			}).ToProto(),
		},
	})

	for _, subId := range subIds {
		msgs = append(msgs, &pb.EventMessageValueOfSubscriptionAdd{
			SubscriptionAdd: &pb.EventObjectSubscriptionAdd{
				SubId:   subId,
				Id:      "id3",
				AfterId: afterId,
			},
		})
	}
	for _, subId := range subIds {
		msgs = append(msgs, &pb.EventMessageValueOfSubscriptionCounters{
			SubscriptionCounters: &pb.EventObjectSubscriptionCounters{
				SubId: subId,
				Total: int64(total),
			},
		})
	}
	return msgs
}
