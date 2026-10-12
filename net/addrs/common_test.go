package addrs

import (
	"net"
	"net/netip"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// GetAddr promises to cache the syscall result after the first call; a value
// receiver silently caches into a copy and re-runs the syscalls every time.
func TestGetAddrCachesIntoSliceElement(t *testing.T) {
	ifaces, err := net.Interfaces()
	require.NoError(t, err)
	if len(ifaces) == 0 {
		t.Skip("no network interfaces on this machine")
	}
	wrapped := WrapInterfaces(ifaces)
	for i := range wrapped {
		first := wrapped[i].GetAddr()
		if !wrapped[i].loaded {
			t.Fatalf("GetAddr did not cache the result for %s", wrapped[i].Name)
		}
		second := wrapped[i].GetAddr()
		if len(first) > 0 {
			assert.Equal(t, &first[0], &second[0], "second GetAddr call for %s must return the cached slice", wrapped[i].Name)
		}
	}
}

// findInterfacePosByIP is the hot path (called per discovered peer entry per
// IP); it must populate the per-interface cache instead of re-dumping
// addresses on every scan.
func TestFindInterfacePosByIPPopulatesCache(t *testing.T) {
	ia, err := GetInterfacesAddrs()
	require.NoError(t, err)
	if len(ia.Interfaces) == 0 {
		t.Skip("no network interfaces on this machine")
	}
	// TEST-NET-1 address: matches no local interface, so the scan visits all
	ia.findInterfacePosByIP(net.ParseIP("192.0.2.1"))
	for i := range ia.Interfaces {
		if !ia.Interfaces[i].loaded {
			t.Fatalf("scan did not populate the addr cache for %s", ia.Interfaces[i].Name)
		}
	}
}

func TestInterfaceAddrFamilies(t *testing.T) {
	ipNet := func(cidr string) net.Addr {
		ip, n, err := net.ParseCIDR(cidr)
		require.NoError(t, err)
		n.IP = ip
		return n
	}
	ipv4 := ipNet("192.168.1.10/24")
	linkLocal := ipNet("fe80::a886:c6ff:febd:f923/64")
	global := ipNet("2a0d:5600:24:1::10/64")
	ula := ipNet("fdf0:1:2:3::10/64")
	mappedV4 := &net.IPNet{IP: net.ParseIP("10.0.0.5"), Mask: net.CIDRMask(120, 128)}

	for _, tc := range []struct {
		name          string
		addrs         []net.Addr
		wantAddr      []net.Addr
		wantLinkLocal []netip.Addr
	}{
		{
			name:          "both families: ipv4 in GetAddr, link-local in its sibling",
			addrs:         []net.Addr{linkLocal, ipv4, global},
			wantAddr:      []net.Addr{ipv4},
			wantLinkLocal: []netip.Addr{netip.MustParseAddr("fe80::a886:c6ff:febd:f923")},
		},
		{
			name:          "ipv6 only: GetAddr is empty, link-local is kept",
			addrs:         []net.Addr{global, ula, linkLocal},
			wantAddr:      []net.Addr{},
			wantLinkLocal: []netip.Addr{netip.MustParseAddr("fe80::a886:c6ff:febd:f923")},
		},
		{
			name:     "global and ULA are never link-local",
			addrs:    []net.Addr{global, ula},
			wantAddr: []net.Addr{},
		},
		{
			name:     "an ipv4 address in the 16-byte form stays ipv4",
			addrs:    []net.Addr{mappedV4},
			wantAddr: []net.Addr{mappedV4},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// given
			iface := WrapInterfaceWithAddrs(net.Interface{Name: "en16"}, tc.addrs)

			// when
			gotAddr := iface.GetAddr()
			gotLinkLocal := iface.GetLinkLocalIPv6()

			// then
			assert.Equal(t, tc.wantAddr, gotAddr)
			assert.Equal(t, tc.wantLinkLocal, gotLinkLocal)
		})
	}
}

func TestHasRoutableIPv4(t *testing.T) {
	ipNet := func(cidr string) net.Addr {
		ip, n, err := net.ParseCIDR(cidr)
		require.NoError(t, err)
		n.IP = ip
		return n
	}
	for _, tc := range []struct {
		name  string
		addrs []net.Addr
		want  bool
	}{
		{"a LAN address", []net.Addr{ipNet("192.168.1.10/24")}, true},
		{"a self-assigned address alone", []net.Addr{ipNet("169.254.10.2/16")}, false},
		{"a self-assigned address beside a LAN one", []net.Addr{ipNet("169.254.10.2/16"), ipNet("10.0.0.5/8")}, true},
		{"link-local IPv6 only", []net.Addr{ipNet("fe80::1/64")}, false},
		{"no address", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// given
			iface := WrapInterfaceWithAddrs(net.Interface{Name: "en5"}, tc.addrs)

			// when
			got := HasRoutableIPv4(&iface)

			// then
			assert.Equal(t, tc.want, got)
		})
	}
}

