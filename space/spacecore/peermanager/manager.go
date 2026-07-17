package peermanager

import (
	"context"
	"errors"
	"fmt"
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
	// responsibleNodeDialTimeout bounds the node lookup in
	// fetchResponsiblePeers. pool.GetOneOf parks single-flight behind an
	// in-flight dial of whichever node id the shuffle picked; on a fresh
	// cellular path one unreachable node's address chain (schemes x addrs
	// dialed sequentially, 10s each) can hold that park for tens of seconds
	// while another responsible node is already connected (observed in the
	// field: iOS Wi-Fi->cellular, 71 spaces parked behind one dial). Giving up
	// early lets the fast retry below re-enter GetOneOf, whose active-conn
	// check then returns any node that connected meanwhile.
	responsibleNodeDialTimeout = time.Second * 10
	// responsibleNodeDialTimeoutSlow replaces the bound after consecutive
	// failures. The tight bound is right for the common case (live conn or a
	// fast dial) but it is shorter than a full scheme fallback: on networks
	// that blackhole QUIC (VPNs, some carriers) every dial needs the whole
	// quic-attempts-then-yamux chain (observed 20-35s on a desktop with a VPN
	// up), and a 10s cancel-and-retry loop re-runs the doomed prefix forever —
	// sync never comes up. Once failures repeat, give the chain room to reach
	// the working transport.
	responsibleNodeDialTimeoutSlow = time.Second * 40
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
	// localPeerDialTimeout is the shared budget for refreshing all local (mDNS)
	// peers in one fetch cycle. After a network switch the peer store still
	// lists peers from the previous network whose addresses are unroutable;
	// unbounded dials to them parked every manager single-flight for the whole
	// scheme x addr chain (~20-40s) while a healthy node connection sat idle.
	// A bounded failure also actually triggers the stale peer's removal.
	localPeerDialTimeout = time.Second * 5
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
	// consecutiveFetchFailures counts failed node lookups since the last
	// success or rebuild signal. A rebuild signal resets it so every fresh
	// connectivity event gets its own fast-retry window, even when the
	// previous state was steady failure (LAN-only device walking out to
	// cellular).
	consecutiveFetchFailures atomic.Int32
	// loopRunning marks the manageResponsiblePeers goroutine as alive; the
	// provider stat counts dead loops to expose managers that can no longer
	// refresh their peers (diagnostics for field stalls).
	loopRunning atomic.Bool

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
		n.warnEmptyPeersWait()
		if err = n.waitResponsiblePeers(ctx, ch); err != nil {
			return nil, err
		}
		return n.GetResponsiblePeers(ctx)
	}
	n.Unlock()
	log.Debug("get responsible peers", zap.Int("peerCount", len(peers)), zap.String("spaceId", n.spaceId))
	return
}

