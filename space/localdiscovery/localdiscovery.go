package localdiscovery

import (
	"context"
	"fmt"
	"github.com/anytypeio/any-sync/accountservice"
	"github.com/anytypeio/any-sync/app"
	"github.com/anytypeio/any-sync/app/logger"
	"github.com/anytypeio/any-sync/net"
	"github.com/anytypeio/go-anytype-middleware/core/anytype/config"
	"github.com/anytypeio/go-anytype-middleware/net/addrs"
	"github.com/libp2p/zeroconf/v2"
	"go.uber.org/zap"
	"strconv"
	"strings"
	"sync"
)

const (
	CName = "client.space.localdiscovery"

	serviceName = "_p2p._localdiscovery"
	mdnsDomain  = "local"
)

var log = logger.NewNamed(CName)

type DiscoveredPeer struct {
	Addrs  []string
	PeerId string
}

type Notifier interface {
	PeerDiscovered(peer DiscoveredPeer)
}

type LocalDiscovery interface {
	SetNotifier(Notifier)
	app.ComponentRunnable
}

type localDiscovery struct {
	server *zeroconf.Server
	peerId string
	addrs  []string
	port   int

	ctx             context.Context
	cancel          context.CancelFunc
	closeWait       sync.WaitGroup
	interfacesAddrs addrs.InterfacesAddrs

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
	addrs := a.MustComponent(config.CName).(net.ConfigGetter).GetNet().Server.ListenAddrs
	l.port, err = getPort(addrs)
	return
}

func (l *localDiscovery) Run(ctx context.Context) (err error) {
	if l.port == 0 {
		return
	}
	l.ctx, l.cancel = context.WithCancel(ctx)
	if err = l.startServer(); err != nil {
		return
	}
	l.startQuerying(l.ctx)
	return
}

func (l *localDiscovery) Name() (name string) {
	return CName
}

func (l *localDiscovery) Close(ctx context.Context) (err error) {
	if len(l.addrs) == 0 {
		return
	}
	l.cancel()
	if l.server != nil {
		l.server.Shutdown()
	}
	l.closeWait.Wait()
	return nil
}

func (l *localDiscovery) startServer() (err error) {
	l.interfacesAddrs, err = addrs.GetInterfacesAddrs()
	if err != nil {
		return
	}
	var ips []string
	for _, addr := range l.interfacesAddrs.Addrs {
		ip := strings.Split(addr.String(), "/")[0]
		ips = append(ips, ip)
	}
	// for now assuming that we have just one address
	l.server, err = zeroconf.RegisterProxy(
		l.peerId,
		serviceName,
		mdnsDomain,
		l.port,
		l.peerId,
		ips,
		nil,
		l.interfacesAddrs.Interfaces,
		zeroconf.TTL(60),
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
		// TODO: check why this happens
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
			l.notifier.PeerDiscovered(peer)
		}
	}
}

func (l *localDiscovery) browse(ctx context.Context, ch chan *zeroconf.ServiceEntry) {
	defer l.closeWait.Done()
	if err := zeroconf.Browse(ctx, serviceName, mdnsDomain, ch, zeroconf.SelectIfaces(l.interfacesAddrs.Interfaces)); err != nil {
		log.Error("browsing failed", zap.Error(err))
	}
}

func getPort(addrs []string) (port int, err error) {
	if len(addrs) == 0 {
		err = fmt.Errorf("addresses are empty")
		return
	}
	split := strings.Split(addrs[0], ":")
	_, portString := split[0], split[1]
	return strconv.Atoi(portString)
}
