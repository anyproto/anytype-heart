package client

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The three-state producer seam: rate limiting is normal operation,
// not an error — the pacer knows when it is sleeping and the retry loop
// knows its attempt count; they need a reporting hook, not new detection.

type recordingHook struct {
	mu        sync.Mutex
	events    []string
	throttled []time.Duration
	attempts  [][2]int
}

func (h *recordingHook) Throttled(resumeIn time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = append(h.events, "throttled")
	h.throttled = append(h.throttled, resumeIn)
}

func (h *recordingHook) Retrying(attempt, attemptsMax int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = append(h.events, "retrying")
	h.attempts = append(h.attempts, [2]int{attempt, attemptsMax})
}

func (h *recordingHook) Recovered() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = append(h.events, "recovered")
}

func (h *recordingHook) snapshot() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.events...)
}

func TestStatusHookRetrying(t *testing.T) {
	t.Run("a transient failure reports attempt N of M, then recovery", func(t *testing.T) {
		// given: the first response fails retryably, the second succeeds
		hook := &recordingHook{}
		calls := 0
		c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			calls++
			if calls == 1 {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			fmt.Fprint(w, `{}`)
		}, WithStatusHook(hook))

		// when
		err := c.Request(context.Background(), http.MethodGet, "/users", nil, nil)

		// then
		require.NoError(t, err)
		assert.Equal(t, []string{"retrying", "recovered"}, hook.snapshot())
		require.Len(t, hook.attempts, 1)
		assert.Equal(t, [2]int{1, fastPolicy().MaxAttempts}, hook.attempts[0],
			"attempt N of M — the client's bounded policy, not a guess")
	})

	t.Run("a clean request reports nothing", func(t *testing.T) {
		// given — the hook is a calm/alarm EDGE signal: a healthy request
		// stream must not spam recoveries
		hook := &recordingHook{}
		c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{}`)
		}, WithStatusHook(hook))

		// when
		require.NoError(t, c.Request(context.Background(), http.MethodGet, "/users", nil, nil))

		// then
		assert.Empty(t, hook.snapshot())
	})
}

func TestStatusHookThrottled(t *testing.T) {
	t.Run("a pushback pause reports throttled with its resume time", func(t *testing.T) {
		// given: one worker's 429 pushed every worker back (the shared
		// pacer); the NEXT request meets the pause
		hook := &recordingHook{}
		c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{}`)
		}, WithStatusHook(hook))
		c.pacer.pushback(80 * time.Millisecond)

		// when
		start := time.Now()
		require.NoError(t, c.Request(context.Background(), http.MethodGet, "/users", nil, nil))

		// then: throttled (with a real resume duration) before the wait,
		// recovered after the request completed
		assert.GreaterOrEqual(t, time.Since(start), 60*time.Millisecond)
		events := hook.snapshot()
		require.NotEmpty(t, events)
		assert.Equal(t, "throttled", events[0])
		assert.Equal(t, "recovered", events[len(events)-1])
		require.NotEmpty(t, hook.throttled)
		assert.Greater(t, hook.throttled[0], time.Duration(0))
		assert.LessOrEqual(t, hook.throttled[0], 80*time.Millisecond)
	})

	t.Run("a cancelled wait does not report recovery", func(t *testing.T) {
		// given — recovery means "back to normal", not "gave up": a run
		// cancelled mid-throttle must not flash a healthy state on its way
		// out
		hook := &recordingHook{}
		c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{}`)
		}, WithStatusHook(hook))
		c.pacer.pushback(5 * time.Second)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
		defer cancel()

		// when
		err := c.Request(ctx, http.MethodGet, "/users", nil, nil)

		// then
		require.Error(t, err)
		events := hook.snapshot()
		assert.Equal(t, []string{"throttled"}, events)
	})
}
