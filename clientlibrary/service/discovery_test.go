package service

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/space/spacecore/localdiscovery"
)

type fakeDiscoveryProxy struct {
	observer DiscoveryObserver
}

func (f *fakeDiscoveryProxy) SetObserver(o DiscoveryObserver) { f.observer = o }
func (f *fakeDiscoveryProxy) RemoveObserver()                 { f.observer = nil }

type fakeObservationResult struct {
	port   int
	ip     string
	peerId string
}

func (r fakeObservationResult) Port() int      { return r.port }
func (r fakeObservationResult) Ip() string     { return r.ip }
func (r fakeObservationResult) PeerId() string { return r.peerId }

type discoveredCall struct {
	peer   localdiscovery.DiscoveredPeer
	ctxErr error
}

// recordingNotifier records every call. A peer id in block makes its
// PeerDiscovered wait until the channel is closed or the ctx is canceled.
type recordingNotifier struct {
	mu         sync.Mutex
	discovered []discoveredCall
	events     []string
	block      map[string]chan struct{}
}

func (r *recordingNotifier) PeerDiscovered(ctx context.Context, peer localdiscovery.DiscoveredPeer, _ localdiscovery.OwnAddresses) {
	r.mu.Lock()
	gate := r.block[peer.PeerId]
	r.mu.Unlock()
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
		}
	}
	r.mu.Lock()
	r.discovered = append(r.discovered, discoveredCall{peer: peer, ctxErr: ctx.Err()})
	r.mu.Unlock()
}

func (r *recordingNotifier) PeerLost(peerId string) {
	r.mu.Lock()
	r.events = append(r.events, "lost:"+peerId)
	r.mu.Unlock()
}

func (r *recordingNotifier) DiscoveryError(code int, message string) {
	r.mu.Lock()
	r.events = append(r.events, fmt.Sprintf("error:%d:%s", code, message))
	r.mu.Unlock()
}

func (r *recordingNotifier) calls() []discoveredCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]discoveredCall(nil), r.discovered...)
}

func (r *recordingNotifier) recordedEvents() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.events...)
}

type bridgeFixture struct {
	proxy    *fakeDiscoveryProxy
	provider *notifierProvider
	notifier *recordingNotifier
}

func newBridgeFixture() *bridgeFixture {
	proxy := &fakeDiscoveryProxy{}
	fx := &bridgeFixture{
		proxy:    proxy,
		provider: newNotifierProvider(proxy),
		notifier: &recordingNotifier{block: map[string]chan struct{}{}},
	}
	fx.provider.Provide(fx.notifier, 4006, "self", "_anytype._tcp")
	return fx
}

