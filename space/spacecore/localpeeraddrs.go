package spacecore

import (
	"context"
	"maps"
	"net"
	"net/netip"
	"slices"
	"strconv"
	"time"

	"github.com/anyproto/any-sync/net/peer"
	"github.com/anyproto/any-sync/net/pool"

	"github.com/anyproto/anytype-heart/net/addrs"
)

// GO-7581: a LAN peer's dial list is put together from several sources.
// The base list is what develop installs: the latest IPv4 or native
// discovery, or the latest inbound exchange, whichever came last. A route is
// an IPv6 link-local address proven on a cable: the peer answered mDNS over
// it, or connected to us over it. Routes go first, each expiring unless
// renewed; with no route the installed list is exactly the base list.

// Route sources: the inbound connection, and mDNS per cable ("mdns:" and the
// decimal interface index, see localdiscovery.DiscoveredPeer.Link).
const (
	routeInbound = "inbound"
	routeMDNS    = "mdns:"
)

// linkRouteTTL: while the peer answers, the browser delivers a cable at least
// every two to three minutes (its redelivery cap, queries at most a minute
// apart), and an exchange renews the inbound route; missing more than one
// round ends a route.
const linkRouteTTL = 4 * time.Minute

// localPeerAddrs is what we know about how to dial one LAN peer.
type localPeerAddrs struct {
	base   []string
	routes map[string]*linkRoute
}

// linkRoute is one source's addresses, each "[ip%index]:port" with the
// decimal index of the interface it was proven on.
type linkRoute struct {
	hostPorts []string
	expiry    *time.Timer
}

// carries reports whether a live connection runs over one of the route's
// addresses (live as liveLinkLocalAddrs lists them).
func (r *linkRoute) carries(live []netip.Addr) bool {
	return slices.ContainsFunc(r.hostPorts, func(hostPort string) bool {
		ip, ok := hostIP(hostPort)
		return ok && slices.Contains(live, ip)
	})
}

// afterFunc is a seam for tests.
var afterFunc = time.AfterFunc

// localPeer is the peer's entry, created on first use. Called under
// localAddrsMu, like every method below but expireRoute.
func (s *service) localPeer(peerId string) *localPeerAddrs {
	if s.localPeers == nil {
		s.localPeers = map[string]*localPeerAddrs{}
	}
	p, ok := s.localPeers[peerId]
	if !ok {
		p = &localPeerAddrs{routes: map[string]*linkRoute{}}
		s.localPeers[peerId] = p
	}
	return p
}

// setBaseAddrs replaces the peer's base list.
func (s *service) setBaseAddrs(peerId string, hostPorts []string) {
	s.localPeer(peerId).base = slices.Clone(hostPorts)
}

// setRoute replaces one source's route and arms its expiry.
func (s *service) setRoute(peerId, source string, hostPorts []string) {
	p := s.localPeer(peerId)
	if old, ok := p.routes[source]; ok {
		old.expiry.Stop()
	}
	route := &linkRoute{hostPorts: slices.Clone(hostPorts)}
	route.expiry = afterFunc(linkRouteTTL, func() { s.expireRoute(peerId, source, route) })
	p.routes[source] = route
}

// dropRoute removes one source's route.
func (s *service) dropRoute(peerId, source string) {
	p, ok := s.localPeers[peerId]
	if !ok {
		return
	}
	if route, ok := p.routes[source]; ok {
		route.expiry.Stop()
		delete(p.routes, source)
	}
}

// installLocalAddrs makes the composed list the peer's dial list.
func (s *service) installLocalAddrs(peerId string) []string {
	p := s.localPeer(peerId)
	installed := composeAddrs(p.routes, p.base, func(index string) bool {
		_, ok := linkLocalZone(index)
		return ok
	})
	s.setLocalPeerAddrs(peerId, installed)
	return installed
}

// forgetLocalPeerAddrs drops everything known about the peer's addresses.
// Its route addresses leave the installed list, as no timer would take them
// out any more; a peer without routes keeps its list, as on develop.
func (s *service) forgetLocalPeerAddrs(peerId string) {
	p, ok := s.localPeers[peerId]
	if !ok {
		return
	}
	if len(p.routes) > 0 {
		s.setLocalPeerAddrs(peerId, p.base)
	}
	s.dropLocalPeer(peerId)
}

// dropLocalPeer stops the peer's route timers and drops its entry.
func (s *service) dropLocalPeer(peerId string) {
	for _, route := range s.localPeers[peerId].routes {
		route.expiry.Stop()
	}
	delete(s.localPeers, peerId)
}

// closeLocalPeerAddrs stops every route on service close; nothing is
// installed or armed after it.
func (s *service) closeLocalPeerAddrs() {
	s.localAddrsMu.Lock()
	defer s.localAddrsMu.Unlock()
	s.localAddrsClosed = true
	for peerId := range s.localPeers {
		s.dropLocalPeer(peerId)
	}
}

