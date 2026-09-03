package v2model

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// A streamed message must be the SAME shape a paginated read returns —
// participant-id authors, marks rendered into text, reactions compacted to
// counts. v1's stream carried its own message DTO, so a client had two
// shapes to learn for one concept.
func TestChatEventFromProto(t *testing.T) {
	opts := ChatMessageOptions{SpaceId: "space1"}

	t.Run("an added message carries the v2 message shape and its state id", func(t *testing.T) {
		ev := ChatEventFromProto(&pb.EventMessage{
			Value: &pb.EventMessageValueOfChatAdd{ChatAdd: &pb.EventChatAdd{
				Id: "m1",
				Message: &model.ChatMessage{
					Id: "m1", OrderId: "o1", StateId: "s7",
					Message: &model.ChatMessageMessageContent{Text: "hello"},
				},
			}},
		}, opts)

		require.NotNil(t, ev)
		assert.Equal(t, ChatEventMessageAdded, ev.Type)
		assert.Equal(t, "s7", ev.Id, "the state id is the resume cursor a client sends back")
		require.NotNil(t, ev.Message)
		assert.Equal(t, "m1", ev.Message.Id)
		assert.Equal(t, "hello", ev.Message.Text)
	})

	t.Run("an updated message is distinguished from an added one", func(t *testing.T) {
		ev := ChatEventFromProto(&pb.EventMessage{
			Value: &pb.EventMessageValueOfChatUpdate{ChatUpdate: &pb.EventChatUpdate{
				Id: "m1",
				Message: &model.ChatMessage{
					Id: "m1", StateId: "s8",
					Message: &model.ChatMessageMessageContent{Text: "edited"},
				},
			}},
		}, opts)

		require.NotNil(t, ev)
		assert.Equal(t, ChatEventMessageUpdated, ev.Type)
		assert.Equal(t, "edited", ev.Message.Text)
	})

	t.Run("a deletion names the message and carries no body", func(t *testing.T) {
		ev := ChatEventFromProto(&pb.EventMessage{
			Value: &pb.EventMessageValueOfChatDelete{ChatDelete: &pb.EventChatDelete{Id: "m1"}},
		}, opts)

		require.NotNil(t, ev)
		assert.Equal(t, ChatEventMessageDeleted, ev.Type)
		assert.Equal(t, "m1", ev.MessageId)
		assert.Nil(t, ev.Message)
	})

	t.Run("a reaction update compacts to counts like a message does", func(t *testing.T) {
		ev := ChatEventFromProto(&pb.EventMessage{
			Value: &pb.EventMessageValueOfChatUpdateReactions{
				ChatUpdateReactions: &pb.EventChatUpdateReactions{
					Id: "m1",
					Reactions: &model.ChatMessageReactions{
						Reactions: map[string]*model.ChatMessageReactionsIdentityList{
							"👍": {Ids: []string{"idA", "idB"}},
						},
					},
				},
			},
		}, opts)

		require.NotNil(t, ev)
		assert.Equal(t, ChatEventReactionsUpdated, ev.Type)
		assert.Equal(t, "m1", ev.MessageId)
		assert.Equal(t, map[string]int{"👍": 2}, ev.Reactions)
		assert.Empty(t, ev.ReactedBy, "identities are not served unless asked for")
	})

	t.Run("full reactions name the participants", func(t *testing.T) {
		ev := ChatEventFromProto(&pb.EventMessage{
			Value: &pb.EventMessageValueOfChatUpdateReactions{
				ChatUpdateReactions: &pb.EventChatUpdateReactions{
					Id: "m1",
					Reactions: &model.ChatMessageReactions{
						Reactions: map[string]*model.ChatMessageReactionsIdentityList{
							"👍": {Ids: []string{"idA"}},
						},
					},
				},
			},
		}, ChatMessageOptions{SpaceId: "space1", FullReactions: true})

		require.NotNil(t, ev)
		require.Len(t, ev.ReactedBy["👍"], 1)
		assert.NotEqual(t, "idA", ev.ReactedBy["👍"][0], "raw identities never cross the API")
	})

	t.Run("an edit carries no id, because an edit does not restamp state", func(t *testing.T) {
		// StateId is stamped once, in BeforeCreate. Emitting it on an edit
		// would set the client's Last-Event-ID to the message's CREATION
		// point, so editing one old message rewinds every connected client.
		ev := ChatEventFromProto(&pb.EventMessage{
			Value: &pb.EventMessageValueOfChatUpdate{ChatUpdate: &pb.EventChatUpdate{
				Id:      "m1",
				Message: &model.ChatMessage{Id: "m1", StateId: "s1"},
			}},
		}, opts)

		require.NotNil(t, ev)
		assert.Empty(t, ev.Id)
	})

	t.Run("a chat state update is forwarded", func(t *testing.T) {
		// §8.7 requires it: v2 publishes ChatState on the paginated read and
		// POST .../read requires last_state_id, so a stream that never
		// reported state would force a client to poll the read it replaced
		ev := ChatEventFromProto(&pb.EventMessage{
			Value: &pb.EventMessageValueOfChatStateUpdate{
				ChatStateUpdate: &pb.EventChatUpdateState{
					State: &model.ChatState{LastStateId: "s9"},
				},
			},
		}, opts)

		require.NotNil(t, ev)
		assert.Equal(t, ChatEventStateUpdated, ev.Type)
		require.NotNil(t, ev.State)
		assert.Equal(t, "s9", ev.State.LastStateId)
	})

	t.Run("a pin change is forwarded, including unpinning", func(t *testing.T) {
		// `pinned` is on the streamed ChatMessage, so without this event the
		// flag goes stale with no way to learn otherwise. False is the
		// meaningful half of a toggle, so it must survive omitempty.
		ev := ChatEventFromProto(&pb.EventMessage{
			Value: &pb.EventMessageValueOfChatUpdatePinnedStatus{
				ChatUpdatePinnedStatus: &pb.EventChatUpdatePinnedStatus{
					Message: &model.ChatMessage{Id: "m1", StateId: "s1"},
				},
			},
		}, opts)

		require.NotNil(t, ev)
		assert.Equal(t, ChatEventPinnedUpdated, ev.Type)
		assert.Equal(t, "m1", ev.MessageId)
		require.NotNil(t, ev.Pinned)
		assert.False(t, *ev.Pinned)
		data, err := json.Marshal(ev)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"pinned":false`)
	})

	t.Run("an unrelated event is not a chat event", func(t *testing.T) {
		assert.Nil(t, ChatEventFromProto(&pb.EventMessage{}, opts))
	})

	t.Run("the wire shape omits what an event does not carry", func(t *testing.T) {
		// a deletion serializing an empty `message` or `reactions` would
		// make a client branch on emptiness rather than on `type`
		ev := ChatEventFromProto(&pb.EventMessage{
			Value: &pb.EventMessageValueOfChatDelete{ChatDelete: &pb.EventChatDelete{Id: "m1"}},
		}, opts)
		data, err := json.Marshal(ev)

		require.NoError(t, err)
		assert.JSONEq(t, `{"type":"message_deleted","message_id":"m1"}`, string(data))
	})
}
