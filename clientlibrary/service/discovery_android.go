package service

import (
	"context"
	"net"
	"strconv"
	"strings"
	"sync"

	"github.com/anyproto/anytype-heart/space/spacecore/localdiscovery"
)

type AndroidDiscoveryProxy interface {
	SetObserver(observer DiscoveryObserver)
	RemoveObserver()
}

type ObservationResult interface {
	Port() int
	Ip() string // in case of multiple IPs, separated by comma
	PeerId() string
}

type DiscoveryObserver interface {
	Port() int
	PeerId() string
	ServiceType() string
	ObserveChange(result ObservationResult)
}

func SetDiscoveryProxy(proxy AndroidDiscoveryProxy) {
	localdiscovery.SetNotifierProvider(newNotifierProvider(proxy))
}

type notifierProvider struct {
	proxy AndroidDiscoveryProxy

	// the context is per Provide generation: the provider outlives an
	// account (SetDiscoveryProxy is called once per process), so Remove
	// canceling a provider-lifetime context would leave every later
	// Provide with a dead context and kill discovery after the first
	// account switch
	mu     sync.Mutex
	cancel context.CancelFunc
}

func newNotifierProvider(proxy AndroidDiscoveryProxy) *notifierProvider {
	return &notifierProvider{proxy: proxy}
}

func (n *notifierProvider) Provide(notifier localdiscovery.Notifier, port int, peerId, serviceName string) {
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
	notifier localdiscovery.Notifier
}

func newDiscoveryObserver(ctx context.Context, port int, peerId, serviceType string, notifier localdiscovery.Notifier) *discoveryObserver {
	return &discoveryObserver{
		ctx:         ctx,
		port:        port,
		peerId:      peerId,
		notifier:    notifier,
		serviceType: serviceType,
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

func (d *discoveryObserver) ObserveChange(result ObservationResult) {
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
	if d.notifier != nil {
		d.notifier.PeerDiscovered(d.ctx, peer, localdiscovery.OwnAddresses{})
	}
}
