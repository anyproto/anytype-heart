package recovery

import (
	"testing"
	"time"

	"github.com/anyproto/any-sync/net"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/space/spacecore/peerstore"
)

func (fx *fixture) localPeersChanges() []*pb.EventAccountRecoveryLocalPeersStateChanged {
	var out []*pb.EventAccountRecoveryLocalPeersStateChanged
	for _, u := range fx.sender.updates() {
		if p, ok := u.Payload.(*pb.EventAccountRecoveryUpdatePayloadOfLocalPeersStateChanged); ok {
			out = append(out, p.LocalPeersStateChanged)
		}
	}
	return out
}

func (fx *fixture) localPeersTrail() []pb.EventAccountRecoveryLocalPeersState {
	var out []pb.EventAccountRecoveryLocalPeersState
	for _, c := range fx.localPeersChanges() {
		out = append(out, c.State)
	}
	return out
}

func TestTracker_PeerSpaceExchange(t *testing.T) {
	t.Run("the exchange answer is reported as a fact and folded into the peer", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		fx.init(t)
		fx.OnTechSpaceId(techSpaceId)
		fx.connected("lan1", "yamux", false, time.Millisecond)

		// when
		fx.peers.exchange("lan1", nil, []string{techSpaceId, "s1"}, false)

		// then
		var got *pb.EventAccountRecoveryPeerSpaceExchange
		for _, u := range fx.sender.updates() {
			if p, ok := u.Payload.(*pb.EventAccountRecoveryUpdatePayloadOfPeerSpaceExchange); ok {
				got = p.PeerSpaceExchange
			}
		}
		want := &pb.EventAccountRecoveryPeerSpaceExchange{PeerId: "lan1", Exchanged: true, HasAccountSpace: true, SharedSpaceCount: 2}
		assert.Equal(t, want, got)
		peer := fx.peer(t, "lan1")
		assert.True(t, peer.Exchanged)
		assert.True(t, peer.HasAccountSpace)
		assert.Equal(t, int32(2), peer.SharedSpaceCount)
	})

	t.Run("a removed peer loses its answer", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		fx.init(t)
		fx.OnTechSpaceId(techSpaceId)
		fx.peers.exchange("lan1", nil, []string{techSpaceId}, false)

		// when
		fx.peers.exchange("lan1", []string{techSpaceId}, nil, true)

		// then
		peer := fx.peer(t, "lan1")
		assert.False(t, peer.Exchanged)
		assert.False(t, peer.HasAccountSpace)
		assert.Equal(t, int32(0), peer.SharedSpaceCount)
	})

	t.Run("without a known tech space id nothing counts as the account", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		fx.init(t)

		// when
		fx.peers.exchange("lan1", nil, []string{techSpaceId}, false)

		// then
		peer := fx.peer(t, "lan1")
		assert.True(t, peer.Exchanged)
		assert.False(t, peer.HasAccountSpace)
		assert.Equal(t, int32(1), peer.SharedSpaceCount)
	})

	t.Run("the observer is registered at init and a real store notifies an empty first registration", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		fx.init(t)
		require.Len(t, fx.peers.observers, 1)
		store := peerstore.New()
		store.AddObserver(fx.peers.observers[0])
		fx.OnTechSpaceId(techSpaceId)
		fx.connected("lan1", "yamux", false, time.Millisecond)

		// when: a cold device's exchange comes back with nothing shared
		store.UpdateLocalPeer("lan1", []string{})

		// then
		peer := fx.peer(t, "lan1")
		assert.True(t, peer.Exchanged, "an empty first registration is still an answer")
		assert.False(t, peer.HasAccountSpace)
		assert.Equal(t, pb.EventAccountRecovery_AccountNotOnLocalPeers, fx.Snapshot().LocalPeers)

		// and an identical re-registration is silent, a changed one flips it
		before := len(fx.sender.updates())
		store.UpdateLocalPeer("lan1", []string{})
		assert.Len(t, fx.sender.updates(), before)
		store.UpdateLocalPeer("lan1", []string{techSpaceId})
		assert.Equal(t, pb.EventAccountRecovery_AccountOnLocalPeer, fx.Snapshot().LocalPeers)
	})
}

