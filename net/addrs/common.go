package addrs

import (
	"net"
	"regexp"
	"strconv"

	"golang.org/x/exp/slices"

	"github.com/anytypeio/go-anytype-middleware/util/slice"
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

var re = regexp.MustCompile(`^(.*?)([0-9]*)$`)

func (i InterfacesAddrs) SortWithPriority(priority []string) {
	f := func(ifName string) (prefix string, num int) {
		res := re.FindStringSubmatch(ifName)

		if len(res) > 1 {
			prefix = res[1]
		}
		if len(res) > 2 {
			num, _ = strconv.Atoi(res[2])
		}
		return
	}

	slices.SortFunc(i.Interfaces, func(a, b net.Interface) bool {
		aPrefix, aNum := f(a.Name)
		bPrefix, bNum := f(b.Name)

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
