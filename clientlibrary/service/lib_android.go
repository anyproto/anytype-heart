package service

import (
	ma "github.com/multiformats/go-multiaddr"
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

func (i *interfaceGetterAdapter) InterfaceAddrs() []ma.InterfaceAddr {
	iter := i.interfaceGetter.InterfaceAddrs()
	addr := iter.Next()
	var res []ma.InterfaceAddr
	for addr != nil {
		res = append(res, ma.InterfaceAddr{
			Ip:     addr.Ip(),
			Prefix: addr.Prefix(),
		})
		addr = iter.Next()
	}
	return res
}

func SetInterfaceAddrsGetter(getter InterfaceAddrsGetter) {
	ma.SetInterfaceAddrsGetter(&interfaceGetterAdapter{
		interfaceGetter: getter,
	})
}
