package localdiscovery

import (
	"context"
	"github.com/anytypeio/any-sync/accountservice"
	"github.com/anytypeio/any-sync/app"
	"github.com/anytypeio/any-sync/net"
	"github.com/anytypeio/go-anytype-middleware/core/anytype/config"
	"go.uber.org/zap"
	"sync"
)

var notifierProvider NotifierProvider
var proxyLock = sync.Mutex{}

type NotifierProvider interface {
	Provide(notifier Notifier, port int, peerId, serviceName string)
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

type localDiscovery struct {
	peerId string
	port   int

	notifier Notifier
}

func (l *localDiscovery) PeerDiscovered(peer DiscoveredPeer) {
	log.Debug("discovered peer", zap.String("peerId", peer.PeerId), zap.Strings("addrs", peer.Addrs))
	if peer.PeerId == l.peerId {
		return
	}
	if l.notifier != nil {
		l.notifier.PeerDiscovered(peer)
	}
}

func New() LocalDiscovery {
	return &localDiscovery{}
}

func (l *localDiscovery) SetNotifier(notifier Notifier) {
	l.notifier = notifier
}

func (l *localDiscovery) Init(a *app.App) (err error) {
	l.peerId = a.MustComponent(accountservice.CName).(accountservice.Service).Account().PeerId
	addrs := a.MustComponent(config.CName).(net.ConfigGetter).GetNet().Server.ListenAddrs
	l.port, err = getPort(addrs)
	return
}

func (l *localDiscovery) Run(ctx context.Context) (err error) {
	if l.port == 0 {
		return
	}
	provider := getNotifierProvider()
	if provider == nil {
		return
	}
	provider.Provide(l, l.port, l.peerId, serviceName)
	return
}

func (l *localDiscovery) Name() (name string) {
	return CName
}

func (l *localDiscovery) Close(ctx context.Context) (err error) {
	provider := getNotifierProvider()
	if provider == nil {
		return
	}
	provider.Remove()
	return nil
}
