package spacecore

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	"storj.io/drpc"

	"github.com/anyproto/any-sync/commonspace/clientspaceproto"
	"github.com/anyproto/any-sync/net/peer"
	"github.com/anyproto/any-sync/net/transport"

	"github.com/anyproto/anytype-heart/space/spacecore/localdiscovery"
)

func (s *service) PeerDiscovered(ctx context.Context, discovered localdiscovery.DiscoveredPeer, own localdiscovery.OwnAddresses) {
	s.peerService.SetPeerAddrs(discovered.PeerId, s.addSchema(discovered.Addrs))
	unaryPeer, err := s.poolManager.UnaryPeerPool().Get(ctx, discovered.PeerId)
	if err != nil {
		return
	}
	allIds, err := s.spaceStorageProvider.AllSpaceIds()
	if err != nil {
		return
	}
	shared, err := s.spaceExchangeV2(ctx, unaryPeer, discovered.PeerId, allIds, own)
	if err != nil {
		// a peer that predates v2 fails the call; fall back so LAN discovery
		// keeps working across a mixed-version rollout
		log.Debug("space exchange v2, falling back to v1", zap.String("peerId", discovered.PeerId), zap.Error(err))
		if shared, err = s.spaceExchangeV1(ctx, unaryPeer, allIds, own); err != nil {
			return
		}
	}
	s.peerStore.UpdateLocalPeer(discovered.PeerId, shared)
}

// spaceExchangeV2 runs the token handshake: send one membership token per
// space we can derive a discovery key for, padded and sorted so the true space
// count stays hidden, then match the peer's proof tokens back to our own
// spaces. Neither side ever transmits a space id.
func (s *service) spaceExchangeV2(ctx context.Context, unaryPeer peer.Peer, peerId string, allIds []string, own localdiscovery.OwnAddresses) ([]string, error) {
	nonce, err := clientspaceproto.NewNonceV2()
	if err != nil {
		return nil, fmt.Errorf("nonce: %w", err)
	}
	selfPeerId := s.wallet.GetDevicePrivkey().GetPublic().PeerId()
	keys := s.discoveryKeys.DiscoveryKeys(ctx, allIds)
	tokens := make([][]byte, 0, len(keys))
	for _, spaceId := range allIds {
		if key, ok := keys[spaceId]; ok {
			tokens = append(tokens, clientspaceproto.RequestTokenV2(key, nonce, selfPeerId, peerId))
		}
	}
	tokens, err = clientspaceproto.PadTokensV2(tokens)
	if err != nil {
		return nil, fmt.Errorf("pad tokens: %w", err)
	}
	var resp *clientspaceproto.SpaceExchangeV2Response
	err = unaryPeer.DoDrpc(ctx, func(conn drpc.Conn) error {
		var dErr error
		resp, dErr = clientspaceproto.NewDRPCClientSpaceClient(conn).SpaceExchangeV2(ctx, &clientspaceproto.SpaceExchangeV2Request{
			Nonce:       nonce,
			SpaceTokens: tokens,
			LocalServer: &clientspaceproto.LocalServer{
				Ips:  own.Addrs,
				Port: int32(own.Port),
			},
		})
		return dErr
	})
	if err != nil {
		return nil, fmt.Errorf("space exchange v2: %w", err)
	}
	received := make(map[string]struct{}, len(resp.SpaceTokens))
	for _, token := range resp.SpaceTokens {
		received[string(token)] = struct{}{}
	}
	var shared []string
	for _, spaceId := range allIds {
		key, ok := keys[spaceId]
		if !ok {
			continue
		}
		if _, ok = received[string(clientspaceproto.ResponseTokenV2(key, nonce, selfPeerId, peerId))]; ok {
			shared = append(shared, spaceId)
		}
	}
	return shared, nil
}

// spaceExchangeV1 is the legacy plaintext handshake, kept only to reach peers
// that predate SpaceExchangeV2. It hands our full space id list to whoever
// asks, so it is never attempted before v2.
func (s *service) spaceExchangeV1(ctx context.Context, unaryPeer peer.Peer, allIds []string, own localdiscovery.OwnAddresses) ([]string, error) {
	var resp *clientspaceproto.SpaceExchangeResponse
	err := unaryPeer.DoDrpc(ctx, func(conn drpc.Conn) error {
		var dErr error
		resp, dErr = clientspaceproto.NewDRPCClientSpaceClient(conn).SpaceExchange(ctx, &clientspaceproto.SpaceExchangeRequest{
			SpaceIds: allIds,
			LocalServer: &clientspaceproto.LocalServer{
				Ips:  own.Addrs,
				Port: int32(own.Port),
			},
		})
		return dErr
	})
	if err != nil {
		return nil, fmt.Errorf("space exchange: %w", err)
	}
	return resp.SpaceIds, nil
}

// addSchema expands each discovered ip:port into explicit transport URLs.
// Local peers are dialed over yamux (TCP) only: a live host whose app has
// quit answers the dial with an RST in one RTT, while a QUIC dial to a dead
// port has to wait out the whole handshake timeout (quic-go has no
// ICMP/ECONNREFUSED handling on the unconnected sockets any-sync uses). A
// host that left the network answers neither transport — that case still
// costs the full dial timeout. The QUIC listener stays bound so
// not-yet-updated clients can still dial us; that inbound connection is
// bidirectional, so mixed versions keep syncing.
func (s *service) addSchema(addrs []string) (res []string) {
	res = make([]string, 0, len(addrs))
	for _, addr := range addrs {
		res = append(res, transport.Yamux+"://"+addr)
	}
	return res
}
