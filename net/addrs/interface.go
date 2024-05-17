//go:build !android
// +build !android

package addrs

import (
	"net"
	"strings"

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
	iAddrs.Addrs = slice.Filter(addrs, func(addr net.Addr) bool { return !strings.HasPrefix(addr.String(), "127.0.0.1") })
	ifaces, err := net.Interfaces()
	if err != nil {
		return
	}
	iAddrs.Interfaces = ifaces

	iAddrs.Interfaces = slice.Filter(iAddrs.Interfaces, func(iface net.Interface) bool {
		return iface.Flags&net.FlagUp != 0 && iface.Flags&net.FlagMulticast != 0 && iface.Flags&net.FlagLoopback == 0
	})
	return
}
