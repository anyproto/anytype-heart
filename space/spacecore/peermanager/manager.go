package peermanager

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/commonspace/spacesyncproto"
	"github.com/anyproto/any-sync/net/streampool"
	"go.uber.org/atomic"
	"storj.io/drpc"

	//nolint:misspell
	"github.com/anyproto/any-sync/commonspace/headsync"
	"github.com/anyproto/any-sync/commonspace/peermanager"
	"github.com/anyproto/any-sync/net/peer"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/syncstatus/nodestatus"
	"github.com/anyproto/anytype-heart/space/spacecore/peerstore"
)

type contextKey string

var (
	ContextPeerFindDeadlineKey  contextKey = "peerFindDeadline"
	ErrPeerFindDeadlineExceeded            = errors.New("peer find deadline exceeded")
)

const (
	// responsiblePeersCheckInterval is the online re-dial/refresh cadence.
	responsiblePeersCheckInterval = time.Second * 20
	// responsiblePeersCheckIntervalOffline stretches the cadence while the
	// device is known offline: still probing (a wrong offline belief must
	// self-correct) but not burning the radio with doomed dials every 20s.
	// A reconnect signals an immediate rebuild, so this adds no recovery
	// latency.
	responsiblePeersCheckIntervalOffline = time.Minute * 2
	// broadcastPeerFindDeadline bounds how long a broadcast may wait for
	// responsible peers to appear. Broadcasts run inside the shared streampool
	// dial workers (only a few of them); without a deadline an offline device
	// parks a worker per broadcast indefinitely and freezes the send path.
	// Dropped broadcasts are recovered by the periodic head-sync.
	broadcastPeerFindDeadline = time.Second * 30
	// responsiblePeersRetryInterval is the re-fetch cadence right after a
	// failed node lookup while the device believes it is online: quick enough
	// to pick up a connection established by another space's dial in the
	// meantime, and not a dial storm — dials are single-flighted in the pool,
	// so concurrent retries share one attempt.
	responsiblePeersRetryInterval = time.Second * 5
	// fastRetryMaxAttempts bounds how many consecutive failed lookups keep the
	// fast cadence. The fast retry exists for the transient window right after
	// a network switch; when the node stays unreachable (LAN without internet,
	// captive portal — local-only P2P keeps syncing meanwhile), decaying back
	// to the normal cadence avoids permanent doomed-dial pressure every 5s.
	fastRetryMaxAttempts = 3
	// localPeerDialTimeout bounds ONE local (mDNS) peer dial. After a network
	// switch the peer store still lists peers from the previous network whose
	// addresses are unroutable; unbounded dials to them parked every manager
	// single-flight for the whole scheme x addr chain (~20-40s) while a healthy
	// node connection sat idle. A bounded failure also actually triggers the
	// stale peer's removal.
	//
	// Per dial, not one budget shared across the whole loop. A shared budget is
	// the same defect as GO-7409: the first stale entry consumes it, every peer
	// behind it then gets an already-expired context, and because the loop
	// reads any error as "stale" those peers are evicted from the peer store
	// without ever being tried. Worse, an already-connected peer hits a select
	// in ocache with both ctx.Done() and the loaded entry ready, so it is
	// dropped on a coin flip. In LocalOnly mode there are no nodes at all and
	// these peers are the only sync path, so that eviction costs sync outright.
	//
	// 2s, the same LAN budget as rpcstore's localPeerConnectTimeout: awake
	// LAN peers connect in well under 0.5s (measured 2026-10-09); a phone
	// still waking its radio can miss it and is evicted until its next mDNS
	// discovery or inbound exchange re-adds it.
	localPeerDialTimeout = time.Second * 2
)

type NodeStatus interface {
	app.Component
	SetNodesStatus(spaceId string, status nodestatus.ConnectionStatus)
	GetNodeStatus(string) nodestatus.ConnectionStatus
}

