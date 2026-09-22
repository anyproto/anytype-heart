package anystorage

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anyproto/any-sync/commonspace/spacestorage"
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

		// when
		second := newWaiter(ctx, s, "space2")

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

	t.Run("callers racing to create one entry still exclude each other", func(t *testing.T) {
		// given: nobody has touched this space, so its entry is created under
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

// Keying the locks by id means the map has to be reclaimed, or SpacePush --
// which reaches WaitSpaceStorage with a remote id before any-sync validates the
// payload -- is a way for a peer to grow it with ids that never name a space.
func TestLockSpace_EntriesAreReclaimed(t *testing.T) {
	ctx := context.Background()

	t.Run("an entry is gone once nobody holds or waits on it", func(t *testing.T) {
		// given
		s := newTestService(t)
		require.Zero(t, s.spaceLocks.held())

		// when
		unlock, err := s.lockSpace(ctx, "space1")
		require.NoError(t, err)
		assert.Equal(t, 1, s.spaceLocks.held(), "a held space must have an entry")
		unlock()

		// then
		assert.Zero(t, s.spaceLocks.held(), "the entry must go when its last caller does")
	})

	t.Run("an entry survives while someone is still queued on it", func(t *testing.T) {
		// given
		s := newTestService(t)
		first, err := s.lockSpace(ctx, "space1")
		require.NoError(t, err)
		second := newWaiter(ctx, s, "space1")
		second.waiting(t, "the second caller must be queued")

		// when: the holder leaves while the waiter is still there
		first()

		// then
		release := second.acquires(t, "the queued caller must take the space")
		assert.Equal(t, 1, s.spaceLocks.held(), "the entry must outlive the caller that made it")
		release()
		assert.Zero(t, s.spaceLocks.held())
	})

	t.Run("ids that never name a space leave nothing behind", func(t *testing.T) {
		// given: what a peer pushing made-up ids at us looks like
		s := newTestService(t)

		// when
		for i := 0; i < 1000; i++ {
			_, err := s.WaitSpaceStorage(ctx, fmt.Sprintf("not-a-space-%d", i))
			require.ErrorIs(t, err, spacestorage.ErrSpaceStorageMissing)
		}

		// then
		assert.Zero(t, s.spaceLocks.held(), "a rejected lookup must not leave an entry")
	})
}
