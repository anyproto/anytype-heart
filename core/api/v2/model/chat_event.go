package v2model

// chat_event.go is the streamed half of the chat contract: what one
// Server-Sent Event carries.

import (
	"github.com/anyproto/anytype-heart/pb"
)

// The event vocabulary. C2: v2 names its own things in snake_case, and these
// are the values of the SSE `event:` line as well as the body's `type`.
const (
	ChatEventMessageAdded     = "message_added"
	ChatEventMessageUpdated   = "message_updated"
	ChatEventMessageDeleted   = "message_deleted"
	ChatEventReactionsUpdated = "reactions_updated"
	// ChatEventStateUpdated carries the unread counters and last_state_id.
	// §8.7 requires the v2 stream to forward it: v2 publishes ChatState on
	// the paginated read and POST .../read REQUIRES last_state_id, so a
	// stream that never reported state changes would force a client to poll
	// the read it replaced.
	ChatEventStateUpdated = "state_updated"
	// ChatEventPinnedUpdated exists because `pinned` is on the streamed
	// ChatMessage: without it the flag goes stale on a live stream with no
	// way to learn otherwise.
	ChatEventPinnedUpdated = "pinned_updated"
	// ChatEventResyncRequired says the stream could not prove it covered
	// the gap since the client's Last-Event-ID. It is not a failure: the
	// events that follow are a fresh window, and anything the client holds
	// outside that window is unverified.
	ChatEventResyncRequired = "resync_required"
)

// ChatEvent is one streamed chat change. It is FLAT and discriminated by
// Type rather than carrying a polymorphic payload: a client switches on one
// field, and the members an event does not use are absent rather than empty,
// so nothing has to branch on emptiness to learn what it received.
//
// Message is the same ChatMessage a paginated read returns — one shape per
// concept, not one per transport.
type ChatEvent struct {
	// Id is the state id the message was CREATED at, and only additions
	// carry one. It rides the SSE `id:` line, and a reconnecting client
	// sends the last one back as Last-Event-ID.
	//
	// Nothing else may set it. A state id is stamped once, in BeforeCreate,
	// so an edit, a pin or a reaction still reports the message's creation
	// point — putting that on the wire would drag a client's cursor
	// backwards to it. A deletion has no surviving row at all.
	Id   string `json:"id,omitempty"`
	Type string `json:"type"`
	// Message is set on message_added and message_updated.
	Message *ChatMessage `json:"message,omitempty"`
	// MessageId names the subject of an event that carries no body:
	// message_deleted and reactions_updated.
	MessageId string `json:"message_id,omitempty"`
	// Reactions is set on reactions_updated, as counts. ReactedBy carries
	// participant ids and is populated only when the caller asked for them;
	// the stream route has no such switch today, so it is always absent
	// there. An empty Reactions means every reaction was removed, which is
	// the one case where absence IS the payload.
	Reactions map[string]int      `json:"reactions,omitempty"`
	ReactedBy map[string][]string `json:"reacted_by,omitempty"`
	// State is set on state_updated: the unread counters and the
	// last_state_id that POST .../read takes back.
	State *ChatState `json:"state,omitempty"`
	// Pinned is set on pinned_updated. A pointer, because false is the
	// meaningful half of a pin toggle and omitempty would erase it.
	Pinned *bool `json:"pinned,omitempty"`
}

// ChatEventFromProto converts one middleware event into the v2 DTO, or nil
// when the event is not a chat change this stream serves.
func ChatEventFromProto(msg *pb.EventMessage, opts ChatMessageOptions) *ChatEvent {
	if ev := msg.GetChatAdd(); ev != nil && ev.Message != nil {
		message := ChatMessageFromProto(ev.Message, opts)
		return &ChatEvent{Id: ev.Message.StateId, Type: ChatEventMessageAdded, Message: &message}
	}
	if ev := msg.GetChatUpdate(); ev != nil && ev.Message != nil {
		// deliberately NO id. StateId is stamped once, in BeforeCreate, and an
		// edit re-marshals the original — so putting it on the wire would set
		// the client's Last-Event-ID to the message's CREATION point. Editing
		// one old message would rewind the cursor of every connected client.
		message := ChatMessageFromProto(ev.Message, opts)
		return &ChatEvent{Type: ChatEventMessageUpdated, Message: &message}
	}
	if ev := msg.GetChatDelete(); ev != nil {
		return &ChatEvent{Type: ChatEventMessageDeleted, MessageId: ev.Id}
	}
	if ev := msg.GetChatStateUpdate(); ev != nil {
		return &ChatEvent{Type: ChatEventStateUpdated, State: ChatStateFromProto(ev.State)}
	}
	if ev := msg.GetChatUpdatePinnedStatus(); ev != nil && ev.Message != nil {
		// no id, for the same reason message_updated carries none: pinning
		// does not restamp the message's state id
		message := ChatMessageFromProto(ev.Message, opts)
		pinned := ev.IsPinned
		return &ChatEvent{
			Type:      ChatEventPinnedUpdated,
			MessageId: ev.Message.Id,
			Message:   &message,
			Pinned:    &pinned,
		}
	}
	if ev := msg.GetChatUpdateReactions(); ev != nil {
		counts, full := reactionsFromProto(ev.Reactions, opts)
		return &ChatEvent{
			Type:      ChatEventReactionsUpdated,
			MessageId: ev.Id,
			Reactions: counts,
			ReactedBy: full,
		}
	}
	return nil
}
