package localdiscovery

import (
	"context"
	"slices"
	"sync"
)

// peerDiscoveries runs PeerDiscovered for one peer at a time: run
// concurrently, an older call could replace newer addresses, or share the
// newer call's dial of the older ones. A discovery that arrives while one
// runs is parked. Discoveries from different sources (the base list, each
// cable, see DiscoveredPeer.Link) are all kept and run in arrival order; a
// newer one from the same source replaces the parked one in place (as the
// app bridge's discoveryObserver keeps only the latest). Without a cable
// selected (direct), a discovery of a peer with nothing queued runs at once,
// as on develop.
type peerDiscoveries struct {
	mu       sync.Mutex
	inflight map[string]*pendingDiscovery
}

type pendingDiscovery struct {
	next []queuedDiscovery
}

type queuedDiscovery struct {
	peer DiscoveredPeer
	own  OwnAddresses
}

// notify hands the discovery to a goroutine and returns at once. The choice
// is made under d.mu: a peer with work queued from while a cable was
// selected keeps queueing after the cable is gone, so a parked older
// discovery can never run after a newer one dispatched directly.
func (d *peerDiscoveries) notify(ctx context.Context, notifier Notifier, peer DiscoveredPeer, own OwnAddresses, direct bool) {
	d.mu.Lock()
	if d.inflight == nil {
		d.inflight = map[string]*pendingDiscovery{}
	}
	if pending, ok := d.inflight[peer.PeerId]; ok {
		q := queuedDiscovery{peer: peer, own: own}
		if i := slices.IndexFunc(pending.next, func(p queuedDiscovery) bool { return p.peer.Link == peer.Link }); i >= 0 {
			pending.next[i] = q
		} else {
			pending.next = append(pending.next, q)
		}
		d.mu.Unlock()
		return
	}
	if direct {
		d.mu.Unlock()
		go notifier.PeerDiscovered(ctx, peer, own)
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
		if len(pending.next) == 0 || ctx.Err() != nil {
			delete(d.inflight, q.peer.PeerId)
			d.mu.Unlock()
			return
		}
		q = pending.next[0]
		pending.next = pending.next[1:]
		d.mu.Unlock()
	}
}
