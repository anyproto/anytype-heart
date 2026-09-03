package recovery

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/anyproto/any-sync/net"
	"github.com/anyproto/any-sync/net/peerobserver"
	"github.com/anyproto/any-sync/net/secureservice/handshake"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/device"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/space/spacecore/localdiscovery"
)

func TestDeviceNetworkStateCNameDrift(t *testing.T) {
	assert.Equal(t, device.CName, deviceNetworkStateCName)
}

func TestTracker_PeerFolding(t *testing.T) {
	t.Run("kind and node types come from nodeconf", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		fx.init(t)

		// when
		fx.dialStarted("coord", 3)
		fx.dialStarted("lan1", 1)

		// then
		ups := fx.sender.updates()
		require.Len(t, ups, 5) // Started, DialStarted, PhaseChanged(Connecting), DialStarted, LocalPeersStateChanged
		want := &pb.EventAccountRecoveryDialStarted{
			PeerId: "coord", Kind: pb.EventAccountRecovery_NetworkNode, NodeTypes: []string{"coordinator"}, AddrsCount: 3,
		}
		assert.Equal(t, want, ups[1].Payload.(*pb.EventAccountRecoveryUpdatePayloadOfDialStarted).DialStarted)
		assert.Equal(t, pb.EventAccountRecovery_LocalPeer, fx.peer(t, "lan1").Kind)
		assert.Nil(t, fx.peer(t, "lan1").NodeTypes)
	})

	t.Run("dialing moves the phase to Connecting", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		fx.init(t)

		// when
		fx.dialStarted("node1", 2)

		// then
		changes := fx.phaseChanges()
		require.Len(t, changes, 1)
		assert.Equal(t, pb.EventAccountRecovery_Connecting, changes[0].Phase)
		assert.Equal(t, pb.EventAccountRecovery_LookingForPeers, changes[0].FromPhase)
	})

	t.Run("connections are counted per peer and clamped at zero", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_WarmStart)
		fx.init(t)
		fx.connected("node1", "quic", false, time.Second)
		fx.connected("node1", "yamux", false, time.Second) // replacement, before the old one closes

		// when
		fx.closed("node1", false) // the superseded connection
		fx.flush()

		// then
		assert.Equal(t, int32(1), fx.peer(t, "node1").OpenConnections)
		assert.Equal(t, "yamux", fx.peer(t, "node1").Transport)

		// and below zero clamps
		fx.closed("node1", false)
		fx.closed("node1", false)
		fx.closed("node1", false)
		fx.flush()
		assert.Equal(t, int32(0), fx.peer(t, "node1").OpenConnections)
		last := fx.lastUpdate(t).Payload.(*pb.EventAccountRecoveryUpdatePayloadOfPeerDisconnected).PeerDisconnected
		assert.Equal(t, int32(0), last.OpenConnections)
	})

	t.Run("dial attempts count since the last connection", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_WarmStart)
		fx.init(t)
		fx.clock.Advance(time.Second)

		// when
		for i := 0; i < 3; i++ {
			fx.dialFailed("node1", net.ErrUnableToConnect, time.Second)
		}

		// then: the first failure is immediate, the rest collapse to a level
		require.Len(t, fx.sender.updates(), 2) // Started, DialFailed{1}
		fx.flush()
		last := fx.lastUpdate(t).Payload.(*pb.EventAccountRecoveryUpdatePayloadOfDialFailed).DialFailed
		assert.Equal(t, int32(3), last.Attempt)
		assert.Equal(t, pb.EventAccountRecovery_PeerUnreachable, last.Error.Class)
		assert.True(t, last.Error.Retryable)
		assert.Equal(t, int32(3), fx.peer(t, "node1").DialAttempts)

		// and a connection resets them
		fx.connected("node1", "quic", false, time.Second)
		assert.Equal(t, int32(0), fx.peer(t, "node1").DialAttempts)
		assert.Nil(t, fx.peer(t, "node1").LastError)
	})

	t.Run("inbound connections carry no duration", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_WarmStart)
		fx.init(t)

		// when
		fx.connected("lan1", "yamux", true, 5*time.Second)

		// then
		ups := fx.sender.updates()
		require.Len(t, ups, 3) // Started, PeerConnected, LocalPeersStateChanged
		got := ups[1].Payload.(*pb.EventAccountRecoveryUpdatePayloadOfPeerConnected).PeerConnected
		want := &pb.EventAccountRecoveryPeerConnected{
			PeerId: "lan1", Kind: pb.EventAccountRecovery_LocalPeer, Direction: pb.EventAccountRecovery_Inbound,
			Addr: "addr:lan1", Transport: "yamux", ProtoVersion: 7, DurationMs: 0, OpenConnections: 1,
		}
		assert.Equal(t, want, got)
	})

	t.Run("the first incompatible-version failure latches the class", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		fx.init(t)
		fx.dialStarted("node1", 2)
		fx.dialFailed("node1", errors.Join(handshake.ErrIncompatibleVersion), time.Second)
		fx.dialFailed("coord", net.ErrUnableToConnect, time.Second) // a later, plainer failure

		// when
		fx.clock.Advance(waitingForNetworkAfter)

		// then
		snap := fx.Snapshot()
		assert.Equal(t, pb.EventAccountRecovery_WaitingForNetwork, snap.Phase)
		changes := fx.phaseChanges()
		last := changes[len(changes)-1]
		assert.Equal(t, pb.EventAccountRecovery_IncompatibleVersion, last.Error.Class)
		assert.False(t, last.Error.Retryable)
	})

	t.Run("a cancelled dial is not an outage signal", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		fx.init(t)
		fx.dialStarted("node1", 2)

		// when
		fx.dialFailed("node1", context.Canceled, time.Second)
		fx.clock.Advance(2 * waitingForNetworkAfter)

		// then
		assert.Equal(t, int32(1), fx.peer(t, "node1").DialAttempts)
		assert.Nil(t, fx.peer(t, "node1").LastError)
		assert.Equal(t, pb.EventAccountRecovery_Connecting, fx.Snapshot().Phase)
	})

	t.Run("unknown kinds and events before begin are ignored", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		fx.init(t)
		fresh := newTracker(fx.clock, coalesceWindow)

		// when
		fx.ObservePeerEvent(peerobserver.Event{Kind: peerobserver.Kind(99), PeerId: "x"})
		fresh.ObservePeerEvent(peerobserver.Event{Kind: peerobserver.KindDialStarted, PeerId: "x"})

		// then
		assert.Len(t, fx.sender.updates(), 1)
		assert.Empty(t, fx.Snapshot().Peers)
		assert.Nil(t, fresh.Snapshot())
	})

	t.Run("hooks are registered at init", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)

		// when
		fx.init(t)

		// then
		require.Len(t, fx.mux.observers, 1)
		assert.Same(t, fx.Tracker, fx.mux.observers[0])
		assert.Len(t, fx.discovery.hooks, 1)
		assert.Len(t, fx.network.hooks, 1)
	})
}

