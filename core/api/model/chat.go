package apimodel

// MessageStyle represents the style of a chat message
type MessageStyle string

const (
	MessageStyleParagraph   MessageStyle = "paragraph"
	MessageStyleHeader1     MessageStyle = "header1"
	MessageStyleHeader2     MessageStyle = "header2"
	MessageStyleHeader3     MessageStyle = "header3"
	MessageStyleHeader4     MessageStyle = "header4"
	MessageStyleQuote       MessageStyle = "quote"
	MessageStyleCode        MessageStyle = "code"
	MessageStyleTitle       MessageStyle = "title"
	MessageStyleCheckbox    MessageStyle = "checkbox"
	MessageStyleMarked      MessageStyle = "marked"
	MessageStyleNumbered    MessageStyle = "numbered"
	MessageStyleToggle      MessageStyle = "toggle"
	MessageStyleDescription MessageStyle = "description"
	MessageStyleCallout     MessageStyle = "callout"
)

// AttachmentType represents the type of a chat message attachment
type AttachmentType string

const (
	AttachmentTypeFile  AttachmentType = "file"
	AttachmentTypeImage AttachmentType = "image"
	AttachmentTypeLink  AttachmentType = "link"
)

// MessageContent represents the content of a chat message
type MessageContent struct {
	Text  string       `json:"text" example:"Hello, world!"` // The text content of the message
	Style MessageStyle `json:"style" example:"paragraph"`    // The style of the message text
	Marks []TextMark   `json:"marks,omitempty"`              // Text formatting marks
}

// TextMark represents a formatting mark in the message text
type TextMark struct {
	Type  string `json:"type" example:"bold"` // The type of mark (bold, italic, link, etc.)
	From  int32  `json:"from" example:"0"`    // Start position of the mark
	To    int32  `json:"to" example:"5"`      // End position of the mark
	Param string `json:"param,omitempty"`     // Optional parameter (e.g., URL for links)
}

// MessageAttachment represents an attachment in a chat message
type MessageAttachment struct {
	Target string         `json:"target" example:"bafyreie6n5l5nkbjal37su54cha4coy7qzuhrnajluzv5qd5jvtsrxkequ"` // The target object ID of the attachment
	Type   AttachmentType `json:"type" example:"file"`                                                          // The type of attachment
}

// ChatMessage represents a chat message in the API response
type ChatMessage struct {
	Object           string              `json:"object" example:"chat_message"`                      // The data model of the object
	Id               string              `json:"id" example:"msg_abc123"`                            // The unique identifier of the message
	OrderId          string              `json:"order_id" example:"2024-01-15T10:30:00Z"`            // The order ID for pagination cursor
	Creator          string              `json:"creator" example:"user_xyz"`                         // The creator identity of the message
	CreatedAt        int64               `json:"created_at" example:"1705312200"`                    // Unix timestamp when the message was created
	ModifiedAt       int64               `json:"modified_at" example:"1705312200"`                   // Unix timestamp when the message was last modified
	ReplyToMessageId string              `json:"reply_to_message_id,omitempty" example:"msg_abc122"` // The ID of the message being replied to
	Content          MessageContent      `json:"content"`                                            // The content of the message
	Attachments      []MessageAttachment `json:"attachments"`                                        // The attachments of the message
	Reactions        map[string][]string `json:"reactions"`                                          // Map of emoji to list of user IDs who reacted
	Read             bool                `json:"read" example:"true"`                                // Whether the message has been read
}

// ChatState represents the state of a chat
type ChatState struct {
	Messages    *UnreadState `json:"messages,omitempty"`      // Unread state for messages
	Mentions    *UnreadState `json:"mentions,omitempty"`      // Unread state for mentions
	LastStateId string       `json:"last_state_id,omitempty"` // The last state ID
}

// UnreadState represents the unread state for a counter type
type UnreadState struct {
	Counter       int32  `json:"counter" example:"5"`       // Number of unread items
	OldestOrderId string `json:"oldest_order_id,omitempty"` // Order ID of the oldest unread item
}

// MessageResponse wraps a single chat message response
type MessageResponse struct {
	Message ChatMessage `json:"message"` // The chat message
}

// MessagesResponse wraps a list of chat messages with chat state
type MessagesResponse struct {
	Messages []ChatMessage `json:"messages"`        // The list of chat messages
	State    *ChatState    `json:"state,omitempty"` // The chat state
}

// CreateMessageRequest represents a request to create a new chat message
type CreateMessageRequest struct {
	Text             string              `json:"text" binding:"required" example:"Hello, world!"`    // The text content of the message
	Style            MessageStyle        `json:"style,omitempty" example:"paragraph"`                // The style of the message text
	ReplyToMessageId string              `json:"reply_to_message_id,omitempty" example:"msg_abc122"` // The ID of the message to reply to
	Attachments      []MessageAttachment `json:"attachments,omitempty"`                              // Attachments to include in the message
}

// UpdateMessageRequest represents a request to update an existing chat message
type UpdateMessageRequest struct {
	Text  string       `json:"text" binding:"required" example:"Updated message text"` // The updated text content
	Style MessageStyle `json:"style,omitempty" example:"paragraph"`                    // The updated style
}

// ToggleReactionRequest represents a request to toggle a reaction on a message
type ToggleReactionRequest struct {
	Emoji string `json:"emoji" binding:"required" example:"👍"` // The emoji to toggle
}

// ToggleReactionResponse represents the response from toggling a reaction
type ToggleReactionResponse struct {
	Added bool `json:"added" example:"true"` // Whether the reaction was added (true) or removed (false)
}

// SearchMessagesRequest represents a request to search chat messages
type SearchMessagesRequest struct {
	Query string `json:"query" binding:"required" example:"hello"` // The search query text
}

// SearchMessageResult represents a single search result
type SearchMessageResult struct {
	ChatId          string      `json:"chat_id" example:"chat_abc123"`         // The chat ID containing the message
	MessageId       string      `json:"message_id" example:"msg_abc123"`       // The message ID
	Score           int64       `json:"score" example:"100"`                   // The relevance score
	Highlight       string      `json:"highlight" example:"...hello world..."` // Highlighted snippet
	HighlightRanges []Range     `json:"highlight_ranges,omitempty"`            // Ranges of highlighted text
	Message         ChatMessage `json:"message"`                               // The full message
}

// Range represents a text range
type Range struct {
	From int32 `json:"from" example:"0"` // Start position
	To   int32 `json:"to" example:"5"`   // End position
}

// SearchMessagesResponse wraps the search results
type SearchMessagesResponse struct {
	Results []SearchMessageResult `json:"results"` // The search results
}
