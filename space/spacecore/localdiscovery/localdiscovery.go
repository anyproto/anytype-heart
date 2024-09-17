//go:build !android
// +build !android

package localdiscovery

import (
	"context"
	"fmt"
	gonet "net"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/anyproto/any-sync/accountservice"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/nodeconf"
	"github.com/anyproto/any-sync/util/periodicsync"
	"github.com/libp2p/zeroconf/v2"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/net/addrs"
	"github.com/anyproto/anytype-heart/space/spacecore/clientserver"
)

type Hook int

var interfacesSortPriority = []string{"wlan", "wl", "en", "eth", "tun", "tap", "utun", "lo"}

type DiscoveryPossibility int

const (
	DiscoveryPossible               DiscoveryPossibility = 0
	DiscoveryNoInterfaces           DiscoveryPossibility = 2
	DiscoveryLocalNetworkRestricted DiscoveryPossibility = 3
)

type HookCallback func(state DiscoveryPossibility)

type localDiscovery struct {
	server *zeroconf.Server
	peerId string
	port   int

	ctx             context.Context
	cancel          context.CancelFunc
	closeWait       sync.WaitGroup
	interfacesAddrs addrs.InterfacesAddrs
	periodicCheck   periodicsync.PeriodicSync
	drpcServer      clientserver.ClientServer
	nodeConf        nodeconf.Configuration

	ipv4        []string
	ipv6        []string
	manualStart bool
	started     bool
	notifier    Notifier
	m           sync.Mutex

	hookMu    sync.Mutex
	hookState DiscoveryPossibility
	hooks     []HookCallback
}

func New() LocalDiscovery {
	return &localDiscovery{hooks: make([]HookCallback, 0)}
}

func (l *localDiscovery) SetNotifier(notifier Notifier) {
	l.notifier = notifier
}

func (l *localDiscovery) Init(a *app.App) (err error) {
	l.manualStart = a.MustComponent(config.CName).(*config.Config).DontStartLocalNetworkSyncAutomatically
	l.nodeConf = a.MustComponent(config.CName).(*config.Config).GetNodeConf()
	l.peerId = a.MustComponent(accountservice.CName).(accountservice.Service).Account().PeerId
	l.periodicCheck = periodicsync.NewPeriodicSync(5, 0, l.refreshInterfaces, log)
	l.drpcServer = app.MustComponent[clientserver.ClientServer](a)
	return
}

func (l *localDiscovery) Run(ctx context.Context) (err error) {
	if l.manualStart && len(l.nodeConf.Nodes) > 0 {
		// let's wait for the explicit command to enable local discovery
		return
	}

	return l.Start()
}

func (l *localDiscovery) Start() (err error) {
	if !l.drpcServer.ServerStarted() {
		l.notifyP2PPossibilityState(DiscoveryNoInterfaces)
		return
	}
	l.m.Lock()
	defer l.m.Unlock()
	if l.started {
		return
	}
	l.started = true

	l.port = l.drpcServer.Port()
	l.periodicCheck.Run()
	return
}

func (l *localDiscovery) Name() (name string) {
	return CName
}

func (l *localDiscovery) Close(ctx context.Context) (err error) {
	if !l.drpcServer.ServerStarted() {
		return
	}
	l.m.Lock()
	if !l.started {
		l.m.Unlock()
		return
	}
	l.m.Unlock()

	l.periodicCheck.Close()
	l.cancel()
	if l.server != nil {
		start := time.Now()
		shutdownFinished := make(chan struct{})
		go func() {
			l.server.Shutdown()
			l.closeWait.Wait()
			close(shutdownFinished)
			spent := time.Since(start)
			if spent.Milliseconds() > 500 {
				log.Warn("zeroconf server shutdown took too long", zap.Duration("spent", spent))
			}
		}()

		select {
		case <-shutdownFinished:
			return nil
		case <-time.After(time.Second * 1):
			// we can't afford to wait for too long
			return nil
		}
	}
	return nil
}

func (l *localDiscovery) RegisterDiscoveryPossibilityHook(hook func(state DiscoveryPossibility)) {
	l.hookMu.Lock()
	defer l.hookMu.Unlock()
	l.hooks = append(l.hooks, hook)
}

