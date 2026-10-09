package localdiscovery

import (
	"net"
	"net/netip"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/anyproto/anytype-heart/net/addrs"
)

func ipNet(cidr string) *net.IPNet {
	ip, n, err := net.ParseCIDR(cidr)
	if err != nil {
		panic(err)
	}
	if ip4 := ip.To4(); ip4 != nil {
		ip = ip4
	}
	n.IP = ip
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
		got := newOwnAddresses(ownAddrs("ios", ifaces), 4006)

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
		got := newOwnAddresses(ownAddrs("darwin", ifaces), 4006)

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
		got := newOwnAddresses(ownAddrs("ios", ifaces), 4006)

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
		gotDesktop := newOwnAddresses(ownAddrs("linux", ifaces), 4006)
		gotIOS := newOwnAddresses(ownAddrs("ios", ifaces), 4006)

		// then
		assert.Equal(t, want, gotDesktop)
		assert.Equal(t, OwnAddresses{Port: 4006}, gotIOS, "the flat list has no interface names, so nothing qualifies on iOS")
	})
}

func TestOwnAddrsLinkLocal(t *testing.T) {
	t.Run("an ipv6-only interface yields a link-local entry outside Addrs and Nets", func(t *testing.T) {
		// given: Wi-Fi plus a cable to a phone that carries only link-local
		ifaces := addrs.InterfacesAddrs{Interfaces: []addrs.NetInterfaceWithAddrCache{
			iface("en0", "192.168.1.10/24", "fe80::1c57:dcff:fe84:9d64/64", "2a0d:5600::10/64"),
			iface("en16", "fe80::a886:c6ff:febd:f923/64"),
		}}
		want := []ownAddr{
			{iface: "en0", net: netip.MustParsePrefix("192.168.1.10/24"), matchable: true, advertised: true},
			{iface: "en16", net: netip.MustParsePrefix("fe80::a886:c6ff:febd:f923/64"), linkLocal: true},
		}

		// when
		got := ownAddrs("darwin", ifaces)

		// then
		assert.Equal(t, want, got)
		own := newOwnAddresses(got, 4006)
		assert.Equal(t, []string{"192.168.1.10"}, own.Addrs)
		assert.Equal(t, prefixes("192.168.1.10/24"), own.Nets)
	})

	t.Run("a cable alone still leaves Addrs and Nets empty", func(t *testing.T) {
		// given: the flat list holds what the interface filter dropped
		ifaces := addrs.InterfacesAddrs{
			Interfaces: []addrs.NetInterfaceWithAddrCache{iface("en16", "fe80::a886:c6ff:febd:f923/64")},
			Addrs:      []net.Addr{ipNet("127.0.0.1/8"), ipNet("10.8.0.2/24"), ipNet("fe80::a886:c6ff:febd:f923/64")},
		}

		// when
		got := ownAddrs("darwin", ifaces)

		// then
		assert.Equal(t, []ownAddr{{iface: "en16", net: netip.MustParsePrefix("fe80::a886:c6ff:febd:f923/64"), linkLocal: true}}, got)
		assert.Equal(t, OwnAddresses{Port: 4006}, newOwnAddresses(got, 4006))
	})

	t.Run("tunnels and windows never yield link-local", func(t *testing.T) {
		// given
		ifaces := addrs.InterfacesAddrs{Interfaces: []addrs.NetInterfaceWithAddrCache{
			iface("utun0", "fe80::f962:71b6:cc79:75c/64"),
			iface("awdl0", "fe80::e8f0:84ff:fe9b:e431/64"),
			iface("Ethernet 2", "fe80::5/64"),
		}}

		// when
		gotDarwin := ownAddrs("darwin", ifaces)
		gotWindows := ownAddrs("windows", ifaces)

		// then
		assert.Empty(t, gotDarwin)
		assert.Empty(t, gotWindows)
	})
}

func TestLinkLocalCandidate(t *testing.T) {
	for _, tc := range []struct {
		goos, iface string
		want        bool
	}{
		{"darwin", "en16", true},
		{"darwin", "bridge0", true},
		{"darwin", "utun3", false},
		{"darwin", "awdl0", false},
		{"darwin", "llw0", false},
		{"darwin", "anpi0", false},
		{"darwin", "ap1", false},
		{"ios", "en2", true},
		{"ios", "pdp_ip0", false},
		{"linux", "enp0s31f6", true},
		{"linux", "eth0", true},
		{"linux", "wlp2s0", true},
		{"linux", "thunderbolt0", true},
		{"linux", "br0", true},
		{"linux", "bond0", true},
		{"linux", "em1", true},
		{"linux", "p1p1", true},
		{"linux", "ppp0", false},
		{"linux", "veth1a2b3c", false},
		{"linux", "docker0", false},
		{"linux", "tailscale0", false},
		{"windows", "Ethernet", false},
	} {
		t.Run(tc.goos+" "+tc.iface, func(t *testing.T) {
			assert.Equal(t, tc.want, linkLocalCandidate(tc.goos, tc.iface))
		})
	}
}

func TestFilterMulticastInterfaces(t *testing.T) {
	// given
	up := net.FlagUp | net.FlagMulticast
	withFlags := func(i addrs.NetInterfaceWithAddrCache, flags net.Flags) addrs.NetInterfaceWithAddrCache {
		i.Flags = flags
		return i
	}
	ifaces := []addrs.NetInterfaceWithAddrCache{
		withFlags(iface("en0", "192.168.1.10/24", "fe80::1/64"), up),
		withFlags(iface("en16", "fe80::a886:c6ff:febd:f923/64"), up),
		withFlags(iface("en13"), up),
		withFlags(iface("en17", "2a0d:5600::10/64"), up),
		withFlags(iface("utun0", "fe80::f962:71b6:cc79:75c/64"), up),
		withFlags(iface("lo0", "127.0.0.1/8", "fe80::1/64"), up|net.FlagLoopback),
		withFlags(iface("en18", "fe80::7/64"), net.FlagUp),
	}

	// when
	got := filterMulticastInterfacesFor("darwin", ifaces)

	// then
	names := make([]string, 0, len(got))
	for _, i := range got {
		names = append(names, i.Name)
	}
	assert.Equal(t, []string{"en0", "en16"}, names)
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

func TestDiscoveryPossibilityOverLinkLocal(t *testing.T) {
	if !linkLocalCandidate(runtime.GOOS, "en16") || runtime.GOOS == "ios" {
		t.Skip("no link-local discovery on " + runtime.GOOS)
	}
	l := &discoveryBase{}

	// given: a cable alone, no ipv4 anywhere
	cableOnly := addrs.InterfacesAddrs{Interfaces: []addrs.NetInterfaceWithAddrCache{iface("en16", "fe80::a886:c6ff:febd:f923/64")}}
	nothing := addrs.InterfacesAddrs{Interfaces: []addrs.NetInterfaceWithAddrCache{iface("en16")}}

	// then
	assert.Equal(t, DiscoveryPossible, l.getDiscoveryPossibility(cableOnly))
	assert.Equal(t, DiscoveryNoInterfaces, l.getDiscoveryPossibility(nothing))
}
