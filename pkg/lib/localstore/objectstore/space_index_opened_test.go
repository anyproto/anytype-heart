package objectstore

import (
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOnSpaceIndexOpened(t *testing.T) {
	t.Run("callback fires on first SpaceIndex call per space", func(t *testing.T) {
		s := NewStoreFixture(t)

		var (
			mu       sync.Mutex
			received []string
		)
		s.OnSpaceIndexOpened(func(spaceId string) {
			mu.Lock()
			defer mu.Unlock()
			received = append(received, spaceId)
		})

		// First open: callback fires
		_ = s.SpaceIndex("spaceA")
		// Second open of same space: no duplicate callback
		_ = s.SpaceIndex("spaceA")
		// Open of a different space: callback fires once
		_ = s.SpaceIndex("spaceB")

		mu.Lock()
		got := append([]string(nil), received...)
		mu.Unlock()
		sort.Strings(got)
		assert.Equal(t, []string{"spaceA", "spaceB"}, got)
	})

	t.Run("registration replays already-opened spaces", func(t *testing.T) {
		s := NewStoreFixture(t)

		// Open spaces before the listener registers.
		_ = s.SpaceIndex("preOpened1")
		_ = s.SpaceIndex("preOpened2")

		var (
			mu       sync.Mutex
			received []string
		)
		s.OnSpaceIndexOpened(func(spaceId string) {
			mu.Lock()
			defer mu.Unlock()
			received = append(received, spaceId)
		})

		mu.Lock()
		got := append([]string(nil), received...)
		mu.Unlock()
		sort.Strings(got)
		assert.Equal(t, []string{"preOpened1", "preOpened2"}, got)
	})

	t.Run("OpenedSpaceIds reports opened spaces", func(t *testing.T) {
		s := NewStoreFixture(t)
		_ = s.SpaceIndex("x")
		_ = s.SpaceIndex("y")
		_ = s.SpaceIndex("x") // idempotent

		ids := s.OpenedSpaceIds()
		sort.Strings(ids)
		// fixture also opens tech-space lazily on first AddObjects call;
		// here we never touched it, so only x and y should be present.
		require.Equal(t, []string{"x", "y"}, ids)
	})

	t.Run("callback fires exactly once under concurrent first-open callers", func(t *testing.T) {
		// Concurrent SpaceIndex callers must not deadlock and must not
		// double-fire the callback. They are deliberately NOT required to
		// block until the callback finishes — data consistency for a
		// racing writer is guaranteed by the subscription layer's
		// persist-then-requery design, not by blocking here. See
		// crossspacesub.TestLazySubscribe_NoDataLossUnderConcurrentOpen.
		s := NewStoreFixture(t)

		var fireCount atomic.Int64
		releaseCallback := make(chan struct{})
		s.OnSpaceIndexOpened(func(spaceId string) {
			fireCount.Add(1)
			<-releaseCallback // hold the firing goroutine for a while
		})

		const callers = 16
		var wg sync.WaitGroup
		for i := 0; i < callers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_ = s.SpaceIndex("raceSpace")
			}()
		}

		// Concurrent callers that did not win the first-open must return
		// promptly even though the callback is still held — no blocking.
		assert.Eventually(t, func() bool {
			// All-but-the-firing goroutine should have returned; the
			// firing one is parked in the callback on releaseCallback.
			return fireCount.Load() == 1
		}, 2*time.Second, 5*time.Millisecond)

		close(releaseCallback)
		wg.Wait()
		assert.Equal(t, int64(1), fireCount.Load(),
			"callback must fire exactly once regardless of concurrent callers")
	})

	t.Run("recursive SpaceIndex from inside callback does not deadlock", func(t *testing.T) {
		// The cross-space sub's callback chain re-enters SpaceIndex for
		// the same space (PromotePending → Search → getSpaceSubscriptions
		// → SpaceIndex). The recursive call must return immediately
		// without waiting on the still-firing callback.
		s := NewStoreFixture(t)

		var (
			recursiveCalled atomic.Bool
			recursiveStore  atomic.Value
		)
		s.OnSpaceIndexOpened(func(spaceId string) {
			// Recurse from the same goroutine.
			store := s.SpaceIndex(spaceId)
			recursiveStore.Store(store)
			recursiveCalled.Store(true)
		})

		done := make(chan struct{})
		go func() {
			_ = s.SpaceIndex("recursiveSpace")
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("deadlock: SpaceIndex did not return")
		}
		assert.True(t, recursiveCalled.Load())
		assert.NotNil(t, recursiveStore.Load())
	})

	t.Run("multiple subscribers all receive notifications", func(t *testing.T) {
		s := NewStoreFixture(t)

		var (
			mu sync.Mutex
			a  []string
			b  []string
		)
		s.OnSpaceIndexOpened(func(spaceId string) {
			mu.Lock()
			a = append(a, spaceId)
			mu.Unlock()
		})
		s.OnSpaceIndexOpened(func(spaceId string) {
			mu.Lock()
			b = append(b, spaceId)
			mu.Unlock()
		})

		_ = s.SpaceIndex("solo")

		mu.Lock()
		gotA := append([]string(nil), a...)
		gotB := append([]string(nil), b...)
		mu.Unlock()
		assert.Equal(t, []string{"solo"}, gotA)
		assert.Equal(t, []string{"solo"}, gotB)
	})
}
