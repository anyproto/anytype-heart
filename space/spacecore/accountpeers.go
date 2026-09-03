package spacecore

import (
	"slices"
	"sync"

	"github.com/anyproto/any-sync/app"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
)

// GO-7492 part 3: a LAN peer whose v2 exchange proves it holds our tech space
// is one of this account's own devices — the tech token is HKDF over the tech
// space's first ACL read key, which only the account keys yield. Such a peer
// may be asked for ANY space: SpacePull serves by id and authorization is
// inherent in the encrypted payload, so the discovery-key exchange is about
// which peers are worth asking, not about permission. A cold device therefore
// registers every proven account peer as a pull candidate for each space it
// knows of but does not hold yet; once a space is on disk the exchange
// decides again, so head-sync never runs against a peer for a space it was
// never confirmed to have.

// SpaceServiceCName mirrors space.CName: space imports this package, so the
// view-based space list is looked up by name. A drift test in space keeps the
// two equal.
const SpaceServiceCName = "client.space"

// knownSpaceIdsProvider is the part of the space service this needs: the
// space ids known from SpaceViews — NOT the storage list, which on a cold
// device is exactly the empty one.
type knownSpaceIdsProvider interface {
	AllSpaceIds() []string
}

// exchangeDirection says which side asked. The two answers mean different
// things and neither may replace the other: an OUTBOUND answer is what the
// peer holds of the list we asked about (disk plus the derived tech space); an
// INBOUND answer is what WE hold of the list the peer asked about — bounded by
// our own disk, so on a cold device it is empty even when the peer holds the
// whole account. The published list is the union of the latest answer per
// direction: a fresh outbound answer can still drop a space the peer no
// longer holds, which a grow-only merge could not.
type exchangeDirection int

const (
	directionOutbound exchangeDirection = iota
	directionInbound
)

// accountPeers is what the exchange established per LAN peer, kept apart from
// the peer store's published list so candidacy can be recomputed without
// clobbering genuine exchange results.
type accountPeers struct {
	mu       sync.Mutex
	proven   map[string]struct{}
	outbound map[string][]string
	inbound  map[string][]string
}

func newAccountPeers() *accountPeers {
	return &accountPeers{
		proven:   map[string]struct{}{},
		outbound: map[string][]string{},
		inbound:  map[string][]string{},
	}
}

// publishLocalPeer records an exchange result for peerId and publishes its
// space list: the union of what either direction confirmed, plus — for a
// peer that proved the account through a v2 token exchange (v1 is plaintext
// and proves nothing) — the tech space itself, always, and every pending
// space. UpdateLocalPeer replaces the list, so the full set is computed here
// every time.
func (s *service) publishLocalPeer(peerId string, shared []string, proof bool, direction exchangeDirection) {
	tech := s.techSpaceExchangeInfo()
	s.accountPeers.mu.Lock()
	switch direction {
	case directionInbound:
		s.accountPeers.inbound[peerId] = slices.Clone(shared)
	default:
		s.accountPeers.outbound[peerId] = slices.Clone(shared)
	}
	if proof && tech.id != "" && slices.Contains(shared, tech.id) {
		if _, known := s.accountPeers.proven[peerId]; !known {
			log.Info("local peer proved the account", zap.String("peerId", peerId))
		}
		s.accountPeers.proven[peerId] = struct{}{}
	}
	_, proven := s.accountPeers.proven[peerId]
	confirmed := union(s.accountPeers.outbound[peerId], s.accountPeers.inbound[peerId])
	s.accountPeers.mu.Unlock()
	if !proven {
		s.peerStore.UpdateLocalPeer(peerId, confirmed)
		return
	}
	s.peerStore.UpdateLocalPeer(peerId, s.provenListLocked(confirmed, tech.id, s.pendingSpaceIds()))
}

// provenListLocked is a proven peer's published list: what was confirmed, the
// tech space it proved — independent of the latest answer, since on a cold
// device nothing else can supply it — and every space we know of but lack.
func (s *service) provenListLocked(confirmed []string, techId string, pending []string) []string {
	list := union(confirmed, pending)
	if techId != "" && !slices.Contains(list, techId) {
		list = append(list, techId)
	}
	return list
}

// refreshProvenPeers republishes every proven peer's list. loading names a
// space about to be pulled that the space service may not list yet.
func (s *service) refreshProvenPeers(loading ...string) {
	tech := s.techSpaceExchangeInfo()
	s.accountPeers.mu.Lock()
	type entry struct {
		peerId    string
		confirmed []string
	}
	entries := make([]entry, 0, len(s.accountPeers.proven))
	for peerId := range s.accountPeers.proven {
		entries = append(entries, entry{peerId, union(s.accountPeers.outbound[peerId], s.accountPeers.inbound[peerId])})
	}
	s.accountPeers.mu.Unlock()
	if len(entries) == 0 {
		return
	}
	pending := s.pendingSpaceIds(loading...)
	for _, e := range entries {
		// the store notifies only on a real change
		s.peerStore.UpdateLocalPeer(e.peerId, s.provenListLocked(e.confirmed, tech.id, pending))
	}
}

// forgetLocalPeer drops a peer the peer manager removed (its dial failed), so
// a later refresh cannot resurrect it: removal is authoritative over both
// directions and the proof. Re-discovery goes through publishLocalPeer again.
func (s *service) forgetLocalPeer(peerId string) {
	s.accountPeers.mu.Lock()
	defer s.accountPeers.mu.Unlock()
	delete(s.accountPeers.proven, peerId)
	delete(s.accountPeers.outbound, peerId)
	delete(s.accountPeers.inbound, peerId)
}

// pendingSpaceIds is every space we know of but do not hold on disk: the
// view-based list from the space service (plus the ids being loaded right
// now), minus the storage list.
func (s *service) pendingSpaceIds(loading ...string) []string {
	known := map[string]struct{}{}
	for _, id := range loading {
		known[id] = struct{}{}
	}
	if s.knownSpaces != nil {
		for _, id := range s.knownSpaces.AllSpaceIds() {
			known[id] = struct{}{}
		}
	}
	delete(known, "")
	delete(known, addr.AnytypeMarketplaceWorkspace)
	onDisk, err := s.spaceStorageProvider.AllSpaceIds()
	if err != nil {
		log.Warn("pending space ids: all space ids", zap.Error(err))
		return nil
	}
	for _, id := range onDisk {
		delete(known, id)
	}
	pending := make([]string, 0, len(known))
	for id := range known {
		pending = append(pending, id)
	}
	slices.Sort(pending)
	return pending
}

func union(a, b []string) []string {
	out := slices.Clone(a)
	for _, id := range b {
		if !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out
}

func lookupKnownSpaces(a *app.App) knownSpaceIdsProvider {
	c := a.Component(SpaceServiceCName)
	if c == nil {
		return nil
	}
	provider, _ := c.(knownSpaceIdsProvider)
	return provider
}
