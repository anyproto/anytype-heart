package localdiscovery

import (
	"context"
	gonet "net"
	"strings"
	"sync"

	"github.com/anyproto/any-sync/accountservice"
	"github.com/anyproto/any-sync/app"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/net/addrs"
	"github.com/anyproto/anytype-heart/space/clientserver"
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

	notifier    Notifier
	drpcServer  clientserver.ClientServer
	manualStart bool
	m           sync.Mutex
}

func (l *localDiscovery) PeerDiscovered(peer DiscoveredPeer, own OwnAddresses) {
	log.Debug("discovered peer", zap.String("peerId", peer.PeerId), zap.Strings("addrs", peer.Addrs))
	if peer.PeerId == l.peerId {
		return
	}
	// TODO: move this to android side
	newAddrs, err := addrs.GetInterfacesAddrs()
	if err != nil {
		return
	}
	var ips []string
	for _, addr := range newAddrs.Addrs {
		ip := strings.Split(addr.String(), "/")[0]
		if gonet.ParseIP(ip).To4() != nil {
			ips = append(ips, ip)
		}
	}
	if l.notifier != nil {
		l.notifier.PeerDiscovered(peer, OwnAddresses{
			Addrs: ips,
			Port:  l.port,
		})
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
	l.drpcServer = a.MustComponent(clientserver.CName).(clientserver.ClientServer)
	l.manualStart = a.MustComponent(config.CName).(*config.Config).DontStartLocalNetworkSyncAutomatically

	return
}

func (l *localDiscovery) Run(ctx context.Context) (err error) {
	if l.manualStart {
		// let's wait for the explicit command to enable local discovery
		return
	}

	return l.Start()
}

func (l *localDiscovery) Start() (err error) {
	l.m.Lock()
	defer l.m.Unlock()
	if !l.drpcServer.ServerStarted() {
		return
	}
	provider := getNotifierProvider()
	if provider == nil {
		return
	}
	provider.Provide(l, l.drpcServer.Port(), l.peerId, serviceName)
	return
}

func (l *localDiscovery) Name() (name string) {
	return CName
}

func (l *localDiscovery) Close(ctx context.Context) (err error) {
	if !l.drpcServer.ServerStarted() {
		return
	}
	provider := getNotifierProvider()
	if provider == nil {
		return
	}
	provider.Remove()
	return nil
}
