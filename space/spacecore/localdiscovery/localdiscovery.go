//go:build !android
// +build !android

package localdiscovery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	gonet "net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/anyproto/any-sync/accountservice"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/net/transport"
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
	periodicCheck   periodicsync.PeriodicSync
	drpcServer      clientserver.ClientServer
	nodeConf        nodeconf.Configuration

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

	// static peers (opt-in via <repoPath>/static-peers.json) — for L3 networks
	// where mDNS does not propagate (routed VPNs, containers, NAT'd subnets).
	repoPath         string
	lastStaticPeers  []staticPeer
	lastStaticMtime  time.Time
	staticMu         sync.Mutex
	lastOwnAddrBytes []byte
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
	l.repoPath = a.MustComponent(config.CName).(*config.Config).GetRepoPath()
	l.peerId = a.MustComponent(accountservice.CName).(accountservice.Service).Account().PeerId
	l.periodicCheck = periodicsync.NewPeriodicSync(5, 0, l.refreshInterfaces, log)
	l.drpcServer = app.MustComponent[clientserver.ClientServer](a)
	l.networkState = app.MustComponent[NetworkStateService](a)
	l.componentCtx, l.componentCtxCancel = context.WithCancel(context.Background())
	l.refreshTrigger = make(chan struct{}, 1)
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
	// Inject statically-configured peers on every tick, before the mDNS path
	// below (which is skipped when there are no multicast interfaces, e.g. on a
	// routed VPN). Opt-in via <repoPath>/static-peers.json; no-op without it.
	l.injectStaticPeers()

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
		zeroconf.TTL(3600), // big ttl because we don't have re-broadcasting
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

// staticPeer is a peer to dial directly, used on L3 networks where mDNS does
// not propagate (routed VPNs: OpenVPN tun, ZeroTier, WireGuard).
type staticPeer struct {
	PeerId    string   `json:"peerId"`
	Addresses []string `json:"addresses"` // each "host:port" (QUIC/UDP)
}

func (l *localDiscovery) staticPeersPath() string {
	return filepath.Join(l.repoPath, "static-peers.json")
}

// staticPeersMtime returns the mtime of static-peers.json and whether the
// file exists. Any other stat error is logged and reported like a missing
// file: the periodic tick retries soon enough.
func (l *localDiscovery) staticPeersMtime() (mtime time.Time, exists bool) {
	path := l.staticPeersPath()
	fi, err := os.Stat(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.Warn("static peers stat failed", zap.Error(err), zap.String("path", path))
		}
		return time.Time{}, false
	}
	return fi.ModTime(), true
}

// readStaticPeersFile reads and parses static-peers.json, returning nil on
// any error (which is logged).
func readStaticPeersFile(path string) (peers []staticPeer) {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Warn("static peers read failed", zap.Error(err), zap.String("path", path))
		return nil
	}
	if err := json.Unmarshal(data, &peers); err != nil {
		log.Warn("static peers parse failed", zap.Error(err), zap.String("path", path))
		return nil
	}
	return peers
}

// loadStaticPeers reads static-peers.json, re-parsing only when its mtime
// changed. Missing file → nil, false (feature stays opt-in). On a read or
// parse error the previously loaded peers are kept, and the mtime is still
// recorded so the broken file is not re-read on every tick.
func (l *localDiscovery) loadStaticPeers() (peers []staticPeer, changed bool) {
	mtime, exists := l.staticPeersMtime()
	if !exists {
		return nil, false
	}
	l.staticMu.Lock()
	defer l.staticMu.Unlock()
	if !mtime.After(l.lastStaticMtime) {
		return l.lastStaticPeers, false
	}
	loaded := readStaticPeersFile(l.staticPeersPath())
	l.lastStaticMtime = mtime
	if loaded == nil {
		return l.lastStaticPeers, false
	}
	l.lastStaticPeers = loaded
	return loaded, true
}

// staticPeerAddrs normalizes configured addresses to bare "host:port", the
// form the PeerDiscovered seam (and mDNS) emits. Full URLs — e.g. a
// listenAddr copied verbatim from the peer's own-address.json ("yamux://…",
// "quic://…") — are accepted and stripped: addSchema pins the transport, and
// both listeners share one port, so host:port identifies the peer either way.
func staticPeerAddrs(addrs []string) []string {
	res := make([]string, 0, len(addrs))
	for _, a := range addrs {
		if i := strings.Index(a, "://"); i != -1 {
			a = a[i+3:]
		}
		if a != "" {
			res = append(res, a)
		}
	}
	return res
}

// staticPeerIds returns the peer ids of peers, for logging.
func staticPeerIds(peers []staticPeer) (ids []string) {
	ids = make([]string, 0, len(peers))
	for _, p := range peers {
		ids = append(ids, p.PeerId)
	}
	return ids
}

