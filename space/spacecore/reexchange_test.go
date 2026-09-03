package spacecore

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/space/spacecore/localdiscovery"
	"github.com/anyproto/anytype-heart/space/spacecore/peerstore"
)

func TestReexchanger(t *testing.T) {
	const window = 20 * time.Millisecond
	waitRounds := func(t *testing.T, runs *atomic.Int32, want int32) {
		t.Helper()
		require.Eventually(t, func() bool { return runs.Load() == want }, time.Second, time.Millisecond)
	}

	t.Run("a burst of triggers inside the window is one round", func(t *testing.T) {
		// given
		var runs atomic.Int32
		r := newReexchanger(window, func() { runs.Add(1) })
		defer r.close()

		// when
		for i := 0; i < 5; i++ {
			r.trigger()
		}

		// then
		waitRounds(t, &runs, 1)
		time.Sleep(3 * window)
		assert.Equal(t, int32(1), runs.Load())
	})

	t.Run("a trigger during a round schedules exactly one more, after a quiet window", func(t *testing.T) {
		// given
		var runs atomic.Int32
		inRound := make(chan struct{})
		release := make(chan struct{})
		var once sync.Once
		r := newReexchanger(window, func() {
			if runs.Add(1) == 1 {
				once.Do(func() { close(inRound) })
				<-release
			}
		})
		defer r.close()
		r.trigger()
		<-inRound

		// when
		r.trigger()
		r.trigger()
		r.trigger()
		close(release)

		// then
		waitRounds(t, &runs, 2)
		time.Sleep(3 * window)
		assert.Equal(t, int32(2), runs.Load())
	})

	t.Run("closed: no round runs, and close waits for a running one", func(t *testing.T) {
		// given
		var runs atomic.Int32
		r := newReexchanger(window, func() { runs.Add(1) })
		r.trigger()

		// when
		r.close()
		time.Sleep(3 * window)

		// then
		assert.Equal(t, int32(0), runs.Load())
		r.trigger()
		time.Sleep(3 * window)
		assert.Equal(t, int32(0), runs.Load())
	})
}

type exchangeCall struct {
	peerId string
	own    *localdiscovery.OwnAddresses
}

func TestExchangeWithKnownPeers(t *testing.T) {
	type change struct {
		peerId        string
		before, after []string
		removed       bool
	}
	newService := func(t *testing.T, fn func(ctx context.Context, peerId string, own *localdiscovery.OwnAddresses) ([]string, error)) (*service, *[]change) {
		t.Helper()
		var (
			mu      sync.Mutex
			changes []change
		)
		store := peerstore.New()
		store.AddObserver(func(peerId string, before, after []string, removed bool) {
			mu.Lock()
			defer mu.Unlock()
			changes = append(changes, change{peerId, before, after, removed})
		})
		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(cancel)
		return &service{peerStore: store, componentCtx: ctx, exchangeFn: fn}, &changes
	}

	t.Run("a peer that shared nothing flips to sharing on the re-exchange; a failing one is left alone", func(t *testing.T) {
		// given: two peers registered from the cold exchange with nothing shared
		var calls []exchangeCall
		s, changes := newService(t, func(_ context.Context, peerId string, own *localdiscovery.OwnAddresses) ([]string, error) {
			calls = append(calls, exchangeCall{peerId, own})
			if peerId == "p2" {
				return nil, errors.New("unreachable")
			}
			return []string{"tech.space", "spaceA"}, nil
		})
		s.peerStore.UpdateLocalPeer("p1", []string{})
		s.peerStore.UpdateLocalPeer("p2", []string{})
		s.rememberOwn(localdiscovery.OwnAddresses{Addrs: []string{"192.168.1.5"}, Port: 4242})

		// when
		s.exchangeWithKnownPeers()

		// then
		want := []change{
			{"p1", nil, []string{}, false},
			{"p2", nil, []string{}, false},
			// the store sorts the list before notifying
			{"p1", []string{}, []string{"spaceA", "tech.space"}, false},
		}
		assert.Equal(t, want, *changes)
		assert.ElementsMatch(t, []string{"p1"}, s.peerStore.LocalPeerIds("tech.space"))
		assert.ElementsMatch(t, []string{"p1", "p2"}, s.peerStore.AllLocalPeers(), "a failed exchange is not a blacklist")
		require.Len(t, calls, 2)
		assert.Equal(t, 4242, calls[0].own.Port)
	})

	t.Run("without known addresses the exchange goes as a probe", func(t *testing.T) {
		// given
		var calls []exchangeCall
		s, _ := newService(t, func(_ context.Context, peerId string, own *localdiscovery.OwnAddresses) ([]string, error) {
			calls = append(calls, exchangeCall{peerId, own})
			return nil, nil
		})
		s.peerStore.UpdateLocalPeer("p1", nil)

		// when
		s.exchangeWithKnownPeers()

		// then
		require.Len(t, calls, 1)
		assert.Nil(t, calls[0].own)
	})

	t.Run("no known peers is a no-op", func(t *testing.T) {
		called := false
		s, _ := newService(t, func(context.Context, string, *localdiscovery.OwnAddresses) ([]string, error) {
			called = true
			return nil, nil
		})
		s.exchangeWithKnownPeers()
		assert.False(t, called)
	})
}