type Updater interface {
	app.ComponentRunnable
	Refresh(spaceId string)
}

type PeerToPeerStatus interface {
	RegisterSpace(spaceId string)
	UnregisterSpace(spaceId string)
}

type clientPeerManager struct {
	spaceId string

	responsibleNodeIds        []string
	responsibleNodeIdsUpdated atomic.Time
	responsibleNodeIdsMu      sync.Mutex

	p         *provider
	peerStore peerstore.PeerStore

	responsiblePeers          []peer.Peer
	watchingPeers             map[string]struct{}
	rebuildResponsiblePeers   chan struct{}
	availableResponsiblePeers chan struct{}
	nodeStatus                NodeStatus
	spaceSyncService          Updater
	streamPool                streampool.StreamPool
	headSync                  headsync.HeadSync
	diffKickRunning           atomic.Bool
	// nodePeer and localPeers are the two sources responsiblePeers is composed
	// of (under the mutex): the latest node verdict, nil after a failed lookup,
	// and what the local pass has found. Each publishes on its own, so neither
	// waits for the other (see fetchResponsiblePeers).
	nodePeer   peer.Peer
	localPeers []peer.Peer
	// nodeLookupRunning keeps one node lookup in flight at a time: a fetch that
	// finds it set runs only the local pass. nodeLookupDone receives a signal
	// after each lookup, so the manage loop re-arms its cadence from the verdict.
	// rebuildGen counts rebuild signals; each lookup compares it with the value
	// it saw at its start (see refetchAfterLookup).
	nodeLookupRunning atomic.Bool
	nodeLookupDone    chan struct{}
	rebuildGen        atomic.Uint64
	// lookupReleased, when set (tests only), runs right after a lookup releases
	// the slot, before refetchAfterLookup
	lookupReleased func()
	// consecutiveFetchFailures counts failed node lookups since the last
	// success or rebuild signal. A rebuild signal resets it so every fresh
	// connectivity event gets its own fast-retry window, even when the
	// previous state was steady failure (LAN-only device walking out to
	// cellular).
	consecutiveFetchFailures atomic.Int32

	ctx       context.Context
	ctxCancel context.CancelFunc
	sync.Mutex

	peerToPeerStatus PeerToPeerStatus
	keepAliveMessage drpc.Message
}

func (n *clientPeerManager) Init(a *app.App) (err error) {
	n.responsibleNodeIds = n.peerStore.ResponsibleNodeIds(n.spaceId)
	n.responsibleNodeIdsUpdated.Store(time.Now())
	n.ctx, n.ctxCancel = context.WithCancel(context.Background())
	n.rebuildResponsiblePeers = make(chan struct{}, 1)
	n.nodeLookupDone = make(chan struct{}, 1)
	n.watchingPeers = make(map[string]struct{})
	n.availableResponsiblePeers = make(chan struct{})
	n.nodeStatus = app.MustComponent[NodeStatus](a)
	n.streamPool = app.MustComponent[streampool.StreamPool](a)
	n.spaceSyncService = app.MustComponent[Updater](a)
	n.peerToPeerStatus = app.MustComponent[PeerToPeerStatus](a)
	// optional (absent in tests): used to kick an immediate diff round when the
	// node connection recovers, replacing the broadcasts dropped while offline
	if c := a.Component(headsync.CName); c != nil {
		n.headSync, _ = c.(headsync.HeadSync)
	}

	var keepAliveMsg = &spacesyncproto.SpaceSubscription{
		SpaceIds: []string{n.spaceId},
		Action:   spacesyncproto.SpaceSubscriptionAction_Subscribe,
	}
	payload, err := keepAliveMsg.MarshalVT()
	if err != nil {
		return
	}
	n.keepAliveMessage = &spacesyncproto.ObjectSyncMessage{
		Payload: payload,
	}
	return
}

func (n *clientPeerManager) Name() (name string) {
	return peermanager.CName
}

