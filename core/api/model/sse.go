package apimodel

// SSEEventType represents the type of SSE event
type SSEEventType string

const (
	SSEEventTypeInitial   SSEEventType = "initial"
	SSEEventTypeAdd       SSEEventType = "add"
	SSEEventTypeUpdate    SSEEventType = "update"
	SSEEventTypeDelete    SSEEventType = "delete"
	SSEEventTypeReactions SSEEventType = "reactions"
	SSEEventTypeState     SSEEventType = "state"
)

// SSEChatEvent represents a chat event sent over SSE
type SSEChatEvent struct {
	Type     SSEEventType  `json:"type"`
	Message  *ChatMessage  `json:"message,omitempty"`
	Messages []ChatMessage `json:"messages,omitempty"`
	State    *ChatState    `json:"state,omitempty"`
	Deleted  string        `json:"deleted,omitempty"`
}
