package localdiscovery

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// blockingNotifier records discoveries and blocks the first until released.
type blockingNotifier struct {
	mu      sync.Mutex
	got     [][]string
	links   []string
	running int
	maxRun  int
	release chan struct{}
	first   chan struct{}
}

func (n *blockingNotifier) PeerDiscovered(_ context.Context, peer DiscoveredPeer, _ OwnAddresses) {
	n.mu.Lock()
	n.running++
	n.maxRun = max(n.maxRun, n.running)
	first := len(n.got) == 0
	n.got = append(n.got, peer.Addrs)
	n.links = append(n.links, peer.Link)
	n.mu.Unlock()
	if first {
		close(n.first)
		<-n.release
	}
	n.mu.Lock()
	n.running--
	n.mu.Unlock()
}

func TestPeerDiscoveriesRunsOnePerPeerAndKeepsTheLatest(t *testing.T) {
	// given: the first discovery of p1 is still dialing
	n := &blockingNotifier{release: make(chan struct{}), first: make(chan struct{})}
	var d peerDiscoveries
	ctx := context.Background()
	d.notify(ctx, n, DiscoveredPeer{PeerId: "p1", Addrs: []string{"192.168.1.62:4006"}}, OwnAddresses{})
	<-n.first

	// when: two more arrive meanwhile
	d.notify(ctx, n, DiscoveredPeer{PeerId: "p1", Addrs: []string{"stale"}}, OwnAddresses{})
	d.notify(ctx, n, DiscoveredPeer{PeerId: "p1", Addrs: []string{"[fe80::62%en16]:4006", "192.168.1.62:4006"}}, OwnAddresses{})
	close(n.release)

	// then: one at a time, the parked stale one skipped
	assert.Eventually(t, func() bool {
		d.mu.Lock()
		defer d.mu.Unlock()
		return len(d.inflight) == 0
	}, time.Second, 10*time.Millisecond)
	n.mu.Lock()
	defer n.mu.Unlock()
	assert.Equal(t, [][]string{{"192.168.1.62:4006"}, {"[fe80::62%en16]:4006", "192.168.1.62:4006"}}, n.got)
	assert.Equal(t, 1, n.maxRun)
}

func TestPeerDiscoveriesKeepsTheLatestPerSource(t *testing.T) {
	// given: a discovery of p1 still running
	n := &blockingNotifier{release: make(chan struct{}), first: make(chan struct{})}
	var d peerDiscoveries
	ctx := context.Background()
	d.notify(ctx, n, DiscoveredPeer{PeerId: "p1", Addrs: []string{"192.168.1.62:4006"}}, OwnAddresses{})
	<-n.first

	// when: a base and a cable discovery are parked, then a newer base one
	d.notify(ctx, n, DiscoveredPeer{PeerId: "p1", Addrs: []string{"192.168.1.63:4006"}}, OwnAddresses{})
	d.notify(ctx, n, DiscoveredPeer{PeerId: "p1", Link: "31", Addrs: []string{"[fe80::62%31]:4006"}}, OwnAddresses{})
	d.notify(ctx, n, DiscoveredPeer{PeerId: "p1", Addrs: []string{"192.168.1.64:4006"}}, OwnAddresses{})
	close(n.release)

	// then: both sources run, in arrival order, the base one at its newest
	assert.Eventually(t, func() bool {
		d.mu.Lock()
		defer d.mu.Unlock()
		return len(d.inflight) == 0
	}, time.Second, 10*time.Millisecond)
	n.mu.Lock()
	defer n.mu.Unlock()
	want := [][]string{{"192.168.1.62:4006"}, {"192.168.1.64:4006"}, {"[fe80::62%31]:4006"}}
	assert.Equal(t, want, n.got)
	assert.Equal(t, 1, n.maxRun)
}

func TestPeerDiscoveriesKeepsEachCable(t *testing.T) {
	// given: a discovery of p1 still running
	n := &blockingNotifier{release: make(chan struct{}), first: make(chan struct{})}
	var d peerDiscoveries
	ctx := context.Background()
	d.notify(ctx, n, DiscoveredPeer{PeerId: "p1", Addrs: []string{"192.168.1.62:4006"}}, OwnAddresses{})
	<-n.first

	// when: two cables are parked, then a newer answer from the first
	d.notify(ctx, n, DiscoveredPeer{PeerId: "p1", Link: "31", Addrs: []string{"[fe80::62%31]:4006"}}, OwnAddresses{})
	d.notify(ctx, n, DiscoveredPeer{PeerId: "p1", Link: "32", Addrs: []string{"[fe80::62%32]:4006"}}, OwnAddresses{})
	d.notify(ctx, n, DiscoveredPeer{PeerId: "p1", Link: "31", Addrs: []string{"[fe80::63%31]:4006"}}, OwnAddresses{})
	close(n.release)

	// then
	assert.Eventually(t, func() bool {
		d.mu.Lock()
		defer d.mu.Unlock()
		return len(d.inflight) == 0
	}, time.Second, 10*time.Millisecond)
	n.mu.Lock()
	defer n.mu.Unlock()
	assert.Equal(t, [][]string{{"192.168.1.62:4006"}, {"[fe80::63%31]:4006"}, {"[fe80::62%32]:4006"}}, n.got)
	assert.Equal(t, []string{"", "31", "32"}, n.links)
}
