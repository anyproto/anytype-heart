package addrs

import (
	"fmt"
	"net"
	"sync"
)

var lock = sync.Mutex{}
var interfaceAddrGetter InterfaceAddrsGetter
var interfaceGetter InterfaceGetter

func SetInterfaceAddrsGetter(getter InterfaceAddrsGetter) {
	lock.Lock()
	defer lock.Unlock()
	interfaceAddrGetter = getter
}

func SetInterfaceGetter(getter InterfaceGetter) {
	lock.Lock()
	defer lock.Unlock()
	interfaceGetter = getter
}

type InterfaceGetter interface {
	Interfaces() []Interface
}

type InterfaceAddrsGetter interface {
	InterfaceAddrs() []InterfaceAddr
}

func maskFromPrefix(prefix, base int) net.IPMask {
	buf := make([]byte, base/8, base/8)
	for i := 0; i < prefix/8; i++ {
		buf[i] = 0xff
	}
	if prefix != base {
		buf[prefix/8] = ((1 << prefix % 8) - 1) << (8 - prefix%8)
	}
	return buf
}

func ipV6MaskFromPrefix(prefix int) net.IPMask {
	return maskFromPrefix(prefix, 128)
}

func ipV4MaskFromPrefix(prefix int) net.IPMask {
	return maskFromPrefix(prefix, 32)
}

func GetInterfacesAddrs() (addrs InterfacesAddrs, err error) {
	lock.Lock()
	if interfaceGetter == nil {
		lock.Unlock()
		return InterfacesAddrs{}, fmt.Errorf("interface getter not set for Android")
	}
	lock.Unlock()
	for _, iface := range interfaceGetter.Interfaces() {
		addrs.Interfaces = append(addrs.Interfaces, iface.Interface)
		unmaskedAddrs := iface.Addrs
		for _, addr := range unmaskedAddrs {
			var mask []byte
			if len(addr.Ip) == 4 {
				mask = ipV4MaskFromPrefix(addr.Prefix)
			} else {
				mask = ipV6MaskFromPrefix(addr.Prefix)
			}
			addrs.Addrs = append(addrs.Addrs, &net.IPNet{
				IP:   addr.Ip,
				Mask: mask,
			})
		}
	}
	return
}
