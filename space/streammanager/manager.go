package streammanager

import (
	"context"
	"github.com/anytypeio/any-sync/app/logger"
	"github.com/anytypeio/any-sync/commonspace/spacesyncproto"
	"github.com/anytypeio/any-sync/net/peer"
	"go.uber.org/zap"
	"golang.org/x/exp/slices"
)

type clientStreamManager struct {
	spaceId            string
	responsiblePeerIds []string
	p                  *provider
}

func (n *clientStreamManager) init() {
	n.responsiblePeerIds = n.p.nodeconf.GetLast().NodeIds(n.spaceId)
}

func (n *clientStreamManager) SendPeer(ctx context.Context, peerId string, msg *spacesyncproto.ObjectSyncMessage) (err error) {
	ctx = logger.CtxWithFields(context.Background(), logger.CtxGetFields(ctx)...)
	if n.isResponsible(peerId) {
		var p peer.Peer
		p, err = n.p.pool.Get(ctx, peerId)
		if err != nil {
			return
		}
		return n.p.streamPool.Send(ctx, msg, p)
	}
	return n.p.streamPool.SendById(ctx, msg, peerId)
}

func (n *clientStreamManager) SendResponsible(ctx context.Context, msg *spacesyncproto.ObjectSyncMessage) (err error) {
	ctx = logger.CtxWithFields(context.Background(), logger.CtxGetFields(ctx)...)
	p, err := n.getResponsiblePeer(ctx)
	if err != nil {
		return
	}
	return n.p.streamPool.Send(ctx, msg, p)
}

func (n *clientStreamManager) Broadcast(ctx context.Context, msg *spacesyncproto.ObjectSyncMessage) (err error) {
	ctx = logger.CtxWithFields(context.Background(), logger.CtxGetFields(ctx)...)
	if e := n.SendResponsible(ctx, msg); e != nil {
		log.Info("broadcast sendResponsible error", zap.Error(e))
	}
	return n.p.streamPool.Broadcast(ctx, msg, n.spaceId)
}

func (n *clientStreamManager) getResponsiblePeer(ctx context.Context) (p peer.Peer, err error) {
	return n.p.pool.GetOneOf(ctx, n.responsiblePeerIds)
}

func (n *clientStreamManager) isResponsible(peerId string) bool {
	return slices.Contains(n.responsiblePeerIds, peerId)
}
