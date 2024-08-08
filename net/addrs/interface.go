//go:build !android
// +build !android

package addrs

import (
	"net"
	"slices"

	"github.com/anyproto/anytype-heart/util/slice"
)

func SetInterfaceAddrsGetter(getter InterfaceAddrsGetter) {}

func SetInterfaceGetter(getter InterfaceGetter) {}

type InterfaceGetter interface {
	Interfaces() []Interface
}

type InterfaceAddrsGetter interface {
	InterfaceAddrs() []InterfaceAddr
}

func GetInterfacesAddrs() (iAddrs InterfacesAddrs, err error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return
	}
	iAddrs.Addrs = addrs
	ifaces, err := net.Interfaces()
	if err != nil {
		return
	}
	iAddrs.Interfaces = ifaces

	iAddrs.Interfaces = slice.Filter(iAddrs.Interfaces, func(iface net.Interface) bool {
		return iface.Flags&net.FlagUp != 0 && iface.Flags&net.FlagMulticast != 0
	})
	return
}

func IsLoopBack(interfaces []net.Interface) bool {
	return len(interfaces) == 1 && slices.ContainsFunc(interfaces, func(n net.Interface) bool {
		return n.Flags&net.FlagLoopback != 0
	})
}
