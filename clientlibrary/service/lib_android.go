package service

// #cgo LDFLAGS: -static-libstdc++
import "C"

import (
	"github.com/anytypeio/go-anytype-middleware/net/addrs"
)

type InterfaceAddr interface {
	Ip() []byte
	Prefix() int
}

var interfaceGetter InterfaceAddrsGetter

type InterfaceAddrIterator interface {
	Next() InterfaceAddr
}

type InterfaceAddrsGetter interface {
	InterfaceAddrs() InterfaceAddrIterator
}

type interfaceGetterAdapter struct {
	interfaceGetter InterfaceAddrsGetter
}

func (i *interfaceGetterAdapter) InterfaceAddrs() []addrs.InterfaceAddr {
	iter := i.interfaceGetter.InterfaceAddrs()
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

func SetInterfaceAddrsGetter(getter InterfaceAddrsGetter) {
	addrs.SetInterfaceAddrsGetter(&interfaceGetterAdapter{
		interfaceGetter: getter,
	})
}
