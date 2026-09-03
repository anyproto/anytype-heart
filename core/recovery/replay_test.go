package recovery

import (
	"context"
	"errors"
	"testing"

	"time"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-sync/commonspace"
	"github.com/anyproto/any-sync/commonspace/spacesyncproto"
	"github.com/anyproto/any-sync/net"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/space/spacecore/localdiscovery"
)

// clientModel is what a client folds the event log into. The replay property
// — the model after every event equals the tracker's own Snapshot — is what
// makes "push and pull cannot drift" a tested fact rather than a promise.
// Each phase of the implementation extends apply with the payloads it adds;
// an unhandled payload fails the test on purpose.
type clientModel struct {
	runId       string
	lastEventId int64
	mode        pb.EventAccountRecoveryMode
	networkId   string
	phase       pb.EventAccountRecoveryPhase
	done        bool
	err         *pb.EventAccountRecoveryErrorInfo
	discovery   pb.EventAccountRecoveryDiscoveryState
	peers       map[string]*pb.EventAccountRecoverySnapshotPeer
	localPeers  pb.EventAccountRecoveryLocalPeersState

	accountFetchStarted bool
	accountFetchAttempt int32
	accountFetchError   *pb.EventAccountRecoveryErrorInfo
	accountReady        bool
	spaces              map[string]*pb.EventAccountRecoverySnapshotSpace
	viewsConfirmed      bool
}

func (m *clientModel) space(id string) *pb.EventAccountRecoverySnapshotSpace {
	if m.spaces == nil {
		m.spaces = map[string]*pb.EventAccountRecoverySnapshotSpace{}
	}
	s, ok := m.spaces[id]
	if !ok {
		s = &pb.EventAccountRecoverySnapshotSpace{SpaceId: id}
		m.spaces[id] = s
	}
	return s
}

func (m *clientModel) peer(id string, kind pb.EventAccountRecoveryPeerKind, nodeTypes []string) *pb.EventAccountRecoverySnapshotPeer {
	if m.peers == nil {
		m.peers = map[string]*pb.EventAccountRecoverySnapshotPeer{}
	}
	p, ok := m.peers[id]
	if !ok {
		p = &pb.EventAccountRecoverySnapshotPeer{PeerId: id}
		m.peers[id] = p
	}
	p.Kind, p.NodeTypes = kind, nodeTypes
	return p
}

