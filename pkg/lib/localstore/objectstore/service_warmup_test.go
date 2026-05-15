package objectstore

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWaitStoresLoaded_OpensAuthoritativeSet(t *testing.T) {
	want := []string{"space-a", "space-b", "space-c"}
	fx := NewStoreFixtureWithSpaceIds(t, want)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, fx.WaitStoresLoaded(ctx))

	opened := map[string]struct{}{}
	for _, id := range fx.OpenedSpaceIds() {
		opened[id] = struct{}{}
	}
	for _, id := range want {
		_, ok := opened[id]
		assert.Truef(t, ok, "space %s should be opened by warm-up", id)
	}
}

func TestWaitStoresLoaded_ContextCancelled(t *testing.T) {
	s := &dsObjectStore{
		loadedCh:     make(chan struct{}), // never closed
		componentCtx: context.Background(),
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := s.WaitStoresLoaded(ctx)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestPreloadConcurrencyOne_LoadsAll(t *testing.T) {
	old := preloadConcurrency
	preloadConcurrency = 1
	t.Cleanup(func() { preloadConcurrency = old })

	want := []string{"s1", "s2", "s3", "s4", "s5"}
	fx := NewStoreFixtureWithSpaceIds(t, want)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, fx.WaitStoresLoaded(ctx))

	opened := map[string]struct{}{}
	for _, id := range fx.OpenedSpaceIds() {
		opened[id] = struct{}{}
	}
	for _, id := range want {
		_, ok := opened[id]
		assert.Truef(t, ok, "space %s should be opened with concurrency=1", id)
	}
}
