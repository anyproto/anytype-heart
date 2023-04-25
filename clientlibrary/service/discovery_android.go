package service

import (
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/space/localdiscovery"
)

type AndroidDiscoveryProxy interface {
	SetObserver(observer DiscoveryObserver)
	RemoveObserver()
}

type ObservationResult interface {
	Port() int
	Ip() string
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
	peer := localdiscovery.DiscoveredPeer{
		Addrs:  []string{fmt.Sprintf("%s:%d", result.Ip(), result.Port())},
		PeerId: result.PeerId(),
	}
	if d.notifier != nil {
		d.notifier.PeerDiscovered(peer)
	}
}