func (n *clientPeerManager) Run(ctx context.Context) (err error) {
	if n.p != nil {
		n.p.registerManager(n)
	}
	go n.peerToPeerStatus.RegisterSpace(n.spaceId)
	go n.manageResponsiblePeers()
	return
}

// signalRebuild requests an immediate responsible-peers re-fetch (non-blocking;
// coalesces with an already-pending request). It also opens a fresh fast-retry
// window: the signal means connectivity plausibly changed, so prior steady
// failures must not slow the retries that follow.
func (n *clientPeerManager) signalRebuild() {
	n.consecutiveFetchFailures.Store(0)
	n.rebuildGen.Inc()
	n.signalFetch()
}

// signalFetch requests an immediate re-fetch without opening a fast-retry
// window: a served peer closing or a LAN peer joining the space says nothing
// about whether connectivity changed.
func (n *clientPeerManager) signalFetch() {
	select {
	case n.rebuildResponsiblePeers <- struct{}{}:
	default:
	}
}

func (n *clientPeerManager) GetNodePeers(ctx context.Context) (peers []peer.Peer, err error) {
	p, err := n.p.pool.GetOneOf(ctx, n.getNodeIds())
	if err == nil {
		peers = []peer.Peer{p}
	}
	return
}

func (n *clientPeerManager) BroadcastMessage(ctx context.Context, msg drpc.Message) (err error) {
	// the context which comes here should not be used. It can be cancelled and thus kill the stream,
	// because the stream can be opened with this context
	ctx = logger.CtxWithFields(context.Background(), logger.CtxGetFields(ctx)...)
	// bound the wait for peers: this getter runs inside a shared streampool
	// dial worker and must not park it until reconnect
	ctx = context.WithValue(ctx, ContextPeerFindDeadlineKey, time.Now().Add(broadcastPeerFindDeadline))
	return n.streamPool.Send(ctx, msg, func(ctx context.Context) (peers []peer.Peer, err error) {
		return n.GetResponsiblePeers(ctx)
	})
}

func (n *clientPeerManager) SendMessage(ctx context.Context, peerId string, msg drpc.Message) (err error) {
	// the context which comes here should not be used. It can be cancelled and thus kill the stream,
	// because the stream can be opened with this context
	ctx = logger.CtxWithFields(context.Background(), logger.CtxGetFields(ctx)...)
	return n.streamPool.Send(ctx, msg, func(ctx context.Context) (peers []peer.Peer, err error) {
		return n.getExactPeer(ctx, peerId)
	})
}

func (n *clientPeerManager) GetResponsiblePeers(ctx context.Context) (peers []peer.Peer, err error) {
	n.Lock()
	// Serve only live peers. Right after a connection-pool flush (foreground
	// resume, network switch) the cached list still holds the just-closed
	// peers until the rebuild swaps it; handing those out made the immediate
	// post-recovery head-sync and opened-object refresh silent no-ops that
	// waited for the next periodic tick (~20s). Filtering makes callers fall
	// into the wait below and sync the moment the fresh dial lands.
	for _, p := range n.responsiblePeers {
		if !p.IsClosed() {
			peers = append(peers, p)
		}
	}
	if dropped := len(n.responsiblePeers) - len(peers); dropped > 0 && n.p != nil {
		n.p.stats.closedPeersFiltered.Add(int64(dropped))
	}
	if len(peers) == 0 {
		if n.availableResponsiblePeers == nil {
			n.availableResponsiblePeers = make(chan struct{})
		}
		ch := n.availableResponsiblePeers
		n.Unlock()
		if err = n.waitResponsiblePeers(ctx, ch); err != nil {
			return nil, err
		}
		return n.GetResponsiblePeers(ctx)
	}
	n.Unlock()
	log.Debug("get responsible peers", zap.Int("peerCount", len(peers)), zap.String("spaceId", n.spaceId))
	return
}