func (m *clientModel) apply(t *testing.T, u *pb.EventAccountRecoveryUpdate) {
	t.Helper()
	if u.RunId != m.runId {
		*m = clientModel{runId: u.RunId}
	}
	require.Equal(t, m.lastEventId+1, u.Id, "ids are contiguous")
	m.lastEventId = u.Id
	switch p := u.Payload.(type) {
	case *pb.EventAccountRecoveryUpdatePayloadOfStarted:
		m.mode = p.Started.Mode
		m.networkId = p.Started.NetworkId
		m.phase = pb.EventAccountRecovery_LookingForPeers
	case *pb.EventAccountRecoveryUpdatePayloadOfPhaseChanged:
		require.Equal(t, m.phase, p.PhaseChanged.FromPhase)
		m.phase = p.PhaseChanged.Phase
		switch m.phase {
		case pb.EventAccountRecovery_Failed:
			m.done = true
			m.err = p.PhaseChanged.Error
		case pb.EventAccountRecovery_Done:
			m.done = true
		}
	case *pb.EventAccountRecoveryUpdatePayloadOfLocalDiscoveryState:
		m.discovery = p.LocalDiscoveryState.State
	case *pb.EventAccountRecoveryUpdatePayloadOfPeerDiscovered:
		e := p.PeerDiscovered
		m.peer(e.PeerId, e.Kind, e.NodeTypes).DiscoveredLocally = true
	case *pb.EventAccountRecoveryUpdatePayloadOfDialStarted:
		e := p.DialStarted
		m.peer(e.PeerId, e.Kind, e.NodeTypes)
	case *pb.EventAccountRecoveryUpdatePayloadOfPeerConnected:
		e := p.PeerConnected
		peer := m.peer(e.PeerId, e.Kind, e.NodeTypes)
		peer.OpenConnections = e.OpenConnections
		peer.Transport, peer.ProtoVersion = e.Transport, e.ProtoVersion
		peer.DialAttempts, peer.LastError = 0, nil
	case *pb.EventAccountRecoveryUpdatePayloadOfDialFailed:
		e := p.DialFailed
		peer := m.peer(e.PeerId, e.Kind, e.NodeTypes)
		peer.DialAttempts, peer.LastError = e.Attempt, e.Error
	case *pb.EventAccountRecoveryUpdatePayloadOfPeerDisconnected:
		e := p.PeerDisconnected
		m.peer(e.PeerId, e.Kind, e.NodeTypes).OpenConnections = e.OpenConnections
	case *pb.EventAccountRecoveryUpdatePayloadOfPeerSpaceExchange:
		e := p.PeerSpaceExchange
		peer, known := m.peers[e.PeerId]
		require.True(t, known, "an exchange answer names a peer the log already announced")
		peer.Exchanged, peer.HasAccountSpace, peer.SharedSpaceCount = e.Exchanged, e.HasAccountSpace, e.SharedSpaceCount
	case *pb.EventAccountRecoveryUpdatePayloadOfLocalPeersStateChanged:
		require.Equal(t, m.localPeers, p.LocalPeersStateChanged.FromState)
		m.localPeers = p.LocalPeersStateChanged.State
	case *pb.EventAccountRecoveryUpdatePayloadOfAccountFetchStarted:
		m.accountFetchStarted = true
		m.accountFetchAttempt = p.AccountFetchStarted.Attempt
	case *pb.EventAccountRecoveryUpdatePayloadOfAccountFetchError:
		m.accountFetchAttempt = p.AccountFetchError.Attempt
		m.accountFetchError = p.AccountFetchError.Error
	case *pb.EventAccountRecoveryUpdatePayloadOfAccountReady:
		m.accountReady = true
		m.accountFetchError = nil
	case *pb.EventAccountRecoveryUpdatePayloadOfSpaceDiscovered:
		e := p.SpaceDiscovered
		_, known := m.spaces[e.SpaceId]
		s := m.space(e.SpaceId)
		s.SpaceViewId, s.Kind = e.SpaceViewId, e.Kind
		if !known {
			s.State = pb.EventAccountRecovery_Queued
		}
	case *pb.EventAccountRecoveryUpdatePayloadOfSpaceStateChanged:
		e := p.SpaceStateChanged
		s := m.space(e.SpaceId)
		require.Equal(t, s.State, e.FromState)
		if e.State == pb.EventAccountRecovery_Removed {
			delete(m.spaces, e.SpaceId)
			if len(m.spaces) == 0 {
				m.spaces = nil
			}
			break
		}
		s.State, s.Error, s.Attempt = e.State, e.Error, e.Attempt
	case *pb.EventAccountRecoveryUpdatePayloadOfFinished:
		m.done = true
		m.viewsConfirmed = p.Finished.ViewsConfirmed
	default:
		t.Fatalf("replay model does not handle %T", u.Payload)
	}
}

func (m *clientModel) assertMatches(t *testing.T, snap *pb.EventAccountRecoverySnapshot) {
	t.Helper()
	want := &clientModel{
		runId:       snap.RunId,
		lastEventId: snap.LastEventId,
		mode:        snap.Mode,
		networkId:   snap.NetworkId,
		phase:       snap.Phase,
		done:        snap.Done,
		err:         snap.Error,
		discovery:   snap.Discovery,
	}
	if len(snap.Peers) > 0 {
		want.peers = map[string]*pb.EventAccountRecoverySnapshotPeer{}
		for _, p := range snap.Peers {
			want.peers[p.PeerId] = p
		}
	}
	want.localPeers = snap.LocalPeers
	want.accountFetchStarted = snap.AccountFetchStarted
	want.accountFetchAttempt = snap.AccountFetchAttempt
	want.accountFetchError = snap.AccountFetchError
	want.accountReady = snap.AccountReady
	want.viewsConfirmed = snap.ViewsConfirmed
	var loaded, failed int32
	if len(snap.Spaces) > 0 {
		want.spaces = map[string]*pb.EventAccountRecoverySnapshotSpace{}
		for _, s := range snap.Spaces {
			want.spaces[s.SpaceId] = s
			switch s.State {
			case pb.EventAccountRecovery_Loaded:
				loaded++
			case pb.EventAccountRecovery_Error:
				failed++
			}
		}
	}
	assert.Equal(t, want, m)
	assert.Equal(t, int32(len(snap.Spaces)), snap.SpacesTotal)
	assert.Equal(t, loaded, snap.SpacesLoaded)
	assert.Equal(t, failed, snap.SpacesFailed)
}

