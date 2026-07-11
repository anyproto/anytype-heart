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
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/commonspace/peermanager"
	"github.com/anyproto/any-sync/net/pool"

	"github.com/anyproto/anytype-heart/space/spacecore/peerstore"
)

// deviceNetworkStateCName mirrors core/device.CName; importing the package
// would create an import cycle (core/device's devices service reaches back
// into spacecore), so the component is looked up by name and the minimal
// networkConnectivity interface below.
const deviceNetworkStateCName = "networkState"

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
}

func (p *provider) Init(a *app.App) (err error) {
	p.peerStore = a.MustComponent(peerstore.CName).(peerstore.PeerStore)
	poolService := a.MustComponent(pool.CName).(pool.Service)
	p.pool = poolService
	p.managers = make(map[*clientPeerManager]struct{})
	// optional: absent in some test apps
	if c := a.Component(deviceNetworkStateCName); c != nil {
		if nc, ok := c.(networkConnectivity); ok {
			p.connectivity = nc
			nc.RegisterConnectivityHook(p.onConnectivityChange)
		}
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
	p.mu.Lock()
	defer p.mu.Unlock()
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
