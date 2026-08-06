package wrapper

// client.go — the wrapper's HTTP transport to the local /v2 server. This is
// the machinery §7.3 says the wrapper owns and the model never authors:
// bearer auth, Idempotency-Key on every mutation, bounded retries that
// resend the EXACT same body with the SAME key (so the server's C8 store
// replays instead of double-applying), and C6 error decoding into
// agent-readable text. If-Match is never sent by the task tools (advisory
// C7 mode — a sync-advanced etag would 409 on noise a small model cannot
// reason about); the CLI exposes it as an advanced flag for scripts.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
)

// DefaultBaseURL is the local API server's default address
// (config.JsonApiListenAddr's conventional port).
const DefaultBaseURL = "http://127.0.0.1:31009"

// clientMaxAttempts bounds the retry loop: 1 try + 2 retries.
const clientMaxAttempts = 3

// Client speaks HTTP to the local API server.
type Client struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
	// Backoff returns the sleep before retry attempt n (1-based). Tests
	// zero it.
	Backoff func(attempt int) time.Duration
}

// NewClient builds a client for the local server.
func NewClient(baseURL, apiKey string) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{
		BaseURL: strings.TrimSuffix(baseURL, "/"),
		APIKey:  apiKey,
		HTTP:    &http.Client{Timeout: 60 * time.Second},
		Backoff: func(attempt int) time.Duration { return time.Duration(attempt) * 200 * time.Millisecond },
	}
}

// apiRequest is one call to the server.
type apiRequest struct {
	method string
	path   string // already escaped, e.g. /v2/spaces/space1/search
	query  url.Values
	body   any
	// idempotencyKey marks a mutation: it is attached as Idempotency-Key
	// and the request becomes retryable (same body, same key → C8 replay).
	idempotencyKey string
	// ifMatch, when set, adds the If-Match precondition (CLI advanced flag).
	ifMatch string
}

// ToolError is a server-reported C6 error surfaced by a tool: the text is
// the server's own agent-tuned message plus its path-addressed issues.
type ToolError struct {
	Status int
	Code   string
	Text   string
	Issues []apimodel.V2Issue
}

func (e *ToolError) Error() string { return e.Text }

// do executes one request with the retry policy: transport errors and
// 429/502/503/504 retry (mutations resend the same key + body); every other
// status returns immediately.
func (c *Client) do(ctx context.Context, r apiRequest) (int, []byte, error) {
	var lastErr error
	for attempt := 1; attempt <= clientMaxAttempts; attempt++ {
		if attempt > 1 && c.Backoff != nil {
			select {
			case <-time.After(c.Backoff(attempt - 1)):
			case <-ctx.Done():
				return 0, nil, ctx.Err()
			}
		}
		status, body, err := c.once(ctx, r)
		switch {
		case err != nil:
			lastErr = err
		case status == http.StatusTooManyRequests ||
			status == http.StatusBadGateway ||
			status == http.StatusServiceUnavailable ||
			status == http.StatusGatewayTimeout:
			lastErr = fmt.Errorf("server answered %d", status)
		default:
			return status, body, nil
		}
	}
	return 0, nil, fmt.Errorf("call %s %s: %w", r.method, r.path, lastErr)
}

func (c *Client) once(ctx context.Context, r apiRequest) (int, []byte, error) {
	target := c.BaseURL + r.path
	if len(r.query) > 0 {
		target += "?" + r.query.Encode()
	}
	var reader io.Reader
	if r.body != nil {
		payload, err := json.Marshal(r.body)
		if err != nil {
			return 0, nil, fmt.Errorf("encode request body: %w", err)
		}
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, r.method, target, reader)
	if err != nil {
		return 0, nil, fmt.Errorf("build request: %w", err)
	}
	if r.body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	if r.idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", r.idempotencyKey)
	}
	if r.ifMatch != "" {
		req.Header.Set("If-Match", r.ifMatch)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return 0, nil, fmt.Errorf("read response: %w", err)
	}
	return resp.StatusCode, body, nil
}

// decode runs a request and unmarshals a 2xx response into out (nil out
// skips decoding); non-2xx becomes a *ToolError carrying the server's text.
func (c *Client) decode(ctx context.Context, r apiRequest, out any) error {
	status, body, err := c.do(ctx, r)
	if err != nil {
		return err
	}
	if status < 200 || status > 299 {
		return decodeAPIError(status, body)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode response of %s %s: %w", r.method, r.path, err)
	}
	return nil
}

// raw runs a request and returns the raw 2xx body (document reads).
func (c *Client) raw(ctx context.Context, r apiRequest) ([]byte, error) {
	status, body, err := c.do(ctx, r)
	if err != nil {
		return nil, err
	}
	if status < 200 || status > 299 {
		return nil, decodeAPIError(status, body)
	}
	return body, nil
}

// decodeAPIError turns a C6 error body into a ToolError; a non-JSON body
// degrades to the raw text.
func decodeAPIError(status int, body []byte) error {
	var v2 apimodel.V2Error
	if err := json.Unmarshal(body, &v2); err != nil || v2.Message == "" {
		return &ToolError{Status: status, Text: fmt.Sprintf("server answered %d: %s", status, strings.TrimSpace(string(body)))}
	}
	var b strings.Builder
	b.WriteString(v2.Message)
	for _, issue := range v2.Issues {
		b.WriteString("\n  ")
		if issue.Path != "" {
			b.WriteString(issue.Path)
			b.WriteString(": ")
		}
		b.WriteString(issue.Message)
		if issue.Hint != "" {
			b.WriteString(" (")
			b.WriteString(issue.Hint)
			b.WriteString(")")
		}
	}
	return &ToolError{Status: status, Code: v2.Code, Text: b.String(), Issues: v2.Issues}
}

// isAmbiguous reports whether an error is the server's 400 ambiguous_input
// — the trigger of the §7.4 ambiguity retry.
func isAmbiguous(err error) bool {
	var te *ToolError
	return errors.As(err, &te) && te.Code == apimodel.V2CodeAmbiguousInput
}

// spacePath escapes one path segment.
func seg(s string) string { return url.PathEscape(s) }
