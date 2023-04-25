//go:build !android
// +build !android

package localdiscovery

import (
	"context"
	"fmt"
	"github.com/anytypeio/any-sync/accountservice"
	"github.com/anytypeio/any-sync/app"
	"github.com/anytypeio/any-sync/util/periodicsync"
	"github.com/anytypeio/go-anytype-middleware/net/addrs"
	"github.com/anytypeio/go-anytype-middleware/space/clientserver"
	"github.com/libp2p/zeroconf/v2"
	"go.uber.org/zap"
	gonet "net"
	"strings"
	"sync"
)

type localDiscovery struct {
	server *zeroconf.Server
	peerId string
	port   int

	ctx             context.Context
	cancel          context.CancelFunc
	closeWait       sync.WaitGroup
	interfacesAddrs addrs.InterfacesAddrs
	periodicCheck   periodicsync.PeriodicSync
	portProvider    clientserver.DRPCServer

	ipv4 []string
	ipv6 []string

	notifier Notifier
}

func New() LocalDiscovery {
	return &localDiscovery{}
}

func (l *localDiscovery) SetNotifier(notifier Notifier) {
	l.notifier = notifier
}

func (l *localDiscovery) Init(a *app.App) (err error) {
	l.peerId = a.MustComponent(accountservice.CName).(accountservice.Service).Account().PeerId
	l.periodicCheck = periodicsync.NewPeriodicSync(30, 0, l.checkAddrs, log)
	l.portProvider = a.MustComponent(clientserver.CName).(clientserver.DRPCServer)
	return
}

func (l *localDiscovery) Run(ctx context.Context) (err error) {
	l.port = l.portProvider.Port()
	l.periodicCheck.Run()
	return
}

func (l *localDiscovery) Name() (name string) {
	return CName
}

func (l *localDiscovery) Close(ctx context.Context) (err error) {
	l.periodicCheck.Close()
	l.cancel()
	if l.server != nil {
		l.server.Shutdown()
		l.closeWait.Wait()
	}
	return nil
}

func (l *localDiscovery) checkAddrs(ctx context.Context) (err error) {
	newAddrs, err := addrs.GetInterfacesAddrs()
	if err != nil {
		return
	}
	if newAddrs.Equal(l.interfacesAddrs) && l.server != nil {
		return
	}
	l.interfacesAddrs = newAddrs
	if l.server != nil {
		l.cancel()
		l.server.Shutdown()
		l.closeWait.Wait()
		l.closeWait = sync.WaitGroup{}
	}
	l.ctx, l.cancel = context.WithCancel(ctx)
	if err = l.startServer(); err != nil {
		return
	}
	l.startQuerying(l.ctx)
	return
}

func (l *localDiscovery) startServer() (err error) {
	l.ipv4 = l.ipv4[:0]
	l.ipv6 = l.ipv6[:0]
	for _, addr := range l.interfacesAddrs.Addrs {
		ip := strings.Split(addr.String(), "/")[0]
		if gonet.ParseIP(ip).To4() != nil {
			l.ipv4 = append(l.ipv4, ip)
		} else {
			l.ipv6 = append(l.ipv6, ip)
		}
	}
	log.Debug("starting mdns server", zap.Strings("ips", l.ipv4), zap.Int("port", l.port), zap.String("peerId", l.peerId))
	// for now assuming that we have just one address
	l.server, err = zeroconf.RegisterProxy(
		l.peerId,
		serviceName,
		mdnsDomain,
		l.port,
		l.peerId,
		l.ipv4,
		nil,
		nil,
		zeroconf.TTL(10),
	)
	return
}

func (l *localDiscovery) startQuerying(ctx context.Context) {
	l.closeWait.Add(2)
	listenCh := make(chan *zeroconf.ServiceEntry, 10)
	go l.readAnswers(listenCh)
	go l.browse(ctx, listenCh)
}

func (l *localDiscovery) readAnswers(ch chan *zeroconf.ServiceEntry) {
	defer l.closeWait.Done()
	for entry := range ch {
		if entry.Instance == l.peerId {
			log.Debug("discovered self")
			continue
		}
		var portAddrs []string
		for _, a := range entry.AddrIPv4 {
			portAddrs = append(portAddrs, fmt.Sprintf("%s:%d", a.String(), entry.Port))
		}
		peer := DiscoveredPeer{
			Addrs:  portAddrs,
			PeerId: entry.Instance,
		}
		log.Debug("discovered peer", zap.Strings("addrs", peer.Addrs), zap.String("peerId", peer.PeerId))
		if l.notifier != nil {
			l.notifier.PeerDiscovered(peer, OwnAddresses{
				Addrs: l.ipv4,
				Port:  l.port,
			})
		}
	}
}

func (l *localDiscovery) browse(ctx context.Context, ch chan *zeroconf.ServiceEntry) {
	defer l.closeWait.Done()
	if err := zeroconf.Browse(ctx, serviceName, mdnsDomain, ch); err != nil {
		log.Error("browsing failed", zap.Error(err))
	}
}
