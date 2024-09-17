package localdiscovery

import (
	"context"
	gonet "net"
	"strings"
	"sync"

	"github.com/anyproto/any-sync/accountservice"
	"github.com/anyproto/any-sync/app"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/net/addrs"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spacecore/clientserver"
)

var notifierProvider NotifierProvider
var proxyLock = sync.Mutex{}

type Hook int

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

	hookMu       sync.Mutex
	hookState    DiscoveryPossibility
	hooks        []HookCallback
	networkState NetworkStateService
}

func (l *localDiscovery) PeerDiscovered(peer DiscoveredPeer, own OwnAddresses) {
	log.Debug("discovered peer", zap.String("peerId", peer.PeerId), zap.Strings("addrs", peer.Addrs))
	if peer.PeerId == l.peerId {
		return
	}
	// TODO: move this to android side
	newAddrs, err := addrs.GetInterfacesAddrs()
	l.notifyP2PPossibilityState(l.getP2PPossibility(newAddrs))

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
	return &localDiscovery{hooks: make([]HookCallback, 0)}
}

func (l *localDiscovery) SetNotifier(notifier Notifier) {
	l.notifier = notifier
}

func (l *localDiscovery) Init(a *app.App) (err error) {
	l.peerId = a.MustComponent(accountservice.CName).(accountservice.Service).Account().PeerId
	l.drpcServer = a.MustComponent(clientserver.CName).(clientserver.ClientServer)
	l.manualStart = a.MustComponent(config.CName).(*config.Config).DontStartLocalNetworkSyncAutomatically
	l.networkState = app.MustComponent[NetworkStateService](a)
	return
}

func (l *localDiscovery) Run(ctx context.Context) (err error) {
	if l.manualStart {
		// let's wait for the explicit command to enable local discovery
		return
	}

	return l.Start()
}

func (l *localDiscovery) refreshInterfaces() {
	newAddrs, err := addrs.GetInterfacesAddrs()
	if err != nil {
		return
	}
	newAddrs.Interfaces = filterMulticastInterfaces(newAddrs.Interfaces)
	l.notifyP2PPossibilityState(l.getP2PPossibility(newAddrs))
}

func (l *localDiscovery) Start() (err error) {
	l.m.Lock()
	defer l.m.Unlock()
	if !l.drpcServer.ServerStarted() {
		l.notifyP2PPossibilityState(DiscoveryNoInterfaces)
		return
	}
	provider := getNotifierProvider()
	if provider == nil {
		return
	}
	provider.Provide(l, l.drpcServer.Port(), l.peerId, serviceName)
	l.networkState.RegisterHook(func(_ model.DeviceNetworkType) {
		l.refreshInterfaces()
	})

	l.refreshInterfaces()
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
