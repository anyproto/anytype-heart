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
		s.publishLocalPeer("dev", []string{tech}, true)

		// then
		assert.ElementsMatch(t, []string{tech, "s1", "s2"}, lists.of("dev"))
		assert.Equal(t, []string{"dev"}, s.peerStore.LocalPeerIds("s2"), "never exchanged about, still a candidate")
	})

	t.Run("an unproven peer stays on its confirmed spaces only", func(t *testing.T) {
		// given
		s, _, lists := newAccountPeersService(t, []string{"s1", "s2"}, nil)

		// when: it shares an unrelated space
		s.publishLocalPeer("stranger", []string{"s1"}, true)

		// then
		assert.Equal(t, []string{"s1"}, lists.of("stranger"))
		assert.Empty(t, s.peerStore.LocalPeerIds("s2"))
	})

	t.Run("a v1 result never proves anything", func(t *testing.T) {
		// given
		s, _, lists := newAccountPeersService(t, []string{"s1"}, nil)
		tech := s.techSpaceExchangeInfo().id

		// when: a plaintext list claims the tech space
		s.publishLocalPeer("legacy", []string{tech}, false)

		// then
		assert.Equal(t, []string{tech}, lists.of("legacy"))
		assert.Empty(t, s.peerStore.LocalPeerIds("s1"))
	})

	t.Run("a space we already hold is decided by the exchange, not by proof", func(t *testing.T) {
		// given: s1 is on disk, s2 is not
		s, _, _ := newAccountPeersService(t, []string{"s1", "s2"}, []string{"s1"})
		tech := s.techSpaceExchangeInfo().id

		// when: the proven peer did not confirm s1
		s.publishLocalPeer("dev", []string{tech}, true)

		// then
		assert.Empty(t, s.peerStore.LocalPeerIds("s1"), "held here, not confirmed there: no head-sync against it")
		assert.Equal(t, []string{"dev"}, s.peerStore.LocalPeerIds("s2"))

		// and a later exchange that confirms s1 keeps it
		s.publishLocalPeer("dev", []string{tech, "s1"}, true)
		assert.Equal(t, []string{"dev"}, s.peerStore.LocalPeerIds("s1"))
	})
}

func TestRefreshProvenPeers(t *testing.T) {
	t.Run("peer proves first, then a space appears and is loaded", func(t *testing.T) {
		// given
		s, views, lists := newAccountPeersService(t, nil, nil)
		tech := s.techSpaceExchangeInfo().id
		s.publishLocalPeer("dev", []string{tech}, true)
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
		s.publishLocalPeer("dev", []string{}, true) // first, cold answer: nothing shared
		require.Empty(t, s.peerStore.LocalPeerIds("s9"))

		// when: the re-exchange proves the account
		s.publishLocalPeer("dev", []string{tech}, true)

		// then
		assert.Equal(t, []string{"dev"}, s.peerStore.LocalPeerIds("s9"))
	})

	t.Run("a removed peer is forgotten and not resurrected", func(t *testing.T) {
		// given
		s, _, _ := newAccountPeersService(t, []string{"s9"}, nil)
		tech := s.techSpaceExchangeInfo().id
		s.publishLocalPeer("dev", []string{tech}, true)
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
