package peermanager

import (
	"context"
	"fmt"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	//nolint:misspell
	"github.com/anyproto/any-sync/commonspace/peermanager"
	"github.com/anyproto/any-sync/commonspace/spacesyncproto"
	"github.com/anyproto/any-sync/net/peer"
	"go.uber.org/zap"
	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/space/spacecore/peerstore"
)

type clientPeerManager struct {
	spaceId            string
	responsibleNodeIds []string
	p                  *provider
	peerStore          peerstore.PeerStore
	sync.Mutex
}

func (n *clientPeerManager) GetNodePeers(ctx context.Context) (peers []peer.Peer, err error) {
	p, err := n.p.pool.GetOneOf(ctx, n.responsibleNodeIds)
	if err == nil {
		peers = []peer.Peer{p}
	}
	return
}

func (n *clientPeerManager) Init(_ *app.App) (err error) {
	n.responsibleNodeIds = n.peerStore.ResponsibleNodeIds(n.spaceId)
	return
}

func (n *clientPeerManager) Name() (name string) {
	return peermanager.CName
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
	p, err := n.p.pool.GetOneOf(ctx, n.responsibleNodeIds)
	if err == nil {
		peers = []peer.Peer{p}
	}
	log.Debug("local responsible peers are", zap.Strings("local peers", n.peerStore.LocalPeerIds(n.spaceId)))
	for _, peerId := range n.peerStore.LocalPeerIds(n.spaceId) {
		if slices.ContainsFunc(peers, func(p peer.Peer) bool { return p.Id() == peerId }) {
			continue
		}
		clientPeer, err := n.p.pool.Get(ctx, peerId)
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
