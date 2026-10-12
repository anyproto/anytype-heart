package v2model

import "encoding/json"

// ChatStatusRequest is both the HTTP body and the published JSON payload.
// Data is kept as raw JSON so arbitrary values and large integers survive.
type ChatStatusRequest struct {
	// Empty text is omitted; the receiving client supplies its localized label.
	Text string `json:"text,omitempty"`
	// ReplyTo is the id of the message the activity answers. Discussion
	// clients show the activity in that message's thread, and at the
	// discussion level when it is empty or names a message they don't have.
	// It is not looked up: a status refreshes every few seconds.
	ReplyTo string          `json:"reply_to,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// MaxChatStatusReplyTo matches the reply_to cap of chat messages.
const MaxChatStatusReplyTo = 256

// ChatStatusResult acknowledges acceptance, not delivery to every subscriber.
type ChatStatusResult struct {
	DryRun bool `json:"dry_run,omitempty"`
}
