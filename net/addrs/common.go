package addrs

import (
	"golang.org/x/exp/slices"
	"net"
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
