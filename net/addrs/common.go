package addrs

import (
	"net"
	"regexp"
	"strconv"

	"github.com/ethereum/go-ethereum/log"
	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/util/slice"
)

type Interface struct {
	net.Interface
	Addrs []InterfaceAddr
}

type InterfaceAddr struct {
	Ip     []byte
	Prefix int
}

type NetInterfaceWithAddrCache struct {
	net.Interface
	cachedAddrs []net.Addr // ipv4 addresses
	cachedErr   error
}
type InterfacesAddrs struct {
	Interfaces []NetInterfaceWithAddrCache
	Addrs      []net.Addr // addrs without attachment to specific interface. Used as a fallback mechanism
}

func WrapInterface(iface net.Interface) NetInterfaceWithAddrCache {
	return NetInterfaceWithAddrCache{
		Interface: iface,
	}
}

func WrapInterfaces(ifaces []net.Interface) []NetInterfaceWithAddrCache {
	var m = make([]NetInterfaceWithAddrCache, 0, len(ifaces))
	for i := range ifaces {
		m = append(m, NetInterfaceWithAddrCache{
			Interface: ifaces[i],
		})
	}
	return m
}

// GetAddr returns ipv4 only addresses for interface or cached one if set
func (i NetInterfaceWithAddrCache) GetAddr() []net.Addr {
	if i.cachedAddrs != nil {
		return i.cachedAddrs
	}
	if i.cachedErr != nil {
		return nil
	}
	i.cachedAddrs, i.cachedErr = i.Addrs()
	if i.cachedErr != nil {
		log.Warn("interface GetAddr error: %v", i.cachedErr)
	}
	// filter-out ipv6
	i.cachedAddrs = slice.Filter(i.cachedAddrs, func(addr net.Addr) bool {
		if ip, ok := addr.(*net.IPNet); ok {
			if ip.IP.To4() == nil {
				return false
			}
		}
		return true
	})
	return i.cachedAddrs
}

func (i InterfacesAddrs) Equal(other InterfacesAddrs) bool {
	if len(other.Interfaces) != len(i.Interfaces) {
		return false
	}
	if len(other.Addrs) != len(i.Addrs) {
		return false
	}
	myStr := getStrings(i)
	otherStr := getStrings(other)
	return slices.Equal(myStr, otherStr)
}

var (
	ifaceRe = regexp.MustCompile(`^([a-z]*?)([0-9]+)$`)
	// ifaceReBusSlot used for prefixBusSlot naming schema used in newer linux distros https://cgit.freedesktop.org/systemd/systemd/tree/src/udev/udev-builtin-net_id.c#n20
	ifaceReBusSlot = regexp.MustCompile(`^([a-z]*?)p([0-9]+)s([0-9a-f]+)$`)
)

func parseInterfaceName(name string) (prefix string, bus int, num int64) {
	// try new-style naming schema first (enp0s3, wlp2s0, ...)
	res := ifaceReBusSlot.FindStringSubmatch(name)
	if len(res) > 0 {
		if len(res) > 1 {
			prefix = res[1]
		}
		if len(res) > 2 {
			bus, _ = strconv.Atoi(res[2])
		}
		if len(res) > 3 {
			numHex := res[3]
			num, _ = strconv.ParseInt(numHex, 16, 32)
		}
		return
	}
	// try old-style naming schema (eth0, wlan0, ...)
	res = ifaceRe.FindStringSubmatch(name)
	if len(res) > 1 {
		prefix = res[1]
	}
	if len(res) > 2 {
		num, _ = strconv.ParseInt(res[2], 10, 32)
	}
	return
}

func (i InterfacesAddrs) SortWithPriority(priority []string) {
	less := func(a, b NetInterfaceWithAddrCache) bool {
		aPrefix, aBus, aNum := parseInterfaceName(a.Name)
		bPrefix, bBus, bNum := parseInterfaceName(b.Name)

		aPrioirity := slice.FindPos(priority, aPrefix)
		bPrioirity := slice.FindPos(priority, bPrefix)

		if aPrefix == bPrefix {
			return aNum < bNum
		} else if aPrioirity == -1 && bPrioirity == -1 {
			// sort alphabetically
			return aPrefix < bPrefix
		} else if aPrioirity != -1 && bPrioirity != -1 {
			// in case we have [eth, wlan]
			if aPrioirity == bPrioirity {
				// prioritize eth0 over wlan0
				return aPrioirity < bPrioirity
			}
			// prioritise wlan1 over eth8
			if aBus != bBus {
				return aBus < bBus
			}
			return aNum < bNum
		} else if aPrioirity != -1 {
			return true
		} else {
			return false
		}
	}
	slices.SortFunc(i.Interfaces, func(a, b NetInterfaceWithAddrCache) int {
		if less(a, b) {
			return -1
		}
		return 1
	})
}

func (i InterfacesAddrs) NetInterfaces() []net.Interface {
	var s = make([]net.Interface, 0, len(i.Interfaces))
	for _, iface := range i.Interfaces {
		s = append(s, iface.Interface)
	}
	return s
}

func getStrings(i InterfacesAddrs) (allStrings []string) {
	for _, i := range i.Interfaces {
		allStrings = append(allStrings, i.Name)
	}
	for _, i := range i.Addrs {
		allStrings = append(allStrings, i.String())
	}
	slices.Sort(allStrings)
	return
}

func (i InterfacesAddrs) GetInterfaceByAddr(addr net.Addr) (net.Interface, bool) {
	for _, iface := range i.Interfaces {
		for _, addrInIface := range iface.GetAddr() {
			if addr.String() == addrInIface.String() {
				return iface.Interface, true
			}
		}
	}
	return net.Interface{}, false
}

// SortIPsLikeInterfaces sort IPs in a way they match sorted interface addresses(via mask matching)
// e.g. we have interfaces
// - en0: 192.168.1.10/24
// - lo0: 127.0.0.1/8
// we pass IPs: 10.124.22.1, 127.0.0.1, 192.168.1.25
// we will get: 192.168.1.25, 127.0.0.1, 10.124.22.1
// 10.124.22.1 does not match any interface, so it will be at the end
func (i InterfacesAddrs) SortIPsLikeInterfaces(ips []net.IP) {
	slices.SortFunc(ips, func(a, b net.IP) int {
		posA, _ := i.findInterfacePosByIP(a)
		posB, _ := i.findInterfacePosByIP(b)

		if posA == -1 && posB != -1 {
			return 1
		}
		if posA != -1 && posB == -1 {
			return -1
		}
		if posA < posB {
			return -1
		} else if posA > posB {
			return 1
		}
		return 0
	})
}

func (i InterfacesAddrs) findInterfacePosByIP(ip net.IP) (pos int, equal bool) {
	for position, iface := range i.Interfaces {
		for _, addr := range iface.GetAddr() {
			if ni, ok := addr.(*net.IPNet); ok {
				if ni.Contains(ip) {
					return position, ni.IP.Equal(ip)
				}
			}
		}
	}
	return -1, false
}
