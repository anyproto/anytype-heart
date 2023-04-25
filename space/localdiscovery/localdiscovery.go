package localdiscovery

import (
	"context"
	"github.com/anytypeio/any-sync/accountservice"
	"github.com/anytypeio/any-sync/app"
	"github.com/anytypeio/any-sync/app/logger"
	"github.com/anytypeio/go-anytype-middleware/net/addrs"
	"github.com/libp2p/zeroconf/v2"
	"go.uber.org/zap"
	"strings"
	"sync"
)

const (
	CName = "client.space.localdiscovery"

	serviceName   = "_p2p._localdiscovery"
	mdnsDomain    = "local"
	anytypePrefix = "anytype="
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
	return
}

func (l *localDiscovery) Run(ctx context.Context) (err error) {
	l.ctx, l.cancel = context.WithCancel(ctx)
	if err = l.startServer(); err != nil {
		return
	}
	l.startListener(l.ctx)
	return
}

func (l *localDiscovery) Name() (name string) {
	return CName
}

func (l *localDiscovery) Close(ctx context.Context) (err error) {
	l.cancel()
	if l.server != nil {
		l.server.Shutdown()
	}
	l.closeWait.Wait()
	return nil
}

func (l *localDiscovery) startServer() (err error) {
	interfaceAddrs, err := addrs.InterfaceAddrs()
	if err != nil {
		return
	}
	var (
		ips  []string
		txts []string
	)
	for _, addr := range interfaceAddrs {
		ip := strings.Split(addr.String(), "/")[0]
		ips = append(ips, ip)
		txts = append(txts, anytypePrefix+ip)
	}
	l.server, err = zeroconf.RegisterProxy(
		l.peerId,
		serviceName,
		mdnsDomain,
		4001,
		l.peerId,
		ips,
		txts,
		nil,
	)
	return
}

func (l *localDiscovery) startListener(ctx context.Context) {
	l.closeWait.Add(2)
	listenCh := make(chan *zeroconf.ServiceEntry, 10)
	go l.readChannel(listenCh)
	go l.writeChannel(ctx, listenCh)
}

func (l *localDiscovery) readChannel(ch chan *zeroconf.ServiceEntry) {
	defer l.closeWait.Done()
	for entry := range ch {
		for _, text := range entry.Text {
			if !strings.HasPrefix(text, anytypePrefix) {
				log.Debug("incorrect prefix, text", zap.String("text", text))
				continue
			}
			peer := DiscoveredPeer{
				Addr:   text[len(anytypePrefix):],
				PeerId: entry.Service,
			}
			log.Debug("discovered peer", zap.String("addr", peer.Addr), zap.String("peerId", peer.PeerId))
			if l.notifier != nil {
				l.notifier.PeerDiscovered(peer)
			}
		}
	}
}

func (l *localDiscovery) writeChannel(ctx context.Context, ch chan *zeroconf.ServiceEntry) {
	defer l.closeWait.Done()
	if err := zeroconf.Browse(ctx, l.peerId, mdnsDomain, ch); err != nil {
		log.Debug("browsing failed", zap.Error(err))
	}
}