func TestReplayProperty(t *testing.T) {
	t.Run("folding the log reproduces the snapshot at every step", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		model := &clientModel{}
		applied := 0
		check := func() {
			t.Helper()
			for _, u := range fx.sender.updates()[applied:] {
				model.apply(t, u)
				applied++
			}
			model.assertMatches(t, fx.Snapshot())
		}

		// when / then
		fx.init(t)
		check()
		fx.Fail(errors.Join(errors.New("init tech space"), spacesyncproto.ErrSpaceIsDeleted))
		check()
		require.NoError(t, fx.Close(context.Background()))
		check()

		// a new run resets the model through the runId change
		fx.Begin(Run{Mode: pb.EventAccountRecovery_WarmStart, Sender: fx.sender})
		fx.init(t)
		check()
	})

	t.Run("the peer layer replays through coalescing, outage and recovery", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		model := &clientModel{}
		applied := 0
		check := func() {
			t.Helper()
			fx.flush()
			for _, u := range fx.sender.updates()[applied:] {
				model.apply(t, u)
				applied++
			}
			model.assertMatches(t, fx.Snapshot())
		}

		// when / then
		fx.init(t)
		check()
		fx.discovery.set(localdiscovery.DiscoveryNoInterfaces)
		fx.OnLocalPeerDiscovered("lan1", []string{"192.168.1.2:4242"})
		check()
		fx.dialStarted("node1", 2)
		fx.dialFailed("node1", net.ErrUnableToConnect, 900*time.Millisecond)
		fx.dialStarted("node1", 2)
		fx.dialFailed("node1", context.DeadlineExceeded, 5*time.Second)
		check()
		fx.clock.Advance(waitingForNetworkAfter) // the outage timer fires: WaitingForNetwork
		check()
		fx.connected("node1", "quic", false, 300*time.Millisecond)
		fx.connected("node1", "yamux", false, 200*time.Millisecond) // replacement
		fx.closed("node1", false)                                   // the superseded one
		check()
		fx.closed("lan1", true) // never connected: clamps at zero
		fx.connected("lan1", "yamux", true, 0)
		check()
		require.NoError(t, fx.Close(context.Background()))
		check()
	})

	t.Run("the account fetch replays through a bounded wait, a failed round and readiness", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		model := &clientModel{}
		applied := 0
		check := func() {
			t.Helper()
			fx.flush()
			for _, u := range fx.sender.updates()[applied:] {
				model.apply(t, u)
				applied++
			}
			model.assertMatches(t, fx.Snapshot())
		}

		// when / then
		fx.init(t)
		fx.OnTechSpaceId(techSpaceId)
		fx.pull(commonspace.PullEventWaiting, techSpaceId, "", nil) // the 15s bounded try
		fx.dialStarted("node1", 2)
		fx.dialFailed("node1", net.ErrUnableToConnect, time.Second)
		check()
		fx.pull(commonspace.PullEventWaiting, techSpaceId, "", nil) // the unbounded retry
		fx.connected("node1", "quic", false, 200*time.Millisecond)  // FetchingAccount
		fx.pull(commonspace.PullEventAttempt, techSpaceId, "node1", nil)
		fx.pull(commonspace.PullEventResult, techSpaceId, "node1", spacesyncproto.ErrPeerIsNotResponsible)
		check()
		fx.pull(commonspace.PullEventAttempt, techSpaceId, "node2", nil)
		fx.pull(commonspace.PullEventResult, techSpaceId, "node2", nil)
		fx.OnAccountReady()
		check()
	})

	t.Run("a full cold recovery replays end to end", func(t *testing.T) {
		// given
		fx := newFixture(t, pb.EventAccountRecovery_ColdRecovery)
		model := &clientModel{}
		applied := 0
		check := func() {
			t.Helper()
			fx.flush()
			for _, u := range fx.sender.updates()[applied:] {
				model.apply(t, u)
				applied++
			}
			model.assertMatches(t, fx.Snapshot())
		}

		// when / then: peers and the account
		fx.init(t)
		fx.OnTechSpaceId(techSpaceId)
		fx.OnLocalPeerDiscovered("lan1", []string{"192.168.1.9:4242"})
		fx.dialStarted("lan1", 1)
		fx.connected("lan1", "yamux", false, 30*time.Millisecond)
		fx.peers.exchange("lan1", nil, []string{}, false) // a cold device asks with no tokens (GO-7492)
		fx.dialStarted("coord", 2)
		fx.connected("coord", "quic", false, 120*time.Millisecond)
		fx.pull(commonspace.PullEventWaiting, techSpaceId, "", nil)
		fx.dialStarted("node1", 2)
		fx.dialFailed("node1", net.ErrUnableToConnect, time.Second)
		fx.dialStarted("node1", 2)
		fx.connected("node1", "yamux", false, 400*time.Millisecond)
		fx.pull(commonspace.PullEventAttempt, techSpaceId, "node1", nil)
		fx.pull(commonspace.PullEventResult, techSpaceId, "node1", nil)
		fx.OnAccountReady()
		fx.OnSpaceViewsInitial(nil) // a fresh device: the local store holds no view
		check()
		require.Equal(t, pb.EventAccountRecovery_LoadingSpaces, model.phase)

		// spaces: one pulled, one optimistic, one failing, one deleted meanwhile
		fx.OnSpaceView("s1", "v1", false)
		fx.OnSpaceView("s2", "v2", false)
		fx.OnSpaceView("s3", "v3", false)
		fx.OnSpaceLoadStarted("s1", false)
		fx.pull(commonspace.PullEventWaiting, "s1", "", nil)
		fx.pull(commonspace.PullEventAttempt, "s1", "node1", nil)
		fx.pull(commonspace.PullEventResult, "s1", "node1", net.ErrUnableToConnect)
		fx.pull(commonspace.PullEventResult, "s1", "node1", net.ErrUnableToConnect)
		check()
		fx.pull(commonspace.PullEventResult, "s1", "node2", nil)
		fx.OnSpaceLoaded("s1", nil, false)
		fx.OnSpaceLoadStarted("s2", true)
		fx.OnSpaceLoadStarted("s3", false)
		fx.OnSpaceLoaded("s3", anystore.ErrCollectionNotFound, false)
		check()
		require.False(t, model.done)

		// the LAN peer re-exchanges once the account is known locally
		fx.peers.exchange("lan1", []string{}, []string{techSpaceId, "s1"}, false)
		check()
		require.Equal(t, pb.EventAccountRecovery_AccountOnLocalPeer, model.localPeers)

		// the tech space's diff knows one more view than we have
		fx.OnHeadSync(techSpaceId, "node1", []string{"v1", "v2", "v3", "v4"}, true)
		check()
		require.False(t, model.done)
		fx.OnSpaceView("s4", "v4", false)
		fx.OnSpaceLoadStarted("s4", false)
		fx.OnSpaceView("s4", "v4", true) // deleted while loading
		check()
		require.True(t, model.done)
		require.True(t, model.viewsConfirmed)
		require.Equal(t, pb.EventAccountRecovery_Done, model.phase)

		// silence after Done, and Close changes nothing
		fx.OnSpaceView("s5", "v5", false)
		fx.closed("node1", false)
		check()
		require.NoError(t, fx.Close(context.Background()))
		check()
	})
}
