package application

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/core/recovery"
	"github.com/anyproto/anytype-heart/space"
)

func TestStartFailure(t *testing.T) {
	t.Run("space not exists is joined with the account-not-found sentinel", func(t *testing.T) {
		// given
		err := fmt.Errorf("can't run service 'client.space': %w", fmt.Errorf("init personal space: %w", space.ErrSpaceNotExists))

		// when
		got := startFailure(context.Background(), err)

		// then
		assert.True(t, errors.Is(got, recovery.ErrAccountNotFound))
		assert.True(t, errors.Is(got, space.ErrSpaceNotExists))
	})

	t.Run("a start that returns after its context was cancelled is a cancel, whatever the component reported", func(t *testing.T) {
		// given
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := fmt.Errorf("can't run service 'client.space': %w", space.ErrSpaceNotExists)

		// when
		got := startFailure(ctx, err)

		// then
		assert.True(t, errors.Is(got, context.Canceled))
		assert.True(t, errors.Is(got, space.ErrSpaceNotExists))
		assert.False(t, errors.Is(got, recovery.ErrAccountNotFound), "no verdict is derived for a cancelled start")
	})

	t.Run("other errors pass through unchanged", func(t *testing.T) {
		err := errors.New("boom")
		assert.Same(t, err, startFailure(context.Background(), err))
	})
}
