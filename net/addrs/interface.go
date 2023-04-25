//go:build !android
// +build !android

package addrs

import "net"

func SetInterfaceAddrsGetter(getter InterfaceAddrsGetter) {}

func SetInterfaceGetter(getter InterfaceGetter) {}

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
