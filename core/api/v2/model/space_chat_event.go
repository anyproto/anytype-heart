package v2model

// space_chat_event.go is the wire contract of the space-wide chat stream
// (GET /v2/spaces/{space_id}/chats/stream): one discriminated envelope per
// Server-Sent Event, carrying space_id on every event.

import (
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// The space stream's own event types. Message events reuse the per-chat
// stream's names (ChatEventMessageAdded and the rest).
const (
	SpaceChatEventChatAdded        = "chat_added"
	SpaceChatEventSnapshotComplete = "snapshot_complete"
	SpaceChatEventChatUpdated      = "chat_updated"
	SpaceChatEventChatRemoved      = "chat_removed"
)

// The space stream's ?include= values. Discussions is the default there.
const (
	ChatIncludeDiscussions = "discussions"
	ChatIncludeNone        = "none"
)

// StreamChatRow is the list_chats row plus the chat's last_state_id, as
// chat_added and chat_updated carry it.
type StreamChatRow struct {
	Id             string `json:"id"`
	Name           string `json:"name"`
	Kind           string `json:"kind"`
	ParentId       string `json:"parent_id,omitempty"`
	UnreadMessages int    `json:"unread_messages"`
	UnreadMentions int    `json:"unread_mentions"`
	LastStateId    string `json:"last_state_id,omitempty"`
}

// ChatCounters is the state a state_updated event carries.
type ChatCounters struct {
	UnreadMessages int    `json:"unread_messages"`
	UnreadMentions int    `json:"unread_mentions"`
	LastStateId    string `json:"last_state_id,omitempty"`
}

// SpaceChatEvent is one event of the space-wide chat stream. Type is also the
// SSE event name; members an event does not use are absent.
type SpaceChatEvent struct {
	Type    string `json:"type"`
	SpaceId string `json:"space_id"`
	// ChatId is set on every chat-scoped event except chat_added and
	// chat_updated, which carry the whole row in Chat.
	ChatId string `json:"chat_id,omitempty"`
	// Kind and ParentId are set on message_added, message_updated and
	// message_deleted.
	Kind     string         `json:"kind,omitempty"`
	ParentId string         `json:"parent_id,omitempty"`
	Chat     *StreamChatRow `json:"chat,omitempty"`
	State    *ChatCounters  `json:"state,omitempty"`
	// Message is the full message, state_id included, on message_added and
	// message_updated.
	Message *ChatMessage `json:"message,omitempty"`
	// MessageId is set on message_deleted, reactions_updated and pinned_updated.
	MessageId string `json:"message_id,omitempty"`
	// Reactions is set on reactions_updated, as counts; absent means every
	// reaction was removed.
	Reactions map[string]int `json:"reactions,omitempty"`
	// Pinned is set on pinned_updated; a pointer, because false is half of
	// the toggle.
	Pinned *bool `json:"pinned,omitempty"`
}

// ChatKind is the kind value of a chat: chat or discussion.
func ChatKind(discussion bool) string {
	if discussion {
		return ChatKindDiscussion
	}
	return ChatKindChat
}

// ReactionCounts compacts reactions to the counts map every message carries.
func ReactionCounts(reactions *model.ChatMessageReactions) map[string]int {
	counts, _ := reactionsFromProto(reactions, ChatMessageOptions{})
	return counts
}
