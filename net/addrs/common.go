package addrs

import (
	"net"
	"regexp"
	"strconv"

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

type InterfacesAddrs struct {
	Interfaces []net.Interface
	Addrs      []net.Addr
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
	slices.SortFunc(i.Interfaces, func(a, b net.Interface) bool {
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
	})
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
