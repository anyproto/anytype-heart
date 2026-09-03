package spacecore

import (
	"context"
	"slices"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/space/spacecore/localdiscovery"
)

// GO-7492: a LAN peer that shares nothing at discovery time may share
// everything seconds later — on a cold device the first exchange happens
// before the tech space and the SpaceViews arrive. Rather than a blacklist,
// the space list simply has to be re-sent once it has changed.
//
// The trigger is loadSpace: it is the one place every path that makes a space
// usable in this process converges (derive, create, pull, LAN push), and it
// is exactly the moment AllSpaceIds grows. Spaces land one after another
// during a cold sync, so the signals are coalesced: a round runs only after
// reexchangeWindow of quiet, one round at a time, and a signal that lands
// mid-round schedules exactly one more. mDNS is not a substitute: the browse
// client suppresses re-delivery of a known peer until its advertised TTL
// (3600 s) expires, so PeerDiscovered re-fires for a known peer about hourly.
var (
	reexchangeWindow      = 2 * time.Second
	reexchangePeerTimeout = 10 * time.Second
)

// reexchanger coalesces "our space list changed" into rounds of run.
type reexchanger struct {
	mu      sync.Mutex
	window  time.Duration
	run     func()
	timer   *time.Timer
	running bool
	pending bool
	closed  bool
	wg      sync.WaitGroup
}

func newReexchanger(window time.Duration, run func()) *reexchanger {
	return &reexchanger{window: window, run: run}
}

// trigger records that the space list changed. It never blocks and never
// runs anything inline: loadSpace calls it from inside the space cache load.
func (r *reexchanger) trigger() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	if r.running {
		r.pending = true
		return
	}
	if r.timer == nil {
		r.timer = time.AfterFunc(r.window, r.fire)
	}
}

func (r *reexchanger) fire() {
	r.mu.Lock()
	r.timer = nil
	if r.closed || r.running {
		r.mu.Unlock()
		return
	}
	r.running = true
	r.wg.Add(1)
	r.mu.Unlock()
	go r.round()
}

func (r *reexchanger) round() {
	defer r.wg.Done()
	r.run()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.running = false
	if r.pending && !r.closed {
		// every round is preceded by a quiet window, the rerun included
		r.pending = false
		r.timer = time.AfterFunc(r.window, r.fire)
	}
}

func (r *reexchanger) close() {
	r.mu.Lock()
	r.closed = true
	if r.timer != nil {
		r.timer.Stop()
		r.timer = nil
	}
	r.mu.Unlock()
	r.wg.Wait()
}

// exchangeWithKnownPeers is one re-exchange round: the outbound handshake with
// every LAN peer the peer store already knows, with the current space list.
// A failed exchange changes nothing — this is not a blacklist; the peer
// manager drops unreachable peers on its own dial failures.
func (s *service) exchangeWithKnownPeers() {
	peers := slices.Clone(s.peerStore.AllLocalPeers())
	if len(peers) == 0 {
		return
	}
	own := s.currentOwn()
	for _, peerId := range peers {
		if s.componentCtx.Err() != nil {
			return
		}
		ctx, cancel := context.WithTimeout(s.componentCtx, reexchangePeerTimeout)
		shared, proof, err := s.exchangeFn(ctx, peerId, own)
		cancel()
		if err != nil {
			log.Debug("re-exchange with local peer failed", zap.String("peerId", peerId), zap.Error(err))
			continue
		}
		s.publishLocalPeer(peerId, shared, proof)
	}
}

// rememberOwn keeps the last addresses local discovery reported for this
// device, so a re-exchange can still tell the peer where to reach us.
func (s *service) rememberOwn(own localdiscovery.OwnAddresses) {
	s.ownMu.Lock()
	defer s.ownMu.Unlock()
	s.lastOwn = &own
}

// currentOwn is nil until local discovery has reported our addresses; the
// exchange then goes without a LocalServer (a probe the peer answers but does
// not record), which is still enough for us to learn what it shares.
func (s *service) currentOwn() *localdiscovery.OwnAddresses {
	s.ownMu.Lock()
	defer s.ownMu.Unlock()
	if s.lastOwn == nil {
		return nil
	}
	own := *s.lastOwn
	return &own
}
