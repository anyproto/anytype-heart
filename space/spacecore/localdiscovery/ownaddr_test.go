package localdiscovery

import (
	"net"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/net/addrs"
)

func ipNet(cidr string) *net.IPNet {
	ip, n, err := net.ParseCIDR(cidr)
	if err != nil {
		panic(err)
	}
	n.IP = ip.To4()
	return n
}

func iface(name string, cidrs ...string) addrs.NetInterfaceWithAddrCache {
	list := make([]net.Addr, 0, len(cidrs))
	for _, c := range cidrs {
		list = append(list, ipNet(c))
	}
	return addrs.WrapInterfaceWithAddrs(net.Interface{Name: name}, list)
}

func prefixes(cidrs ...string) []netip.Prefix {
	res := make([]netip.Prefix, 0, len(cidrs))
	for _, c := range cidrs {
		res = append(res, netip.MustParsePrefix(c))
	}
	return res
}

func TestAdvertisable(t *testing.T) {
	for _, tc := range []struct {
		name  string
		goos  string
		iface string
		ip    string
		want  bool
	}{
		{"ios wi-fi", "ios", "en0", "192.168.1.62", true},
		{"ios wired adapter", "ios", "en2", "192.168.1.63", true},
		{"ios personal hotspot", "ios", "bridge100", "172.20.10.1", true},
		{"ios cellular", "ios", "pdp_ip0", "10.179.12.4", false},
		{"ios vpn", "ios", "utun3", "10.8.0.2", false},
		{"ios ipsec", "ios", "ipsec0", "10.9.0.2", false},
		{"ios awdl", "ios", "awdl0", "169.254.10.1", false},
		{"ios unknown interface", "ios", "", "192.168.1.62", false},
		{"desktop tailscale", "darwin", "utun4", "100.101.102.103", false},
		{"desktop shared address space edge", "linux", "eth0", "100.127.255.255", false},
		{"desktop just outside shared address space", "linux", "eth0", "100.128.0.1", true},
		{"desktop vpn keeps a private address", "darwin", "utun4", "10.8.0.2", true},
		{"android wi-fi", "android", "wlan0", "192.168.1.70", true},
		{"windows unnamed", "windows", "", "192.168.1.80", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, advertisable(tc.goos, tc.iface, netip.MustParseAddr(tc.ip)))
		})
	}
}