// injectStaticPeers emits every statically-configured peer through the same
// PeerDiscovered seam as mDNS. No-op when static-peers.json is absent, so the
// feature is opt-in and own-address.json is not written for users who don't
// use it.
func (l *localDiscovery) injectStaticPeers() {
	peers, changed := l.loadStaticPeers()
	if peers == nil {
		return
	}
	if changed {
		log.Info("static peers loaded", zap.Strings("peerIds", staticPeerIds(peers)), zap.Int("count", len(peers)))
	}
	// Always enumerate ALL interfaces (including VPN/overlay) for the dial-back
	// hint + own-address.json — l.ipv4 only has multicast interfaces, which
	// excludes VPN tunnels (wt0, tun, etc.).
	own := l.enumerateAllAddresses()
	for _, p := range peers {
		l.notifyStaticPeer(p, own)
	}
	l.writeOwnAddress(own)
}

// notifyStaticPeer announces one configured peer, skipping entries with no
// peer id, our own id, or no usable address. `own` is a best-effort dial-back
// hint; the remote also learns our address from the connection source during
// SpaceExchange.
func (l *localDiscovery) notifyStaticPeer(p staticPeer, own OwnAddresses) {
	addrs := staticPeerAddrs(p.Addresses)
	if l.notifier == nil || p.PeerId == "" || p.PeerId == l.peerId || len(addrs) == 0 {
		return
	}
	// explicitly use componentCtx, like the mDNS path, so the peer connection
	// is not interrupted by an interface refresh
	go l.notifier.PeerDiscovered(
		l.componentCtx,
		DiscoveredPeer{PeerId: p.PeerId, Addrs: addrs},
		own,
	)
}

// enumerateAllAddresses returns all non-loopback IPv4 addresses across ALL
// interfaces (including VPN/overlay), not just multicast ones. Used for the
// dial-back hint and own-address.json, since l.ipv4 only has multicast
// interfaces, which excludes VPN tunnels (wt0, tun, etc.).
func (l *localDiscovery) enumerateAllAddresses() OwnAddresses {
	iaddrs, err := getInterfacesAddrs()
	if err != nil {
		return OwnAddresses{Port: l.port}
	}
	var ips []string
	for i := range iaddrs.Interfaces {
		ips = append(ips, ifaceIPv4Addrs(&iaddrs.Interfaces[i])...)
	}
	return OwnAddresses{Addrs: ips, Port: l.port}
}

// ifaceIPv4Addrs returns the non-loopback IPv4 addresses of one interface.
func ifaceIPv4Addrs(iface *addrs.NetInterfaceWithAddrCache) (ips []string) {
	for _, a := range iface.GetAddr() {
		if s := ipv4String(a); s != "" {
			ips = append(ips, s)
		}
	}
	return ips
}

// ipv4String returns the dotted form of addr if it is a non-loopback IPv4
// address, "" otherwise.
func ipv4String(addr gonet.Addr) string {
	ip, ok := addrs.AddrToIP(addr)
	if !ok || ip == nil {
		return ""
	}
	v4 := ip.To4()
	if v4 == nil || v4.IsLoopback() {
		return ""
	}
	return v4.String()
}

// ownAddressDoc renders own-address.json content in static-peers.json entry
// format (peerId + listen addresses), so the user can copy the file — or just
// the addresses array — verbatim into the remote peer's static-peers.json; no
// extra fields, since the yamux:// address already carries the port and
// schema. listenAddrs are advertised as yamux:// to match addSchema, which
// pins yamux (TCP) for local peer dials; the QUIC listener stays bound on the
// same port for peers that still dial QUIC.
func ownAddressDoc(peerId string, own OwnAddresses) (data []byte) {
	listen := make([]string, 0, len(own.Addrs))
	for _, a := range own.Addrs {
		listen = append(listen, fmt.Sprintf("%s://%s:%d", transport.Yamux, a, own.Port))
	}
	data, err := json.MarshalIndent([]staticPeer{{PeerId: peerId, Addresses: listen}}, "", "  ")
	if err != nil {
		return nil
	}
	return data
}

// writeOwnAddress persists own-address.json, skipping the write when the
// content is unchanged to avoid disk churn. Only called when
// static-peers.json exists (opt-in).
func (l *localDiscovery) writeOwnAddress(own OwnAddresses) {
	data := ownAddressDoc(l.peerId, own)
	if data == nil || slices.Equal(data, l.lastOwnAddrBytes) {
		return
	}
	l.lastOwnAddrBytes = data
	if err := os.WriteFile(filepath.Join(l.repoPath, "own-address.json"), data, 0640); err != nil {
		log.Warn("write own-address failed", zap.Error(err))
	}
}
