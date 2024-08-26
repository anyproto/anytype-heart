//go:build !android
// +build !android

package addrs

import (
	"net"
	"slices"
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
	iAddrs.Interfaces = filterInterfaces(WrapInterfaces(ifaces))
	return
}

func IsLoopBack(interfaces []net.Interface) bool {
	return len(interfaces) == 1 && slices.ContainsFunc(interfaces, func(n net.Interface) bool {
		return n.Flags&net.FlagLoopback != 0
	})
}
