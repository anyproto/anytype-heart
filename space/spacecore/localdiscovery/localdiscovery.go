//go:build !android
// +build !android

package localdiscovery

import (
	"context"
	"fmt"
	gonet "net"
	"slices"
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
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spacecore/clientserver"
)

type Hook int

var interfacesSortPriority = []string{"wlan", "wl", "en", "eth", "tun", "tap", "utun", "lo"}

// queryStopTimeout bounds how long a refresh waits for the previous
// generation's query goroutines to stop. A var so tests can shorten it.
var queryStopTimeout = 5 * time.Second

// getInterfacesAddrs is a seam for tests to inject enumeration failures.
var getInterfacesAddrs = addrs.GetInterfacesAddrs

type localDiscovery struct {
	server *zeroconf.Server
	peerId string
	port   int

	componentCtx       context.Context
	componentCtxCancel context.CancelFunc
	queryCtx           context.Context
	queryCtxCancel     context.CancelFunc
	// closeWait tracks the query goroutines of the current server generation.
	// A fresh WaitGroup per generation (created in startQuerying, under l.m):
	// reusing one value across generations raced Close's Wait against the
	// refresh reassigning it.
	closeWait       *sync.WaitGroup
	interfacesAddrs addrs.InterfacesAddrs
	periodicCheck      periodicsync.PeriodicSync
	drpcServer         clientserver.ClientServer
	nodeConf           nodeconf.Configuration

	ipv4        []string
	ipv6        []string
	manualStart bool
	started     bool
	notifier    Notifier
	m           sync.Mutex
	refreshMu   sync.Mutex // serializes refreshInterfaces so l.m can be released across the server teardown
	// refreshTrigger (buffered 1) coalesces network-change refresh requests
	// for the refreshWorker goroutine
	refreshTrigger chan struct{}

	hookMu       sync.Mutex
	hookState    DiscoveryPossibility
	hooks        []HookCallback
	networkState NetworkStateService
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
	l.networkState = app.MustComponent[NetworkStateService](a)
	l.componentCtx, l.componentCtxCancel = context.WithCancel(context.Background())
	l.refreshTrigger = make(chan struct{}, 1)
	return
}

func (l *localDiscovery) Run(ctx context.Context) (err error) {
	return l.Start()
}

func (l *localDiscovery) Start() (err error) {
	if !l.drpcServer.ServerStarted() {
		l.discoveryPossibilitySetState(DiscoveryNoInterfaces)
		return
	}
	l.m.Lock()
	defer l.m.Unlock()
	if l.started {
		return
	}
	l.started = true
	l.networkState.RegisterHook(func(_ model.DeviceNetworkType) {
		l.onNetworkStateChanged()
	})
	go l.refreshWorker()

	l.port = l.drpcServer.Port()
	l.periodicCheck.Run()
	return
}

func (l *localDiscovery) Name() (name string) {
	return CName
}