func TestTracker_LocalPeersState(t *testing.T) {
	t.Run("found, connecting, unreachable", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		fx.init(t)
		require.Equal(t, pb.EventAccountRecovery_NoLocalPeers, fx.Snapshot().LocalPeers)

		// when
		fx.OnLocalPeerDiscovered("lan1", []string{"192.168.1.2:4242"})
		fx.dialStarted("lan1", 1)
		fx.dialFailed("lan1", net.ErrUnableToConnect, time.Second)

		// then
		want := []pb.EventAccountRecoveryLocalPeersState{
			pb.EventAccountRecovery_LocalPeersConnecting, pb.EventAccountRecovery_LocalPeersUnreachable,
		}
		assert.Equal(t, want, fx.localPeersTrail())
		changes := fx.localPeersChanges()
		assert.Equal(t, pb.EventAccountRecovery_NoLocalPeers, changes[0].FromState)
		assert.Equal(t, pb.EventAccountRecovery_LocalPeersConnecting, changes[1].FromState)
	})

	t.Run("the negative verdict waits for every connected peer to answer", func(t *testing.T) {
		// given: two LAN peers dial concurrently
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		fx.init(t)
		fx.OnTechSpaceId(techSpaceId)
		fx.OnLocalPeerDiscovered("lan1", nil)
		fx.OnLocalPeerDiscovered("lan2", nil)
		fx.connected("lan1", "yamux", false, time.Millisecond)
		fx.connected("lan2", "yamux", false, time.Millisecond)

		// when
		fx.peers.exchange("lan1", nil, []string{}, false)
		require.Equal(t, pb.EventAccountRecovery_LocalPeersConnecting, fx.Snapshot().LocalPeers, "lan2 has not answered")
		fx.peers.exchange("lan2", nil, []string{"other.space"}, false)

		// then
		assert.Equal(t, pb.EventAccountRecovery_AccountNotOnLocalPeers, fx.Snapshot().LocalPeers)
	})

	t.Run("one positive answer wins at once, whatever the others say", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		fx.init(t)
		fx.OnTechSpaceId(techSpaceId)
		fx.connected("lan1", "yamux", false, time.Millisecond)
		fx.connected("lan2", "yamux", false, time.Millisecond)
		fx.connected("lan3", "yamux", false, time.Millisecond) // never answers
		fx.peers.exchange("lan1", nil, []string{}, false)

		// when
		fx.peers.exchange("lan2", nil, []string{techSpaceId}, false)

		// then
		assert.Equal(t, pb.EventAccountRecovery_AccountOnLocalPeer, fx.Snapshot().LocalPeers)
	})

	t.Run("a negative verdict flips to positive on the re-exchange", func(t *testing.T) {
		// given: the cold-device answer (GO-7492), then the account arrives
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		fx.init(t)
		fx.OnTechSpaceId(techSpaceId)
		fx.connected("lan1", "yamux", false, time.Millisecond)
		fx.peers.exchange("lan1", nil, []string{}, false)
		require.Equal(t, pb.EventAccountRecovery_AccountNotOnLocalPeers, fx.Snapshot().LocalPeers)

		// when
		fx.peers.exchange("lan1", []string{}, []string{techSpaceId, "s1"}, false)

		// then
		assert.Equal(t, pb.EventAccountRecovery_AccountOnLocalPeer, fx.Snapshot().LocalPeers)
		last := fx.localPeersChanges()[len(fx.localPeersChanges())-1]
		assert.Equal(t, pb.EventAccountRecovery_AccountNotOnLocalPeers, last.FromState)
	})

	t.Run("a removed peer no longer counts; a still-connected negative one does", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		fx.init(t)
		fx.OnTechSpaceId(techSpaceId)
		fx.connected("lan1", "yamux", false, time.Millisecond)
		fx.connected("lan2", "yamux", false, time.Millisecond)
		fx.peers.exchange("lan1", nil, []string{techSpaceId}, false)
		fx.peers.exchange("lan2", nil, []string{}, false)
		require.Equal(t, pb.EventAccountRecovery_AccountOnLocalPeer, fx.Snapshot().LocalPeers)

		// when: lan1's dial fails later and the peer manager drops it
		fx.closed("lan1", false)
		fx.dialFailed("lan1", net.ErrUnableToConnect, time.Second)
		fx.peers.exchange("lan1", []string{techSpaceId}, nil, true)

		// then
		assert.Equal(t, pb.EventAccountRecovery_AccountNotOnLocalPeers, fx.Snapshot().LocalPeers)
	})

	t.Run("network nodes never take part", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		fx.init(t)

		// when
		fx.dialStarted("node1", 2)
		fx.dialFailed("node1", net.ErrUnableToConnect, time.Second)
		fx.connected("coord", "quic", false, time.Millisecond)

		// then
		assert.Equal(t, pb.EventAccountRecovery_NoLocalPeers, fx.Snapshot().LocalPeers)
		assert.Empty(t, fx.localPeersChanges())
	})
}
