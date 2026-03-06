package rpcstore

/*
AI generated

Name: Remote File Block Store Factory
Scope: global

## Responsibility
- Factory for creating RpcStore instances that communicate with file node peers
- Provides pool and peer store dependencies to each RpcStore instance
*/

import (
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/net/pool"

	"github.com/anyproto/anytype-heart/space/spacecore/peerstore"
)

const CName = "common.commonfile.rpcstore"

var log = logger.NewNamed(CName)

func New() Service {
	return &service{}
}

type Service interface {
	NewStore() RpcStore
	app.Component
}

type service struct {
	pool      pool.Pool
	peerStore peerstore.PeerStore
}

func (s *service) Init(a *app.App) (err error) {
	s.pool = a.MustComponent(pool.CName).(pool.Pool)
	s.peerStore = a.MustComponent(peerstore.CName).(peerstore.PeerStore)
	return
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) NewStore() RpcStore {
	return newStore(s.pool, s.peerStore)
}