func TestIPv4Prefix(t *testing.T) {
	for _, tc := range []struct {
		name   string
		addr   net.Addr
		want   netip.Prefix
		wantOk bool
	}{
		{"4-byte mask", ipNet("192.168.1.5/24"), netip.MustParsePrefix("192.168.1.5/24"), true},
		{"16-byte mask", &net.IPNet{IP: net.ParseIP("10.0.0.7"), Mask: net.CIDRMask(96+16, 128)}, netip.MustParsePrefix("10.0.0.7/16"), true},
		{"bare address", &net.IPAddr{IP: net.ParseIP("10.0.0.7")}, netip.MustParsePrefix("10.0.0.7/32"), true},
		{"ipv6", ipNetV6("fe80::1/64"), netip.Prefix{}, false},
		{"not an ip", &net.UnixAddr{Name: "x"}, netip.Prefix{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ipv4Prefix(tc.addr)
			assert.Equal(t, tc.wantOk, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}

func ipNetV6(cidr string) *net.IPNet {
	ip, n, err := net.ParseCIDR(cidr)
	if err != nil {
		panic(err)
	}
	n.IP = ip
	return n
}

func TestMatchable(t *testing.T) {
	for _, tc := range []struct {
		name  string
		goos  string
		iface string
		ip    string
		want  bool
	}{
		{"ios wi-fi", "ios", "en0", "192.168.1.62", true},
		{"ios vpn carrying mdns", "ios", "utun3", "10.8.0.2", true},
		{"ios cellular with a /8", "ios", "pdp_ip0", "10.179.12.4", false},
		{"ios unknown interface", "ios", "", "192.168.1.62", false},
		{"desktop vpn", "darwin", "utun8", "10.8.0.2", true},
		{"desktop tailscale", "darwin", "utun4", "100.101.102.103", false},
		{"windows unnamed", "windows", "", "192.168.1.80", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, matchable(tc.goos, tc.iface, netip.MustParseAddr(tc.ip)))
		})
	}
}

func TestOwnIPv4(t *testing.T) {
	t.Run("ios advertises wi-fi and hotspot, matches the vpn, drops cellular", func(t *testing.T) {
		// given: the order iOS enumerates them in, cellular first
		ifaces := addrs.InterfacesAddrs{Interfaces: []addrs.NetInterfaceWithAddrCache{
			iface("pdp_ip0", "10.179.12.4/8"),
			iface("en0", "192.168.1.62/24"),
			iface("utun3", "10.8.0.2/24"),
			iface("bridge100", "172.20.10.1/28"),
		}}
		want := OwnAddresses{
			Addrs: []string{"192.168.1.62", "172.20.10.1"},
			Nets:  prefixes("192.168.1.62/24", "10.8.0.2/24", "172.20.10.1/28"),
			Port:  4006,
		}

		// when
		got := newOwnAddresses(ownIPv4("ios", ifaces), 4006)

		// then
		assert.Equal(t, want, got)
	})

	t.Run("desktop drops only the shared address space", func(t *testing.T) {
		// given
		ifaces := addrs.InterfacesAddrs{
			Interfaces: []addrs.NetInterfaceWithAddrCache{
				iface("en0", "192.168.1.10/24"),
				iface("utun4", "100.101.102.103/32"),
				iface("utun8", "10.8.0.2/24"),
			},
			// the flat list is only a fallback: unused while interfaces carry ipv4
			Addrs: []net.Addr{ipNet("192.168.1.10/24"), ipNet("172.30.0.1/16")},
		}
		want := OwnAddresses{
			Addrs: []string{"192.168.1.10", "10.8.0.2"},
			Nets:  prefixes("192.168.1.10/24", "10.8.0.2/24"),
			Port:  4006,
		}

		// when
		got := newOwnAddresses(ownIPv4("darwin", ifaces), 4006)

		// then
		assert.Equal(t, want, got)
	})

	t.Run("cellular only on ios leaves nothing to advertise or match", func(t *testing.T) {
		// given
		ifaces := addrs.InterfacesAddrs{
			Interfaces: []addrs.NetInterfaceWithAddrCache{iface("pdp_ip0", "10.179.12.4/8")},
			Addrs:      []net.Addr{ipNet("10.179.12.4/8")},
		}

		// when
		got := newOwnAddresses(ownIPv4("ios", ifaces), 4006)

		// then
		assert.Empty(t, got.Addrs)
		assert.Empty(t, got.Nets, "an ipv4 seen on an interface rules out the flat-list fallback")
	})

	t.Run("fallback to the flat list applies the address rules", func(t *testing.T) {
		// given: interfaces without addresses
		ifaces := addrs.InterfacesAddrs{
			Interfaces: []addrs.NetInterfaceWithAddrCache{iface("en0")},
			Addrs:      []net.Addr{ipNet("100.64.0.9/10"), ipNet("192.168.1.10/24"), ipNet("192.168.1.10/24")},
		}
		want := OwnAddresses{Addrs: []string{"192.168.1.10"}, Nets: prefixes("192.168.1.10/24"), Port: 4006}

		// when
		gotDesktop := newOwnAddresses(ownIPv4("linux", ifaces), 4006)
		gotIOS := newOwnAddresses(ownIPv4("ios", ifaces), 4006)

		// then
		assert.Equal(t, want, gotDesktop)
		assert.Equal(t, OwnAddresses{Port: 4006}, gotIOS, "the flat list has no interface names, so nothing qualifies on iOS")
	})
}

func TestOwnAddressesFor(t *testing.T) {
	// an iPhone on Wi-Fi with a VPN that carries mDNS and an unrelated
	// office-style subnet; only Wi-Fi and the hotspot are advertised
	own := newOwnAddresses([]ownAddr{
		{iface: "en0", net: netip.MustParsePrefix("192.168.1.10/24"), matchable: true, advertised: true},
		{iface: "utun3", net: netip.MustParsePrefix("10.8.0.2/24"), matchable: true},
		{iface: "en2", net: netip.MustParsePrefix("10.20.0.5/16"), matchable: true, advertised: true},
		{iface: "pdp_ip0", net: netip.MustParsePrefix("10.179.12.4/8")},
	}, 4006)
	peer := func(ips ...string) []netip.Addr {
		res := make([]netip.Addr, 0, len(ips))
		for _, ip := range ips {
			res = append(res, netip.MustParseAddr(ip))
		}
		return res
	}

	for _, tc := range []struct {
		name    string
		peerIPs []netip.Addr
		want    []string
	}{
		{"same wi-fi subnet", peer("192.168.1.62"), []string{"192.168.1.10"}},
		{"peer over the vpn gets the unadvertised vpn address", peer("10.8.0.7"), []string{"10.8.0.2"}},
		{"multi-homed peer on two of our subnets", peer("10.20.3.3", "192.168.1.62"), []string{"192.168.1.10", "10.20.0.5"}},
		{"cellular /8 never matches a 10.x lan peer", peer("10.99.1.1"), []string{"192.168.1.10", "10.20.0.5"}},
		{"ipv4-mapped peer address", peer("::ffff:192.168.1.62"), []string{"192.168.1.10"}},
		{"routed peer gets the advertised list", peer("192.168.7.7"), []string{"192.168.1.10", "10.20.0.5"}},
		{"unknown peer gets the advertised list", nil, []string{"192.168.1.10", "10.20.0.5"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, own.For(tc.peerIPs))
		})
	}

	t.Run("without subnets the plain list goes out", func(t *testing.T) {
		plain := OwnAddresses{Addrs: []string{"192.168.1.10"}, Port: 4006}
		assert.Equal(t, []string{"192.168.1.10"}, plain.For(peer("10.0.0.1")))
	})
}
