package main

import (
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

// a completion must survive an endpoint that goes away and comes back: the
// laptop serving a local model sleeps mid-run, and burning the attempt on
// the first dial timeout cost every remaining cell of the matrix.
func TestCompleteRetriesUntilTheEndpointAnswers(t *testing.T) {
	const okBody = `{"choices":[{"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}]}`

	newFixture := func(t *testing.T, handler http.HandlerFunc) (*chatClient, *int32) {
		t.Helper()
		var calls int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(&calls, 1)
			handler(w, r)
		}))
		t.Cleanup(srv.Close)
		c := newChatClient(srv.URL, "", 5*time.Second)
		c.retryBudget = time.Minute
		c.sleep = func(context.Context, time.Duration) error { return nil } // no real waiting
		return c, &calls
	}

	t.Run("a 5xx is retried until it succeeds", func(t *testing.T) {
		// given
		var seen int32
		fx, calls := newFixture(t, func(w http.ResponseWriter, r *http.Request) {
			if atomic.AddInt32(&seen, 1) < 3 {
				w.WriteHeader(http.StatusBadGateway)
				return
			}
			fmt.Fprint(w, okBody)
		})
		want := "hi"

		// when
		got, err := fx.complete(context.Background(), "stub", nil, nil, 0)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, got.Message.Content)
		assert.Equal(t, int32(3), atomic.LoadInt32(calls), "two failures then the answer")
	})

	t.Run("a 4xx is the server rejecting this call, so it is not retried", func(t *testing.T) {
		// given
		fx, calls := newFixture(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, "no such model")
		})

		// when
		_, err := fx.complete(context.Background(), "stub", nil, nil, 0)

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "answered 400")
		assert.Equal(t, int32(1), atomic.LoadInt32(calls), "a rejection repeats, so it is asked once")
	})

	t.Run("a zero budget keeps the old fail-fast behaviour", func(t *testing.T) {
		// given
		fx, calls := newFixture(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		})
		fx.retryBudget = 0

		// when
		_, err := fx.complete(context.Background(), "stub", nil, nil, 0)

		// then
		require.Error(t, err)
		assert.Equal(t, int32(1), atomic.LoadInt32(calls))
	})

	t.Run("a cancelled context stops the retry loop", func(t *testing.T) {
		// given
		fx, _ := newFixture(t, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		})
		ctx, cancel := context.WithCancel(context.Background())
		fx.sleep = func(context.Context, time.Duration) error { cancel(); return context.Canceled }

		// when
		_, err := fx.complete(ctx, "stub", nil, nil, 0)

		// then
		require.Error(t, err)
	})
}
