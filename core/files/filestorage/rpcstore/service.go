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
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/net/pool"

	"github.com/anyproto/anytype-heart/core/anytype/config"
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
	conf      *config.Config
}

func (s *service) Init(a *app.App) (err error) {
	s.pool = a.MustComponent(pool.CName).(pool.Pool)
	s.peerStore = a.MustComponent(peerstore.CName).(peerstore.PeerStore)
	// config is optional (not registered in rpcstore tests); localPeerTimeouts
	// falls back to the defaults when it is missing
	s.conf, _ = app.GetComponent[*config.Config](a)
	return
}

func (s *service) Name() (name string) {
	return CName
}

// localPeerTimeouts returns the configured per-request timeout for local
// peers and the ban duration after a failed fetch, falling back to the
// defaults when no config component is registered (e.g. in tests). Config is
// read at store-creation time so stores always see current values.
func (s *service) localPeerTimeouts() (timeout, banTtl time.Duration) {
	if s.conf != nil {
		return s.conf.LocalPeerTimeout(), s.conf.LocalPeerBanTtl()
	}
	return time.Duration(config.DefaultLocalPeerTimeoutMs) * time.Millisecond,
		time.Duration(config.DefaultLocalPeerBanTtlSec) * time.Second
}

func (s *service) NewStore() RpcStore {
	timeout, banTtl := s.localPeerTimeouts()
	return newStore(s.pool, s.peerStore, timeout, banTtl)
}