// waitResponsiblePeers blocks until a rebuild publishes live peers (ch is
// closed), the optional peer-find deadline passes, or ctx is done.
func (n *clientPeerManager) waitResponsiblePeers(ctx context.Context, ch <-chan struct{}) error {
	deadline, _ := ctx.Value(ContextPeerFindDeadlineKey).(time.Time)
	if deadline.IsZero() {
		select {
		case <-ch:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if time.Now().After(deadline) {
		return ErrPeerFindDeadlineExceeded
	}
	select {
	case <-ch:
		return nil
	case <-time.After(time.Until(deadline)):
		return ErrPeerFindDeadlineExceeded
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (n *clientPeerManager) getExactPeer(ctx context.Context, peerId string) (peers []peer.Peer, err error) {
	p, err := n.p.pool.Get(ctx, peerId)
	if err != nil {
		return nil, err
	}
	return []peer.Peer{p}, nil
}

func (n *clientPeerManager) getStreamResponsiblePeers(ctx context.Context) (peers []peer.Peer, err error) {
	var peerIds []string
	// lookup in common pool for existing connection
	p, nodeErr := n.p.pool.GetOneOf(ctx, n.getNodeIds())
	if nodeErr != nil {
		log.Warn("failed to get responsible peer from common pool", zap.Error(nodeErr))
	} else {
		peerIds = []string{p.Id()}
	}
	peerIds = append(peerIds, n.peerStore.LocalPeerIds(n.spaceId)...)
	for _, peerId := range peerIds {
		p, err := n.p.pool.Get(ctx, peerId)
		if err != nil {
			n.peerStore.RemoveLocalPeer(peerId)
			log.Warn("failed to get peer from stream pool", zap.String("peerId", peerId), zap.Error(err))
			continue
		}
		peers = append(peers, p)
	}

	// set node error if no local peers
	if len(peers) == 0 {
		err = fmt.Errorf("failed to get peers for stream")
	}
	return
}

func (n *clientPeerManager) manageResponsiblePeers() {
	for {
		n.fetchResponsiblePeers()
		if !n.waitNextFetch() {
			return
		}
	}
}

// waitNextFetch blocks until the next fetch is due, the cadence timer firing
// or a fetch being signalled, and reports false once the manager is closed. A
// node verdict landing meanwhile re-arms the timer: the verdict decides the
// cadence, and counting it from the verdict keeps "retry 5s after a failed
// lookup" what it was when the loop itself waited the lookup out.
func (n *clientPeerManager) waitNextFetch() bool {
	timer := time.NewTimer(n.nextCheckInterval())
	defer timer.Stop()
	for {
		select {
		case <-timer.C:
			return true
		case <-n.rebuildResponsiblePeers:
			return true
		case <-n.nodeLookupDone:
			timer.Reset(n.nextCheckInterval())
		case <-n.ctx.Done():
			return false
		}
	}
}

// nextCheckInterval stretches the re-dial cadence while the device is known
// offline — unless LAN peers are present: "no internet" does not mean "no
// sync" (local-only P2P must keep refreshing at full cadence). Conversely,
// for the first few failed node lookups it shortens the cadence: right after
// a network switch the failure was likely a bounded wait behind a slow dial,
// and another space's dial may land a usable connection any moment. Once the
// failures look steady-state (node genuinely unreachable, e.g. LAN-only
// setups) the cadence decays back to normal.
func (n *clientPeerManager) nextCheckInterval() time.Duration {
	if n.p != nil && n.p.isOffline() && len(n.peerStore.LocalPeerIds(n.spaceId)) == 0 {
		return responsiblePeersCheckIntervalOffline
	}
	if n.nodeStatus != nil && n.nodeStatus.GetNodeStatus(n.spaceId) == nodestatus.ConnectionError &&
		n.consecutiveFetchFailures.Load() <= fastRetryMaxAttempts {
		return responsiblePeersRetryInterval
	}
	return responsiblePeersCheckInterval
}

// fetchResponsiblePeers refreshes both sources of the served list at once: the
// node lookup on its own goroutine, the local pass here. Sequencing them, in
// either order, stalls one behind the other. Node first left every space
// without its reachable cable peer for 20-70s after an interface loss cut the
// internet: the walk blocked in DNS while the peer's live connection sat in
// the pool. Local first would hold a healthy node behind stale mDNS entries
// after a switch to cellular (GO-7379).
func (n *clientPeerManager) fetchResponsiblePeers() {
	n.startNodeLookup()
	n.fetchLocalPeers()
}

// startNodeLookup runs a node lookup in the background, unless one is already
// in flight: the pool would single-flight the dial anyway, but each waiter
// would hold a goroutine and could apply a stale verdict over a fresher one.
func (n *clientPeerManager) startNodeLookup() {
	if !n.nodeLookupRunning.CompareAndSwap(false, true) {
		return
	}
	// read before the dial: every rebuild counted so far is behind this lookup
	startGen := n.rebuildGen.Load()
	go func() {
		defer func() {
			n.nodeLookupRunning.Store(false)
			if n.lookupReleased != nil {
				n.lookupReleased()
			}
			n.refetchAfterLookup(startGen)
			select {
			case n.nodeLookupDone <- struct{}{}:
			default:
			}
		}()
		n.lookupNode()
	}()
}

// refetchAfterLookup asks for another fetch when the verdict just applied may
// already be out of date. A fetch requested meanwhile found the lookup in
// flight and ran only the local pass, so the space would otherwise wait for
// the next tick (5s-2min):
//   - a rebuild was signalled since this lookup started (startGen): a node
//     reachable again would wait for the fast retry, where the synchronous
//     loop took the pending rebuild at once;
//   - the node it served is closed already: closed on arrival (a flush raced
//     the lookup), it was never published, so no watcher reports it; closed
//     while the lookup was finishing, its watcher's fetch was spent.
//
// It runs after the slot is released and consumes nothing, so lookups whose
// tails overlap cannot take each other's requests: a lookup holding the slot
// when this fetch lands runs the same check when it ends, against what it saw
// at its own start. Plain fetch signals and cadence ticks are not replayed:
// after a long offline walk they would start another at once. At most once
// per lookup, and the next one replaces the node, so a closed node cannot spin
// the loop.
func (n *clientPeerManager) refetchAfterLookup(startGen uint64) {
	if n.ctx.Err() != nil {
		return
	}
	n.Lock()
	closed := n.nodePeer != nil && n.nodePeer.IsClosed()
	n.Unlock()
	if closed || n.rebuildGen.Load() != startGen {
		n.signalFetch()
	}
}

// lookupNode finds one responsible node connection and serves it, or serves
// none after a failure: GetOneOf returns a live connection before dialing, so
// a failure means there is none.
func (n *clientPeerManager) lookupNode() {
	prevStatus := n.nodeStatus.GetNodeStatus(n.spaceId)
	// Deliberately unbounded, as it was before GO-7379. GetOneOf walks every
	// responsible node x every scheme x every address, and each attempt is
	// already capped by the transports (yamux net.Dialer.Timeout, quic
	// HandshakeIdleTimeout), so the walk terminates on its own. A bound here
	// has to be larger than that whole product or it truncates the walk: a
	// single address that accepts UDP and never replies then consumes it all
	// and the yamux fallback is never attempted, which cost self-hosted users
	// sync entirely. See GO-7410 for the layered fix that lets a bound be
	// reintroduced safely. Nothing waits for the walk: the local peers are
	// served meanwhile.
	p, err := n.p.pool.GetOneOf(n.ctx, n.getNodeIds())
	if n.ctx.Err() != nil {
		// closed meanwhile
		return
	}
	if err == nil {
		n.consecutiveFetchFailures.Store(0)
		n.nodeStatus.SetNodesStatus(n.spaceId, nodestatus.Online)
		n.setNodePeer(p)
		if prevStatus == nodestatus.ConnectionError {
			// Node connection just recovered. Head-update broadcasts attempted
			// while offline were dropped (bounded by broadcastPeerFindDeadline),
			// so without a kick the changes made offline would only reach other
			// devices on the next periodic diff round (up to ~20s later than
			// pre-drop behavior). An immediate diff round restores — and beats —
			// the old "parked broadcast fires on reconnect" delivery latency,
			// and its trailing KeepAlive re-establishes the broadcast stream
			// and space subscription.
			n.kickDiffSyncOnReconnect()
		}
	} else {
		n.consecutiveFetchFailures.Inc()
		log.Info("can't get node peers", zap.Error(err))
		n.nodeStatus.SetNodesStatus(n.spaceId, nodestatus.ConnectionError)
		n.setNodePeer(nil)
	}
	n.spaceSyncService.Refresh(n.spaceId)
}

// fetchLocalPeers connects the space's local (mDNS) peers and serves each one
// the moment it lands; a peer already connected, inbound or outbound, comes
// from the pool without a dial. Peers the pass has not reached yet stay served
// until it ends, and then the list becomes exactly what it found. Each dial is
// bounded separately, so one stale entry costs only its own budget: on failure
// the stale peer is removed, so a Wi-Fi-era entry costs at most one bounded
// attempt after the switch instead of an unbounded dial chain per cycle.
func (n *clientPeerManager) fetchLocalPeers() {
	peerIds := n.peerStore.LocalPeerIds(n.spaceId)
	peers := make([]peer.Peer, 0, len(peerIds))
	for _, peerId := range peerIds {
		p, err := n.getLocalPeer(peerId)
		if err != nil {
			if n.ctx.Err() != nil {
				// closing: the failure says nothing about the peer, and the
				// store is shared by every space
				return
			}
			n.peerStore.RemoveLocalPeer(peerId)
			log.Warn("failed to get local from net pool", zap.String("peerId", peerId), zap.Error(err))
			continue
		}
		peers = append(peers, p)
		n.serveLocalPeer(p)
	}
	n.setLocalPeers(peers)
}

// getLocalPeer connects a local peer within one bounded budget. A failed dial
// is checked against the connections already in the pool before it counts:
// after a flush the dial races the peer's own reconnect, and the pool does not
// hand an inbound connection to a dial already under way, so a peer that
// connected to us meanwhile would be evicted with a live connection.
func (n *clientPeerManager) getLocalPeer(peerId string) (peer.Peer, error) {
	ctx, cancel := context.WithTimeout(n.ctx, localPeerDialTimeout)
	defer cancel()
	p, err := n.p.pool.Get(ctx, peerId)
	if err == nil {
		return p, nil
	}
	if n.ctx.Err() == nil {
		// Pick never dials, and it shares the dial's budget
		if p, pickErr := n.p.pool.Pick(ctx, peerId); pickErr == nil {
			return p, nil
		}
	}
	return nil, fmt.Errorf("get local peer: %w", err)
}

func (n *clientPeerManager) setNodePeer(p peer.Peer) {
	n.Lock()
	defer n.Unlock()
	n.nodePeer = p
	n.publishLocked()
}

// serveLocalPeer serves p in place of the entry with its id, or in addition.
func (n *clientPeerManager) serveLocalPeer(p peer.Peer) {
	n.Lock()
	defer n.Unlock()
	if i := slices.IndexFunc(n.localPeers, func(lp peer.Peer) bool { return lp.Id() == p.Id() }); i >= 0 {
		n.localPeers[i] = p
	} else {
		n.localPeers = append(n.localPeers, p)
	}
	n.publishLocked()
	if p.IsClosed() {
		// closed before it could be served (the peer reconnecting): publishing
		// left it out, so no watcher reports it; the next pass, right after
		// this one, gets the new connection
		n.signalFetch()
	}
}

func (n *clientPeerManager) setLocalPeers(peers []peer.Peer) {
	n.Lock()
	defer n.Unlock()
	n.localPeers = peers
	n.publishLocked()
}

// publishLocked composes the served list from the live peers of its two
// sources, swaps it in and wakes the waiters; safe to call any number of times
// (watchers are registered once per peer). A source keeps a closed peer until
// its next result, e.g. the node until a lookup started after a flush returns;
// composing it in would hand it a fresh watcher, which would find it closed
// and signal yet another fetch, and wake the waiters with nothing to serve.
func (n *clientPeerManager) publishLocked() {
	peers := make([]peer.Peer, 0, 1+len(n.localPeers))
	if n.nodePeer != nil && !n.nodePeer.IsClosed() {
		peers = append(peers, n.nodePeer)
	}
	for _, p := range n.localPeers {
		if !p.IsClosed() {
			peers = append(peers, p)
		}
	}
	for _, p := range peers {
		if _, ok := n.watchingPeers[p.Id()]; !ok {
			n.watchingPeers[p.Id()] = struct{}{}
			go func(pr peer.Peer) {
				n.watchPeer(pr)
			}(p)
		}
	}
	log.Debug("set responsible peers", zap.Int("peerCount", len(peers)), zap.String("spaceId", n.spaceId))
	n.responsiblePeers = peers
	if len(peers) > 0 && n.availableResponsiblePeers != nil {
		close(n.availableResponsiblePeers)
		n.availableResponsiblePeers = nil
	}
}

// kickDiffSyncOnReconnect runs one immediate head-sync (diff) round in the
// background. Single-flight: a still-running kick from a rapid
// offline/online flap is not stacked.
func (n *clientPeerManager) kickDiffSyncOnReconnect() {
	if n.headSync == nil || !n.diffKickRunning.CompareAndSwap(false, true) {
		return
	}
	if n.p != nil {
		n.p.stats.reconnectDiffKicks.Inc()
	}
	go func() {
		defer n.diffKickRunning.Store(false)
		ctx, cancel := context.WithTimeout(n.ctx, time.Minute)
		defer cancel()
		if err := n.headSync.DiffSync(ctx); err != nil && n.ctx.Err() == nil {
			log.Info("diff sync on reconnect", zap.String("spaceId", n.spaceId), zap.Error(err))
		}
	}()
}

// watchPeer signals a fetch once p closes. Its id is released first: the fetch
// may serve a new connection to the same peer at once, and that one must get
// a watcher of its own.
func (n *clientPeerManager) watchPeer(p peer.Peer) {
	select {
	case <-p.CloseChan():
	case <-n.ctx.Done():
	}
	n.Lock()
	delete(n.watchingPeers, p.Id())
	n.Unlock()
	if n.ctx.Err() == nil {
		n.signalFetch()
	}
}

func (n *clientPeerManager) getNodeIds() []string {
	if len(n.responsibleNodeIds) != 0 && time.Since(n.responsibleNodeIdsUpdated.Load()) < time.Minute {
		return n.responsibleNodeIds
	}
	n.responsibleNodeIdsMu.Lock()
	defer n.responsibleNodeIdsMu.Unlock()
	n.responsibleNodeIds = n.peerStore.ResponsibleNodeIds(n.spaceId)
	n.responsibleNodeIdsUpdated.Store(time.Now())
	return n.responsibleNodeIds
}

func (n *clientPeerManager) KeepAlive(ctx context.Context) {
	_ = n.BroadcastMessage(ctx, n.keepAliveMessage)
}

func (n *clientPeerManager) Close(ctx context.Context) (err error) {
	if n.p != nil {
		n.p.unregisterManager(n)
	}
	n.ctxCancel()
	n.peerToPeerStatus.UnregisterSpace(n.spaceId)
	return
}