// expireRoute runs when a route went linkRouteTTL without being renewed. A
// live connection over one of its addresses renews it: a cable in use needs
// no new answer or exchange. Otherwise the route leaves. Either way the list
// is composed again, so a route whose interface became unusable leaves by
// its deadline.
func (s *service) expireRoute(peerId, source string, route *linkRoute) {
	// asked before taking localAddrsMu: Pick may wait on the pool, and a
	// zone name is looked up in the interface list
	live := liveLinkLocalAddrs(s, peerId)
	s.localAddrsMu.Lock()
	defer s.localAddrsMu.Unlock()
	p, ok := s.localPeers[peerId]
	if !ok || p.routes[source] != route {
		// renewed, dropped, forgotten or closed meanwhile
		return
	}
	if route.carries(live) {
		route.expiry = afterFunc(linkRouteTTL, func() { s.expireRoute(peerId, source, route) })
	} else {
		delete(p.routes, source)
	}
	s.installLocalAddrs(peerId)
}

// liveLinkLocalAddrs lists the IPv6 link-local addresses the pooled
// connections to the peer run over, zoned by the decimal index of a usable
// interface, without dialing. Called outside localAddrsMu. A var so tests
// can stub it.
//
// Pick returns the incoming connection first, and the pool lists no other:
// a live outgoing cable connection hidden behind an incoming one does not
// renew its route. The peer is then dialed over the rest of its list (the
// incoming connection itself stays in use), and the next cable discovery
// puts the route back.
var liveLinkLocalAddrs = pooledLinkLocalAddrs

func pooledLinkLocalAddrs(s *service, peerId string) []netip.Addr {
	if s.poolManager == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var live []netip.Addr
	for _, p := range []pool.Pool{s.poolManager.UnaryPeerPool(), s.poolManager.StreamPeerPool()} {
		pr, err := p.Pick(ctx, peerId)
		if err != nil {
			continue
		}
		addr := peer.CtxPeerAddr(pr.Context())
		if !isLinkLocalAddr(addr) {
			continue
		}
		// the connection's zone is the name Go gave its interface index
		// when it was set up; resolved here, while the connection is live
		ip, _ := hostIP(addr)
		index, ok := linkLocalZone(ip.Zone())
		if !ok {
			continue
		}
		if ip = ip.WithZone(index); !slices.Contains(live, ip) {
			live = append(live, ip)
		}
	}
	return live
}

// composeAddrs is the dial list: the inbound route (the path the peer itself
// last used), then the cable routes by source, then the base list, each
// address once. A route address is zoned with the decimal index of the
// interface it was proven on; it is left out once usable says that interface
// is gone, down or has routable IPv4. A zone name is never looked up here: a
// name taken over by another cable would move the route there. Without a
// route the base list goes as it is, as on develop, and no interface is
// looked up.
func composeAddrs(routes map[string]*linkRoute, base []string, usable func(index string) bool) []string {
	if len(routes) == 0 {
		return slices.Clone(base)
	}
	// "inbound" sorts before every "mdns:" source
	sources := slices.Sorted(maps.Keys(routes))
	// one lookup per interface: each goes through the interface list
	checked := map[string]bool{}
	usableOnce := func(index string) bool {
		ok, seen := checked[index]
		if !seen {
			ok = usable(index)
			checked[index] = ok
		}
		return ok
	}
	var res []string
	for _, source := range sources {
		for _, hostPort := range routes[source].hostPorts {
			if routeUsable(hostPort, usableOnce) && !slices.Contains(res, hostPort) {
				res = append(res, hostPort)
			}
		}
	}
	for _, hostPort := range base {
		if !slices.Contains(res, hostPort) {
			res = append(res, hostPort)
		}
	}
	return res
}

// routeUsable reports whether a route address is zoned with the decimal index
// of a usable interface.
func routeUsable(hostPort string, usable func(index string) bool) bool {
	ip, ok := hostIP(hostPort)
	if !ok {
		return false
	}
	index, err := strconv.Atoi(ip.Zone())
	return err == nil && strconv.Itoa(index) == ip.Zone() && usable(ip.Zone())
}

// linkLocalZone resolves a zone (an interface name or decimal index) to the
// decimal index of an interface that is up and has no routable IPv4 address:
// link-local is used only on such a link (see addrs.HasRoutableIPv4). It goes
// through addrs, which on Android asks the app (netlink is closed to apps
// there). A var so tests can stub it.
var linkLocalZone = func(zone string) (string, bool) {
	ifaces, err := addrs.GetInterfacesAddrs()
	if err != nil {
		return "", false
	}
	return usableLinkZone(ifaces.Interfaces, zone)
}

func usableLinkZone(ifaces []addrs.NetInterfaceWithAddrCache, zone string) (string, bool) {
	index, errAtoi := strconv.Atoi(zone)
	for i := range ifaces {
		iface := &ifaces[i]
		if iface.Name != zone && (errAtoi != nil || iface.Index != index) {
			continue
		}
		if iface.Flags&net.FlagUp == 0 || addrs.HasRoutableIPv4(iface) {
			return "", false
		}
		return strconv.Itoa(iface.Index), true
	}
	return "", false
}
