package llmclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	openai "github.com/sashabaranov/go-openai"
)

// Sentinel errors, mapped from provider responses. They mirror the Rpc.AI
// error codes so adapters can translate without re-parsing.
var (
	ErrAuthRequired        = errors.New("llm: authentication failed")
	ErrModelNotFound       = errors.New("llm: model not found")
	ErrRateLimited         = errors.New("llm: rate limited")
	ErrEndpointUnreachable = errors.New("llm: endpoint unreachable")
	ErrEmptyResponse       = errors.New("llm: empty response")
	// ErrResponseTruncated means the provider stopped at the token cap, so the
	// content is a fragment. Distinct from a parse failure on purpose: a
	// corrective retry would truncate identically, and the user needs to be
	// told their plan was too large rather than that their model answered
	// badly.
	ErrResponseTruncated = errors.New("llm: response truncated at the token limit")
)

// Request is one structured-output completion.
type Request struct {
	System string
	User   string
	// SchemaName labels the schema for the provider ("import_plan").
	SchemaName string
	// Schema is a strict JSON schema; the model's output is constrained to it
	// (OpenAI strict mode; grammar-compiled on ollama/LM Studio/llama.cpp).
	Schema json.RawMessage
	// MaxTokens caps the completion; 0 = provider default.
	MaxTokens int
}

// Usage reports provider-side token accounting for the call.
type Usage struct {
	PromptTokens     int
	CompletionTokens int
}

type Client struct {
	api   *openai.Client
	model string
	retry RetryPolicy
	sleep func(ctx context.Context, d time.Duration) error
}

// Option configures a Client.
type Option func(*options)

type options struct {
	httpClient *http.Client
	retry      RetryPolicy
}

// WithHTTPClient injects the transport (tests use an httptest server's client).
func WithHTTPClient(hc *http.Client) Option { return func(o *options) { o.httpClient = hc } }

// WithRetryPolicy overrides DefaultRetryPolicy.
func WithRetryPolicy(p RetryPolicy) Option { return func(o *options) { o.retry = p } }

// maxResponseBytes caps how much of a completion response the client will
// read — a wedged or hostile endpoint must not stream unbounded bytes.
const maxResponseBytes = 10 << 20

func New(cfg Config, opts ...Option) (*Client, error) {
	if cfg.Endpoint == "" || cfg.Model == "" {
		return nil, fmt.Errorf("llm client requires endpoint and model")
	}
	o := options{retry: DefaultRetryPolicy()}
	for _, opt := range opts {
		opt(&o)
	}
	if o.retry.MaxAttempts < 1 {
		o.retry.MaxAttempts = 1
	}
	apiCfg := openai.DefaultConfig(cfg.Token)
	apiCfg.BaseURL = cfg.Endpoint
	httpClient := o.httpClient
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	bounded := *httpClient
	bounded.Transport = &boundedTransport{base: httpClient.Transport, limit: maxResponseBytes}
	apiCfg.HTTPClient = &bounded
	return &Client{
		api:   openai.NewClientWithConfig(apiCfg),
		model: cfg.Model,
		retry: o.retry,
		sleep: sleepCtx,
	}, nil
}

// boundedTransport truncates every response body at limit bytes; an
// over-limit JSON body simply fails to decode.
type boundedTransport struct {
	base  http.RoundTripper
	limit int64
}

func (t *boundedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	resp, err := base.RoundTrip(req)
	if resp != nil && resp.Body != nil {
		resp.Body = &limitedBody{Reader: io.LimitReader(resp.Body, t.limit), closer: resp.Body}
	}
	return resp, err
}

type limitedBody struct {
	io.Reader
	closer io.Closer
}

func (b *limitedBody) Close() error { return b.closer.Close() }

