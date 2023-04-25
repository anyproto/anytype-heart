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
	if n.isResponsible(peerId) {
		return n.p.streamPool.Send(ctx, msg, func(ctx context.Context) (peers []peer.Peer, err error) {
			return n.getStreamResponsiblePeers(ctx, peerId)
		})
	}
	return n.p.streamPool.SendById(ctx, msg, peerId)
}

func (n *clientPeerManager) SendResponsible(ctx context.Context, msg *spacesyncproto.ObjectSyncMessage) (err error) {
	ctx = logger.CtxWithFields(context.Background(), logger.CtxGetFields(ctx)...)
	return n.p.streamPool.Send(ctx, msg, func(ctx context.Context) (peers []peer.Peer, err error) {
		return n.getStreamResponsiblePeers(ctx, "")
	})
}

func (n *clientPeerManager) Broadcast(ctx context.Context, msg *spacesyncproto.ObjectSyncMessage) (err error) {
	ctx = logger.CtxWithFields(context.Background(), logger.CtxGetFields(ctx)...)
	if e := n.SendResponsible(ctx, msg); e != nil {
		log.Info("broadcast sendResponsible error", zap.Error(e))
	}
	return n.p.streamPool.Broadcast(ctx, msg, n.spaceId)
}

func (n *clientPeerManager) GetResponsiblePeers(ctx context.Context) (peers []peer.Peer, err error) {
	p, err := n.p.commonPool.GetOneOf(ctx, n.responsiblePeerIds)
	if err == nil {
		peers = []peer.Peer{p}
	}
	for _, peerId := range n.peerStore.LocalPeerIds(n.spaceId) {
		if err != nil || slices.ContainsFunc(peers, func(p peer.Peer) bool { return p.Id() == peerId }) {
			continue
		}
		clientPeer, err := n.p.commonPool.Get(ctx, peerId)
		if err != nil {
			continue
		}
		peers = append(peers, clientPeer)
	}
	if err != nil && len(peers) > 0 {
		err = nil
	}
	return
}

func (n *clientPeerManager) getStreamResponsiblePeers(ctx context.Context, exactId string) (peers []peer.Peer, err error) {
	if exactId != "" {
		p, err := n.p.pool.Get(ctx, exactId)
		if err != nil {
			return
		}
		return []peer.Peer{p}, nil
	}

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

func (n *clientPeerManager) isResponsible(peerId string) bool {
	return slices.Contains(n.responsiblePeerIds, peerId)
}