func TestTracker_WaitingForNetwork(t *testing.T) {
	t.Run("an idle close alone never enters", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_WarmStart)
		fx.init(t)
		fx.dialStarted("node1", 2)
		fx.connected("node1", "quic", false, time.Second)

		// when: the pool's idle TTL closes it, no reason attached
		fx.closed("node1", false)
		fx.clock.Advance(3 * waitingForNetworkAfter)

		// then
		assert.Equal(t, pb.EventAccountRecovery_Connecting, fx.Snapshot().Phase)
		assert.Equal(t, 0, fx.clock.pendingTimers())
	})

	t.Run("enters 10s after a failed dial with nothing open, not before", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		fx.init(t)
		fx.dialStarted("node1", 2)
		fx.dialFailed("node1", net.ErrUnableToConnect, time.Second)

		// when
		fx.clock.Advance(waitingForNetworkAfter - time.Millisecond)
		before := fx.Snapshot().Phase
		fx.clock.Advance(time.Millisecond)

		// then
		assert.Equal(t, pb.EventAccountRecovery_Connecting, before)
		assert.Equal(t, pb.EventAccountRecovery_WaitingForNetwork, fx.Snapshot().Phase)
		changes := fx.phaseChanges()
		require.Len(t, changes, 2)
		want := &pb.EventAccountRecoveryPhaseChanged{
			Phase:                   pb.EventAccountRecovery_WaitingForNetwork,
			FromPhase:               pb.EventAccountRecovery_Connecting,
			PreviousPhaseDurationMs: waitingForNetworkAfter.Milliseconds(),
			Error: &pb.EventAccountRecoveryErrorInfo{
				Class: pb.EventAccountRecovery_PeerUnreachable, Retryable: true, DebugMessage: "no peer reachable",
			},
		}
		assert.Equal(t, want, changes[1])
	})

	t.Run("a connection within the window cancels the outage", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		fx.init(t)
		fx.dialStarted("node1", 2)
		fx.dialFailed("node1", net.ErrUnableToConnect, time.Second)
		fx.clock.Advance(waitingForNetworkAfter / 2)

		// when
		fx.connected("node1", "quic", false, time.Second)
		fx.clock.Advance(2 * waitingForNetworkAfter)

		// then
		assert.Equal(t, pb.EventAccountRecovery_Connecting, fx.Snapshot().Phase)
		assert.Len(t, fx.phaseChanges(), 1)
		assert.Equal(t, 0, fx.clock.pendingTimers())
	})

	t.Run("a connection lifts the overlay back to the base phase", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		fx.init(t)
		fx.dialStarted("node1", 2)
		fx.dialFailed("node1", net.ErrUnableToConnect, time.Second)
		fx.clock.Advance(waitingForNetworkAfter)
		require.Equal(t, pb.EventAccountRecovery_WaitingForNetwork, fx.Snapshot().Phase)

		// when
		fx.connected("node1", "quic", false, time.Second)

		// then
		changes := fx.phaseChanges()
		last := changes[len(changes)-1]
		assert.Equal(t, pb.EventAccountRecovery_Connecting, last.Phase)
		assert.Equal(t, pb.EventAccountRecovery_WaitingForNetwork, last.FromPhase)
		assert.Nil(t, last.Error)
	})

	t.Run("device offline at init enters immediately with NoNetwork", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_WarmStart)
		fx.network.offline = true

		// when
		fx.init(t)

		// then
		changes := fx.phaseChanges()
		require.Len(t, changes, 1)
		assert.Equal(t, pb.EventAccountRecovery_WaitingForNetwork, changes[0].Phase)
		assert.Equal(t, pb.EventAccountRecovery_LookingForPeers, changes[0].FromPhase)
		assert.Equal(t, pb.EventAccountRecovery_NoNetwork, changes[0].Error.Class)
		assert.True(t, changes[0].Error.Retryable)
		assert.Equal(t, int64(2), fx.lastUpdate(t).Id, "Started, then the overlay")
	})

	t.Run("the connectivity hook enters at once; online alone does not lift", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_WarmStart)
		fx.init(t)
		fx.dialStarted("node1", 2)

		// when
		fx.network.set(false)
		require.Equal(t, pb.EventAccountRecovery_WaitingForNetwork, fx.Snapshot().Phase)
		fx.network.set(true)

		// then
		assert.Equal(t, pb.EventAccountRecovery_WaitingForNetwork, fx.Snapshot().Phase)
		fx.connected("node1", "quic", false, time.Second)
		assert.Equal(t, pb.EventAccountRecovery_Connecting, fx.Snapshot().Phase)
	})

	t.Run("no interfaces makes the outage a NoNetwork", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		fx.init(t)
		fx.discovery.set(localdiscovery.DiscoveryNoInterfaces)
		fx.dialStarted("node1", 0)
		fx.dialFailed("node1", errors.New("addrs for peer not found"), 0)

		// when
		fx.clock.Advance(waitingForNetworkAfter)

		// then
		changes := fx.phaseChanges()
		last := changes[len(changes)-1]
		assert.Equal(t, pb.EventAccountRecovery_WaitingForNetwork, last.Phase)
		assert.Equal(t, pb.EventAccountRecovery_NoNetwork, last.Error.Class)
	})

	t.Run("a terminal phase wins over the overlay", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		fx.init(t)
		fx.dialStarted("node1", 2)
		fx.dialFailed("node1", net.ErrUnableToConnect, time.Second)
		fx.Fail(errors.New("boom"))

		// when
		fx.clock.Advance(waitingForNetworkAfter)

		// then
		assert.Equal(t, pb.EventAccountRecovery_Failed, fx.Snapshot().Phase)
	})
}

