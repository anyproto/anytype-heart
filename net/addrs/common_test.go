package addrs

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

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
