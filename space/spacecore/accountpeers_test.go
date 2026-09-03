package spacecore

import (
	"context"
	"testing"

	"github.com/anyproto/any-sync/commonspace/spacesyncproto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/space/spacecore/peerstore"
	"github.com/anyproto/anytype-heart/space/spacecore/storage/mock_storage"
)

type fakeKnownSpaces struct {
	ids []string
}

func (f *fakeKnownSpaces) AllSpaceIds() []string { return f.ids }

// peerLists records the latest list the peer store published per peer: the
// store exposes peers per space, and the tests need spaces per peer.
type peerLists struct {
	last map[string][]string
}

func (p *peerLists) of(peerId string) []string { return p.last[peerId] }

// newAccountPeersService is a keyed service over a real peer store, with the
// view-based space list and the on-disk list both under test control.
func newAccountPeersService(t *testing.T, known []string, onDisk []string) (*service, *fakeKnownSpaces, *peerLists) {
	t.Helper()
	store := mock_storage.NewMockClientStorage(t)
	store.EXPECT().AllSpaceIds().RunAndReturn(func() ([]string, error) { return onDisk, nil }).Maybe()
	s := newKeyedService(t, 1, store)
	views := &fakeKnownSpaces{ids: known}
	lists := &peerLists{last: map[string][]string{}}
	s.peerStore = peerstore.New()
	s.accountPeers = newAccountPeers()
	s.knownSpaces = views
	s.peerStore.AddObserver(func(peerId string, _, after []string, removed bool) {
		if removed {
			s.forgetLocalPeer(peerId)
			delete(lists.last, peerId)
			return
		}
		lists.last[peerId] = after
	})
	return s, views, lists
}

func TestPublishLocalPeer(t *testing.T) {
	t.Run("a peer that proves the account becomes a candidate for every space we lack", func(t *testing.T) {
		// given: views know s1 and s2, nothing is on disk yet
		s, _, lists := newAccountPeersService(t, []string{"s1", "s2"}, nil)
		tech := s.techSpaceExchangeInfo().id

		// when: the exchange confirmed only the tech space
		s.publishLocalPeer("dev", []string{tech}, true, directionOutbound)

		// then
		assert.ElementsMatch(t, []string{tech, "s1", "s2"}, lists.of("dev"))
		assert.Equal(t, []string{"dev"}, s.peerStore.LocalPeerIds("s2"), "never exchanged about, still a candidate")
	})

	t.Run("an unproven peer stays on its confirmed spaces only", func(t *testing.T) {
		// given
		s, _, lists := newAccountPeersService(t, []string{"s1", "s2"}, nil)

		// when: it shares an unrelated space
		s.publishLocalPeer("stranger", []string{"s1"}, true, directionOutbound)

		// then
		assert.Equal(t, []string{"s1"}, lists.of("stranger"))
		assert.Empty(t, s.peerStore.LocalPeerIds("s2"))
	})

	t.Run("a v1 result never proves anything", func(t *testing.T) {
		// given
		s, _, lists := newAccountPeersService(t, []string{"s1"}, nil)
		tech := s.techSpaceExchangeInfo().id

		// when: a plaintext list claims the tech space
		s.publishLocalPeer("legacy", []string{tech}, false, directionOutbound)

		// then
		assert.Equal(t, []string{tech}, lists.of("legacy"))
		assert.Empty(t, s.peerStore.LocalPeerIds("s1"))
	})

	t.Run("a space we already hold is decided by the exchange, not by proof", func(t *testing.T) {
		// given: s1 is on disk, s2 is not
		s, _, _ := newAccountPeersService(t, []string{"s1", "s2"}, []string{"s1"})
		tech := s.techSpaceExchangeInfo().id

		// when: the proven peer did not confirm s1
		s.publishLocalPeer("dev", []string{tech}, true, directionOutbound)

		// then
		assert.Empty(t, s.peerStore.LocalPeerIds("s1"), "held here, not confirmed there: no head-sync against it")
		assert.Equal(t, []string{"dev"}, s.peerStore.LocalPeerIds("s2"))

		// and a later exchange that confirms s1 keeps it
		s.publishLocalPeer("dev", []string{tech, "s1"}, true, directionOutbound)
		assert.Equal(t, []string{"dev"}, s.peerStore.LocalPeerIds("s1"))
	})
}