func Test_parseInterfaceName(t *testing.T) {
	type args struct {
		name string
	}
	tests := []struct {
		name       string
		args       args
		wantPrefix string
		wantType   NamingType
		wantBus    int64
		wantNum    int64
	}{
		{"eth0", args{"eth0"}, "eth", NamingTypeOld, 0, 0},
		{"eth1", args{"eth1"}, "eth", NamingTypeOld, 0, 1},
		{"eth10", args{"eth10"}, "eth", NamingTypeOld, 0, 10},
		{"enp0s10", args{"enp0s10"}, "en", NamingTypeBusSlot, 0, 0x10},
		{"wlp0s20f3", args{"wlp0s20f3"}, "wl", NamingTypeBusSlot, 0, 0x20f3},
		{"tun0", args{"tun0"}, "tun", NamingTypeOld, 0, 0},
		{"tap0", args{"tap0"}, "tap", NamingTypeOld, 0, 0},
		{"lo0", args{"lo0"}, "lo", NamingTypeOld, 0, 0},
		{"lo1", args{"lo1"}, "lo", NamingTypeOld, 0, 1},
		{"lo10", args{"lo10"}, "lo", NamingTypeOld, 0, 10},
		{"wlx001122334455", args{"wlx001122334455"}, "wl", NamingTypeMac, 0x001122334455, 0},
		{"wlxffffffffffff", args{"wlxffffffffffff"}, "wl", NamingTypeMac, 0xffffffffffff, 0},
		{"eno16777736", args{"eno16777736"}, "en", NamingTypeOnboard, 0x16777736, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPrefix, gotType, gotBus, gotNum := parseInterfaceName(tt.args.name)
			if gotPrefix != tt.wantPrefix {
				t.Errorf("parseInterfaceName() gotPrefix = %v, want %v", gotPrefix, tt.wantPrefix)
			}
			if gotBus != tt.wantBus {
				t.Errorf("parseInterfaceName() gotBus = %v, want %v", gotBus, tt.wantBus)
			}
			if gotNum != tt.wantNum {
				t.Errorf("parseInterfaceName() gotNum = %v, want %v", gotNum, tt.wantNum)
			}
			if gotType != tt.wantType {
				t.Errorf("parseInterfaceName() gotType = %v, want %v", gotType, tt.wantType)
			}
		})
	}
}

// TestInterfaceSorterSorting tests sorting a list of interface names using the Compare method.
func TestInterfaceSorterSorting(t *testing.T) {
	sorter := interfaceComparer{
		priority: []string{"wl", "wlan", "en", "eth", "tun", "tap", "utun"},
	}

	// List of interfaces to sort
	interfaces := []string{
		"tap0",
		"awdl0",
		"eth0",
		"eno1",
		"wlp2s0",
		"ens2",
		"enp0s3",
		"tun0",
		"wlan0",
		"enx001122334455",
		"wlx001122334455",
	}

	// Expected order after sorting
	expected := []string{
		"wlp2s0",          // Wireless LAN on PCI bus
		"wlx001122334455", // Wireless LAN with MAC address
		"wlan0",           // Old-style Wireless LAN
		"eno1",            // Highest priority (onboard Ethernet)
		"enp0s3",          // PCI bus Ethernet
		"enx001122334455", // Ethernet with MAC address
		"ens2",            // Hotplug Ethernet
		"eth0",            // Old-style Ethernet
		"tun0",            // VPN TUN interface
		"tap0",            // VPN TAP interface
		"awdl0",
	}

	// Sorting the interfaces using the Compare method
	sort.Slice(interfaces, func(i, j int) bool {
		return sorter.Compare(interfaces[i], interfaces[j]) < 0
	})

	// Assert the sorted order matches the expected order
	assert.Equal(t, expected, interfaces, "The interfaces should be sorted correctly according to priority.")
}

func TestNetAddrsEqualUnordered(t *testing.T) {
	a := func(s string) net.Addr { return &net.IPAddr{IP: net.ParseIP(s)} }
	wifi, x, y := a("192.168.1.10"), a("fe80::1"), a("fe80::2")

	assert.True(t, NetAddrsEqualUnordered([]net.Addr{wifi, x}, []net.Addr{x, wifi}))
	assert.False(t, NetAddrsEqualUnordered([]net.Addr{wifi, x, x}, []net.Addr{wifi, x, y}), "a duplicate must not stand in for a new address")
	assert.False(t, NetAddrsEqualUnordered([]net.Addr{wifi, x, y}, []net.Addr{wifi, x, x}))
	assert.True(t, NetAddrsEqualUnordered([]net.Addr{x, x}, []net.Addr{x, x}))
}

func TestInterfacesAddrsEqual(t *testing.T) {
	ll := &net.IPNet{IP: net.ParseIP("fe80::a886:c6ff:febd:f923"), Mask: net.CIDRMask(64, 128)}
	wifi := &net.IPNet{IP: net.ParseIP("192.168.1.10").To4(), Mask: net.CIDRMask(24, 32)}
	set := func(cableIndex int, cableName string, cableAddrs, wifiAddrs []net.Addr) InterfacesAddrs {
		return InterfacesAddrs{
			Interfaces: []NetInterfaceWithAddrCache{
				WrapInterfaceWithAddrs(net.Interface{Index: 4, Name: "en0"}, wifiAddrs),
				WrapInterfaceWithAddrs(net.Interface{Index: cableIndex, Name: cableName}, cableAddrs),
			},
			Addrs: []net.Addr{wifi, ll},
		}
	}
	base := set(31, "en16", []net.Addr{ll}, []net.Addr{wifi})

	assert.True(t, base.Equal(set(31, "en16", []net.Addr{ll}, []net.Addr{wifi})))
	assert.False(t, base.Equal(set(42, "en16", []net.Addr{ll}, []net.Addr{wifi})), "the cable came back under a new index")
	assert.False(t, base.Equal(set(42, "en17", []net.Addr{ll}, []net.Addr{wifi})), "the cable came back under a new name")
	assert.False(t, base.Equal(set(31, "en16", nil, []net.Addr{wifi, ll})), "the address moved to another interface")
}