func (l *localDiscovery) refreshInterfaces(ctx context.Context) (err error) {
	newAddrs, err := addrs.GetInterfacesAddrs()
	if !addrs.NetAddrsEqualUnordered(l.interfacesAddrs.Addrs, newAddrs.Addrs) {
		// only replace existing interface structs in case if we have a different set of addresses
		// this optimization allows to save syscalls to get addrs for every iface, as we have a cache
		newAddrs.Interfaces = filterMulticastInterfaces(newAddrs.Interfaces)
		newAddrs.SortInterfacesWithPriority(interfacesSortPriority)
		fmt.Printf("#p2p local discovery: new interfaces(%d) %v\n", len(newAddrs.Interfaces), newAddrs.NetInterfaces())

	}

	l.notifyP2PPossibilityState(l.getP2PPossibility(newAddrs))
	if newAddrs.Equal(l.interfacesAddrs) && l.server != nil {
		// we do additional check after we filter and sort multicast interfaces
		// so this equal check is more precise
		return
	}
	l.interfacesAddrs = newAddrs
	if l.server != nil {
		l.cancel()
		l.server.Shutdown()
		l.closeWait.Wait()
		l.closeWait = sync.WaitGroup{}
	}
	l.ctx, l.cancel = context.WithCancel(ctx)
	if err = l.startServer(); err != nil {
		return fmt.Errorf("starting mdns server: %w", err)
	}
	l.startQuerying(l.ctx)
	return
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

func (l *localDiscovery) startServer() (err error) {
	l.ipv4 = l.ipv4[:0]
	ipv4, _ := l.getAddresses() // ignore ipv6 for now
	for _, ip := range ipv4 {
		l.ipv4 = append(l.ipv4, ip.String())
	}
	log.Debug("starting mdns server", zap.Strings("ips", l.ipv4), zap.Int("port", l.port), zap.String("peerId", l.peerId))
	l.server, err = zeroconf.RegisterProxy(
		l.peerId,
		serviceName,
		mdnsDomain,
		l.port,
		l.peerId,
		l.ipv4, // do not include ipv6 addresses, because they are disabled
		nil,
		l.interfacesAddrs.NetInterfaces(),
		zeroconf.TTL(3600), // big ttl because we don't have re-broadcasting
		zeroconf.ServerSelectIPTraffic(zeroconf.IPv4), // disable ipv6 for now
		zeroconf.WriteTimeout(time.Second*3),
	)
	return
}

func (l *localDiscovery) startQuerying(ctx context.Context) {
	l.closeWait.Add(2)
	listenCh := make(chan *zeroconf.ServiceEntry, 10)
	go l.readAnswers(listenCh)
	go l.browse(ctx, listenCh)
}

func (l *localDiscovery) readAnswers(ch chan *zeroconf.ServiceEntry) {
	defer l.closeWait.Done()
	for entry := range ch {
		if entry.Instance == l.peerId {
			log.Debug("discovered self")
			continue
		}
		var portAddrs []string
		l.interfacesAddrs.SortIPsLikeInterfaces(entry.AddrIPv4)
		for _, a := range entry.AddrIPv4 {
			portAddrs = append(portAddrs, fmt.Sprintf("%s:%d", a.String(), entry.Port))
		}
		peer := DiscoveredPeer{
			Addrs:  portAddrs,
			PeerId: entry.Instance,
		}
		log.Debug("discovered peer", zap.Strings("addrs", peer.Addrs), zap.String("peerId", peer.PeerId))
		if l.notifier != nil {
			l.notifier.PeerDiscovered(peer, OwnAddresses{
				Addrs: l.ipv4,
				Port:  l.port,
			})
		}
	}
}

func (l *localDiscovery) browse(ctx context.Context, ch chan *zeroconf.ServiceEntry) {
	defer l.closeWait.Done()
	if err := zeroconf.Browse(ctx, serviceName, mdnsDomain, ch,
		zeroconf.ClientWriteTimeout(time.Second*3),
		zeroconf.SelectIfaces(l.interfacesAddrs.NetInterfaces()),
		zeroconf.SelectIPTraffic(zeroconf.IPv4)); err != nil {
		log.Error("browsing failed", zap.Error(err))
	}
}

func (l *localDiscovery) GetOwnAddresses() OwnAddresses {
	return OwnAddresses{
		Addrs: l.ipv4,
		Port:  l.port,
	}
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
