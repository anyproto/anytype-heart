package peermanager

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	//nolint:misspell
	"github.com/anyproto/any-sync/commonspace/peermanager"
	"github.com/anyproto/any-sync/commonspace/spacesyncproto"
	"github.com/anyproto/any-sync/net/peer"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/space/spacecore/peerstore"
)

type clientPeerManager struct {
	spaceId            string
	responsibleNodeIds []string
	p                  *provider
	peerStore          peerstore.PeerStore

	responsiblePeers          []peer.Peer
	watchingPeers             map[string]struct{}
	rebuildResponsiblePeers   chan struct{}
	availableResponsiblePeers chan struct{}

	ctx       context.Context
	ctxCancel context.CancelFunc
	sync.Mutex
}

func (n *clientPeerManager) Init(_ *app.App) (err error) {
	n.responsibleNodeIds = n.peerStore.ResponsibleNodeIds(n.spaceId)
	n.ctx, n.ctxCancel = context.WithCancel(context.Background())
	n.rebuildResponsiblePeers = make(chan struct{}, 1)
	n.watchingPeers = make(map[string]struct{})
	n.availableResponsiblePeers = make(chan struct{})
	go n.manageResponsiblePeers()
	return
}

func (n *clientPeerManager) Name() (name string) {
	return peermanager.CName
}

func (n *clientPeerManager) Run(ctx context.Context) (err error) {
	return
}

func (n *clientPeerManager) GetNodePeers(ctx context.Context) (peers []peer.Peer, err error) {
	p, err := n.p.pool.GetOneOf(ctx, n.responsibleNodeIds)
	if err == nil {
		peers = []peer.Peer{p}
	}
	return
}

func (n *clientPeerManager) SendPeer(ctx context.Context, peerId string, msg *spacesyncproto.ObjectSyncMessage) (err error) {
	// TODO: peer manager will be changed to not have this possibility
	// use context.Background()
	//
	// explanation:
	// the context which comes here should not be used. It can be cancelled and thus kill the stream,
	// because the stream will be opened with this context
	ctx = logger.CtxWithFields(context.Background(), logger.CtxGetFields(ctx)...)
	return n.p.streamPool.Send(ctx, msg, func(ctx context.Context) (peers []peer.Peer, err error) {
		return n.getExactPeer(ctx, peerId)
	})
}

func (n *clientPeerManager) Broadcast(ctx context.Context, msg *spacesyncproto.ObjectSyncMessage) (err error) {
	// the context which comes here should not be used. It can be cancelled and thus kill the stream,
	// because the stream can be opened with this context
	ctx = logger.CtxWithFields(context.Background(), logger.CtxGetFields(ctx)...)
	return n.p.streamPool.Send(ctx, msg, func(ctx context.Context) (peers []peer.Peer, err error) {
		return n.getStreamResponsiblePeers(ctx)
	})
}

func (n *clientPeerManager) GetResponsiblePeers(ctx context.Context) (peers []peer.Peer, err error) {
	n.Lock()
	if len(n.responsiblePeers) == 0 {
		if n.availableResponsiblePeers == nil {
			n.availableResponsiblePeers = make(chan struct{})
		}
		ch := n.availableResponsiblePeers
		n.Unlock()
		select {
		case <-ch:
			return n.GetResponsiblePeers(ctx)
		case <-ctx.Done():
			return nil, ctx.Err()
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
	p, nodeErr := n.p.pool.GetOneOf(ctx, n.responsibleNodeIds)
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
		case <-time.After(time.Minute):
		case <-n.rebuildResponsiblePeers:
		case <-n.ctx.Done():
			return
		}
	}
}

func (n *clientPeerManager) fetchResponsiblePeers() {
	var peers []peer.Peer
	p, err := n.p.pool.GetOneOf(n.ctx, n.responsibleNodeIds)
	if err == nil {
		peers = []peer.Peer{p}
	} else {
		log.Info("can't get node peers", zap.Error(err))
	}

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

func (n *clientPeerManager) Close(ctx context.Context) (err error) {
	n.ctxCancel()
	return
}
