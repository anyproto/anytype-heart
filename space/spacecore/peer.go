package spacecore

import (
	"context"

	"storj.io/drpc"

	"github.com/anyproto/any-sync/commonspace/clientspaceproto"

	"github.com/anyproto/anytype-heart/space/spacecore/clientserver"
	"github.com/anyproto/anytype-heart/space/spacecore/localdiscovery"
)

func (s *service) PeerDiscovered(ctx context.Context, peer localdiscovery.DiscoveredPeer, own localdiscovery.OwnAddresses) {
	s.peerService.SetPeerAddrs(peer.PeerId, s.addSchema(peer.Addrs))
	unaryPeer, err := s.poolManager.UnaryPeerPool().Get(ctx, peer.PeerId)
	if err != nil {
		return
	}
	allIds, err := s.spaceStorageProvider.AllSpaceIds()
	if err != nil {
		return
	}
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
	s.peerStore.UpdateLocalPeer(peer.PeerId, resp.SpaceIds)
}

func (s *service) addSchema(addrs []string) (res []string) {
	res = make([]string, 0, len(addrs))
	for _, addr := range addrs {
		res = append(res, clientserver.PreferredSchema+"://"+addr)
	}
	return res
}
