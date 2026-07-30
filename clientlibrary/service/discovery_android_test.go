package service

import (
	"context"
	"testing"

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

type recordingNotifier struct {
	ctxErr error
	called bool
	peer   localdiscovery.DiscoveredPeer
}

func (r *recordingNotifier) PeerDiscovered(ctx context.Context, peer localdiscovery.DiscoveredPeer, own localdiscovery.OwnAddresses) {
	r.called = true
	r.ctxErr = ctx.Err()
	r.peer = peer
}

// Newer Android NSD can report IPv6 addresses; "%s:%d" produces unparseable
// strings like fe80::1:4006 — they must be formatted with brackets.
func TestDiscoveryObserver_ObserveChangeFormatsIPv6WithBrackets(t *testing.T) {
	proxy := &fakeDiscoveryProxy{}
	p := newNotifierProvider(proxy)
	notifier := &recordingNotifier{}
	p.Provide(notifier, 4006, "peer1", "_anytype._tcp")

	proxy.observer.ObserveChange(fakeObservationResult{port: 4006, ip: "192.168.1.5,fe80::1", peerId: "peer2"})

	want := []string{"192.168.1.5:4006", "[fe80::1]:4006"}
	if len(notifier.peer.Addrs) != len(want) {
		t.Fatalf("got addrs %v, want %v", notifier.peer.Addrs, want)
	}
	for i := range want {
		if notifier.peer.Addrs[i] != want[i] {
			t.Fatalf("got addrs %v, want %v", notifier.peer.Addrs, want)
		}
	}
}

// After an account switch (Remove + Provide), the new observer must run with a
// live context — not the canceled one of the previous generation.
func TestNotifierProvider_ProvideAfterRemoveUsesLiveContext(t *testing.T) {
	proxy := &fakeDiscoveryProxy{}
	p := newNotifierProvider(proxy)

	first := &recordingNotifier{}
	p.Provide(first, 4006, "peer1", "_anytype._tcp")
	p.Remove()

	second := &recordingNotifier{}
	p.Provide(second, 4007, "peer2", "_anytype._tcp")
	proxy.observer.ObserveChange(fakeObservationResult{port: 4007, ip: "192.168.1.5", peerId: "peer3"})

	if !second.called {
		t.Fatal("expected the second notifier to be called")
	}
	if second.ctxErr != nil {
		t.Fatalf("observer after Remove+Provide got a dead context: %v", second.ctxErr)
	}
}

// Remove must still cancel the context handed to the current generation's
// observer, so in-flight space exchanges are aborted on account stop.
func TestNotifierProvider_RemoveCancelsCurrentObserverContext(t *testing.T) {
	proxy := &fakeDiscoveryProxy{}
	p := newNotifierProvider(proxy)

	notifier := &recordingNotifier{}
	p.Provide(notifier, 4006, "peer1", "_anytype._tcp")
	observer := proxy.observer
	p.Remove()

	observer.ObserveChange(fakeObservationResult{port: 4006, ip: "192.168.1.5", peerId: "peer3"})
	if notifier.ctxErr == nil {
		t.Fatal("expected the removed generation's context to be canceled")
	}
}
