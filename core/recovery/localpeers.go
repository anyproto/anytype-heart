package recovery

import (
	"slices"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/space/spacecore/peerstore"
)

// peerStoreObservable is the part of peerstore.PeerStore the tracker needs.
// Observers are invoked with the store's lock released, after the space
// exchange with a LAN peer (spacecore.PeerDiscovered outbound,
// rpchandler.LocalPeerDiscovered inbound) — the same seam core/peerstatus
// consumes.
type peerStoreObservable interface {
	AddObserver(observer peerstore.Observer)
}

// onLocalPeerSpaces folds a space-exchange answer. It reports the FACT the
// exchange returned (does the peer hold our tech space, how many spaces are
// shared) rather than a verdict: until GO-7492 a cold device asks with zero
// tokens and every LAN peer answers "nothing shared", so a field named
// "account not found" would be reliably wrong exactly where it matters. The
// fold's aggregate (refreshLocalPeersLocked) is what a client renders.
func (t *Tracker) onLocalPeerSpaces(peerId string, _ []string, spaceIdsAfter []string, peerRemoved bool) {
	defer containTelemetry("local peer spaces")
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.begun || t.terminalLocked() || peerId == "" {
		return
	}
	p := t.peerLocked(peerId)
	if peerRemoved {
		p.exchanged, p.hasAccountSpace, p.sharedSpaceCount = false, false, 0
	} else {
		p.exchanged = true
		p.hasAccountSpace = t.account.techSpaceId != "" && slices.Contains(spaceIdsAfter, t.account.techSpaceId)
		p.sharedSpaceCount = len(spaceIdsAfter)
	}
	t.markLocked(&pb.EventAccountRecoveryUpdatePayloadOfPeerSpaceExchange{PeerSpaceExchange: &pb.EventAccountRecoveryPeerSpaceExchange{
		PeerId:           peerId,
		Exchanged:        p.exchanged,
		HasAccountSpace:  p.hasAccountSpace,
		SharedSpaceCount: int32(p.sharedSpaceCount),
	}}, nil)
	t.refreshLocalPeersLocked()
}

// localPeersStateLocked derives the LAN headline over every LocalPeer. The
// aggregate rule: the negative verdict may surface only once every connected
// LAN peer has answered negatively (a connected peer still awaiting its
// exchange blocks it), and one positive answer wins at once, whatever the
// others say — several LAN peers dial concurrently, and the first to hold
// the account is the one that matters.
func (t *Tracker) localPeersStateLocked() pb.EventAccountRecoveryLocalPeersState {
	var withAccount, answeredNo, connecting, unreachable int
	for _, p := range t.net.peers {
		if p.kind != pb.EventAccountRecovery_LocalPeer {
			continue
		}
		switch {
		case p.exchanged && p.hasAccountSpace:
			withAccount++
		case p.exchanged:
			answeredNo++
		case p.open > 0:
			connecting++ // connected, the exchange has not answered yet
		case p.dialAttempts > 0:
			unreachable++
		default:
			connecting++ // discovered or dialing
		}
	}
	switch {
	case withAccount > 0:
		return pb.EventAccountRecovery_AccountOnLocalPeer
	case connecting > 0:
		return pb.EventAccountRecovery_LocalPeersConnecting
	case answeredNo > 0:
		return pb.EventAccountRecovery_AccountNotOnLocalPeers
	case unreachable > 0:
		return pb.EventAccountRecovery_LocalPeersUnreachable
	default:
		return pb.EventAccountRecovery_NoLocalPeers
	}
}

// refreshLocalPeersLocked publishes a LocalPeersStateChanged when the derived
// aggregate moved. Called after every peer event and every exchange answer.
func (t *Tracker) refreshLocalPeersLocked() {
	next := t.localPeersStateLocked()
	if next == t.net.localPeers {
		return
	}
	from := t.net.localPeers
	t.net.localPeers = next
	t.markLocked(&pb.EventAccountRecoveryUpdatePayloadOfLocalPeersStateChanged{LocalPeersStateChanged: &pb.EventAccountRecoveryLocalPeersStateChanged{
		State:     next,
		FromState: from,
	}}, nil)
}
