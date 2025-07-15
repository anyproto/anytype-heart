package addrs

import (
	"cmp"
	"fmt"
	"net"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/util/slice"
)

var log = logging.Logger("anytype-net")

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
	Addrs      []net.Addr // addrs without attachment to specific interface. Used as cheap(1 syscall) way to check if smth has changed
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

func AddrToIP(addr net.Addr) (net.IP, bool) {
	switch ip := addr.(type) {
	case *net.IPNet:
		return ip.IP, true
	case *net.IPAddr:
		return ip.IP, true
	default:
		return nil, false
	}
}

// GetAddr returns ipv4 only addresses for interface or cached one if set
func (i NetInterfaceWithAddrCache) GetAddr() []net.Addr {
	if i.cachedAddrs != nil {
		return i.cachedAddrs
	}
	if i.cachedErr != nil {
		return nil
	}
	i.cachedAddrs, i.cachedErr = i.Interface.Addrs()
	if i.cachedErr != nil {
		log.Warn("interface GetAddr error: %v", i.cachedErr)
	}
	// filter-out ipv6
	i.cachedAddrs = slice.Filter(i.cachedAddrs, func(addr net.Addr) bool {
		if ip, ok := AddrToIP(addr); ok {
			if ip.To4() == nil {
				return false
			}
		}
		return true
	})
	return i.cachedAddrs
}

func NetAddrsEqualUnordered(a, b []net.Addr) bool {
	if len(a) != len(b) {
		return false
	}
	for _, addr := range a {
		found := false
		for _, addr2 := range b {
			if addr.String() == addr2.String() {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
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
	// compare slices without order
	if !slices.Equal(myStr, otherStr) {
		log.Debug(fmt.Sprintf("addrs compare: strings mismatch: %v != %v", myStr, otherStr))
		return false
	}
	return true
}

var (
	ifaceRe        = regexp.MustCompile(`^([a-z]*?)([0-9]+)$`)
	ifaceWindowsRe = regexp.MustCompile(`^(.*?)([0-9]*)$`)

	// ifaceReBusSlot used for prefixBusSlot naming schema used in newer linux distros https://cgit.freedesktop.org/systemd/systemd/tree/src/udev/udev-builtin-net_id.c#n20
	ifaceReBusSlot = regexp.MustCompile(`^(?P<type>enp|eno|ens|enx|wlp|wlx)(?P<bus>[0-9a-fA-F]*)s?(?P<slot>[0-9a-fA-F]*)?$`)
)

func cleanInterfaceName(name string) (clean string, namingType NamingType) {
	if strings.HasPrefix(name, "en") ||
		strings.HasPrefix(name, "wl") ||
		strings.HasPrefix(name, "eth") {

		lastSymbol := name[len(name)-1]
		switch NamingType(lastSymbol) {
		case NamingTypeBusSlot, NamingTypeHotplug, NamingTypeMac, NamingTypeOnboard:
			return name[0 : len(name)-1], NamingType(lastSymbol)
		}
	}

	return name, NamingTypeOld
}

type NamingType string

const (
	NamingTypeOld     NamingType = ""
	NamingTypeOnboard NamingType = "o"
	NamingTypeBusSlot NamingType = "p"
	NamingTypeMac     NamingType = "x"
	NamingTypeHotplug NamingType = "s"
)

func (n NamingType) Priority() int {
	switch n {
	case NamingTypeOld:
		return 0
	case NamingTypeOnboard:
		return 1
	case NamingTypeBusSlot:
		return 2
	case NamingTypeMac:
		return 3
	case NamingTypeHotplug:
		return 4
	default:
		return 5
	}
}

// parseInterfaceName parses interface name and returns prefix, naming type, bus number and slot number
// e.g. enp0s3 -> en, NamingTypeBusSlot, 0, 3
// bus and slot are interpreted as hex numbers
// bus is also used for mac address
// in case of enx001122334455 -> en, NamingTypeMac, 0x001122334455, 0
func parseInterfaceName(name string) (iface string, namingType NamingType, busNum int64, num int64) {
	if runtime.GOOS == "windows" {
		name, num = parseInterfaceWindowsName(name)
		return
	}
	// try new-style naming schema first (enp0s3, wlp2s0, ...)
	res := ifaceReBusSlot.FindStringSubmatch(name)
	if len(res) > 0 {

		for i, subName := range ifaceReBusSlot.SubexpNames() {
			if i > 0 && res[i] != "" {
				switch subName {
				case "type":
					iface, namingType = cleanInterfaceName(res[i])
				case "bus":
					busNum, _ = strconv.ParseInt(res[i], 16, 64)
				case "slot": // or mac
					num, _ = strconv.ParseInt(res[i], 16, 64)
				}
			}
		}
		return
	}
	// try old-style naming schema (eth0, wlan0, ...)
	res = ifaceRe.FindStringSubmatch(name)
	if len(res) > 1 {
		iface = res[1]
	}
	if len(res) > 2 {
		num, _ = strconv.ParseInt(res[2], 10, 32)
	}
	if iface == "" {

	}
	return
}

func parseInterfaceWindowsName(name string) (iface string, num int64) {
	res := ifaceWindowsRe.FindStringSubmatch(name)
	if len(res) > 1 {
		iface = res[1]
	}
	if len(res) > 2 {
		num, _ = strconv.ParseInt(res[2], 10, 32)
	}
	return
}

type interfaceComparer struct {
	priority []string
}

func (i interfaceComparer) Compare(a, b string) int {
	aPrefix, aType, aBus, aNum := parseInterfaceName(a)
	bPrefix, bType, bBus, bNum := parseInterfaceName(b)

	aPrioirity := slice.FindPos(i.priority, aPrefix)
	bPrioirity := slice.FindPos(i.priority, bPrefix)

	if aPrioirity != -1 && bPrioirity != -1 || aPrioirity == -1 && bPrioirity == -1 {
		if aPrefix != bPrefix {
			if aPrioirity != -1 && bPrioirity != -1 {
				// prioritize by priority
				return cmp.Compare(aPrioirity, bPrioirity)
			} else {
				// prioritize by prefix
				return cmp.Compare(aPrefix, bPrefix)
			}
		}
		if aType != bType {
			return cmp.Compare(aType.Priority(), bType.Priority())
		}
		if aBus != bBus {
			return cmp.Compare(aBus, bBus)
		}
		if aNum != bNum {
			return cmp.Compare(aNum, bNum)
		}
		// shouldn't be a case
		return cmp.Compare(a, b)
	}

	if aPrioirity == -1 {
		return 1
	} else {
		return -1
	}
}

func (i InterfacesAddrs) SortInterfacesWithPriority(priority []string) {
	sorter := interfaceComparer{priority: priority}

	compare := func(a, b NetInterfaceWithAddrCache) int {
		return sorter.Compare(a.Name, b.Name)
	}
	slices.SortFunc(i.Interfaces, compare)
}

func (i InterfacesAddrs) InterfaceNames() []string {
	var names = make([]string, 0, len(i.Interfaces))
	for _, iface := range i.Interfaces {
		names = append(names, iface.Name)
	}
	return names
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
			switch a := addr.(type) {
			case *net.IPNet:
				if a.Contains(ip) {
					return position, a.IP.Equal(ip)
				}
			case *net.IPAddr:
				if a.IP.Equal(ip) {
					return position, true
				}
			}
		}
	}
	return -1, false
}
