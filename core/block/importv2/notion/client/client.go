// Package client is the Notion API transport layer: auth/versioning, JSON
// codec, a shared rate pacer (~3 rps with 429/529 pushback propagated across
// workers), bounded ctx-aware retries, and typed errors. The Transport and
// FileFetcher seams make the whole crawl replayable without a network
// (go-vcr cassettes and hand-written mocks).
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"golang.org/x/time/rate"
)

const (
	BaseURL = "https://api.notion.com/v1"
	// apiVersion pins the data-sources API generation: older versions get
	// ZERO search results for any database a user split into multiple data
	// sources — silent, total data loss for that database and its pages.
	apiVersion = "2025-09-03"

	// requestsPerSecond follows Notion's documented average; bursts are
	// allowed by the API, pushback is handled via Retry-After.
	requestsPerSecond = 3
)

// Transport performs exactly one Notion API round trip. Production wraps
// http.Client; tests substitute recorders or hostile fakes.
type Transport interface {
	Do(req *http.Request) (*http.Response, error)
}

// FileFetcher downloads one (pre-signed, expiring) file URL into dst.
type FileFetcher interface {
	Fetch(ctx context.Context, url string, dst io.Writer) error
}

type Client struct {
	transport Transport
	baseURL   string
	token     string
	retry     RetryPolicy
	pacer     *pacer
	status    StatusHook
}

type Option func(*Client)

func WithTransport(transport Transport) Option {
	return func(c *Client) { c.transport = transport }
}

func WithBaseURL(baseURL string) Option {
	return func(c *Client) { c.baseURL = baseURL }
}

func WithRetryPolicy(policy RetryPolicy) Option {
	return func(c *Client) { c.retry = policy }
}

// WithRateLimit overrides the request pacing (tests use a high limit).
func WithRateLimit(requestsPerSecond float64) Option {
	return func(c *Client) { c.pacer = newPacer(rate.Limit(requestsPerSecond)) }
}

func NewClient(token string, opts ...Option) *Client {
	c := &Client{
		transport: &http.Client{Timeout: time.Minute},
		baseURL:   BaseURL,
		token:     token,
		retry:     DefaultRetryPolicy(),
		pacer:     newPacer(rate.Limit(requestsPerSecond)),
		status:    noopStatusHook{},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Request performs one logical API call: paced, retried within bounds,
// decoded into out (which may be nil).
func (c *Client) Request(ctx context.Context, method, path string, body any, out any) error {
	var payload []byte
	if body != nil {
		var err error
		payload, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request body: %w", err)
		}
	}

	deadline := time.Now().Add(c.retry.TotalBudget)
	var lastErr error
	signalled := false // a Throttled/Retrying edge was reported for this call
	for attempt := 0; attempt < c.retry.MaxAttempts; attempt++ {
		if attempt > 0 {
			delay := c.retry.backoff(attempt)
			if retryAfter := retryAfterOf(lastErr); retryAfter > 0 {
				delay = retryAfter
			}
			if time.Now().Add(delay).After(deadline) {
				break // budget exhausted, report the last error
			}
			c.status.Retrying(attempt, c.retry.MaxAttempts)
			signalled = true
			if err := sleepCtx(ctx, delay); err != nil {
				return err
			}
		}
		// A pending pushback pause is the calm THROTTLED state, not an
		// error: report it with its resume time before waiting it out.
		if pause := c.pacer.pauseRemaining(); pause > 0 {
			c.status.Throttled(pause)
			signalled = true
		}
		if err := c.pacer.Wait(ctx); err != nil {
			return err // cancelled mid-wait: no recovery — the run is stopping
		}

		lastErr = c.doOnce(ctx, method, path, payload, out)
		if lastErr == nil {
			if signalled {
				c.status.Recovered()
			}
			return nil
		}
		if !isRetryable(lastErr) {
			return lastErr
		}
		if retryAfter := retryAfterOf(lastErr); retryAfter > 0 {
			// One worker's 429 pushes every worker back.
			c.pacer.pushback(retryAfter)
		}
	}
	return fmt.Errorf("retries exhausted: %w", lastErr)
}

func (c *Client) doOnce(ctx context.Context, method, path string, payload []byte, out any) error {
	var bodyReader io.Reader
	if payload != nil {
		bodyReader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Notion-Version", apiVersion)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	res, err := c.transport.Do(req)
	if err != nil {
		return &transportError{err: err}
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return errorFromResponse(res)
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(res.Body).Decode(out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
