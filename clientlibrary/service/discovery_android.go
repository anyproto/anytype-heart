package service

import (
	"fmt"
	"strings"

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
}

func newNotifierProvider(proxy AndroidDiscoveryProxy) *notifierProvider {
	return &notifierProvider{
		proxy: proxy,
	}
}

func (n *notifierProvider) Provide(notifier localdiscovery.Notifier, port int, peerId, serviceName string) {
	n.proxy.SetObserver(newDiscoveryObserver(port, peerId, serviceName, notifier))
}

func (n *notifierProvider) Remove() {
	n.proxy.RemoveObserver()
}

type discoveryObserver struct {
	port        int
	peerId      string
	serviceType string

	notifier localdiscovery.Notifier
}

func newDiscoveryObserver(port int, peerId, serviceType string, notifier localdiscovery.Notifier) *discoveryObserver {
	return &discoveryObserver{
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
	for _, ip := range ips {
		addrs = append(addrs, fmt.Sprintf("%s:%d", ip, result.Port()))
	}
	peer := localdiscovery.DiscoveredPeer{
		Addrs:  addrs,
		PeerId: result.PeerId(),
	}
	if d.notifier != nil {
		d.notifier.PeerDiscovered(peer, localdiscovery.OwnAddresses{})
	}
}
