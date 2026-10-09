package localdiscovery

import (
	gonet "net"
	"net/netip"
	"runtime"
	"slices"
	"strings"

	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/net/addrs"
)

// GO-7580: the addresses a LAN peer is told to dial us at. Every address in
// the list is a dial the peer may spend its budget on, so the list carries
// only addresses a LAN peer can reach.

// sharedAddressSpace is 100.64.0.0/10 (RFC 6598): carrier-grade NAT, and the
// range overlay VPNs such as Tailscale assign. A LAN peer never reaches us there.
var sharedAddressSpace = netip.MustParsePrefix("100.64.0.0/10")

// advertisable reports whether an own address on iface belongs in the list a
// peer gets when none of our subnets contains it (and in the desktop mDNS
// records).
// On iOS only en* (Wi-Fi, USB/Thunderbolt Ethernet) and bridge* (Personal
// Hotspot) carry LAN traffic — the interfaces mDNSResponder itself advertises
// on. pdp_ip* is cellular (a 10.x carrier address that may even collide with
// the LAN's range), utun*/ipsec* are VPN tunnels, awdl*/llw* are AWDL; an
// interface whose name is unknown is rejected there for the same reason.
func advertisable(goos, iface string, ip netip.Addr) bool {
	if !matchable(goos, iface, ip) {
		return false
	}
	if goos == "ios" {
		return strings.HasPrefix(iface, "en") || strings.HasPrefix(iface, "bridge")
	}
	return true
}

// matchable reports whether an own address may be matched against a peer's
// subnet at all. Its mask says nothing for cellular: iOS often gives pdp_ip* a
// /8, which would "contain" a peer on a 10.x office LAN. The shared address
// space is never a LAN either. Everything else, VPN tunnels included, may
// match: a VPN that carries mDNS is a real path to the peer.
func matchable(goos, iface string, ip netip.Addr) bool {
	if sharedAddressSpace.Contains(ip.Unmap()) {
		return false
	}
	if goos == "ios" {
		// an address without an interface name may be cellular as well
		return iface != "" && !strings.HasPrefix(iface, "pdp_ip")
	}
	return true
}

// ipv4Prefix converts an interface address into the address together with its
// subnet length; ok is false for anything but IPv4.
func ipv4Prefix(addr gonet.Addr) (netip.Prefix, bool) {
	var (
		ip   gonet.IP
		mask gonet.IPMask
	)
	switch a := addr.(type) {
	case *gonet.IPNet:
		ip, mask = a.IP, a.Mask
	case *gonet.IPAddr:
		ip = a.IP
	default:
		return netip.Prefix{}, false
	}
	ip4 := ip.To4()
	if ip4 == nil {
		return netip.Prefix{}, false
	}
	ones, bits := mask.Size()
	switch bits {
	case 32:
	case 128:
		// an IPv4 mask in the 16-byte form
		ones -= 96
	default:
		// no usable mask: the address alone
		ones = 32
	}
	if ones < 0 || ones > 32 {
		ones = 32
	}
	return netip.PrefixFrom(netip.AddrFrom4([4]byte(ip4)), ones), true
}

// ownAddr is one IPv4 address of this device with its interface subnet.
// advertised says whether it goes into the list a peer gets when none of our
// subnets contains it.
type ownAddr struct {
	iface      string
	net        netip.Prefix
	matchable  bool
	advertised bool
}

// ownIPv4 lists this device's IPv4 addresses with their subnets, in interface
// order. Called under the embedding type's mutex.
func (l *discoveryBase) ownIPv4() []ownAddr {
	return ownIPv4(runtime.GOOS, l.interfacesAddrs)
}

func ownIPv4(goos string, ifaces addrs.InterfacesAddrs) []ownAddr {
	var own []ownAddr
	for i := range ifaces.Interfaces {
		iface := &ifaces.Interfaces[i]
		for _, addr := range iface.GetAddr() {
			if prefix, ok := ipv4Prefix(addr); ok {
				own = append(own, newOwnAddr(goos, iface.Name, prefix))
			}
		}
	}
	if len(own) > 0 {
		return own
	}

	// fallback in case the interfaces carry no ipv4 addresses: the flat
	// address list has no interface names, so only the address rules apply
	// (and nothing is advertised on iOS)
	byIP := map[netip.Addr]netip.Prefix{}
	var ips []gonet.IP
	for _, addr := range ifaces.Addrs {
		prefix, ok := ipv4Prefix(addr)
		if !ok {
			continue
		}
		if _, dup := byIP[prefix.Addr()]; dup {
			continue
		}
		byIP[prefix.Addr()] = prefix
		ips = append(ips, prefix.Addr().AsSlice())
	}
	ifaces.SortIPsLikeInterfaces(ips)
	for _, ip := range ips {
		addr, _ := netip.AddrFromSlice(ip)
		own = append(own, newOwnAddr(goos, "", byIP[addr]))
	}
	return own
}

func newOwnAddr(goos, iface string, prefix netip.Prefix) ownAddr {
	return ownAddr{
		iface:      iface,
		net:        prefix,
		matchable:  matchable(goos, iface, prefix.Addr()),
		advertised: advertisable(goos, iface, prefix.Addr()),
	}
}

// newOwnAddresses describes the listening endpoint: the matchable addresses
// are the candidates for the subnet match, the advertised ones form the
// fallback list.
func newOwnAddresses(list []ownAddr, port int) OwnAddresses {
	own := OwnAddresses{Port: port}
	for _, a := range list {
		if a.matchable {
			own.Nets = append(own.Nets, a.net)
		}
		if a.advertised {
			own.Addrs = append(own.Addrs, a.net.Addr().String())
		}
	}
	return own
}

// logOwnAddresses reports what a peer may be told about each interface, once
// per interface change.
func logOwnAddresses(list []ownAddr) {
	fields := make([]string, 0, len(list))
	for _, a := range list {
		verdict := "dropped"
		switch {
		case a.advertised:
			verdict = "advertised"
		case a.matchable:
			verdict = "match-only"
		}
		fields = append(fields, a.iface+" "+a.net.String()+" "+verdict)
	}
	log.Info("own lan addresses", zap.Strings("addrs", fields))
}

// For narrows the addresses to the ones on a subnet that contains one of the
// peer's addresses: a peer on our LAN needs nothing else, and every extra
// address is a dial it may waste. The match runs over every interface but
// cellular and the shared address space (see matchable) — a peer discovered
// over a VPN that carries mDNS gets our VPN address. A peer
// on none of our subnets (a routed LAN, a stale address) gets the advertised
// list, as does every peer when the subnets are unknown.
func (o OwnAddresses) For(peerIPs []netip.Addr) []string {
	if len(o.Nets) == 0 {
		return o.Addrs
	}
	var matched []string
	for _, n := range o.Nets {
		if slices.ContainsFunc(peerIPs, func(ip netip.Addr) bool { return n.Contains(ip.Unmap()) }) {
			matched = append(matched, n.Addr().String())
		}
	}
	if len(matched) == 0 {
		return o.Addrs
	}
	return matched
}
