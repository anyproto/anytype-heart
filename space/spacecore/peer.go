package spacecore

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
	"storj.io/drpc"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace"
	"github.com/anyproto/any-sync/commonspace/clientspaceproto"
	"github.com/anyproto/any-sync/net/peer"
	"github.com/anyproto/any-sync/net/pool"
	"github.com/anyproto/any-sync/net/transport"

	"github.com/anyproto/anytype-heart/net/addrs"
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
	// under localAddrsMu: an inbound connection recorded between reading
	// the remembered link-local and installing the list must not be lost
	s.localAddrsMu.Lock()
	s.rememberDiscovered(discovered.PeerId, discovered.Addrs)
	s.setLocalPeerAddrs(discovered.PeerId, s.withInboundLinkLocal(discovered.PeerId, discovered.Addrs))
	s.localAddrsMu.Unlock()
	s.rememberOwn(own)
	shared, proof, err := s.exchangeOutbound(ctx, discovered.PeerId, &own)
	if err != nil {
		return
	}
	s.publishLocalPeer(discovered.PeerId, shared, proof, directionOutbound)
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
		localServer = localServerOf(*own, s.peerIPsOf(peerId, peer.CtxPeerAddr(unaryPeer.Context())))
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

// localServerOf describes this device's listening endpoint to a LAN peer
// known at peerIPs.
func localServerOf(own localdiscovery.OwnAddresses, peerIPs []netip.Addr) *clientspaceproto.LocalServer {
	return &clientspaceproto.LocalServer{
		Ips: own.For(peerIPs),
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

// setLocalPeerAddrs makes hostPorts ("ip:port") the addresses a LAN peer is
// dialed at, and remembers its IPs for the subnet match in localServerOf.
func (s *service) setLocalPeerAddrs(peerId string, hostPorts []string) {
	ips := make([]netip.Addr, 0, len(hostPorts))
	for _, hostPort := range hostPorts {
		if ip, ok := hostIP(hostPort); ok {
			ips = append(ips, ip)
		}
	}
	s.ownMu.Lock()
	if s.peerIPs == nil {
		s.peerIPs = map[string][]netip.Addr{}
	}
	if s.localAddrs == nil {
		s.localAddrs = map[string][]string{}
	}
	s.peerIPs[peerId] = ips
	s.localAddrs[peerId] = slices.Clone(hostPorts)
	s.ownMu.Unlock()
	// addSchema pins the transport for local peers (yamux); see its comment
	s.peerService.SetPeerAddrs(peerId, s.addSchema(hostPorts))
}

// peerIPsOf is the peer's known addresses plus the one the live connection
// to it uses (connAddr, from peer.CtxPeerAddr; empty when unknown).
func (s *service) peerIPsOf(peerId, connAddr string) []netip.Addr {
	s.ownMu.Lock()
	ips := slices.Clone(s.peerIPs[peerId])
	s.ownMu.Unlock()
	if ip, ok := hostIP(connAddr); ok && !slices.Contains(ips, ip) {
		ips = append(ips, ip)
	}
	return ips
}

func (s *service) forgetPeerIPs(peerId string) {
	s.ownMu.Lock()
	defer s.ownMu.Unlock()
	delete(s.peerIPs, peerId)
	delete(s.localAddrs, peerId)
	delete(s.discoveredAddrs, peerId)
	s.dropInboundLinkLocalLocked(peerId)
}

// inboundLinkLocalTTL bounds how long an inbound link-local address outlives
// the connection it came from without a newer one: the AAAA TTL heart's
// responder announces, so it never lingers longer than a discovered one.
const inboundLinkLocalTTL = 2 * time.Minute

// inboundLinkLocal is the link-local address a peer last connected from.
// expiry takes it out of the installed address list when it isn't renewed.
type inboundLinkLocal struct {
	hostPort string
	at       time.Time
	expiry   *time.Timer
}

// timeNow and afterFunc are seams for tests.
var (
	timeNow   = time.Now
	afterFunc = time.AfterFunc
)

// rememberInboundLinkLocal is called under localAddrsMu.
func (s *service) rememberInboundLinkLocal(peerId, hostPort string) {
	s.ownMu.Lock()
	defer s.ownMu.Unlock()
	if s.inboundLinkLocal == nil {
		s.inboundLinkLocal = map[string]*inboundLinkLocal{}
	}
	s.dropInboundLinkLocalLocked(peerId)
	remembered := &inboundLinkLocal{hostPort: hostPort, at: timeNow()}
	remembered.expiry = afterFunc(inboundLinkLocalTTL, func() { s.expireInboundLinkLocal(peerId, remembered) })
	s.inboundLinkLocal[peerId] = remembered
}

// forgetInboundLinkLocal is called under localAddrsMu.
func (s *service) forgetInboundLinkLocal(peerId string) {
	s.ownMu.Lock()
	defer s.ownMu.Unlock()
	s.dropInboundLinkLocalLocked(peerId)
}

func (s *service) dropInboundLinkLocalLocked(peerId string) {
	if remembered, ok := s.inboundLinkLocal[peerId]; ok {
		remembered.expiry.Stop()
		delete(s.inboundLinkLocal, peerId)
	}
}

// rememberDiscovered keeps the latest discovery list of a peer: discovery
// reports a peer again only when its addresses change, so this is still
// what discovery says when a remembered inbound address expires.
func (s *service) rememberDiscovered(peerId string, addrs []string) {
	s.ownMu.Lock()
	defer s.ownMu.Unlock()
	if s.discoveredAddrs == nil {
		s.discoveredAddrs = map[string][]string{}
	}
	s.discoveredAddrs[peerId] = slices.Clone(addrs)
}

// liveLinkLocalConn reports whether we hold a connection to the peer over an
// IPv6 link-local address, without dialing. A var so tests can stub it.
var liveLinkLocalConn = func(s *service, peerId string) bool {
	if s.poolManager == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	for _, p := range []pool.Pool{s.poolManager.UnaryPeerPool(), s.poolManager.StreamPeerPool()} {
		if pr, err := p.Pick(ctx, peerId); err == nil && isLinkLocalAddr(peer.CtxPeerAddr(pr.Context())) {
			return true
		}
	}
	return false
}

// expireInboundLinkLocal runs when a remembered address went unrenewed for
// inboundLinkLocalTTL. A live link-local connection to the peer renews it:
// exchanges, the only other renewal, run on discovery and on space list
// changes, not while a cable simply stays in use. Otherwise it leaves the
// installed list, not only the next discovery's, so it can't keep taking
// the dial budget first; the link-local addresses discovery last reported
// (the same route, perhaps zoned by index instead of name) take its place.
func (s *service) expireInboundLinkLocal(peerId string, remembered *inboundLinkLocal) {
	s.localAddrsMu.Lock()
	defer s.localAddrsMu.Unlock()
	s.ownMu.Lock()
	current := s.inboundLinkLocal[peerId]
	s.ownMu.Unlock()
	if current != remembered {
		// renewed or forgotten meanwhile
		return
	}
	if liveLinkLocalConn(s, peerId) {
		s.ownMu.Lock()
		remembered.at = timeNow()
		remembered.expiry = afterFunc(inboundLinkLocalTTL, func() { s.expireInboundLinkLocal(peerId, remembered) })
		s.ownMu.Unlock()
		return
	}
	s.ownMu.Lock()
	delete(s.inboundLinkLocal, peerId)
	installed, ok := s.localAddrs[peerId]
	discovered := slices.Clone(s.discoveredAddrs[peerId])
	s.ownMu.Unlock()
	if !ok || !slices.Contains(installed, remembered.hostPort) {
		return
	}
	rest := slices.DeleteFunc(slices.Clone(installed), func(a string) bool { return a == remembered.hostPort })
	var restored []string
	for _, a := range discovered {
		if isLinkLocalAddr(a) && !slices.Contains(rest, a) {
			restored = append(restored, a)
		}
	}
	s.setLocalPeerAddrs(peerId, append(restored, rest...))
}

// linkLocalUsable reports whether the interface a zone names (by name or
// decimal index) is up and has no routable IPv4 address: link-local is only
// used on such a link (GO-7581). It goes through addrs, which on Android
// asks the app (netlink is closed to apps there). A var so tests can stub it.
var linkLocalUsable = func(zone string) bool {
	ifaces, err := addrs.GetInterfacesAddrs()
	if err != nil {
		return false
	}
	index, errAtoi := strconv.Atoi(zone)
	for i := range ifaces.Interfaces {
		iface := &ifaces.Interfaces[i]
		if iface.Name != zone && (errAtoi != nil || iface.Index != index) {
			continue
		}
		return iface.Flags&net.FlagUp != 0 && !addrs.HasRoutableIPv4(iface)
	}
	return false
}

// withInboundLinkLocal puts the link-local address the peer last connected
// from in front of addrs when addrs has no link-local address of its own.
// A phone's native discovery resolves IPv4 only, so over a cable it finds
// nothing usable (or an IPv4 address the cable can't reach); without this
// it would replace the one address that works, recorded by recordLocalPeer.
// The address is dropped once it is older than inboundLinkLocalTTL and no
// link-local connection to the peer is live, its interface is gone, down or
// has gained routable IPv4, or the peer connected to us another way
// (recordLocalPeer).
func (s *service) withInboundLinkLocal(peerId string, addrs []string) []string {
	s.ownMu.Lock()
	remembered, ok := s.inboundLinkLocal[peerId]
	var (
		hostPort string
		at       time.Time
	)
	if ok {
		hostPort, at = remembered.hostPort, remembered.at
	}
	s.ownMu.Unlock()
	if !ok || slices.ContainsFunc(addrs, isLinkLocalAddr) {
		return addrs
	}
	ip, _ := hostIP(hostPort)
	expired := timeNow().Sub(at) > inboundLinkLocalTTL && !liveLinkLocalConn(s, peerId)
	if expired || !linkLocalUsable(ip.Zone()) {
		s.ownMu.Lock()
		if s.inboundLinkLocal[peerId] == remembered {
			s.dropInboundLinkLocalLocked(peerId)
		}
		s.ownMu.Unlock()
		return addrs
	}
	return append([]string{hostPort}, addrs...)
}

// isLinkLocalAddr reports whether hostPort is a dialable IPv6 link-local
// address; an IPv4 link-local one (169.254/16) is just another IPv4 address.
func isLinkLocalAddr(hostPort string) bool {
	ip, ok := hostIP(hostPort)
	return ok && ip.Is6() && ip.IsLinkLocalUnicast() && ip.Zone() != ""
}

// hostIP extracts the IP from "ip:port" or a transport URL such as
// "yamux://ip:port".
func hostIP(addr string) (netip.Addr, bool) {
	if _, rest, found := strings.Cut(addr, "://"); found {
		addr = rest
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return netip.Addr{}, false
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}, false
	}
	return ip.Unmap(), true
}
