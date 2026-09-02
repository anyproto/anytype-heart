package spacecore

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	"storj.io/drpc"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace"
	"github.com/anyproto/any-sync/commonspace/clientspaceproto"
	"github.com/anyproto/any-sync/net/peer"
	"github.com/anyproto/any-sync/net/transport"

	"github.com/anyproto/anytype-heart/space/spacecore/localdiscovery"
)

// PeerDiscoveryObserver receives every LAN discovery, before the peer is
// dialed. The recovery status tracker (core.recovery) implements it; this
// package owns the single localdiscovery.SetNotifier slot, so the forward
// lives here rather than competing for it — Android's SetNotifierProvider
// path lands here too. Looked up by name because the tracker lives above this
// package; a drift test keeps the constant equal to recovery.CName.
type PeerDiscoveryObserver interface {
	OnLocalPeerDiscovered(peerId string, addrs []string)
}

const recoveryCName = "core.recovery"

func lookupPeerDiscoveryObserver(a *app.App) PeerDiscoveryObserver {
	c := a.Component(recoveryCName)
	if c == nil {
		return nil
	}
	observer, _ := c.(PeerDiscoveryObserver)
	return observer
}

// lookupPullObserver resolves the same tracker as a commonspace.PullObserver;
// loadSpace injects it into every space's Deps (tech, personal, shareable,
// streamable, one-to-one all route through it).
func lookupPullObserver(a *app.App) commonspace.PullObserver {
	c := a.Component(recoveryCName)
	if c == nil {
		return nil
	}
	observer, _ := c.(commonspace.PullObserver)
	return observer
}

func (s *service) PeerDiscovered(ctx context.Context, discovered localdiscovery.DiscoveredPeer, own localdiscovery.OwnAddresses) {
	if s.peerDiscoveryObserver != nil {
		s.peerDiscoveryObserver.OnLocalPeerDiscovered(discovered.PeerId, discovered.Addrs)
	}
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
			LocalServer: localServerOf(own),
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
func (s *service) spaceExchangeV1(ctx context.Context, unaryPeer peer.Peer, allIds []string, own localdiscovery.OwnAddresses) ([]string, error) {
	var resp *clientspaceproto.SpaceExchangeResponse
	err := unaryPeer.DoDrpc(ctx, func(conn drpc.Conn) error {
		var dErr error
		resp, dErr = clientspaceproto.NewDRPCClientSpaceClient(conn).SpaceExchange(ctx, &clientspaceproto.SpaceExchangeRequest{
			SpaceIds:    allIds,
			LocalServer: localServerOf(own),
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
