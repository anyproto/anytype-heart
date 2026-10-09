package localdiscovery

import (
	"context"
	"fmt"
	"sync"

	"github.com/anyproto/any-sync/accountservice"
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/util/periodicsync"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/net/addrs"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spacecore/clientserver"
)

// The provider-backed implementation: discovery runs in the platform (Android
// NsdManager in anytype-kotlin, iOS DNS-SD in anytype-swift) behind the
// clientlibrary DiscoveryProxy bridge, and heart only receives the results.
// Android always uses it; iOS uses it when the app installed a proxy before
// the account started (see newForInstalledProvider). It compiles on every
// platform so its tests run in CI.

var notifierProvider NotifierProvider
var proxyLock = sync.Mutex{}

// ProviderNotifier is what the platform bridge reports to: discoveries plus
// the removal and error events only a platform discovery stack can deliver.
type ProviderNotifier interface {
	Notifier
	// PeerLost reports that the peer's service disappeared from every
	// interface (a goodbye or an expired record).
	PeerLost(peerId string)
	// DiscoveryError reports a platform discovery error code (iOS
	// kDNSServiceErr_*, Android NsdManager FAILURE_*). Code 0 reports a
	// healthy (re)registration and clears an earlier error state.
	DiscoveryError(code int, message string)
}

type NotifierProvider interface {
	Provide(notifier ProviderNotifier, port int, peerId, serviceName string)
	Remove()
}

func SetNotifierProvider(provider NotifierProvider) {
	// TODO: change to less ad-hoc mechanism and provide default way of injecting components from outside
	proxyLock.Lock()
	defer proxyLock.Unlock()
	notifierProvider = provider
}

func getNotifierProvider() NotifierProvider {
	proxyLock.Lock()
	defer proxyLock.Unlock()
	return notifierProvider
}

// backgroundPauser is implemented by clientserver once its iOS listener
// lifecycle exists (GO-7577); asserted so this compiles either way.
type backgroundPauser interface {
	SetPauseOnBackground(enabled bool)
}

// dnsServicePolicyDenied is kDNSServiceErr_PolicyDenied: the user denied (or
// has not yet answered) the iOS Local Network permission.
const dnsServicePolicyDenied = -65570

type providerDiscovery struct {
	discoveryBase

	peerId string
	port   int

	notifier      Notifier
	drpcServer    clientserver.ClientServer
	manualStart   bool
	periodicCheck periodicsync.PeriodicSync

	networkState NetworkStateService

	// m guards interfacesAddrs and policyDenied: refreshInterfaces writes
	// interfacesAddrs from the periodic goroutine and the networkState hook
	// while PeerDiscovered reads it from a platform thread
	m            sync.Mutex
	policyDenied bool
}

func (l *providerDiscovery) PeerDiscovered(ctx context.Context, peer DiscoveredPeer, _ OwnAddresses) {
	log.Debug("discovered peer", zap.String("peerId", peer.PeerId), zap.Strings("addrs", peer.Addrs))
	if peer.PeerId == l.peerId {
		return
	}

	l.m.Lock()
	own := newOwnAddresses(l.ownIPv4(), l.port)
	l.m.Unlock()
	l.clearPolicyDenied()
	if l.notifier != nil {
		l.notifier.PeerDiscovered(ctx, peer, own)
	}
}

func newProvider() *providerDiscovery {
	return &providerDiscovery{}
}

func (l *providerDiscovery) SetNotifier(notifier Notifier) {
	l.notifier = notifier
}

func (l *providerDiscovery) Init(a *app.App) (err error) {
	l.peerId = a.MustComponent(accountservice.CName).(accountservice.Service).Account().PeerId
	l.drpcServer = a.MustComponent(clientserver.CName).(clientserver.ClientServer)
	l.manualStart = a.MustComponent(config.CName).(*config.Config).DontStartLocalNetworkSyncAutomatically
	l.networkState = app.MustComponent[NetworkStateService](a)
	l.periodicCheck = periodicsync.NewPeriodicSync(5, 0, l.refreshInterfaces, log)

	return
}

func (l *providerDiscovery) Run(ctx context.Context) (err error) {
	if l.manualStart {
		// let's wait for the explicit command to enable local discovery
		return
	}

	return l.Start()
}

func (l *providerDiscovery) refreshInterfaces(_ context.Context) error {
	// serializes the periodic check against the networkState hook as well
	l.m.Lock()
	defer l.m.Unlock()
	newAddrs, err := addrs.GetInterfacesAddrs()
	if err != nil {
		return fmt.Errorf("get interfaces addrs: %w", err)
	}
	if addrs.NetAddrsEqualUnordered(newAddrs.Addrs, l.interfacesAddrs.Addrs) {
		return nil
	}

	newAddrs.Interfaces = filterMulticastInterfaces(newAddrs.Interfaces)
	l.interfacesAddrs = newAddrs
	logOwnAddresses(l.ownIPv4())
	state := l.getDiscoveryPossibility(newAddrs)
	if l.policyDenied {
		// an interface change does not lift a platform permission denial;
		// only the platform proving discovery works again does
		state = DiscoveryLocalNetworkRestricted
	}
	l.discoveryPossibilitySetState(state)
	return nil
}

func (l *providerDiscovery) Start() (err error) {
	if !l.drpcServer.ServerStarted() {
		l.discoveryPossibilitySetState(DiscoveryNoInterfaces)
		return
	}
	provider := getNotifierProvider()
	if provider == nil {
		return
	}
	provider.Provide(l, l.drpcServer.Port(), l.peerId, serviceName)
	// the platform withdraws our registration on background, so the LAN port
	// may close with it (GO-7577; a no-op where the listener has no lifecycle)
	if pauser, ok := l.drpcServer.(backgroundPauser); ok {
		pauser.SetPauseOnBackground(true)
	}
	l.networkState.RegisterHook(func(_ model.DeviceNetworkType) {
		_ = l.refreshInterfaces(context.Background())
	})

	l.port = l.drpcServer.Port()
	l.periodicCheck.Run()
	return
}

func (l *providerDiscovery) Name() (name string) {
	return CName
}

func (l *providerDiscovery) Close(ctx context.Context) (err error) {
	if !l.drpcServer.ServerStarted() {
		return
	}
	l.periodicCheck.Close()
	provider := getNotifierProvider()
	if provider == nil {
		return
	}
	provider.Remove()
	return nil
}

func (l *providerDiscovery) PeerLost(peerId string) {
	// phase 1: observability only. A lost peer is not dialed less yet; see
	// docs/superpowers/specs/2026-10-09-ios-native-mdns-design.md
	log.Info("local peer lost", zap.String("peerId", peerId))
}

func (l *providerDiscovery) DiscoveryError(code int, message string) {
	if code == 0 {
		l.clearPolicyDenied()
		return
	}
	log.Warn("platform discovery error", zap.Int("code", code), zap.String("message", message))
	if code != dnsServicePolicyDenied {
		return
	}
	l.m.Lock()
	l.policyDenied = true
	l.m.Unlock()
	l.discoveryPossibilitySetState(DiscoveryLocalNetworkRestricted)
}

// clearPolicyDenied recomputes the discovery possibility after the platform
// proved discovery works again (a registration or a discovered peer).
func (l *providerDiscovery) clearPolicyDenied() {
	l.m.Lock()
	if !l.policyDenied {
		l.m.Unlock()
		return
	}
	l.policyDenied = false
	current := l.interfacesAddrs
	l.m.Unlock()
	l.discoveryPossibilitySetState(l.getDiscoveryPossibility(current))
}
