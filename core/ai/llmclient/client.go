package llmclient

import (
	"context"
	"encoding/json"
	"errors"
)

// Sentinel errors, mapped from provider responses.
var (
	ErrAuthRequired        = errors.New("llm: authentication failed")
	ErrModelNotFound       = errors.New("llm: model not found")
	ErrRateLimited         = errors.New("llm: rate limited")
	ErrEndpointUnreachable = errors.New("llm: endpoint unreachable")
	ErrEmptyResponse       = errors.New("llm: empty response")
	ErrResponseTruncated   = errors.New("llm: response truncated at the token limit")
)

// Request is one structured-output completion.
type Request struct {
	System         string
	User           string
	SchemaName     string
	Schema         json.RawMessage
	MaxTokens      int
	ReasoningEffort string
}

// Usage reports provider-side token accounting for the call.
type Usage struct {
	PromptTokens     int
	CompletionTokens int
}

// Client is the offline no-op implementation.
type Client struct{}

type options struct {
	httpClient *struct{}
	retry      RetryPolicy
}

// Option configures a Client.
type Option func(*options)

// WithHTTPClient is a no-op in offline builds.
func WithHTTPClient(hc interface{}) Option { return func(o *options) {} }

// WithRetryPolicy is a no-op in offline builds.
func WithRetryPolicy(p RetryPolicy) Option { return func(o *options) { o.retry = p } }

// New returns an offline client that always fails.
func New(cfg Config, opts ...Option) (*Client, error) {
	return nil, ErrEndpointUnreachable
}

// CompleteJSON returns an endpoint-unreachable error in offline builds.
func (c *Client) CompleteJSON(ctx context.Context, req Request) (json.RawMessage, Usage, error) {
	return nil, Usage{}, ErrEndpointUnreachable
}
