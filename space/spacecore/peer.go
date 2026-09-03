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
	s.rememberOwn(own)
	shared, proof, err := s.exchangeOutbound(ctx, discovered.PeerId, &own)
	if err != nil {
		return
	}
	s.publishLocalPeer(discovered.PeerId, shared, proof)
}

// exchangeOutbound is the outbound handshake with one LAN peer: the v2 token
// exchange over the request list (disk plus the derived tech space), with the
// v1 fallback for peers that predate v2. own may be nil (re-exchange before
// local discovery reported our addresses): the peer then answers without
// recording us. The same function serves discovery and re-exchange, so the
// two cannot drift. proof is true for a v2 result: its tokens are keyed
// proofs, while v1 is a plaintext list that proves nothing.
func (s *service) exchangeOutbound(ctx context.Context, peerId string, own *localdiscovery.OwnAddresses) (shared []string, proof bool, err error) {
	unaryPeer, err := s.poolManager.UnaryPeerPool().Get(ctx, peerId)
	if err != nil {
		return nil, false, fmt.Errorf("get peer: %w", err)
	}
	allIds, err := s.exchangeRequestIds()
	if err != nil {
		return nil, false, err
	}
	var localServer *clientspaceproto.LocalServer
	if own != nil {
		localServer = localServerOf(*own)
	}
	shared, err = s.spaceExchangeV2(ctx, unaryPeer, peerId, allIds, localServer)
	if err != nil {
		// a peer that predates v2 fails the call; fall back so LAN discovery
		// keeps working across a mixed-version rollout
		log.Debug("space exchange v2, falling back to v1", zap.String("peerId", peerId), zap.Error(err))
		if shared, err = s.spaceExchangeV1(ctx, unaryPeer, allIds, localServer); err != nil {
			return nil, false, err
		}
		return shared, false, nil
	}
	return shared, true, nil
}

// spaceExchangeV2 runs the token handshake: send one membership token per
// space we can derive a discovery key for, padded and sorted so the true space
// count stays hidden, then match the peer's proof tokens back to our own
// spaces. Neither side ever transmits a space id.
func (s *service) spaceExchangeV2(ctx context.Context, unaryPeer peer.Peer, peerId string, allIds []string, localServer *clientspaceproto.LocalServer) ([]string, error) {
	nonce, err := clientspaceproto.NewNonceV2()
	if err != nil {
		return nil, fmt.Errorf("nonce: %w", err)
	}
	selfPeerId := s.wallet.GetDevicePrivkey().GetPublic().PeerId()
	keys := s.discoveryKeys.DiscoveryKeys(ctx, allIds)
	// we are the caller, so the pair is ordered (us, them)
	tokens, err := requestTokens(allIds, keys, nonce, selfPeerId, peerId)
	if err != nil {
		return nil, fmt.Errorf("build request tokens: %w", err)
	}
	var resp *clientspaceproto.SpaceExchangeV2Response
	err = unaryPeer.DoDrpc(ctx, func(conn drpc.Conn) error {
		var dErr error
		resp, dErr = clientspaceproto.NewDRPCClientSpaceClient(conn).SpaceExchangeV2(ctx, &clientspaceproto.SpaceExchangeV2Request{
			Nonce:       nonce,
			SpaceTokens: tokens,
			LocalServer: localServer,
		})
		return dErr
	})
	if err != nil {
		return nil, fmt.Errorf("space exchange v2: %w", err)
	}
	return matchProofs(allIds, keys, resp.SpaceTokens, nonce, selfPeerId, peerId), nil
}

// spaceExchangeV1 is the legacy plaintext handshake, kept only to reach peers
// that predate SpaceExchangeV2. It hands our full space id list to whoever
// asks, so it is never attempted before v2.
func (s *service) spaceExchangeV1(ctx context.Context, unaryPeer peer.Peer, allIds []string, localServer *clientspaceproto.LocalServer) ([]string, error) {
	var resp *clientspaceproto.SpaceExchangeResponse
	err := unaryPeer.DoDrpc(ctx, func(conn drpc.Conn) error {
		var dErr error
		resp, dErr = clientspaceproto.NewDRPCClientSpaceClient(conn).SpaceExchange(ctx, &clientspaceproto.SpaceExchangeRequest{
			SpaceIds:    allIds,
			LocalServer: localServer,
		})
		return dErr
	})
	if err != nil {
		return nil, fmt.Errorf("space exchange: %w", err)
	}
	return resp.SpaceIds, nil
}

// localServerOf describes this device's listening endpoint to a LAN peer.
func localServerOf(own localdiscovery.OwnAddresses) *clientspaceproto.LocalServer {
	return &clientspaceproto.LocalServer{
		Ips: own.Addrs,
		// our own drpc listener port, so always within uint16
		Port: int32(own.Port), // #nosec G115
	}
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
