package v2model

import "encoding/json"

// ChatStatusRequest is both the HTTP body and the published JSON payload.
// Data is kept as raw JSON so arbitrary values and large integers survive.
type ChatStatusRequest struct {
	// Empty text is omitted; the receiving client supplies its localized label.
	Text string          `json:"text,omitempty"`
	Data json.RawMessage `json:"data,omitempty"`
}

// ChatStatusResult acknowledges acceptance, not delivery to every subscriber.
type ChatStatusResult struct {
	DryRun bool `json:"dry_run,omitempty"`
}
