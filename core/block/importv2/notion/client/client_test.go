package client

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fastPolicy() RetryPolicy {
	return RetryPolicy{
		MaxAttempts: 3,
		BaseDelay:   time.Millisecond,
		MaxDelay:    10 * time.Millisecond,
		TotalBudget: 2 * time.Second,
	}
}

func newTestClient(t *testing.T, handler http.HandlerFunc, opts ...Option) (*Client, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	opts = append([]Option{
		WithBaseURL(server.URL),
		WithRetryPolicy(fastPolicy()),
		WithRateLimit(1000),
	}, opts...)
	return NewClient("test-token", opts...), server
}

func TestRequestRetries(t *testing.T) {
	t.Run("429 storm is bounded by max attempts", func(t *testing.T) {
		// given
		var calls atomic.Int64
		c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			w.Header().Set("Retry-After", "0.001")
			w.WriteHeader(http.StatusTooManyRequests)
			fmt.Fprint(w, `{"code":"rate_limited","message":"slow down"}`)
		})

		// when
		err := c.Request(context.Background(), http.MethodGet, "/users", nil, nil)

		// then
		assert.ErrorIs(t, err, ErrRateLimited)
		assert.Equal(t, int64(3), calls.Load(), "v1 retried unboundedly; v2 must stop")
	})

	t.Run("retry-after is honored for its attempt without compounding", func(t *testing.T) {
		// given — first response asks for a 150ms pause, second succeeds
		var calls atomic.Int64
		c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			if calls.Add(1) == 1 {
				w.Header().Set("Retry-After", "0.15")
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			fmt.Fprint(w, `{}`)
		})

		// when
		start := time.Now()
		err := c.Request(context.Background(), http.MethodGet, "/users", nil, nil)

		// then
		require.NoError(t, err)
		assert.GreaterOrEqual(t, time.Since(start), 150*time.Millisecond)
		assert.Equal(t, int64(2), calls.Load())
	})

	t.Run("5xx and 529 are retried, auth is not", func(t *testing.T) {
		for _, tc := range []struct {
			name      string
			status    int
			wantCalls int64
			wantErr   error
		}{
			{"500 retried to exhaustion", http.StatusInternalServerError, 3, ErrUnavailable},
			{"529 treated as rate limiting", 529, 3, ErrRateLimited},
			{"401 fails immediately", http.StatusUnauthorized, 1, ErrUnauthorized},
			{"403 fails immediately", http.StatusForbidden, 1, ErrForbidden},
			{"404 fails immediately", http.StatusNotFound, 1, ErrNotFound},
		} {
			t.Run(tc.name, func(t *testing.T) {
				// given
				var calls atomic.Int64
				c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					w.WriteHeader(tc.status)
				})

				// when
				err := c.Request(context.Background(), http.MethodGet, "/x", nil, nil)

				// then
				assert.ErrorIs(t, err, tc.wantErr)
				assert.Equal(t, tc.wantCalls, calls.Load())
			})
		}
	})

	t.Run("cancellation interrupts the backoff", func(t *testing.T) {
		// given — long backoff, cancel mid-wait
		c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}, WithRetryPolicy(RetryPolicy{
			MaxAttempts: 5, BaseDelay: 10 * time.Second, MaxDelay: 10 * time.Second, TotalBudget: time.Minute,
		}))
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			time.Sleep(50 * time.Millisecond)
			cancel()
		}()

		// when
		start := time.Now()
		err := c.Request(ctx, http.MethodGet, "/x", nil, nil)

		// then
		assert.ErrorIs(t, err, context.Canceled)
		assert.Less(t, time.Since(start), 2*time.Second)
	})

	t.Run("headers and decoding", func(t *testing.T) {
		// given
		var gotAuth, gotVersion string
		c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("Authorization")
			gotVersion = r.Header.Get("Notion-Version")
			fmt.Fprint(w, `{"object":"list","results":[{"id":"abc"}]}`)
		})
		var out struct {
			Results []struct {
				Id string `json:"id"`
			} `json:"results"`
		}

		// when
		err := c.Request(context.Background(), http.MethodPost, "/search", map[string]any{"page_size": 100}, &out)

		// then
		require.NoError(t, err)
		assert.Equal(t, "Bearer test-token", gotAuth)
		assert.Equal(t, apiVersion, gotVersion)
		require.Len(t, out.Results, 1)
		assert.Equal(t, "abc", out.Results[0].Id)
	})
}

func TestPacerPushback(t *testing.T) {
	t.Run("one worker's 429 pauses the others", func(t *testing.T) {
		// given
		p := newPacer(1000)
		p.pushback(120 * time.Millisecond)

		// when — an unrelated waiter arrives after the pushback
		start := time.Now()
		require.NoError(t, p.Wait(context.Background()))

		// then
		assert.GreaterOrEqual(t, time.Since(start), 100*time.Millisecond)
	})
}

type resettableBuffer struct {
	bytes.Buffer
}

func (b *resettableBuffer) Reset() error {
	b.Buffer.Reset()
	return nil
}

func TestFileFetcher(t *testing.T) {
	t.Run("downloads and retries transient failures", func(t *testing.T) {
		// given — first attempt 500, second succeeds
		var calls atomic.Int64
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if calls.Add(1) == 1 {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			fmt.Fprint(w, "file-bytes")
		}))
		defer server.Close()
		fetcher := NewFileFetcher(WithFetcherRetryPolicy(fastPolicy()))
		var dst resettableBuffer

		// when
		err := fetcher.Fetch(context.Background(), server.URL, &dst)

		// then
		require.NoError(t, err)
		assert.Equal(t, "file-bytes", dst.String())
		assert.Equal(t, int64(2), calls.Load())
	})

	t.Run("stalled download is interrupted, not hung", func(t *testing.T) {
		// given — server sends a few bytes then goes silent
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.(http.Flusher).Flush()
			fmt.Fprint(w, "partial")
			w.(http.Flusher).Flush()
			time.Sleep(5 * time.Second)
		}))
		defer server.Close()
		fetcher := NewFileFetcher(
			WithFetcherRetryPolicy(RetryPolicy{MaxAttempts: 1, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond, TotalBudget: time.Second}),
			WithFetcherStallTimeout(100*time.Millisecond),
		)
		var dst resettableBuffer

		// when
		start := time.Now()
		err := fetcher.Fetch(context.Background(), server.URL, &dst)

		// then — v1's stall monitor could not interrupt a hung read
		require.Error(t, err)
		assert.Less(t, time.Since(start), 2*time.Second)
	})

	t.Run("partial write without reset support is not blindly retried", func(t *testing.T) {
		// given — connection breaks mid-body every time
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", "100")
			fmt.Fprint(w, "short")
			w.(http.Flusher).Flush()
			conn, _, _ := w.(http.Hijacker).Hijack()
			conn.Close()
		}))
		defer server.Close()
		fetcher := NewFileFetcher(WithFetcherRetryPolicy(fastPolicy()))
		var plain bytes.Buffer

		// when
		err := fetcher.Fetch(context.Background(), server.URL, &plain)

		// then
		require.Error(t, err)
	})
}
