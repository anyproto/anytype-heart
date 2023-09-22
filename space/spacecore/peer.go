package spacecore

import (
	"context"

	"go.uber.org/zap"
	"storj.io/drpc"

	"github.com/anyproto/anytype-heart/space/spacecore/clientserver"
	"github.com/anyproto/anytype-heart/space/spacecore/clientspaceproto"
	"github.com/anyproto/anytype-heart/space/spacecore/localdiscovery"
)

func (s *service) PeerDiscovered(peer localdiscovery.DiscoveredPeer, own localdiscovery.OwnAddresses) {
	s.peerService.SetPeerAddrs(peer.PeerId, s.addSchema(peer.Addrs))
	ctx := context.Background()
	unaryPeer, err := s.poolManager.UnaryPeerPool().Get(ctx, peer.PeerId)
	if err != nil {
		return
	}
	allIds, err := s.spaceStorageProvider.AllSpaceIds()
	if err != nil {
		return
	}
	log.Debug("sending info about spaces to peer", zap.String("peer", peer.PeerId), zap.Strings("spaces", allIds))
	var resp *clientspaceproto.SpaceExchangeResponse
	err = unaryPeer.DoDrpc(ctx, func(conn drpc.Conn) error {
		resp, err = clientspaceproto.NewDRPCClientSpaceClient(conn).SpaceExchange(ctx, &clientspaceproto.SpaceExchangeRequest{
			SpaceIds: allIds,
			LocalServer: &clientspaceproto.LocalServer{
				Ips:  own.Addrs,
				Port: int32(own.Port),
			},
		})
		return err
	})
	if err != nil {
		return
	}
	log.Debug("got peer ids from peer", zap.String("peer", peer.PeerId), zap.Strings("spaces", resp.SpaceIds))
	s.peerStore.UpdateLocalPeer(peer.PeerId, resp.SpaceIds)
}

func (s *service) addSchema(addrs []string) (res []string) {
	res = make([]string, 0, len(addrs))
	for _, addr := range addrs {
		res = append(res, clientserver.PreferredSchema+"://"+addr)
	}
	return res
}
