package service

// #cgo LDFLAGS: -static-libstdc++
import "C"

import (
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/net/addrs"
	"net"
)

type InterfaceAddr interface {
	Ip() []byte
	Prefix() int
}

type NetInterface interface {
	Index() int
	MTU() int
	Name() string
	HardwareAddr() []byte
	Flags() int
	InterfaceAddrIter() InterfaceAddrIterator
}

type InterfaceAddrIterator interface {
	Next() InterfaceAddr
}

type InterfaceAddrsGetter interface {
	InterfaceAddrs() InterfaceAddrIterator
}

type InterfaceGetter interface {
	Interfaces() InterfaceIterator
}

type InterfaceIterator interface {
	Next() NetInterface
}

type interfaceGetterAdapter struct {
	interfaceGetter InterfaceGetter
}

func (i *interfaceGetterAdapter) InterfaceAddrs(iter InterfaceAddrIterator) []addrs.InterfaceAddr {
	addr := iter.Next()
	var res []addrs.InterfaceAddr
	for addr != nil {
		res = append(res, addrs.InterfaceAddr{
			Ip:     addr.Ip(),
			Prefix: addr.Prefix(),
		})
		addr = iter.Next()
	}
	return res
}

func (i *interfaceGetterAdapter) Interfaces() []addrs.Interface {
	iter := i.interfaceGetter.Interfaces()
	addr := iter.Next()
	var res []addrs.Interface
	for addr != nil {
		iface := addrs.Interface{
			Interface: net.Interface{
				Index:        addr.Index(),
				MTU:          addr.MTU(),
				Name:         addr.Name(),
				Flags:        net.Flags(addr.Flags()),
				HardwareAddr: addr.HardwareAddr(),
			},
			Addrs: i.InterfaceAddrs(addr.InterfaceAddrIter()),
		}
		res = append(res, iface)
		fmt.Println("[printing interface]", iface)
		addr = iter.Next()
	}
	return res
}

func SetInterfaceGetter(getter InterfaceGetter) {
	addrs.SetInterfaceGetter(&interfaceGetterAdapter{
		interfaceGetter: getter,
	})
}
