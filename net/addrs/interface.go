//go:build !android
// +build !android

package addrs

import "net"

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
	return
}