func TestRefreshProvenPeers(t *testing.T) {
	t.Run("peer proves first, then a space appears and is loaded", func(t *testing.T) {
		// given
		s, views, lists := newAccountPeersService(t, nil, nil)
		tech := s.techSpaceExchangeInfo().id
		s.publishLocalPeer("dev", []string{tech}, true, directionOutbound)
		require.Empty(t, s.peerStore.LocalPeerIds("s9"))

		// when: loadSpace is about to pull s9 (the view list may not have it yet)
		s.refreshProvenPeers("s9")

		// then
		assert.Equal(t, []string{"dev"}, s.peerStore.LocalPeerIds("s9"))

		// and once s9 is on disk candidacy ends until the exchange confirms it
		views.ids = []string{"s9"}
		s.spaceStorageProvider = func() *mock_storage.MockClientStorage {
			st := mock_storage.NewMockClientStorage(t)
			st.EXPECT().AllSpaceIds().Return([]string{"s9"}, nil).Maybe()
			return st
		}()
		s.refreshProvenPeers()
		assert.Empty(t, s.peerStore.LocalPeerIds("s9"))
		assert.Equal(t, []string{tech}, lists.of("dev"), "the confirmed result is intact")
	})

	t.Run("space appears first, then the peer proves", func(t *testing.T) {
		// given
		s, _, _ := newAccountPeersService(t, []string{"s9"}, nil)
		tech := s.techSpaceExchangeInfo().id
		s.publishLocalPeer("dev", []string{}, true, directionOutbound) // first, cold answer: nothing shared
		require.Empty(t, s.peerStore.LocalPeerIds("s9"))

		// when: the re-exchange proves the account
		s.publishLocalPeer("dev", []string{tech}, true, directionOutbound)

		// then
		assert.Equal(t, []string{"dev"}, s.peerStore.LocalPeerIds("s9"))
	})

	t.Run("a removed peer is forgotten and not resurrected", func(t *testing.T) {
		// given
		s, _, _ := newAccountPeersService(t, []string{"s9"}, nil)
		tech := s.techSpaceExchangeInfo().id
		s.publishLocalPeer("dev", []string{tech}, true, directionOutbound)
		require.Equal(t, []string{"dev"}, s.peerStore.LocalPeerIds("s9"))

		// when: the peer manager drops it after a failed dial
		s.peerStore.RemoveLocalPeer("dev")
		s.refreshProvenPeers("s10")

		// then
		assert.Empty(t, s.peerStore.AllLocalPeers())
		assert.Empty(t, s.peerStore.LocalPeerIds("s10"))
	})

	t.Run("no proven peers is a no-op that never touches disk", func(t *testing.T) {
		s, _, _ := newAccountPeersService(t, []string{"s9"}, nil)
		s.spaceStorageProvider = mock_storage.NewMockClientStorage(t) // AllSpaceIds would fail the test
		s.refreshProvenPeers("s9")
		assert.Empty(t, s.peerStore.AllLocalPeers())
	})
}

