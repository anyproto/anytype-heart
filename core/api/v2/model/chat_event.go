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
	// Id is the chat state id this event was observed at. It rides the SSE
	// `id:` line, and a reconnecting client sends the last one back as
	// Last-Event-ID to resume. Deletions carry none: the row is gone, so
	// there is no state to name (see the route's resume note).
	Id   string `json:"id,omitempty"`
	Type string `json:"type"`
	// Message is set on message_added and message_updated.
	Message *ChatMessage `json:"message,omitempty"`
	// MessageId names the subject of an event that carries no body:
	// message_deleted and reactions_updated.
	MessageId string `json:"message_id,omitempty"`
	// Reactions and ReactedBy mirror ChatMessage: counts by default,
	// participant ids only under the same opt-in.
	Reactions map[string]int      `json:"reactions,omitempty"`
	ReactedBy map[string][]string `json:"reacted_by,omitempty"`
}

// ChatEventFromProto converts one middleware event into the v2 DTO, or nil
// when the event is not a chat change this stream serves.
func ChatEventFromProto(msg *pb.EventMessage, opts ChatMessageOptions) *ChatEvent {
	if ev := msg.GetChatAdd(); ev != nil && ev.Message != nil {
		message := ChatMessageFromProto(ev.Message, opts)
		return &ChatEvent{Id: ev.Message.StateId, Type: ChatEventMessageAdded, Message: &message}
	}
	if ev := msg.GetChatUpdate(); ev != nil && ev.Message != nil {
		message := ChatMessageFromProto(ev.Message, opts)
		return &ChatEvent{Id: ev.Message.StateId, Type: ChatEventMessageUpdated, Message: &message}
	}
	if ev := msg.GetChatDelete(); ev != nil {
		return &ChatEvent{Type: ChatEventMessageDeleted, MessageId: ev.Id}
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