func TestTracker_LocalDiscovery(t *testing.T) {
	t.Run("possibility changes publish once per state", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_WarmStart)
		fx.init(t)

		// when
		fx.discovery.set(localdiscovery.DiscoveryNoInterfaces)
		fx.discovery.set(localdiscovery.DiscoveryNoInterfaces)
		fx.discovery.set(localdiscovery.DiscoveryLocalNetworkRestricted)
		fx.discovery.set(localdiscovery.DiscoveryPossible)

		// then
		ups := fx.sender.updates()
		require.Len(t, ups, 4)
		var got []pb.EventAccountRecoveryDiscoveryState
		for _, u := range ups[1:] {
			got = append(got, u.Payload.(*pb.EventAccountRecoveryUpdatePayloadOfLocalDiscoveryState).LocalDiscoveryState.State)
		}
		want := []pb.EventAccountRecoveryDiscoveryState{
			pb.EventAccountRecovery_NoInterfaces, pb.EventAccountRecovery_Restricted, pb.EventAccountRecovery_Possible,
		}
		assert.Equal(t, want, got)
		assert.Equal(t, pb.EventAccountRecovery_Possible, fx.Snapshot().Discovery)
	})

	t.Run("a LAN peer is announced once", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_WarmStart)
		fx.init(t)

		// when
		fx.OnLocalPeerDiscovered("lan1", []string{"192.168.1.2:4242"})
		fx.OnLocalPeerDiscovered("lan1", []string{"192.168.1.2:4242"})

		// then
		ups := fx.sender.updates()
		require.Len(t, ups, 3) // Started, PeerDiscovered, LocalPeersStateChanged
		want := &pb.EventAccountRecoveryPeerDiscovered{
			PeerId: "lan1", Addrs: []string{"192.168.1.2:4242"}, Kind: pb.EventAccountRecovery_LocalPeer,
		}
		assert.Equal(t, want, ups[1].Payload.(*pb.EventAccountRecoveryUpdatePayloadOfPeerDiscovered).PeerDiscovered)
		assert.True(t, fx.peer(t, "lan1").DiscoveredLocally)
	})
}