func (l *localDiscovery) Close(ctx context.Context) (err error) {
	l.componentCtxCancel()
	l.periodicCheck.Close() // safe to close if not started

	if !l.drpcServer.ServerStarted() {
		return
	}

	l.m.Lock()
	if !l.started {
		l.m.Unlock()
		return
	}
	server := l.server
	closeWait := l.closeWait
	l.m.Unlock()

	if server != nil {
		start := time.Now()
		shutdownFinished := make(chan struct{})
		go func() {
			server.Shutdown()
			if closeWait != nil {
				closeWait.Wait()
			}
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

// onNetworkStateChanged is called when the device network type changes (e.g.
// cellular -> wifi, or plugging the phone into a Mac by cable). After such a
// transition the previous mDNS socket is usually dead — iOS leaves it in
// ENOTCONN — yet its interface addresses can look unchanged, so refreshInterfaces
// would skip the rebuild and the recv loop would keep hitting the dead socket
// (throttled by the backoff, but never recovering). We clear the cached
// addresses to force a full teardown + rebuild on the fresh interface. Network
// type is intentionally ignored: we can't tell wifi from a USB-cable LAN here,
// and both should keep discovery running.
//
// The rebuild itself runs on refreshWorker: it can take seconds (goodbye
// packets with write deadlines, self-connect probing), and this hook is
// invoked synchronously from the DeviceNetworkStateSet RPC under networkMu,
// so it must not block.
func (l *localDiscovery) onNetworkStateChanged() {
	l.m.Lock()
	l.interfacesAddrs = addrs.InterfacesAddrs{}
	l.m.Unlock()
	select {
	case l.refreshTrigger <- struct{}{}:
	default:
		// a refresh is already pending; it will pick up the cleared state
	}
}

func (l *localDiscovery) refreshWorker() {
	for {
		select {
		case <-l.componentCtx.Done():
			return
		case <-l.refreshTrigger:
			if err := l.refreshInterfaces(l.componentCtx); err != nil {
				log.Warn("refreshing interfaces on network change failed", zap.Error(err))
			}
		}
	}
}

func (l *localDiscovery) refreshInterfaces(ctx context.Context) (err error) {
	// refreshMu serializes the whole refresh so the periodic check and the
	// network-change hook can't interleave a rebuild while l.m is released below.
	l.refreshMu.Lock()
	defer l.refreshMu.Unlock()

	l.m.Lock()
	newAddrs, err := getInterfacesAddrs()
	if err != nil {
		// a transient enumeration failure must not be treated as "no
		// interfaces": that would tear the running server down
		l.m.Unlock()
		return fmt.Errorf("get interfaces addrs: %w", err)
	}
	if addrs.NetAddrsEqualUnordered(l.interfacesAddrs.Addrs, newAddrs.Addrs) {
		// this optimization allows to save syscalls to get addrs for every iface
		// also we may receive a new ip address on the existing interface
		l.discoveryPossibilitySwapState(func(currentState DiscoveryPossibility) DiscoveryPossibility {
			if currentState != DiscoveryLocalNetworkRestricted {
				return currentState
			}
			// do the check only if we are in restricted state, just in case it was disabled
			return l.getDiscoveryPossibility(newAddrs)
		})
		l.m.Unlock()
		return
	}

	newAddrs.Interfaces = filterMulticastInterfaces(newAddrs.Interfaces)
	newAddrs.SortInterfacesWithPriority(interfacesSortPriority)
	l.discoveryPossibilitySetState(l.getDiscoveryPossibility(newAddrs))

	if newAddrs.Equal(l.interfacesAddrs) && l.server != nil {
		// we do additional check after we filter and sort multicast interfaces
		// so this equal check is more precise
		l.m.Unlock()
		return
	}
	log.With(zap.Strings("ifaces", newAddrs.InterfaceNames())).Info("net interfaces configuration changed")
	l.interfacesAddrs = newAddrs
	server := l.server
	closeWait := l.closeWait
	l.server = nil
	if server != nil {
		l.queryCtxCancel()
	}
	l.m.Unlock()

	// Tear the old server down WITHOUT holding l.m: Shutdown + closeWait.Wait can
	// block on readAnswers, which itself needs l.m — holding it here deadlocks
	// (this mirrors Close). refreshMu keeps concurrent refreshes out of this window.
	if server != nil {
		server.Shutdown()
		if closeWait != nil && !waitWithTimeout(closeWait, queryStopTimeout) {
			// A stuck goroutine must not wedge every future refresh (and the
			// networkState hooks behind it); log and rebuild on a fresh
			// generation instead.
			log.Error("zeroconf query goroutines did not stop in time, proceeding with rebuild")
		}
	}

	l.m.Lock()
	defer l.m.Unlock()
	if len(l.interfacesAddrs.Interfaces) == 0 {
		return nil
	}
	// in case app close is called in between, exit fast, do not start server
	select {
	case <-l.componentCtx.Done():
		return
	default:
	}
	l.queryCtx, l.queryCtxCancel = context.WithCancel(l.componentCtx)
	if err = l.startServer(); err != nil {
		// the addr snapshot was already committed above; clear it so the next
		// periodic tick does not see "unchanged" and skip the retry forever
		l.interfacesAddrs = addrs.InterfacesAddrs{}
		return fmt.Errorf("starting mdns server: %w", err)
	}
	l.startQuerying(l.queryCtx)
	log.Debug("mdns server started")
	return
}

func (l *localDiscovery) startServer() (err error) {
	l.ipv4 = l.ipv4[:0]
	ipv4, _ := l.getAddresses() // ignore ipv6 for now
	for _, ip := range ipv4 {
		l.ipv4 = append(l.ipv4, ip.String())
	}
	log.Info("starting mdns server", zap.Strings("ips", l.ipv4), zap.Int("port", l.port), zap.String("peerId", l.peerId))
	l.server, err = zeroconf.RegisterProxy(
		l.peerId,
		serviceName,
		mdnsDomain,
		l.port,
		l.peerId,
		l.ipv4, // do not include ipv6 addresses, because they are disabled
		nil,
		l.interfacesAddrs.NetInterfaces(),
		zeroconf.TTL(3600),                            // big ttl because we don't have re-broadcasting
		zeroconf.ServerSelectIPTraffic(zeroconf.IPv4), // disable ipv6 for now
		zeroconf.WriteTimeout(time.Second*3),
	)
	return
}

// startQuerying is called under l.m.
func (l *localDiscovery) startQuerying(ctx context.Context) {
	closeWait := &sync.WaitGroup{}
	closeWait.Add(2)
	l.closeWait = closeWait
	listenCh := make(chan *zeroconf.ServiceEntry, 10)
	// snapshot the interfaces under l.m instead of letting browse read the
	// field unlocked later, racing with the next refresh
	ifaces := l.interfacesAddrs.NetInterfaces()

	go l.readAnswers(closeWait, listenCh)
	go l.browse(ctx, closeWait, ifaces, listenCh)
}

func (l *localDiscovery) readAnswers(closeWait *sync.WaitGroup, ch chan *zeroconf.ServiceEntry) {
	defer closeWait.Done()
	for entry := range ch {
		if entry.Instance == l.peerId {
			log.Debug("discovered self")
			continue
		}
		var portAddrs []string
		l.m.Lock()
		l.interfacesAddrs.SortIPsLikeInterfaces(entry.AddrIPv4)
		for _, a := range entry.AddrIPv4 {
			portAddrs = append(portAddrs, fmt.Sprintf("%s:%d", a.String(), entry.Port))
		}
		peer := DiscoveredPeer{
			Addrs:  portAddrs,
			PeerId: entry.Instance,
		}
		log.Debug("discovered peer", zap.Strings("addrs", portAddrs), zap.String("peerId", peer.PeerId))
		if l.notifier != nil {
			addrs := slices.Clone(l.ipv4)
			// explicitly use componentCtx, instead of queryCtx here, because we don't want to interrupt the peer connection if we refreshed interfaces and restarted service
			go l.notifier.PeerDiscovered(l.componentCtx, peer, OwnAddresses{
				Addrs: addrs,
				Port:  l.port,
			})
		}
		l.m.Unlock()
	}
}

func (l *localDiscovery) browse(ctx context.Context, closeWait *sync.WaitGroup, ifaces []gonet.Interface, ch chan *zeroconf.ServiceEntry) {
	defer closeWait.Done()
	if err := zeroconf.Browse(ctx, serviceName, mdnsDomain, ch,
		zeroconf.ClientWriteTimeout(time.Second*3),
		zeroconf.SelectIfaces(ifaces),
		zeroconf.SelectIPTraffic(zeroconf.IPv4)); err != nil {
		log.Error("browsing failed", zap.Error(err))
	}
}

// waitWithTimeout waits for wg up to timeout; false means the wait timed out.
func waitWithTimeout(wg *sync.WaitGroup, timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

func (l *localDiscovery) GetOwnAddresses() OwnAddresses {
	return OwnAddresses{
		Addrs: l.ipv4,
		Port:  l.port,
	}
}
