package anystorage

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// blocked reports whether f is still running after a short grace period. It is
// the only way to assert "this caller is waiting" without reaching into the
// lock, and it never reports a false positive: acquired is closed before the
// grace period starts elapsing.
func blocked(acquired <-chan struct{}) bool {
	select {
	case <-acquired:
		return false
	case <-time.After(200 * time.Millisecond):
		return true
	}
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
		acquired := make(chan struct{})
		go func() {
			secondUnlock, err := s.lockSpace(ctx, "space1")
			if err == nil {
				close(acquired)
				secondUnlock()
			}
		}()

		// then
		assert.True(t, blocked(acquired), "the second caller must wait while the first holds the space")
		unlock()
		select {
		case <-acquired:
		case <-time.After(10 * time.Second):
			t.Fatal("the second caller never acquired the space after it was released")
		}
	})

	t.Run("a caller for another space is not held up", func(t *testing.T) {
		// given
		s := newTestService(t)
		unlock, err := s.lockSpace(ctx, "space1")
		require.NoError(t, err)
		defer unlock()

		// when
		acquired := make(chan struct{})
		go func() {
			otherUnlock, err := s.lockSpace(ctx, "space2")
			if err == nil {
				close(acquired)
				otherUnlock()
			}
		}()

		// then
		assert.False(t, blocked(acquired), "spaces must not serialize against each other")
	})

	t.Run("a queued caller gives up when its context is cancelled", func(t *testing.T) {
		// given
		s := newTestService(t)
		unlock, err := s.lockSpace(ctx, "space1")
		require.NoError(t, err)
		cancelCtx, cancel := context.WithCancel(ctx)

		// when
		failed := make(chan error, 1)
		go func() {
			_, err := s.lockSpace(cancelCtx, "space1")
			failed <- err
		}()
		cancel()

		// then
		select {
		case err := <-failed:
			require.ErrorIs(t, err, context.Canceled)
		case <-time.After(10 * time.Second):
			t.Fatal("a cancelled caller kept waiting for the space")
		}
		// the holder is unaffected and the space is still usable afterwards
		unlock()
		again, err := s.lockSpace(ctx, "space1")
		require.NoError(t, err)
		again()
	})

	t.Run("releasing twice does not hand the space to two callers", func(t *testing.T) {
		// given
		s := newTestService(t)
		unlock, err := s.lockSpace(ctx, "space1")
		require.NoError(t, err)

		// when
		unlock()
		unlock()

		// then
		first, err := s.lockSpace(ctx, "space1")
		require.NoError(t, err)
		defer first()
		acquired := make(chan struct{})
		go func() {
			secondUnlock, err := s.lockSpace(ctx, "space1")
			if err == nil {
				close(acquired)
				secondUnlock()
			}
		}()
		assert.True(t, blocked(acquired), "a double release must not leave a spare token behind")
	})
}