// CompleteJSON runs one non-streaming completion constrained by req.Schema and
// returns the raw JSON content. Temperature is forced to zero. Transient
// failures (429, 5xx, transport) are retried per the policy; auth and
// model-not-found fail immediately.
func (c *Client) CompleteJSON(ctx context.Context, req Request) (json.RawMessage, Usage, error) {
	apiReq := openai.ChatCompletionRequest{
		Model: c.model,
		// go-openai omits a zero temperature from the payload, letting the
		// provider default (usually 1) win; the smallest nonzero float is the
		// documented way to actually send ~0.
		Temperature: math.SmallestNonzeroFloat32,
		MaxTokens:   req.MaxTokens,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: req.System},
			{Role: openai.ChatMessageRoleUser, Content: req.User},
		},
		ResponseFormat: &openai.ChatCompletionResponseFormat{
			Type: openai.ChatCompletionResponseFormatTypeJSONSchema,
			JSONSchema: &openai.ChatCompletionResponseFormatJSONSchema{
				Name:   req.SchemaName,
				Schema: req.Schema,
				Strict: true,
			},
		},
	}

	var lastErr error
	temperatureDropped := false
	for attempt := 0; attempt < c.retry.MaxAttempts; attempt++ {
		if attempt > 0 {
			if err := c.sleep(ctx, c.backoff(attempt)); err != nil {
				return nil, Usage{}, fmt.Errorf("retry backoff: %w", err)
			}
		}
		resp, err := c.api.CreateChatCompletion(ctx, apiReq)
		if err != nil {
			// Reasoning models (o-series, gpt-5 class) reject any explicit
			// temperature with a 400: drop the parameter once and retry the
			// same attempt — determinism is those models' default anyway.
			if !temperatureDropped && isTemperatureRejection(err) {
				temperatureDropped = true
				apiReq.Temperature = 0 // omitempty: the field disappears
				attempt--
				continue
			}
			mapped, retryable := classify(err)
			lastErr = mapped
			if !retryable || ctx.Err() != nil {
				return nil, Usage{}, mapped
			}
			continue
		}
		usage := Usage{PromptTokens: resp.Usage.PromptTokens, CompletionTokens: resp.Usage.CompletionTokens}
		if len(resp.Choices) == 0 {
			return nil, usage, ErrEmptyResponse
		}
		msg := resp.Choices[0].Message
		if msg.Refusal != "" {
			return nil, usage, fmt.Errorf("%w: model refused: %s", ErrEmptyResponse, msg.Refusal)
		}
		if resp.Choices[0].FinishReason == openai.FinishReasonLength {
			return nil, usage, fmt.Errorf("%w: %d completion tokens", ErrResponseTruncated, usage.CompletionTokens)
		}
		if msg.Content == "" {
			return nil, usage, ErrEmptyResponse
		}
		return json.RawMessage(msg.Content), usage, nil
	}
	return nil, Usage{}, fmt.Errorf("llm call failed after %d attempts: %w", c.retry.MaxAttempts, lastErr)
}

func (c *Client) backoff(attempt int) time.Duration {
	shift := attempt - 1
	if shift > 16 { // beyond any sane policy; prevents duration overflow
		shift = 16
	}
	d := c.retry.BaseDelay << shift
	if d <= 0 {
		d = c.retry.BaseDelay
	}
	if c.retry.MaxDelay > 0 && d > c.retry.MaxDelay {
		d = c.retry.MaxDelay
	}
	return d
}

// isTemperatureRejection spots the reasoning-model 400 for an explicit
// temperature parameter.
func isTemperatureRejection(err error) bool {
	var apiErr *openai.APIError
	if !errors.As(err, &apiErr) || apiErr.HTTPStatusCode != http.StatusBadRequest {
		return false
	}
	return strings.Contains(strings.ToLower(apiErr.Message), "temperature")
}

// classify maps a go-openai error onto the package sentinels and decides
// whether retrying can help.
func classify(err error) (mapped error, retryable bool) {
	var apiErr *openai.APIError
	if errors.As(err, &apiErr) {
		return classifyStatus(apiErr.HTTPStatusCode, err)
	}
	var reqErr *openai.RequestError
	if errors.As(err, &reqErr) {
		if reqErr.HTTPStatusCode > 0 {
			return classifyStatus(reqErr.HTTPStatusCode, err)
		}
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err, false
	}
	// Anything without an HTTP status is a transport-level failure.
	return fmt.Errorf("%w: %v", ErrEndpointUnreachable, err), true
}

func classifyStatus(status int, err error) (error, bool) {
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return fmt.Errorf("%w: %v", ErrAuthRequired, err), false
	case status == http.StatusNotFound:
		return fmt.Errorf("%w: %v", ErrModelNotFound, err), false
	case status == http.StatusTooManyRequests:
		return fmt.Errorf("%w: %v", ErrRateLimited, err), true
	case status >= 500:
		return fmt.Errorf("%w: %v", ErrEndpointUnreachable, err), true
	default:
		return err, false
	}
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
