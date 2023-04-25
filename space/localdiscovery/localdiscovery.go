package localdiscovery

import (
	"context"
	"fmt"
	"github.com/anytypeio/any-sync/accountservice"
	"github.com/anytypeio/any-sync/app"
	"github.com/anytypeio/any-sync/app/logger"
	"github.com/anytypeio/any-sync/net"
	"github.com/anytypeio/go-anytype-middleware/core/anytype/config"
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
	Addr   string
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

	ctx       context.Context
	cancel    context.CancelFunc
	closeWait sync.WaitGroup

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
	l.addrs = a.MustComponent(config.CName).(net.ConfigGetter).GetNet().Server.ListenAddrs
	return
}

func (l *localDiscovery) Run(ctx context.Context) (err error) {
	if len(l.addrs) == 0 {
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
	// for now assuming that we have just one address
	split := strings.Split(l.addrs[0], ":")
	ip, portString := split[0], split[1]
	port, err := strconv.Atoi(portString)
	if err != nil {
		return
	}
	l.server, err = zeroconf.RegisterProxy(
		l.peerId,
		serviceName,
		mdnsDomain,
		port,
		l.peerId,
		[]string{ip},
		nil,
		nil,
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
			continue
		}
		peer := DiscoveredPeer{
			Addr:   fmt.Sprintf("%s:%d", entry.AddrIPv4, entry.Port),
			PeerId: entry.Instance,
		}
		log.Debug("discovered peer", zap.String("addr", peer.Addr), zap.String("peerId", peer.PeerId))
		if l.notifier != nil {
			l.notifier.PeerDiscovered(peer)
		}
	}
}

func (l *localDiscovery) browse(ctx context.Context, ch chan *zeroconf.ServiceEntry) {
	defer l.closeWait.Done()
	if err := zeroconf.Browse(ctx, serviceName, mdnsDomain, ch); err != nil {
		log.Debug("browsing failed", zap.Error(err))
	}
}
