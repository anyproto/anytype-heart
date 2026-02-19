package objectsubscription

import (
	"context"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/cheggaaa/mb/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/subscription/mock_subscription"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/tests/testutil"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type fixture struct {
	subService subscription.Service
}

func newFixture(t *testing.T) *fixture {
	ctx := context.Background()
	objectStore := objectstore.NewStoreFixture(t)
	eventService := mock_event.NewMockSender(t)
	subService := subscription.New()
	a := new(app.App)
	a.Register(objectStore)
	a.Register(testutil.PrepareMock(ctx, a, eventService))
	a.Register(subService)

	err := a.Start(ctx)
	require.NoError(t, err)

	return &fixture{
		subService: subService,
	}
}

func makeSubscriptionAdd(id string) *pb.EventMessage {
	return event.NewMessage("", &pb.EventMessageValueOfSubscriptionAdd{
		SubscriptionAdd: &pb.EventObjectSubscriptionAdd{
			Id: id,
		},
	})
}

func makeSubscriptionRemove(id string) *pb.EventMessage {
	return event.NewMessage("", &pb.EventMessageValueOfSubscriptionRemove{
		SubscriptionRemove: &pb.EventObjectSubscriptionRemove{
			Id: id,
		},
	})
}

func makeDetailsSet(id string) *pb.EventMessage {
	return event.NewMessage("", &pb.EventMessageValueOfObjectDetailsSet{
		ObjectDetailsSet: &pb.EventObjectDetailsSet{
			Id: id,
			Details: domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
				bundle.RelationKeyId: domain.String(id),
				"key1":               domain.String("value1"),
			}).ToProto(),
		},
	})
}

func makeDetailsUnset(id string) *pb.EventMessage {
	return event.NewMessage("", &pb.EventMessageValueOfObjectDetailsUnset{
		ObjectDetailsUnset: &pb.EventObjectDetailsUnset{
			Id:   id,
			Keys: []string{"key1", "key2"},
		},
	})
}

func makeDetailsAmend(id string) *pb.EventMessage {
	return event.NewMessage("", &pb.EventMessageValueOfObjectDetailsAmend{
		ObjectDetailsAmend: &pb.EventObjectDetailsAmend{
			Id: id,
			Details: []*pb.EventObjectDetailsAmendKeyValue{
				{
					Key:   "key3",
					Value: pbtypes.String("value3"),
				},
			},
		},
	})
}

func makeStructs(ids []string) []*domain.Details {
	structs := make([]*domain.Details, len(ids))
	for i, id := range ids {
		structs[i] = domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
			bundle.RelationKeyId: domain.String(id),
		},
		)
	}
	return structs
}

func TestIdSubscription(t *testing.T) {
	subService := mock_subscription.NewMockService(t)
	events := mb.New[*pb.EventMessage](0)
	records := makeStructs([]string{"1", "2", "3"})
	// for details amend, set and unset we just check that we handle them correctly (i.e. do nothing)
	messages := []*pb.EventMessage{
		makeSubscriptionRemove("2"),
		makeDetailsSet("1"),
		makeDetailsUnset("2"),
		makeDetailsAmend("3"),
		makeSubscriptionAdd("4"),
		makeDetailsSet("4"),
		makeSubscriptionRemove("1"),
		makeSubscriptionAdd("3"),
		makeDetailsSet("3"),
		makeSubscriptionRemove("5"),
	}
	for _, msg := range messages {
		err := events.Add(context.Background(), msg)
		require.NoError(t, err)
	}
	subscribeResponse := &subscription.SubscribeResponse{
		Output:  events,
		Records: records,
	}
	subService.EXPECT().Search(mock.Anything).Return(subscribeResponse, nil)
	sub := NewIdSubscription(subService, subscription.SubscribeRequest{})
	err := sub.Run()
	require.NoError(t, err)
	time.Sleep(100 * time.Millisecond)
	ids := make(map[string]struct{})
	sub.Iterate(func(id string, _ struct{}) bool {
		ids[id] = struct{}{}
		return true
	})
	require.Len(t, ids, 2)
	require.Contains(t, ids, "3")
	require.Contains(t, ids, "4")

	t.Run("Get", func(t *testing.T) {
		got, ok := sub.Get("3")
		assert.True(t, ok)
		assert.Equal(t, struct{}{}, got)

		_, ok = sub.Get("100")
		assert.False(t, ok)
	})

	t.Run("Has", func(t *testing.T) {
		got := sub.Has("4")
		assert.True(t, got)

		got = sub.Has("100")
		assert.False(t, got)
	})
}

func TestSubscriptionFromQueue(t *testing.T) {
	events := mb.New[*pb.EventMessage](0)
	// for details amend, set and unset we just check that we handle them correctly (i.e. do nothing)
	messages := []*pb.EventMessage{
		makeSubscriptionAdd("4"),
		makeDetailsSet("4"),
		makeSubscriptionRemove("4"),
		makeSubscriptionAdd("3"),
		makeDetailsSet("3"),
	}
	for _, msg := range messages {
		err := events.Add(context.Background(), msg)
		require.NoError(t, err)
	}
	sub := NewIdSubscriptionFromQueue(events, nil)
	err := sub.Run()
	require.NoError(t, err)
	time.Sleep(100 * time.Millisecond)
	ids := make(map[string]struct{})
	sub.Iterate(func(id string, _ struct{}) bool {
		ids[id] = struct{}{}
		return true
	})
	require.Len(t, ids, 1)
	require.Contains(t, ids, "3")
}
