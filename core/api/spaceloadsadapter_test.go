package api

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/clientspace"
)

// fakeSpaceGetter is space.Service.Get: a known space is returned, a loading
// one blocks until the context ends, any other id fails.
type fakeSpaceGetter struct {
	loaded  map[string]bool
	loading map[string]bool
}

func (f fakeSpaceGetter) Get(ctx context.Context, id string) (clientspace.Space, error) {
	if f.loaded[id] {
		return nil, nil
	}
	if f.loading[id] {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return nil, space.ErrSpaceNotExists
}

func TestSpaceLoadsAdapter(t *testing.T) {
	getter := fakeSpaceGetter{loaded: map[string]bool{"loaded": true}, loading: map[string]bool{"loading": true}}
	adapter := newSpaceLoadsAdapter(getter)

	t.Run("a loaded space returns at once", func(t *testing.T) {
		require.NoError(t, adapter.WaitLoaded(context.Background(), "loaded"))
	})

	t.Run("a loading space waits no longer than the context", func(t *testing.T) {
		// given
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		done := make(chan error, 1)

		// when
		go func() { done <- adapter.WaitLoaded(ctx, "loading") }()

		// then
		select {
		case err := <-done:
			require.ErrorIs(t, err, context.DeadlineExceeded)
		case <-time.After(5 * time.Second):
			t.Fatal("WaitLoaded outlived its context")
		}
	})

	t.Run("the space lookup's error is kept in the chain", func(t *testing.T) {
		err := adapter.WaitLoaded(context.Background(), "unknown")
		require.Error(t, err)
		assert.True(t, errors.Is(err, space.ErrSpaceNotExists))
		assert.Contains(t, err.Error(), "get space unknown")
	})
}
