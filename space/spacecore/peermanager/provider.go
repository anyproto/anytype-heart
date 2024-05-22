package peermanager

import (
	"context"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/commonspace/peermanager"
	"github.com/anyproto/any-sync/net/netmodule"
	"github.com/anyproto/any-sync/net/streampool"

	"github.com/anyproto/anytype-heart/space/spacecore"
	"github.com/anyproto/anytype-heart/space/spacecore/peerstore"
)

func New() peermanager.PeerManagerProvider {
	return &provider{}
}

const CName = peermanager.CName

var log = logger.NewNamed(CName)

type provider struct {
	netModule  netmodule.NetModule
	streamPool streampool.StreamPool
	peerStore  peerstore.PeerStore
}

func (p *provider) Init(a *app.App) (err error) {
	p.netModule = app.MustComponent[netmodule.NetModule](a)
	p.peerStore = a.MustComponent(peerstore.CName).(peerstore.PeerStore)
	p.streamPool = a.MustComponent(spacecore.CName).(spacecore.SpaceCoreService).StreamPool()
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
	return pm, nil
}