// warnEmptyPeersWait surfaces (rate-limited process-wide, 30s) that callers
// are waiting on an empty responsible-peers list, with the fields that decide
// whether the manager can ever refresh it — the exact data needed to diagnose
// a field stall where sync waits silently.
func (n *clientPeerManager) warnEmptyPeersWait() {
	if n.p == nil {
		return
	}
	now := time.Now().Unix()
	last := n.p.stats.lastEmptyWaitWarnUnix.Load()
	if now-last < 30 || !n.p.stats.lastEmptyWaitWarnUnix.CompareAndSwap(last, now) {
		return
	}
	log.Warn("waiting on empty responsible peers",
		zap.String("spaceId", n.spaceId),
		zap.Bool("managerClosed", n.ctx.Err() != nil),
		zap.Bool("loopRunning", n.loopRunning.Load()),
		zap.Bool("offline", n.p.isOffline()),
		zap.Int("localPeers", len(n.peerStore.LocalPeerIds(n.spaceId))),
		zap.Int32("consecutiveFetchFailures", n.consecutiveFetchFailures.Load()))
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
	n.loopRunning.Store(true)
	defer n.loopRunning.Store(false)
	for {
		n.fetchResponsiblePeers()
		select {
		case <-time.After(n.nextCheckInterval()):
		case <-n.rebuildResponsiblePeers:
		case <-n.ctx.Done():
			return
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
		n.p.stats.choseOfflineInterval.Inc()
		return responsiblePeersCheckIntervalOffline
	}
	if n.nodeStatus != nil && n.nodeStatus.GetNodeStatus(n.spaceId) == nodestatus.ConnectionError &&
		n.consecutiveFetchFailures.Load() <= fastRetryMaxAttempts {
		if n.p != nil {
			n.p.stats.choseFastRetry.Inc()
		}
		return responsiblePeersRetryInterval
	}
	if n.p != nil {
		n.p.stats.choseNormalInterval.Inc()
	}
	return responsiblePeersCheckInterval
}

func (n *clientPeerManager) fetchResponsiblePeers() {
	var peers []peer.Peer
	prevStatus := n.nodeStatus.GetNodeStatus(n.spaceId)
	// Bound the lookup: GetOneOf may park single-flight behind another
	// space's in-flight dial to an unreachable node; waitLoad honors ctx, so
	// the retry loop stays in control instead of inheriting the slowest
	// dial's schedule. After repeated failures the bound escalates so a slow
	// but working scheme fallback (QUIC-blocked networks) can complete.
	bound := responsibleNodeDialTimeout
	if n.consecutiveFetchFailures.Load() >= 2 {
		bound = responsibleNodeDialTimeoutSlow
	}
	dialCtx, cancel := context.WithTimeout(n.ctx, bound)
	p, err := n.p.pool.GetOneOf(dialCtx, n.getNodeIds())
	cancel()
	if err == nil {
		n.consecutiveFetchFailures.Store(0)
		peers = []peer.Peer{p}
		n.nodeStatus.SetNodesStatus(n.spaceId, nodestatus.Online)
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
	}
	n.spaceSyncService.Refresh(n.spaceId)
	if len(peers) > 0 {
		// Publish the node connection before touching local peers: the
		// head-sync sweep and broadcasts wait on this list, and the local
		// dials below can burn seconds on stale mDNS entries after a network
		// switch (observed in the field: all managers parked behind one dial
		// to a Wi-Fi-era local peer unroutable from cellular, while a healthy
		// node connection sat idle until the fetch finished).
		n.publishResponsiblePeers(peers)
	}
	peerIds := n.peerStore.LocalPeerIds(n.spaceId)
	if len(peerIds) > 0 {
		// Bound the local dials as one budget: on failure the stale peer is
		// removed, so a Wi-Fi-era entry costs at most one bounded pass after
		// the switch instead of an unbounded dial chain per cycle.
		localCtx, cancelLocal := context.WithTimeout(n.ctx, localPeerDialTimeout)
		for _, peerId := range peerIds {
			p, err := n.p.pool.Get(localCtx, peerId)
			if err != nil {
				n.peerStore.RemoveLocalPeer(peerId)
				log.Warn("failed to get local from net pool", zap.String("peerId", peerId), zap.Error(err))
				continue
			}
			peers = append(peers, p)
		}
		cancelLocal()
	}
	n.publishResponsiblePeers(peers)
}

// publishResponsiblePeers swaps the served list and wakes the waiters; safe to
// call more than once per fetch (watchers are registered once per peer).
func (n *clientPeerManager) publishResponsiblePeers(peers []peer.Peer) {
	n.Lock()
	defer n.Unlock()

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

func (n *clientPeerManager) watchPeer(p peer.Peer) {
	defer func() {
		n.Lock()
		defer n.Unlock()
		delete(n.watchingPeers, p.Id())
	}()

	select {
	case <-p.CloseChan():
		select {
		case n.rebuildResponsiblePeers <- struct{}{}:
		default:
		}
	case <-n.ctx.Done():
		return
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
