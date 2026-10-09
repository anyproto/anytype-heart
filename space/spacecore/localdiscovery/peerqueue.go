package localdiscovery

import (
	"context"
	"sync"
)

// peerDiscoveries runs PeerDiscovered for one peer at a time. A browser may
// deliver a peer again when an answer on another link adds an address; run
// concurrently, the older call could replace the newer addresses, or share
// the newer call's dial of the older ones. A discovery that arrives while one
// runs is parked, and only the latest one runs next (as in the app bridge's
// discoveryObserver).
type peerDiscoveries struct {
	mu       sync.Mutex
	inflight map[string]*pendingDiscovery
}

type pendingDiscovery struct {
	next *queuedDiscovery
}

type queuedDiscovery struct {
	peer DiscoveredPeer
	own  OwnAddresses
}

// notify hands the discovery to a goroutine and returns at once.
func (d *peerDiscoveries) notify(ctx context.Context, notifier Notifier, peer DiscoveredPeer, own OwnAddresses) {
	d.mu.Lock()
	if d.inflight == nil {
		d.inflight = map[string]*pendingDiscovery{}
	}
	if pending, ok := d.inflight[peer.PeerId]; ok {
		pending.next = &queuedDiscovery{peer: peer, own: own}
		d.mu.Unlock()
		return
	}
	d.inflight[peer.PeerId] = &pendingDiscovery{}
	d.mu.Unlock()
	go d.run(ctx, notifier, queuedDiscovery{peer: peer, own: own})
}

func (d *peerDiscoveries) run(ctx context.Context, notifier Notifier, q queuedDiscovery) {
	for {
		notifier.PeerDiscovered(ctx, q.peer, q.own)
		d.mu.Lock()
		pending := d.inflight[q.peer.PeerId]
		if pending.next == nil || ctx.Err() != nil {
			delete(d.inflight, q.peer.PeerId)
			d.mu.Unlock()
			return
		}
		q, pending.next = *pending.next, nil
		d.mu.Unlock()
	}
}
