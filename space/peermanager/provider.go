package peermanager

import (
	"context"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/commonspace/peermanager"
	"github.com/anyproto/any-sync/net/pool"
	"github.com/anyproto/any-sync/net/streampool"

	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/space/peerstore"
)

func New() peermanager.PeerManagerProvider {
	return &provider{}
}

const CName = peermanager.CName

var log = logger.NewNamed(CName)

type provider struct {
	pool       pool.Pool
	commonPool pool.Pool
	streamPool streampool.StreamPool
	peerStore  peerstore.PeerStore
}

func (p *provider) Init(a *app.App) (err error) {
	p.peerStore = a.MustComponent(peerstore.CName).(peerstore.PeerStore)
	poolService := a.MustComponent(pool.CName).(pool.Service)
	p.commonPool = poolService
	p.pool = poolService.NewPool("space_stream")
	p.streamPool = a.MustComponent(space.CName).(space.Service).StreamPool()
	return nil
}

func (p *provider) Name() (name string) {
	return CName
}

func (p *provider) NewPeerManager(ctx context.Context, spaceId string) (peermanager.PeerManager, error) {
	pm := &clientPeerManager{
		p:         p,
		spaceId:   spaceId,
		peerStore: p.peerStore,
	}
	pm.init()
	return pm, nil
}

func (p *provider) UnaryPeerPool() pool.Pool {
	return p.commonPool
}

func (p *provider) StreamPeerPool() pool.Pool {
	return p.pool
}
