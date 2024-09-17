package localdiscovery

import (
	gonet "net"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"

	"github.com/anyproto/anytype-heart/net/addrs"
	"github.com/anyproto/anytype-heart/util/slice"
)

const (
	CName = "client.space.localdiscovery"

	serviceName = "_anytype._tcp"
	mdnsDomain  = "local"
)

var log = logger.NewNamed(CName)

type DiscoveredPeer struct {
	Addrs  []string
	PeerId string
}

type OwnAddresses struct {
	Addrs []string
	Port  int
}

type Notifier interface {
	PeerDiscovered(peer DiscoveredPeer, own OwnAddresses)
}

type LocalDiscovery interface {
	SetNotifier(Notifier)
	Start() error // Start the local discovery. Used when automatic start is disabled.
	app.ComponentRunnable
}

// filterMulticastInterfaces filters out interfaces that doesn't make sense to use for multicast discovery.
// Also filters out loopback interfaces to make less mess.
// Please note: this call do a number of underlying syscalls to get addrs for each interface, but they will be cached after first call.
func filterMulticastInterfaces(ifaces []addrs.NetInterfaceWithAddrCache) []addrs.NetInterfaceWithAddrCache {
	return slice.Filter(ifaces, func(iface addrs.NetInterfaceWithAddrCache) bool {
		if iface.Flags&gonet.FlagUp != 0 && iface.Flags&gonet.FlagMulticast != 0 && iface.Flags&gonet.FlagLoopback == 0 {
			if len(iface.GetAddr()) > 0 {
				return true
			}
		}
		return false
	})
}
