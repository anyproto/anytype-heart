package rpcstore

import (
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/net/pool"

	"github.com/anyproto/anytype-heart/space/spacecore/peerstore"
)

const CName = "common.commonfile.rpcstore"

var log = logger.NewNamed(CName)

func New() Service {
	return &service{peerUpdateCh: make(chan struct{}, 1)}
}

type Service interface {
	NewStore() RpcStore
	app.Component
}

type service struct {
	pool         pool.Pool
	peerStore    peerstore.PeerStore
	peerUpdateCh chan struct{}
}

func (s *service) Init(a *app.App) (err error) {
	s.pool = a.MustComponent(pool.CName).(pool.Pool)
	s.peerStore = a.MustComponent(peerstore.CName).(peerstore.PeerStore)
	s.peerStore.AddObserver(func(peerId string, _, spaceIds []string, peerRemoved bool) {
		select {
		case s.peerUpdateCh <- struct{}{}:
		default:
		}
	})
	return
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) NewStore() RpcStore {
	cm := newClientManager(s.pool, s.peerStore, s.peerUpdateCh)
	return newStore(cm)
}
