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
	go n.peerToPeerStatus.RegisterSpace(n.spaceId)
	go n.manageResponsiblePeers()
	return
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
	if len(n.responsiblePeers) == 0 {
		deadline, _ := ctx.Value(ContextPeerFindDeadlineKey).(time.Time)
		if n.availableResponsiblePeers == nil {
			n.availableResponsiblePeers = make(chan struct{})
		}
		ch := n.availableResponsiblePeers
		n.Unlock()
		if !deadline.IsZero() {
			if time.Now().After(deadline) {
				return nil, ErrPeerFindDeadlineExceeded
			}
			select {
			case <-ch:
				return n.GetResponsiblePeers(ctx)
			case <-time.After(time.Until(deadline)):
				return nil, ErrPeerFindDeadlineExceeded
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		} else {
			select {
			case <-ch:
				return n.GetResponsiblePeers(ctx)
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}
	peers = n.responsiblePeers
	n.Unlock()
	log.Debug("get responsible peers", zap.Int("peerCount", len(peers)), zap.String("spaceId", n.spaceId))
	return
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
		select {
		case <-time.After(time.Second * 20):
		case <-n.rebuildResponsiblePeers:
		case <-n.ctx.Done():
			return
		}
	}
}

func (n *clientPeerManager) fetchResponsiblePeers() {
	var peers []peer.Peer
	p, err := n.p.pool.GetOneOf(n.ctx, n.getNodeIds())
	if err == nil {
		peers = []peer.Peer{p}
		n.nodeStatus.SetNodesStatus(n.spaceId, nodestatus.Online)
	} else {
		log.Info("can't get node peers", zap.Error(err))
		n.nodeStatus.SetNodesStatus(n.spaceId, nodestatus.ConnectionError)
	}
	n.spaceSyncService.Refresh(n.spaceId)
	peerIds := n.peerStore.LocalPeerIds(n.spaceId)
	for _, peerId := range peerIds {
		p, err := n.p.pool.Get(n.ctx, peerId)
		if err != nil {
			n.peerStore.RemoveLocalPeer(peerId)
			log.Warn("failed to get local from net pool", zap.String("peerId", peerId), zap.Error(err))
			continue
		}
		peers = append(peers, p)
	}

	n.Lock()
	defer n.Unlock()

	for _, p = range peers {
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
	n.ctxCancel()
	n.peerToPeerStatus.UnregisterSpace(n.spaceId)
	return
}
