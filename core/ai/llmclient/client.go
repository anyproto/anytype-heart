package llmclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	// ReasoningEffort tunes how much a reasoning model thinks before
	// answering ("none", "low", "medium", "high", …; the accepted set is
	// provider- and model-specific). Empty sends nothing.
	//
	// It serves two ends. On hosted reasoning models it trades latency and
	// tokens for depth. On a LOCAL thinking model served through ollama's
	// OpenAI-compatible endpoint, "none" switches thinking off entirely —
	// without it the thought consumes the token budget and the answer comes
	// back empty or truncated. Models that do not know the parameter reject
	// it, so it is dropped and retried rather than failing the call.
	ReasoningEffort string
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

// nearZeroTemperature is the smallest temperature the client sends. go-openai
// omits a literal 0 from the payload (omitempty), letting the provider default
// win, so SOME nonzero value has to go on the wire. It must be a NORMAL float,
// not math.SmallestNonzeroFloat32: samplers scale logits by 1/T in float32,
// and 1/1.4e-45 overflows to +Inf, which on ollama 0.32.4 (measured,
// gemma4:e4b) deterministically produced word salad — thinking disabled, the
// json_schema grammar unapplied, high-entropy tokens — while 1e-8 behaves
// byte-identically to a true 0.
const nearZeroTemperature = 1e-8

func New(cfg Config, opts ...Option) (*Client, error) {
	if cfg.Endpoint == "" || cfg.Model == "" {
		return nil, fmt.Errorf("llm client requires endpoint and model")
	}
	api, o := newAPIClient(cfg, opts...)
	return &Client{
		api:   api,
		model: cfg.Model,
		retry: o.retry,
		sleep: sleepCtx,
	}, nil
}

// newAPIClient builds the underlying go-openai client shared by New (chat
// completions) and ListModels (catalog discovery, which has no model yet):
// same base URL, bearer token, and bounded transport either way, so a
// listing call is bound by the identical safety rails as a completion call.
func newAPIClient(cfg Config, opts ...Option) (*openai.Client, options) {
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
	return openai.NewClientWithConfig(apiCfg), o
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
		Model:           c.model,
		Temperature:     nearZeroTemperature,
		MaxTokens:       req.MaxTokens,
		ReasoningEffort: req.ReasoningEffort,
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
	maxTokensSwitched := false
	effortDropped := false
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
			// The same models reject max_tokens and demand
			// max_completion_tokens. Switching on rejection rather than by
			// model name keeps this provider-agnostic: local servers
			// (ollama, LM Studio, llama.cpp) generally speak only the legacy
			// parameter, so it stays the default and moves only when refused.
			if !maxTokensSwitched && isMaxTokensRejection(err) {
				maxTokensSwitched = true
				apiReq.MaxCompletionTokens, apiReq.MaxTokens = apiReq.MaxTokens, 0
				attempt--
				continue
			}
			// A model that does not know reasoning_effort 400s on it. The same
			// config may be pointed at a reasoning model or a plain one, so
			// the parameter degrades instead of failing the plan step.
			if !effortDropped && isReasoningEffortRejection(err) {
				effortDropped = true
				apiReq.ReasoningEffort = "" // omitempty: the field disappears
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

// isTemperatureRejection spots a refusal of the explicit temperature — either
// go-openai's CLIENT-side guard for reasoning models, which never sends a
// request, or a provider's 400. Checking only for the 400 misses every gpt-5
// and o-series model, since go-openai stops those before the wire.
func isTemperatureRejection(err error) bool {
	if errors.Is(err, openai.ErrReasoningModelLimitationsOther) {
		return true
	}
	var apiErr *openai.APIError
	if !errors.As(err, &apiErr) || apiErr.HTTPStatusCode != http.StatusBadRequest {
		return false
	}
	return strings.Contains(strings.ToLower(apiErr.Message), "temperature")
}

// isMaxTokensRejection spots the 400 newer OpenAI models return for the
// legacy max_tokens parameter ("this model is not supported MaxTokens, please
// use MaxCompletionTokens"). Without this the request is misread as a
// transport failure and retried to exhaustion, so every gpt-5 class model
// fails the plan step outright.
func isMaxTokensRejection(err error) bool {
	// go-openai refuses the parameter for reasoning models CLIENT-side, so
	// this arrives as a plain sentinel with no request ever sent — checking
	// only for an APIError would miss every gpt-5 and o-series model.
	if errors.Is(err, openai.ErrReasoningModelMaxTokensDeprecated) {
		return true
	}
	var apiErr *openai.APIError
	if !errors.As(err, &apiErr) || apiErr.HTTPStatusCode != http.StatusBadRequest {
		return false
	}
	// Providers that do send it back word it as "Unsupported parameter:
	// 'max_tokens' is not supported with this model."
	message := strings.ToLower(apiErr.Message)
	return strings.Contains(message, "max_tokens") || strings.Contains(message, "maxtokens")
}

// isReasoningEffortRejection spots a refusal of the reasoning_effort
// parameter by a model that has no notion of it.
func isReasoningEffortRejection(err error) bool {
	var apiErr *openai.APIError
	if !errors.As(err, &apiErr) || apiErr.HTTPStatusCode != http.StatusBadRequest {
		return false
	}
	return strings.Contains(strings.ToLower(apiErr.Message), "reasoning_effort")
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
