package service

import (
	"context"
	"net"
	"strconv"
	"strings"
	"sync"

	"github.com/anyproto/anytype-heart/space/spacecore/localdiscovery"
)

// The LAN discovery bridge: the mobile app runs discovery on the platform
// stack (Android NsdManager in anytype-kotlin, iOS DNS-SD in anytype-swift)
// and reports results to heart through these gomobile interfaces. See
// docs/superpowers/specs/2026-10-09-ios-native-mdns-design.md.

// DiscoveryProxy is implemented by the app. Heart calls SetObserver when an
// account starts LAN discovery and RemoveObserver when it stops; the app
// advertises the observer's PeerId/Port and browses ServiceType meanwhile.
type DiscoveryProxy interface {
	SetObserver(observer DiscoveryObserver)
	RemoveObserver()
}

// ObservationResult is implemented by the app: one discovered peer.
type ObservationResult interface {
	Port() int
	Ip() string // in case of multiple IPs, separated by comma
	PeerId() string
}

// DiscoveryObserver is implemented by heart and called by the app. Every
// Observe* call returns immediately: the work runs on heart's goroutines, so
// the app may call from any thread, including its discovery queue.
type DiscoveryObserver interface {
	Port() int
	PeerId() string
	ServiceType() string
	ObserveChange(result ObservationResult)
	// ObserveLost reports that the peer's service disappeared from every
	// interface.
	ObserveLost(peerId string)
	// ObserveError reports a platform discovery error (iOS kDNSServiceErr_*,
	// Android NsdManager FAILURE_*). Code 0 reports a healthy
	// (re)registration and clears an earlier error state.
	ObserveError(code int, message string)
}

// SetDiscoveryProxy installs the app's discovery proxy, once per process and
// before any account starts.
func SetDiscoveryProxy(proxy DiscoveryProxy) {
	localdiscovery.SetNotifierProvider(newNotifierProvider(proxy))
}

type notifierProvider struct {
	proxy DiscoveryProxy

	// the context is per Provide generation: the provider outlives an
	// account (SetDiscoveryProxy is called once per process), so Remove
	// canceling a provider-lifetime context would leave every later
	// Provide with a dead context and kill discovery after the first
	// account switch
	mu     sync.Mutex
	cancel context.CancelFunc
}

func newNotifierProvider(proxy DiscoveryProxy) *notifierProvider {
	return &notifierProvider{proxy: proxy}
}

func (n *notifierProvider) Provide(notifier localdiscovery.ProviderNotifier, port int, peerId, serviceName string) {
	ctx, cancel := context.WithCancel(context.Background())
	n.mu.Lock()
	if n.cancel != nil {
		n.cancel()
	}
	n.cancel = cancel
	n.mu.Unlock()
	n.proxy.SetObserver(newDiscoveryObserver(ctx, port, peerId, serviceName, notifier))
}

func (n *notifierProvider) Remove() {
	n.mu.Lock()
	cancel := n.cancel
	n.cancel = nil
	n.mu.Unlock()
	if cancel != nil {
		cancel() // in order to cancel undergoing peers' space exchange requests
	}
	n.proxy.RemoveObserver()
}

type discoveryObserver struct {
	port        int
	peerId      string
	serviceType string

	ctx      context.Context
	notifier localdiscovery.ProviderNotifier

	// inflight holds one entry per peer whose PeerDiscovered is running; a
	// change reported meanwhile is parked in next (the latest one wins) and
	// runs once the current call returns
	mu       sync.Mutex
	inflight map[string]*pendingDiscovery
	// events queues lost/error reports for one draining goroutine, so they
	// reach the notifier in the order the app reported them (an error code 0
	// must not overtake the denial it clears)
	events        []func()
	eventsRunning bool
}

type pendingDiscovery struct {
	next *localdiscovery.DiscoveredPeer
}

func newDiscoveryObserver(ctx context.Context, port int, peerId, serviceType string, notifier localdiscovery.ProviderNotifier) *discoveryObserver {
	return &discoveryObserver{
		ctx:         ctx,
		port:        port,
		peerId:      peerId,
		notifier:    notifier,
		serviceType: serviceType,
		inflight:    map[string]*pendingDiscovery{},
	}
}

func (d *discoveryObserver) Port() int {
	return d.port
}

func (d *discoveryObserver) PeerId() string {
	return d.peerId
}

func (d *discoveryObserver) ServiceType() string {
	return d.serviceType
}

// ObserveChange hands the discovery to a goroutine and returns: the space
// exchange it triggers dials the peer and can take seconds, which must not
// hold the app's discovery thread.
func (d *discoveryObserver) ObserveChange(result ObservationResult) {
	if d.notifier == nil || d.ctx.Err() != nil {
		// a removed generation: the account stopped discovery
		return
	}
	// in the newer android API it can return multiple IPs separated by comma
	// sorry, slices are not supported in the bridge :'(
	var ips = strings.Split(result.Ip(), ",")
	var addrs = make([]string, 0, len(ips))
	port := strconv.Itoa(result.Port())
	for _, ip := range ips {
		if ip == "" {
			continue
		}
		// JoinHostPort brackets IPv6 addresses; "%s:%d" produced unparseable
		// fe80::1:4006-style strings
		addrs = append(addrs, net.JoinHostPort(ip, port))
	}
	peer := localdiscovery.DiscoveredPeer{
		Addrs:  addrs,
		PeerId: result.PeerId(),
	}

	d.mu.Lock()
	if pending, ok := d.inflight[peer.PeerId]; ok {
		pending.next = &peer
		d.mu.Unlock()
		return
	}
	d.inflight[peer.PeerId] = &pendingDiscovery{}
	d.mu.Unlock()
	go d.discover(peer)
}

// discover runs PeerDiscovered for one peer, then the latest change parked
// while it ran, until none is left.
func (d *discoveryObserver) discover(peer localdiscovery.DiscoveredPeer) {
	for {
		d.notifier.PeerDiscovered(d.ctx, peer, localdiscovery.OwnAddresses{})
		d.mu.Lock()
		pending := d.inflight[peer.PeerId]
		if pending.next == nil || d.ctx.Err() != nil {
			delete(d.inflight, peer.PeerId)
			d.mu.Unlock()
			return
		}
		peer, pending.next = *pending.next, nil
		d.mu.Unlock()
	}
}

func (d *discoveryObserver) ObserveLost(peerId string) {
	d.enqueue(func() { d.notifier.PeerLost(peerId) })
}

func (d *discoveryObserver) ObserveError(code int, message string) {
	d.enqueue(func() { d.notifier.DiscoveryError(code, message) })
}

func (d *discoveryObserver) enqueue(event func()) {
	if d.notifier == nil || d.ctx.Err() != nil {
		return
	}
	d.mu.Lock()
	d.events = append(d.events, event)
	if d.eventsRunning {
		d.mu.Unlock()
		return
	}
	d.eventsRunning = true
	d.mu.Unlock()
	go d.drainEvents()
}

func (d *discoveryObserver) drainEvents() {
	for {
		d.mu.Lock()
		if len(d.events) == 0 || d.ctx.Err() != nil {
			d.events = nil
			d.eventsRunning = false
			d.mu.Unlock()
			return
		}
		event := d.events[0]
		d.events = d.events[1:]
		d.mu.Unlock()
		event()
	}
}
