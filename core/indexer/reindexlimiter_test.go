package indexer

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMaxConcurrentSpaceReindexFor(t *testing.T) {
	t.Run("mobile platforms are capped at 2", func(t *testing.T) {
		assert.Equal(t, 2, maxConcurrentSpaceReindexFor("ios"))
		assert.Equal(t, 2, maxConcurrentSpaceReindexFor("android"))
	})

	t.Run("other platforms are capped at 4", func(t *testing.T) {
		assert.Equal(t, 4, maxConcurrentSpaceReindexFor("darwin"))
		assert.Equal(t, 4, maxConcurrentSpaceReindexFor("linux"))
		assert.Equal(t, 4, maxConcurrentSpaceReindexFor("windows"))
	})
}

// acquireAsync runs acquire in a goroutine and reports its result on the
// returned channel, so tests can assert both blocking and completion.
func acquireAsync(l *reindexLimiter, ctx context.Context, spaceId string) <-chan bool {
	res := make(chan bool, 1)
	go func() {
		res <- l.acquire(ctx, spaceId)
	}()
	return res
}

func TestReindexLimiter(t *testing.T) {
	ctx := context.Background()

	t.Run("acquires immediately while slots are free, then blocks", func(t *testing.T) {
		// given
		l := newReindexLimiter(2, nil)

		// when
		require.True(t, l.acquire(ctx, "space1"))
		require.True(t, l.acquire(ctx, "space2"))
		blocked := acquireAsync(l, ctx, "space3")

		// then
		select {
		case <-blocked:
			t.Fatal("third acquire succeeded beyond the limit")
		case <-time.After(100 * time.Millisecond):
		}
		require.Eventually(t, func() bool { return l.waitingCount() == 1 }, time.Second, 5*time.Millisecond)

		// when a slot frees
		l.release()

		// then the waiter gets it
		select {
		case ok := <-blocked:
			assert.True(t, ok)
		case <-time.After(time.Second):
			t.Fatal("waiter never got the freed slot")
		}
	})

	t.Run("without opened spaces waiters are granted in FIFO order", func(t *testing.T) {
		// given
		l := newReindexLimiter(1, nil)
		require.True(t, l.acquire(ctx, "space1"))

		first := acquireAsync(l, ctx, "spaceFirst")
		require.Eventually(t, func() bool { return l.waitingCount() == 1 }, time.Second, 5*time.Millisecond)
		second := acquireAsync(l, ctx, "spaceSecond")
		require.Eventually(t, func() bool { return l.waitingCount() == 2 }, time.Second, 5*time.Millisecond)

		// when
		l.release()

		// then
		select {
		case <-first:
		case <-time.After(time.Second):
			t.Fatal("first waiter never granted")
		}
		select {
		case <-second:
			t.Fatal("second waiter granted before the first released")
		case <-time.After(100 * time.Millisecond):
		}
		l.release()
		select {
		case <-second:
		case <-time.After(time.Second):
			t.Fatal("second waiter never granted")
		}
	})

	t.Run("freed slot goes to a waiting opened space before earlier waiters", func(t *testing.T) {
		// given
		l := newReindexLimiter(1, func() map[string]struct{} {
			return map[string]struct{}{"spaceOpened": {}}
		})
		require.True(t, l.acquire(ctx, "space1"))

		background := acquireAsync(l, ctx, "spaceBackground")
		require.Eventually(t, func() bool { return l.waitingCount() == 1 }, time.Second, 5*time.Millisecond)
		opened := acquireAsync(l, ctx, "spaceOpened")
		require.Eventually(t, func() bool { return l.waitingCount() == 2 }, time.Second, 5*time.Millisecond)

		// when
		l.release()

		// then the opened space overtakes the earlier background waiter
		select {
		case <-opened:
		case <-time.After(time.Second):
			t.Fatal("opened space never granted")
		}
		select {
		case <-background:
			t.Fatal("background space granted while opened space held the slot")
		case <-time.After(100 * time.Millisecond):
		}
		l.release()
		select {
		case <-background:
		case <-time.After(time.Second):
			t.Fatal("background space never granted")
		}
	})

	t.Run("already-cancelled ctx is not granted even when slots are free", func(t *testing.T) {
		// given
		l := newReindexLimiter(2, nil)
		cancelledCtx, cancel := context.WithCancel(ctx)
		cancel()

		// when
		granted := l.acquire(cancelledCtx, "space1")

		// then
		assert.False(t, granted)
		// both slots stayed free for live callers; bounded ctx so a wrongly
		// held slot fails the test instead of hanging it
		liveCtx, liveCancel := context.WithTimeout(ctx, time.Second)
		defer liveCancel()
		require.True(t, l.acquire(liveCtx, "space2"))
		require.True(t, l.acquire(liveCtx, "space3"))
	})

	t.Run("cancelled waiter leaves the queue and is not granted", func(t *testing.T) {
		// given
		l := newReindexLimiter(1, nil)
		require.True(t, l.acquire(ctx, "space1"))

		waitCtx, cancel := context.WithCancel(ctx)
		cancelled := acquireAsync(l, waitCtx, "spaceCancelled")
		require.Eventually(t, func() bool { return l.waitingCount() == 1 }, time.Second, 5*time.Millisecond)
		remaining := acquireAsync(l, ctx, "spaceRemaining")
		require.Eventually(t, func() bool { return l.waitingCount() == 2 }, time.Second, 5*time.Millisecond)

		// when
		cancel()

		// then
		select {
		case ok := <-cancelled:
			assert.False(t, ok)
		case <-time.After(time.Second):
			t.Fatal("cancelled waiter never returned")
		}
		require.Eventually(t, func() bool { return l.waitingCount() == 1 }, time.Second, 5*time.Millisecond)

		// and the slot still hands over to the remaining waiter
		l.release()
		select {
		case ok := <-remaining:
			assert.True(t, ok)
		case <-time.After(time.Second):
			t.Fatal("remaining waiter never granted")
		}
	})
}
