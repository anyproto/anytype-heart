package peermanager

/*
AI generated

Name: Peer Manager Provider
Scope: global

## Responsibility
- Factory for creating per-space clientPeerManager instances
- Provides shared connection pool access (unary and stream)
- Bridges device connectivity signals to all per-space managers: on every
  connectivity recovery (network switch, wake, interface change) it signals an
  immediate responsible-peers rebuild; while the device is known offline the
  managers stretch their re-dial cadence
*/

import (
	"context"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/debugstat"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/commonspace/peermanager"
	"github.com/anyproto/any-sync/net/pool"
	"go.uber.org/atomic"

	"github.com/anyproto/anytype-heart/space/spacecore/peerstore"
)

// DeviceNetworkStateCName mirrors core/device.CName; importing the package
// would create an import cycle (core/device's devices service reaches back
// into spacecore), so the component is looked up by name and the minimal
// networkConnectivity interface below. A drift test in core/device asserts
// the two constants stay equal.
const DeviceNetworkStateCName = "networkState"

func New() peermanager.PeerManagerProvider {
	return &provider{}
}

const CName = peermanager.CName

var log = logger.NewNamed(CName)

// networkConnectivity is the part of device.NetworkState the peer managers
// need; kept minimal so tests don't have to build the whole component.
type networkConnectivity interface {
	RegisterConnectivityHook(hook func(online bool))
	IsOffline() bool
}

type provider struct {
	pool         pool.Pool
	peerStore    peerstore.PeerStore
	connectivity networkConnectivity

	mu       sync.Mutex
	managers map[*clientPeerManager]struct{}

	stats providerStats
}

// providerStats counts how the connectivity-driven peer machinery is
// exercised: single atomic ops on event-driven paths, snapshot built only
// when the debug stat endpoint asks.
type providerStats struct {
	connectivityEvents  atomic.Int64
	rebuildSignals      atomic.Int64
	reconnectDiffKicks  atomic.Int64
	closedPeersFiltered atomic.Int64
	// interval decisions per manage-loop cycle: names which cadence branch
	// fired, so a field dump shows why loops wait as long as they do
	choseOfflineInterval atomic.Int64
	choseFastRetry       atomic.Int64
	choseNormalInterval  atomic.Int64
	// lastEmptyWaitWarnUnix rate-limits the empty-peers wait warning
	lastEmptyWaitWarnUnix atomic.Int64
}

type providerStat struct {
	Managers            int   `json:"managers"`
	Offline             bool  `json:"offline"`
	ConnectivityEvents  int64 `json:"connectivityEvents"`
	RebuildSignals      int64 `json:"rebuildSignals"`
	ReconnectDiffKicks  int64 `json:"reconnectDiffKicks"`
	ClosedPeersFiltered int64 `json:"closedPeersFiltered"`

	ChoseOfflineInterval int64 `json:"choseOfflineInterval"`
	ChoseFastRetry       int64 `json:"choseFastRetry"`
	ChoseNormalInterval  int64 `json:"choseNormalInterval"`
	// live registry introspection (computed on stat query only)
	ManagersNoPeers  int `json:"managersNoPeers"`
	ManagersLoopDead int `json:"managersLoopDead"`
}

func (p *provider) ProvideStat() any {
	p.mu.Lock()
	managers := len(p.managers)
	noPeers, loopDead := 0, 0
	for m := range p.managers {
		m.Lock()
		if len(m.responsiblePeers) == 0 {
			noPeers++
		}
		m.Unlock()
		if !m.loopRunning.Load() {
			loopDead++
		}
	}
	p.mu.Unlock()
	return providerStat{
		Managers:            managers,
		Offline:             p.isOffline(),
		ConnectivityEvents:  p.stats.connectivityEvents.Load(),
		RebuildSignals:      p.stats.rebuildSignals.Load(),
		ReconnectDiffKicks:  p.stats.reconnectDiffKicks.Load(),
		ClosedPeersFiltered: p.stats.closedPeersFiltered.Load(),

		ChoseOfflineInterval: p.stats.choseOfflineInterval.Load(),
		ChoseFastRetry:       p.stats.choseFastRetry.Load(),
		ChoseNormalInterval:  p.stats.choseNormalInterval.Load(),
		ManagersNoPeers:      noPeers,
		ManagersLoopDead:     loopDead,
	}
}

func (p *provider) StatId() string { return CName }

func (p *provider) StatType() string { return CName }

func (p *provider) Init(a *app.App) (err error) {
	p.peerStore = a.MustComponent(peerstore.CName).(peerstore.PeerStore)
	poolService := a.MustComponent(pool.CName).(pool.Service)
	p.pool = poolService
	p.managers = make(map[*clientPeerManager]struct{})
	// optional: absent in some test apps
	if c := a.Component(DeviceNetworkStateCName); c != nil {
		if nc, ok := c.(networkConnectivity); ok {
			p.connectivity = nc
			nc.RegisterConnectivityHook(p.onConnectivityChange)
		} else {
			// the component exists but no longer satisfies our interface — the
			// rebuild/backoff feature is silently off, which must not go unnoticed
			log.Warn("networkState component does not implement networkConnectivity; connectivity-driven peer rebuild disabled")
		}
	}
	if statService, err := app.GetComponent[debugstat.StatService](a); err == nil {
		statService.AddProvider(p)
	}
	return nil
}

func (p *provider) Name() (name string) {
	return CName
}

// onConnectivityChange kicks every space's responsible-peers rebuild so a
// reconnect re-dials immediately (instead of waiting for the periodic tick)
// and a disconnect flips node status to ConnectionError right away.
func (p *provider) onConnectivityChange(online bool) {
	p.stats.connectivityEvents.Inc()
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stats.rebuildSignals.Add(int64(len(p.managers)))
	for m := range p.managers {
		m.signalRebuild()
	}
}

func (p *provider) isOffline() bool {
	if p.connectivity == nil {
		return false
	}
	return p.connectivity.IsOffline()
}

func (p *provider) registerManager(m *clientPeerManager) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.managers != nil {
		p.managers[m] = struct{}{}
	}
}

func (p *provider) unregisterManager(m *clientPeerManager) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.managers != nil {
		delete(p.managers, m)
	}
}

func (p *provider) NewPeerManager(ctx context.Context, spaceId string) (peermanager.PeerManager, error) {
	pm := &clientPeerManager{
		p:         p,
		spaceId:   spaceId,
		peerStore: p.peerStore,
	}
	return pm, nil
}

func (p *provider) UnaryPeerPool() pool.Pool {
	return p.pool
}

func (p *provider) StreamPeerPool() pool.Pool {
	return p.pool
}