func TestColdDevicePath(t *testing.T) {
	t.Run("offline cold device: the peer proves, dials back with nothing, and the tech space stays pullable", func(t *testing.T) {
		// given: empty spacestore, no views yet, no node — the user's run
		s, views, lists := newAccountPeersService(t, nil, nil)
		tech := s.techSpaceExchangeInfo().id

		// when: our outbound exchange proves the peer holds the account
		s.publishLocalPeer("dev", []string{tech}, true, directionOutbound)
		require.Equal(t, []string{"dev"}, s.peerStore.LocalPeerIds(tech))

		// and 122 ms later the peer dials us back; the responder-side answer
		// is bounded by OUR empty disk
		s.publishLocalPeer("dev", []string{}, true, directionInbound)

		// then: the outbound proof is intact and the tech space has a source
		assert.Equal(t, []string{"dev"}, s.peerStore.LocalPeerIds(tech), "the inbound answer must not reduce the outbound one")
		assert.Equal(t, []string{tech}, lists.of("dev"))

		// the pull begins: loadSpace names the tech space, it is still a candidate
		s.refreshProvenPeers(tech)
		assert.Equal(t, []string{"dev"}, s.peerStore.LocalPeerIds(tech))

		// the tech space lands on disk; the peer proved it, so it stays
		disk := []string{tech}
		s.spaceStorageProvider = func() *mock_storage.MockClientStorage {
			st := mock_storage.NewMockClientStorage(t)
			st.EXPECT().AllSpaceIds().RunAndReturn(func() ([]string, error) { return disk, nil }).Maybe()
			return st
		}()
		s.refreshProvenPeers()
		assert.Equal(t, []string{"dev"}, s.peerStore.LocalPeerIds(tech))

		// views arrive from the tech space and s1 starts loading: a candidate
		views.ids = []string{"s1"}
		s.refreshProvenPeers("s1")
		assert.Equal(t, []string{"dev"}, s.peerStore.LocalPeerIds("s1"))
		assert.ElementsMatch(t, []string{tech, "s1"}, lists.of("dev"))

		// s1 lands; candidacy ends until the re-exchange confirms it
		disk = []string{tech, "s1"}
		s.refreshProvenPeers()
		assert.Empty(t, s.peerStore.LocalPeerIds("s1"))
		assert.Equal(t, []string{"dev"}, s.peerStore.LocalPeerIds(tech))

		// the re-exchange asks about disk + tech and the peer confirms both
		s.publishLocalPeer("dev", []string{tech, "s1"}, true, directionOutbound)
		assert.Equal(t, []string{"dev"}, s.peerStore.LocalPeerIds("s1"))
	})

	t.Run("an inbound answer never reduces an outbound one, proven or not", func(t *testing.T) {
		// given
		s, _, lists := newAccountPeersService(t, nil, nil)
		s.publishLocalPeer("friend", []string{"s1"}, true, directionOutbound)

		// when
		s.publishLocalPeer("friend", []string{}, true, directionInbound)
		require.Equal(t, []string{"s1"}, lists.of("friend"))
		s.publishLocalPeer("friend", []string{"s2"}, true, directionInbound)
		require.ElementsMatch(t, []string{"s1", "s2"}, lists.of("friend"))

		// then: a fresh outbound answer replaces only the outbound half
		s.publishLocalPeer("friend", []string{"s2"}, true, directionOutbound)
		assert.Equal(t, []string{"s2"}, lists.of("friend"), "s1 is gone: the peer no longer holds it")
	})

	t.Run("a proven peer's list always carries the tech space", func(t *testing.T) {
		// given
		s, _, lists := newAccountPeersService(t, nil, nil)
		tech := s.techSpaceExchangeInfo().id
		s.publishLocalPeer("dev", []string{tech}, true, directionOutbound)

		// when: a later outbound answer somehow lacks it
		s.publishLocalPeer("dev", []string{"s1"}, true, directionOutbound)

		// then
		assert.ElementsMatch(t, []string{"s1", tech}, lists.of("dev"))
	})

	t.Run("forgetting a peer drops both directions and the proof", func(t *testing.T) {
		// given
		s, _, lists := newAccountPeersService(t, []string{"s9"}, nil)
		tech := s.techSpaceExchangeInfo().id
		s.publishLocalPeer("dev", []string{tech}, true, directionOutbound)
		s.publishLocalPeer("dev", []string{"s2"}, true, directionInbound)

		// when
		s.peerStore.RemoveLocalPeer("dev")
		s.refreshProvenPeers("s9")

		// then
		assert.Nil(t, lists.of("dev"))
		assert.Empty(t, s.peerStore.AllLocalPeers())
	})
}

func TestSpacePullAnswersMissingFromDisk(t *testing.T) {
	// given: the space is not on disk; Get would load (and pull on the
	// caller's behalf), so the handler must not reach it
	store := mock_storage.NewMockClientStorage(t)
	store.EXPECT().SpaceExists("absent").Return(false)
	handler := &rpcHandler{s: &service{spaceStorageProvider: store}}

	// when
	resp, err := handler.SpacePull(context.Background(), &spacesyncproto.SpacePullRequest{Id: "absent"})

	// then
	assert.Nil(t, resp)
	assert.ErrorIs(t, err, spacesyncproto.ErrSpaceMissing)
}
