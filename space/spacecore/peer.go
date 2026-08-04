package spacecore

import (
	"context"

	"storj.io/drpc"

	"github.com/anyproto/any-sync/commonspace/clientspaceproto"
	"github.com/anyproto/any-sync/net/transport"

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

// addSchema expands each discovered ip:port into explicit transport URLs.
// Local peers are dialed over yamux (TCP) only: a dead peer answers the dial
// with an RST in one RTT, while a QUIC dial to a dead port has to wait out the
// whole handshake timeout (quic-go has no ICMP/ECONNREFUSED handling on the
// unconnected sockets any-sync uses). The QUIC listener stays bound so
// not-yet-updated clients can still dial us; that inbound connection is
// bidirectional, so mixed versions keep syncing.
func (s *service) addSchema(addrs []string) (res []string) {
	res = make([]string, 0, len(addrs))
	for _, addr := range addrs {
		res = append(res, transport.Yamux+"://"+addr)
	}
	return res
}
