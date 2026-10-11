package spacecore

import (
	"maps"
	"net"
	"net/netip"
	"slices"
	"strconv"
	"time"

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
	// exchanged is the installed list at the last outbound exchange, when it
	// succeeded; nil otherwise
	exchanged []string
}

type linkRoute struct {
	hostPorts []string
	expiry    *time.Timer
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
	installed := composeAddrs(p.routes, p.base, linkLocalZone)
	s.setLocalPeerAddrs(peerId, installed)
	return installed
}

// exchangedWith reports whether the last outbound exchange with the peer
// succeeded over exactly the installed list.
func (s *service) exchangedWith(peerId string, installed []string) bool {
	p, ok := s.localPeers[peerId]
	return ok && slices.Equal(p.exchanged, installed)
}

// markExchanged records the list an outbound exchange ran over, nil when it
// failed. A peer forgotten meanwhile stays forgotten.
func (s *service) markExchanged(peerId string, installed []string) {
	if p, ok := s.localPeers[peerId]; ok {
		p.exchanged = slices.Clone(installed)
	}
}

// forgetLocalPeerAddrs drops everything known about the peer's addresses.
func (s *service) forgetLocalPeerAddrs(peerId string) {
	p, ok := s.localPeers[peerId]
	if !ok {
		return
	}
	for _, route := range p.routes {
		route.expiry.Stop()
	}
	delete(s.localPeers, peerId)
}

// expireRoute runs when a route went linkRouteTTL without being renewed. A
// live link-local connection to the peer renews it: a cable in use needs no
// new answer or exchange. Otherwise the route leaves the installed list.
func (s *service) expireRoute(peerId, source string, route *linkRoute) {
	// asked before taking localAddrsMu: Pick may wait on the pool
	live := liveLinkLocalConn(s, peerId)
	s.localAddrsMu.Lock()
	defer s.localAddrsMu.Unlock()
	p, ok := s.localPeers[peerId]
	if !ok || p.routes[source] != route {
		// renewed, dropped or forgotten meanwhile
		return
	}
	if live {
		route.expiry = afterFunc(linkRouteTTL, func() { s.expireRoute(peerId, source, route) })
		return
	}
	delete(p.routes, source)
	s.installLocalAddrs(peerId)
}

// composeAddrs is the dial list: the inbound route (the path the peer itself
// last used), then the cable routes by source, then the base list, each
// address once. A route address goes in zoned with the decimal index of its
// interface (zone, see linkLocalZone): the same cable named by an inbound
// connection and numbered by mDNS is then one address, and a name is not
// looked up again at dial time through Go's zone cache, which can be stale
// for a minute after a cable is plugged back in. A route whose interface is
// gone, down or has routable IPv4 now is left out. Without a route the base
// list goes as it is, as on develop, and no interface is looked up.
func composeAddrs(routes map[string]*linkRoute, base []string, zone func(string) (string, bool)) []string {
	if len(routes) == 0 {
		return slices.Clone(base)
	}
	// "inbound" sorts before every "mdns:" source
	sources := slices.Sorted(maps.Keys(routes))
	// one lookup per zone: each goes through the interface list
	indexes := map[string]string{}
	usable := func(z string) (string, bool) {
		index, ok := indexes[z]
		if !ok {
			index, _ = zone(z)
			indexes[z] = index
		}
		return index, index != ""
	}
	var res []string
	for _, source := range sources {
		for _, hostPort := range routes[source].hostPorts {
			if addr, ok := routeAddr(hostPort, usable); ok && !slices.Contains(res, addr) {
				res = append(res, addr)
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

// routeAddr is a route address as "[ip%index]:port"; ok is false when its
// zone names no usable interface.
func routeAddr(hostPort string, zone func(string) (string, bool)) (string, bool) {
	host, port, err := net.SplitHostPort(hostPort)
	if err != nil {
		return "", false
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return "", false
	}
	index, ok := zone(ip.Zone())
	if !ok {
		return "", false
	}
	return net.JoinHostPort(ip.WithZone(index).String(), port), true
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
