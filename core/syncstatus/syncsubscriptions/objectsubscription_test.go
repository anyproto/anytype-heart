package syncsubscriptions

import (
	"context"
	"testing"
	"time"

	"github.com/cheggaaa/mb/v3"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/subscription/mock_subscription"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func makeSubscriptionAdd(id string) *pb.EventMessage {
	return &pb.EventMessage{
		Value: &pb.EventMessageValueOfSubscriptionAdd{
			SubscriptionAdd: &pb.EventObjectSubscriptionAdd{
				Id: id,
			},
		},
	}
}

func makeSubscriptionRemove(id string) *pb.EventMessage {
	return &pb.EventMessage{
		Value: &pb.EventMessageValueOfSubscriptionRemove{
			SubscriptionRemove: &pb.EventObjectSubscriptionRemove{
				Id: id,
			},
		},
	}
}

func makeDetailsSet(id string) *pb.EventMessage {
	return &pb.EventMessage{
		Value: &pb.EventMessageValueOfObjectDetailsSet{
			ObjectDetailsSet: &pb.EventObjectDetailsSet{
				Id: id,
				Details: &types.Struct{
					Fields: map[string]*types.Value{
						"key1": pbtypes.String("value1"),
					},
				},
			},
		},
	}
}

func makeDetailsUnset(id string) *pb.EventMessage {
	return &pb.EventMessage{
		Value: &pb.EventMessageValueOfObjectDetailsUnset{
			ObjectDetailsUnset: &pb.EventObjectDetailsUnset{
				Id:   id,
				Keys: []string{"key1", "key2"},
			},
		},
	}
}

func makeDetailsAmend(id string) *pb.EventMessage {
	return &pb.EventMessage{
		Value: &pb.EventMessageValueOfObjectDetailsAmend{
			ObjectDetailsAmend: &pb.EventObjectDetailsAmend{
				Id: id,
				Details: []*pb.EventObjectDetailsAmendKeyValue{
					{
						Key:   "key3",
						Value: pbtypes.String("value3"),
					},
				},
			},
		},
	}
}

func makeStructs(ids []string) []*types.Struct {
	structs := make([]*types.Struct, len(ids))
	for i, id := range ids {
		structs[i] = &types.Struct{
			Fields: map[string]*types.Value{
				bundle.RelationKeyId.String(): pbtypes.String(id),
			},
		}
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
		makeSubscriptionRemove("1"),
		makeSubscriptionAdd("3"),
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
}
