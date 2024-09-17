package localdiscovery

import (
	"fmt"
	gonet "net"
	"runtime"
	"strings"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"

	"github.com/anyproto/anytype-heart/net/addrs"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/slice"
)

const (
	CName = "client.space.localdiscovery"

	serviceName = "_anytype._tcp"
	mdnsDomain  = "local"
)

var log = logger.NewNamed(CName)

type DiscoveryPossibility int

const (
	DiscoveryPossible               DiscoveryPossibility = 0
	DiscoveryNoInterfaces           DiscoveryPossibility = 2
	DiscoveryLocalNetworkRestricted DiscoveryPossibility = 3
)

type HookCallback func(state DiscoveryPossibility)

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

type NetworkStateService interface {
	RegisterHook(hook func(network model.DeviceNetworkType))
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

func (l *localDiscovery) getP2PPossibility(newAddrs addrs.InterfacesAddrs) DiscoveryPossibility {
	// some sophisticated logic for ios, because of possible Local Network Restrictions
	var err error
	interfaces := newAddrs.Interfaces
	for _, iface := range interfaces {
		if runtime.GOOS == "ios" {
			// on ios we have to check only en interfaces
			if !strings.HasPrefix(iface.Name, "en") {
				// en1 used for wifi
				// en2 used for wired connection
				continue
			}
		}
		addrs := iface.GetAddr()
		if len(addrs) == 0 {
			continue
		}
		for _, addr := range addrs {
			if ip, ok := addr.(*gonet.IPNet); ok {
				ipv4 := ip.IP.To4()
				if ipv4 == nil {
					continue
				}
				err = testSelfConnection(ipv4.String())
				if err != nil {
					log.Warn(fmt.Sprintf("self connection via %s to %s failed: %v", iface.Name, ipv4.String(), err))
				} else {
					return DiscoveryPossible
				}
				break
			}
		}
	}
	if err != nil {
		// todo: double check network state provided by the client?
		return DiscoveryLocalNetworkRestricted
	}
	return DiscoveryNoInterfaces
}

func (l *localDiscovery) notifyP2PPossibilityState(state DiscoveryPossibility) {
	l.hookMu.Lock()
	defer l.hookMu.Unlock()
	if state == l.hookState {
		return
	}
	l.hookState = state
	for _, callback := range l.hooks {
		callback(state)
	}
}

func (l *localDiscovery) RegisterDiscoveryPossibilityHook(hook func(state DiscoveryPossibility)) {
	l.hookMu.Lock()
	defer l.hookMu.Unlock()
	l.hooks = append(l.hooks, hook)
}
