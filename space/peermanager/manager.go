package peermanager

import (
	"context"
	"github.com/anytypeio/any-sync/app/logger"
	"github.com/anytypeio/any-sync/commonspace/spacesyncproto"
	"github.com/anytypeio/any-sync/net/peer"
	"go.uber.org/zap"
	"golang.org/x/exp/slices"
)

type clientPeerManager struct {
	spaceId            string
	responsiblePeerIds []string
	p                  *provider
}

func (n *clientPeerManager) init() {
	n.responsiblePeerIds = n.p.nodeconf.GetLast().NodeIds(n.spaceId)
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
	if err != nil {
		return
	}
	return []peer.Peer{p}, nil
}

func (n *clientPeerManager) getStreamResponsiblePeers(ctx context.Context, exactId string) (peers []peer.Peer, err error) {
	if exactId == "" {
		// lookup in common pool for existing connection
		p, e := n.p.commonPool.GetOneOf(ctx, n.responsiblePeerIds)
		if e != nil {
			return nil, e
		}
		exactId = p.Id()
	}

	p, err := n.p.pool.Get(ctx, exactId)
	if err != nil {
		return
	}
	return []peer.Peer{p}, nil
}

func (n *clientPeerManager) isResponsible(peerId string) bool {
	return slices.Contains(n.responsiblePeerIds, peerId)
}
