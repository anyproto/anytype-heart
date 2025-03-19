package client

import (
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/net/peerservice"
	"github.com/anyproto/any-sync/net/pool"

	"github.com/anyproto/anytype-heart/core/anytype/config"
)

func New() Client {
	return &client{}
}

const CName = "push.client"

type Config struct {
	PeerId string
	Addr   []string
}

type Client interface {
	app.Component
}

type client struct {
	pool        pool.Pool
	peerService peerservice.PeerService
	peerIds     []string
}

func (p *client) Init(a *app.App) (err error) {
	p.pool = app.MustComponent[pool.Pool](a)
	p.peerService = app.MustComponent[peerservice.PeerService](a)
	cfg := app.MustComponent[*config.Config](a).GetPushConfig()
	p.peerService.SetPeerAddrs(cfg.PeerId, cfg.Addr)
	p.peerIds = append(p.peerIds, cfg.PeerId)
	return
}

func (p *client) Name() (name string) {
	return CName
}
