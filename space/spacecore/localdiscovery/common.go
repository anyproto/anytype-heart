package localdiscovery

import (
	"fmt"
	gonet "net"
	"runtime"
	"strings"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"go.uber.org/zap"

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

func (l *localDiscovery) getDiscoveryPossibility(newAddrs addrs.InterfacesAddrs) DiscoveryPossibility {
	// some sophisticated logic for ios, because of possible Local Network Restrictions
	var err error
	// we can extend it later to check on another platforms
	var checkSelfConnect = runtime.GOOS == "ios"
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
				if !checkSelfConnect {
					return DiscoveryPossible
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

func (l *localDiscovery) discoveryPossibilitySetState(state DiscoveryPossibility) {
	l.discoveryPossibilitySwapState(func(_ DiscoveryPossibility) DiscoveryPossibility {
		return state
	})
}

func (l *localDiscovery) discoveryPossibilitySwapState(f func(currentState DiscoveryPossibility) DiscoveryPossibility) {
	l.hookMu.Lock()
	defer l.hookMu.Unlock()
	newState := f(l.hookState)
	if l.hookState == newState {
		return
	}
	l.hookState = newState
	log.Debug("discovery possibility state changed", zap.Int("state", int(newState)))
	for _, callback := range l.hooks {
		callback(newState)
	}
}

func (l *localDiscovery) RegisterDiscoveryPossibilityHook(hook func(state DiscoveryPossibility)) {
	l.hookMu.Lock()
	defer l.hookMu.Unlock()
	l.hooks = append(l.hooks, hook)
}

func (l *localDiscovery) getAddresses() (ipv4, ipv6 []gonet.IP) {
	for _, iface := range l.interfacesAddrs.Interfaces {
		for _, addr := range iface.GetAddr() {
			ip := addr.(*gonet.IPNet).IP
			if ip.To4() != nil {
				ipv4 = append(ipv4, ip)
			} else {
				ipv6 = append(ipv6, ip)
			}
		}
	}

	if len(ipv4) == 0 {
		// fallback in case we have no ipv4 addresses from interfaces
		for _, addr := range l.interfacesAddrs.Addrs {
			ip := strings.Split(addr.String(), "/")[0]
			ipVal := gonet.ParseIP(ip)
			if ipVal.To4() != nil {
				ipv4 = append(ipv4, ipVal)
			} else {
				ipv6 = append(ipv6, ipVal)
			}
		}
		l.interfacesAddrs.SortIPsLikeInterfaces(ipv4)
	}
	return
}
