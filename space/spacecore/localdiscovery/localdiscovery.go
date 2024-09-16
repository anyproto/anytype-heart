//go:build !android
// +build !android

package localdiscovery

import (
	"context"
	"fmt"
	gonet "net"
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

const (
	PeerToPeerImpossible Hook = 0
	PeerToPeerPossible   Hook = 1
)

type HookCallback func()

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

	hookMu sync.Mutex
	hooks  map[Hook][]HookCallback
}

func New() LocalDiscovery {
	return &localDiscovery{hooks: make(map[Hook][]HookCallback, 0)}
}

func (l *localDiscovery) SetNotifier(notifier Notifier) {
	l.notifier = notifier
}

func (l *localDiscovery) Init(a *app.App) (err error) {
	l.manualStart = a.MustComponent(config.CName).(*config.Config).DontStartLocalNetworkSyncAutomatically
	l.nodeConf = a.MustComponent(config.CName).(*config.Config).GetNodeConf()
	l.peerId = a.MustComponent(accountservice.CName).(accountservice.Service).Account().PeerId
	l.periodicCheck = periodicsync.NewPeriodicSync(5, 0, l.checkAddrs, log)
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
		l.executeHook(PeerToPeerImpossible)
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

func (l *localDiscovery) RegisterP2PNotPossible(hook func()) {
	l.hookMu.Lock()
	defer l.hookMu.Unlock()
	l.hooks[PeerToPeerImpossible] = append(l.hooks[PeerToPeerImpossible], hook)
}

func (l *localDiscovery) RegisterResetNotPossible(hook func()) {
	l.hookMu.Lock()
	defer l.hookMu.Unlock()
	l.hooks[PeerToPeerPossible] = append(l.hooks[PeerToPeerPossible], hook)
}

func (l *localDiscovery) checkAddrs(ctx context.Context) (err error) {
	newAddrs, err := addrs.GetInterfacesAddrs()
	l.notifyPeerToPeerStatus(newAddrs)
	if err != nil {
		return fmt.Errorf("getting iface addresses: %w", err)
	}

	newAddrs.SortInterfacesWithPriority(interfacesSortPriority)

	if newAddrs.Equal(l.interfacesAddrs) && l.server != nil {
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
		zeroconf.TTL(60),
		zeroconf.ServerSelectIPTraffic(zeroconf.IPv4), // disable ipv6 for now
		zeroconf.WriteTimeout(time.Second*1),
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
	newAddrs, err := addrs.GetInterfacesAddrs()
	l.notifyPeerToPeerStatus(newAddrs)

	if err != nil {
		return
	}
	newAddrs.SortInterfacesWithPriority(interfacesSortPriority)
	if err := zeroconf.Browse(ctx, serviceName, mdnsDomain, ch,
		zeroconf.ClientWriteTimeout(time.Second*1),
		zeroconf.SelectIfaces(newAddrs.NetInterfaces()),
		zeroconf.SelectIPTraffic(zeroconf.IPv4)); err != nil {
		log.Error("browsing failed", zap.Error(err))
	}
}

func (l *localDiscovery) notifyPeerToPeerStatus(newAddrs addrs.InterfacesAddrs) {
	if l.notifyP2PNotPossible(newAddrs) {
		l.executeHook(PeerToPeerImpossible)
	} else {
		l.executeHook(PeerToPeerPossible)
	}
}

func (l *localDiscovery) notifyP2PNotPossible(newAddrs addrs.InterfacesAddrs) bool {
	return len(newAddrs.Interfaces) == 0 || addrs.IsLoopBack(newAddrs.NetInterfaces())
}

func (l *localDiscovery) executeHook(hook Hook) {
	hooks := l.getHooks(hook)
	for _, callback := range hooks {
		callback()
	}
}

func (l *localDiscovery) getHooks(hook Hook) []HookCallback {
	l.hookMu.Lock()
	defer l.hookMu.Unlock()
	if hooks, ok := l.hooks[hook]; ok {
		callback := make([]HookCallback, 0, len(hooks))
		callback = append(callback, hooks...)
		return callback
	}
	return nil
}