func TestDiscoveryObserver(t *testing.T) {
	// Newer Android NSD can report IPv6 addresses; "%s:%d" produces unparseable
	// strings like fe80::1:4006 — they must be formatted with brackets.
	t.Run("formats ipv6 addresses with brackets", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			// given
			fx := newBridgeFixture()
			want := []string{"192.168.1.5:4006", "[fe80::1]:4006"}

			// when
			fx.proxy.observer.ObserveChange(fakeObservationResult{port: 4006, ip: "192.168.1.5,fe80::1", peerId: "peer2"})
			synctest.Wait()

			// then
			calls := fx.notifier.calls()
			require.Len(t, calls, 1)
			assert.Equal(t, want, calls[0].peer.Addrs)
			assert.Equal(t, "peer2", calls[0].peer.PeerId)
		})
	})

	t.Run("ObserveChange returns while the exchange is still running", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			// given
			fx := newBridgeFixture()
			gate := make(chan struct{})
			fx.notifier.block["peer2"] = gate

			// when: the app's discovery thread reports a peer whose exchange blocks
			fx.proxy.observer.ObserveChange(fakeObservationResult{port: 4006, ip: "192.168.1.5", peerId: "peer2"})

			// then: the call already returned; the exchange finishes on its own
			assert.Empty(t, fx.notifier.calls())
			close(gate)
			synctest.Wait()
			assert.Len(t, fx.notifier.calls(), 1)
		})
	})

	t.Run("changes during a running exchange collapse into one follow-up with the latest addresses", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			// given
			fx := newBridgeFixture()
			gate := make(chan struct{})
			fx.notifier.block["peer2"] = gate
			observer := fx.proxy.observer
			observer.ObserveChange(fakeObservationResult{port: 4006, ip: "10.0.0.1", peerId: "peer2"})
			synctest.Wait()

			// when
			observer.ObserveChange(fakeObservationResult{port: 4006, ip: "10.0.0.2", peerId: "peer2"})
			observer.ObserveChange(fakeObservationResult{port: 4006, ip: "10.0.0.3", peerId: "peer2"})
			close(gate)
			synctest.Wait()

			// then
			calls := fx.notifier.calls()
			require.Len(t, calls, 2)
			assert.Equal(t, []string{"10.0.0.1:4006"}, calls[0].peer.Addrs)
			assert.Equal(t, []string{"10.0.0.3:4006"}, calls[1].peer.Addrs)
		})
	})

	t.Run("a blocked peer does not delay another peer", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			// given
			fx := newBridgeFixture()
			gate := make(chan struct{})
			fx.notifier.block["slow"] = gate
			observer := fx.proxy.observer

			// when
			observer.ObserveChange(fakeObservationResult{port: 4006, ip: "10.0.0.1", peerId: "slow"})
			observer.ObserveChange(fakeObservationResult{port: 4006, ip: "10.0.0.2", peerId: "fast"})
			synctest.Wait()

			// then
			calls := fx.notifier.calls()
			require.Len(t, calls, 1)
			assert.Equal(t, "fast", calls[0].peer.PeerId)
			close(gate)
		})
	})

	t.Run("lost and error events arrive in the order the app reported them", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			// given
			fx := newBridgeFixture()
			observer := fx.proxy.observer
			want := []string{"error:-65570:denied", "lost:peer2", "error:0:", "lost:peer3"}

			// when
			observer.ObserveError(-65570, "denied")
			observer.ObserveLost("peer2")
			observer.ObserveError(0, "")
			observer.ObserveLost("peer3")
			synctest.Wait()

			// then
			assert.Equal(t, want, fx.notifier.recordedEvents())
		})
	})
}

func TestNotifierProvider(t *testing.T) {
	// After an account switch (Remove + Provide), the new observer must run with a
	// live context — not the canceled one of the previous generation.
	t.Run("Provide after Remove uses a live context", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			// given
			fx := newBridgeFixture()
			fx.provider.Remove()
			second := &recordingNotifier{}
			fx.provider.Provide(second, 4007, "self2", "_anytype._tcp")

			// when
			fx.proxy.observer.ObserveChange(fakeObservationResult{port: 4007, ip: "192.168.1.5", peerId: "peer3"})
			synctest.Wait()

			// then
			calls := second.calls()
			require.Len(t, calls, 1)
			assert.NoError(t, calls[0].ctxErr)
		})
	})

	// Remove must cancel the context of the current generation, so in-flight
	// space exchanges are aborted on account stop.
	t.Run("Remove cancels an exchange in flight", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			// given
			fx := newBridgeFixture()
			fx.notifier.block["peer2"] = make(chan struct{}) // never released
			fx.proxy.observer.ObserveChange(fakeObservationResult{port: 4006, ip: "192.168.1.5", peerId: "peer2"})
			synctest.Wait()

			// when
			fx.provider.Remove()
			synctest.Wait()

			// then
			calls := fx.notifier.calls()
			require.Len(t, calls, 1)
			assert.ErrorIs(t, calls[0].ctxErr, context.Canceled)
		})
	})

	t.Run("reports to a removed generation are dropped", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			// given
			fx := newBridgeFixture()
			observer := fx.proxy.observer
			fx.provider.Remove()

			// when: the app still holds the old observer and reports to it
			observer.ObserveChange(fakeObservationResult{port: 4006, ip: "192.168.1.5", peerId: "peer2"})
			observer.ObserveLost("peer2")
			observer.ObserveError(-65570, "denied")
			synctest.Wait()

			// then
			assert.Empty(t, fx.notifier.calls())
			assert.Empty(t, fx.notifier.recordedEvents())
		})
	})
}
