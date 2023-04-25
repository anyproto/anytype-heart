package peermanager

import (
	"context"
	"fmt"
	"github.com/anytypeio/any-sync/app/logger"
	"github.com/anytypeio/any-sync/commonspace/spacesyncproto"
	"github.com/anytypeio/any-sync/net/peer"
	"github.com/anytypeio/go-anytype-middleware/space/peerstore"
	"go.uber.org/zap"
	"golang.org/x/exp/slices"
	"sync"
)

type clientPeerManager struct {
	spaceId            string
	responsiblePeerIds []string
	p                  *provider
	peerStore          peerstore.PeerStore
	sync.Mutex
}

func (n *clientPeerManager) init() {
	n.responsiblePeerIds = n.peerStore.ResponsibleNodeIds(n.spaceId)
}

func (n *clientPeerManager) SendPeer(ctx context.Context, peerId string, msg *spacesyncproto.ObjectSyncMessage) (err error) {
	ctx = logger.CtxWithFields(context.Background(), logger.CtxGetFields(ctx)...)
	return n.p.streamPool.Send(ctx, msg, func(ctx context.Context) (peers []peer.Peer, err error) {
		return n.getExactPeer(ctx, peerId)
	})
}

func (n *clientPeerManager) Broadcast(ctx context.Context, msg *spacesyncproto.ObjectSyncMessage) (err error) {
	ctx = logger.CtxWithFields(context.Background(), logger.CtxGetFields(ctx)...)
	return n.p.streamPool.Send(ctx, msg, func(ctx context.Context) (peers []peer.Peer, err error) {
		return n.getStreamResponsiblePeers(ctx)
	})
}

func (n *clientPeerManager) GetResponsiblePeers(ctx context.Context) (peers []peer.Peer, err error) {
	p, err := n.p.commonPool.GetOneOf(ctx, n.responsiblePeerIds)
	if err == nil {
		peers = []peer.Peer{p}
	}
	log.Debug("local responsible peers are", zap.Strings("local peers", n.peerStore.LocalPeerIds(n.spaceId)))
	for _, peerId := range n.peerStore.LocalPeerIds(n.spaceId) {
		if slices.ContainsFunc(peers, func(p peer.Peer) bool { return p.Id() == peerId }) {
			continue
		}
		clientPeer, err := n.p.commonPool.Get(ctx, peerId)
		if err != nil {
			log.Debug("removing peer", zap.String("peerId", peerId), zap.Error(err))
			n.peerStore.RemoveLocalPeer(peerId)
			continue
		}
		peers = append(peers, clientPeer)
	}
	if err != nil && len(peers) > 0 {
		err = nil
	}
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
	p, nodeErr := n.p.commonPool.GetOneOf(ctx, n.responsiblePeerIds)
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
