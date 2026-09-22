package anystorage

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// waiter calls lockSpace in the background. started closes before the call, so
// a subsequent "still waiting" assertion cannot pass merely because the
// goroutine had not run yet; acquired closes once it holds the space.
type waiter struct {
	started  chan struct{}
	acquired chan struct{}
	failed   chan error
	unlock   chan func()
}

func newWaiter(ctx context.Context, s *storageService, id string) *waiter {
	w := &waiter{
		started:  make(chan struct{}),
		acquired: make(chan struct{}),
		failed:   make(chan error, 1),
		unlock:   make(chan func(), 1),
	}
	go func() {
		close(w.started)
		unlock, err := s.lockSpace(ctx, id)
		if err != nil {
			w.failed <- err
			return
		}
		w.unlock <- unlock
		close(w.acquired)
	}()
	<-w.started
	return w
}

// waiting asserts the caller has not taken the space. The grace period only
// risks a false pass (a goroutine descheduled past it), never a false failure,
// so a loaded machine cannot turn a correct lock red here.
func (w *waiter) waiting(t *testing.T, msg string) {
	t.Helper()
	select {
	case <-w.acquired:
		t.Fatal(msg)
	case err := <-w.failed:
		t.Fatalf("%s: lockSpace failed: %v", msg, err)
	case <-time.After(200 * time.Millisecond):
	}
}

// acquires asserts the caller gets the space, waiting generously: a correct
// lock hands it over at once, and only a broken one runs out the clock.
func (w *waiter) acquires(t *testing.T, msg string) func() {
	t.Helper()
	select {
	case <-w.acquired:
		return <-w.unlock
	case err := <-w.failed:
		t.Fatalf("%s: lockSpace failed: %v", msg, err)
	case <-time.After(30 * time.Second):
		t.Fatal(msg)
	}
	return nil
}

// The race regression tests depend on the scheduler landing a reader inside the
// creator's initialization window, so they prove the bug is gone but cannot
// prove the lock is what does it. These pin the lock's own contract.
func TestLockSpace(t *testing.T) {
	ctx := context.Background()

	t.Run("a second caller for one space waits for the first", func(t *testing.T) {
		// given
		s := newTestService(t)
		unlock, err := s.lockSpace(ctx, "space1")
		require.NoError(t, err)

		// when
		second := newWaiter(ctx, s, "space1")

		// then
		second.waiting(t, "the second caller must wait while the first holds the space")
		unlock()
		second.acquires(t, "the second caller never got the space after it was released")()
	})

	t.Run("a caller for another space is not held up", func(t *testing.T) {
		// given
		s := newTestService(t)
		unlock, err := s.lockSpace(ctx, "space1")
		require.NoError(t, err)
		defer unlock()

		// when: an id that does not share space1's shard
		other := otherShardId(t, "space1")
		second := newWaiter(ctx, s, other)

		// then
		second.acquires(t, "spaces must not serialize against each other")()
	})

	t.Run("a queued caller gives up when its context is cancelled", func(t *testing.T) {
		// given
		s := newTestService(t)
		unlock, err := s.lockSpace(ctx, "space1")
		require.NoError(t, err)
		cancelCtx, cancel := context.WithCancel(ctx)
		queued := newWaiter(cancelCtx, s, "space1")
		queued.waiting(t, "the caller must be queued before its context is cancelled")

		// when
		cancel()

		// then
		select {
		case err := <-queued.failed:
			require.ErrorIs(t, err, context.Canceled)
		case <-queued.acquired:
			t.Fatal("a cancelled caller took the space out from under its holder")
		case <-time.After(30 * time.Second):
			t.Fatal("a cancelled caller kept waiting for the space")
		}
		// the holder is unaffected and the space is usable once it releases
		unlock()
		again, err := s.lockSpace(ctx, "space1")
		require.NoError(t, err)
		again()
	})

	t.Run("a stale release cannot let two callers hold one space", func(t *testing.T) {
		// given: the ordering a select/default release would survive -- A
		// releases, B takes the space, and only then does A release again
		s := newTestService(t)
		unlockA, err := s.lockSpace(ctx, "space1")
		require.NoError(t, err)
		unlockA()
		b := newWaiter(ctx, s, "space1")
		unlockB := b.acquires(t, "B never took the released space")

		// when
		unlockA()

		// then
		c := newWaiter(ctx, s, "space1")
		c.waiting(t, "a stale release handed the space to C while B still held it")
		unlockB()
		c.acquires(t, "C never got the space after B released")()
	})

	t.Run("two spaces sharing a shard queue and both finish", func(t *testing.T) {
		// given: a collision costs the second space one open, and must never
		// cost it the space -- nothing takes a second lock while holding one,
		// so a shared shard cannot deadlock
		s := newTestService(t)
		colliding := sameShardId(t, "space1")
		unlock, err := s.lockSpace(ctx, "space1")
		require.NoError(t, err)

		// when
		second := newWaiter(ctx, s, colliding)

		// then
		second.waiting(t, "a colliding id shares the shard, so it has to queue")
		unlock()
		second.acquires(t, "a colliding id must get the shard once it is free")()
	})

	t.Run("callers racing to create one shard still exclude each other", func(t *testing.T) {
		// given: nobody has touched this space, so the shard is created under
		// contention rather than by a prior sequential caller
		s := newTestService(t)
		const callers = 16
		var inside, maxInside atomic.Int32
		var wg sync.WaitGroup

		// when
		for i := 0; i < callers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				unlock, err := s.lockSpace(ctx, "space1")
				if err != nil {
					return
				}
				defer unlock()
				n := inside.Add(1)
				for {
					got := maxInside.Load()
					if n <= got || maxInside.CompareAndSwap(got, n) {
						break
					}
				}
				time.Sleep(time.Millisecond)
				inside.Add(-1)
			}()
		}
		wg.Wait()

		// then
		assert.Equal(t, int32(1), maxInside.Load(), "only one caller at a time may hold a space")
	})
}

// sameShardId finds an id that collides with base. Spreading the locks over
// shards means two unrelated spaces can share one, so what a collision costs is
// part of the design and worth pinning.
func sameShardId(t *testing.T, base string) string {
	t.Helper()
	for i := 0; i < 100000; i++ {
		candidate := fmt.Sprintf("space-%d", i)
		if candidate != base && spaceLockShard(candidate) == spaceLockShard(base) {
			return candidate
		}
	}
	t.Fatal("no colliding id found")
	return ""
}

// otherShardId finds an id that hashes to a different lock shard than base, so
// a test of cross-space independence is not silently testing a collision.
func otherShardId(t *testing.T, base string) string {
	t.Helper()
	for i := 0; i < 10000; i++ {
		candidate := fmt.Sprintf("space-%d", i)
		if spaceLockShard(candidate) != spaceLockShard(base) {
			return candidate
		}
	}
	t.Fatal("no id found on a different shard")
	return ""
}
