//go:generate mockgen -destination mock_rpcstore/mock_rpcstore.go github.com/anyproto/anytype-heart/core/filestorage/rpcstore Service,RpcStore
package rpcstore

import (
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/net/pool"
	"github.com/anyproto/any-sync/nodeconf"

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
	nodeconf     nodeconf.Service
	peerStore    peerstore.PeerStore
	mx           sync.Mutex
	peerUpdateCh chan struct{}
}

func (s *service) Init(a *app.App) (err error) {
	s.pool = a.MustComponent(pool.CName).(pool.Pool)
	s.nodeconf = a.MustComponent(nodeconf.CName).(nodeconf.Service)
	s.peerStore = a.MustComponent(peerstore.CName).(peerstore.PeerStore)
	s.peerStore.AddObserver(func(peerId string, spaceIds []string) {
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
	cm := newClientManager(s, s.peerUpdateCh)
	return &store{
		s:  s,
		cm: cm,
	}
}

func (s *service) fileNodePeers() []string {
	return s.peerStore.ResponsibleFilePeers()
}

func (s *service) allLocalPeers() []string {
	s.mx.Lock()
	defer s.mx.Unlock()
	return s.peerStore.AllLocalPeers()
}
