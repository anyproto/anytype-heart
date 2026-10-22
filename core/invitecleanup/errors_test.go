package invitecleanup

import (
	"context"
	"errors"

	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithRetry(t *testing.T) {
	// keep the tests fast: the real delays are seconds
	original := retryDelays
	retryDelays = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}
	t.Cleanup(func() { retryDelays = original })

	t.Run("transient error is retried until it succeeds", func(t *testing.T) {
		// given a call that fails twice before working
		calls := 0
		err := withRetry(context.Background(), func() error {
			calls++
			if calls < 3 {
				return errors.New("connection reset")
			}
			return nil
		})

		require.NoError(t, err)
		assert.Equal(t, 3, calls)
	})

	t.Run("transient error gives up after the last delay", func(t *testing.T) {
		calls := 0
		err := withRetry(context.Background(), func() error {
			calls++
			return errors.New("connection reset")
		})

		require.Error(t, err)
		assert.Equal(t, len(retryDelays)+1, calls)
	})

	t.Run("permanent error is not retried", func(t *testing.T) {
		// given an error no amount of retrying will fix
		calls := 0
		err := withRetry(context.Background(), func() error {
			calls++
			return permanent(errInviteLive)
		})

		require.ErrorIs(t, err, errInviteLive)
		assert.Equal(t, 1, calls)
	})

	t.Run("cancelled context stops the retries", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		calls := 0
		err := withRetry(ctx, func() error {
			calls++
			return errors.New("connection reset")
		})

		require.ErrorIs(t, err, context.Canceled)
		assert.Equal(t, 1, calls)
	})
}

func TestIsPermanent(t *testing.T) {
	t.Run("unrecognised errors are transient, so they get another chance", func(t *testing.T) {
		assert.False(t, isPermanent(errors.New("some unexpected transport failure")))
	})

	t.Run("permanence survives wrapping", func(t *testing.T) {
		err := errors.Join(errors.New("context"), permanent(errGuestInvite))
		assert.True(t, isPermanent(err))
	})
}
