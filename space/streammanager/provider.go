package streammanager

import (
	"context"
	"github.com/anytypeio/any-sync/app"
	"github.com/anytypeio/any-sync/app/logger"
	"github.com/anytypeio/any-sync/commonspace/objectsync"
	"github.com/anytypeio/any-sync/commonspace/streammanager"
	"github.com/anytypeio/any-sync/net/pool"
	"github.com/anytypeio/any-sync/net/streampool"
	"github.com/anytypeio/any-sync/nodeconf"
	"github.com/anytypeio/go-anytype-middleware/space"
)

func New() streammanager.StreamManagerProvider {
	return &provider{}
}

const CName = streammanager.CName

var log = logger.NewNamed(CName)

type provider struct {
	nodeconf   nodeconf.Service
	pool       pool.Pool
	streamPool streampool.StreamPool
}

func (p *provider) Init(a *app.App) (err error) {
	p.nodeconf = a.MustComponent(nodeconf.CName).(nodeconf.Service)
	p.pool = a.MustComponent(pool.CName).(pool.Service).NewPool("space_stream")
	p.streamPool = a.MustComponent(space.CName).(space.Service).StreamPool()
	return nil
}

func (p *provider) Name() (name string) {
	return CName
}

func (p *provider) NewStreamManager(ctx context.Context, spaceId string) (sm objectsync.StreamManager, err error) {
	nm := &clientStreamManager{p: p, spaceId: spaceId}
	nm.init()
	return nm, nil
}
