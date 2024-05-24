//go:generate mockgen -destination mock_rpcstore/mock_rpcstore.go github.com/anyproto/anytype-heart/core/filestorage/rpcstore Service,RpcStore
package rpcstore

import (
	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	net2 "github.com/anyproto/any-sync/net"

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
	netService   net2.Service
	peerStore    peerstore.PeerStore
	peerUpdateCh chan struct{}
}

func (s *service) Init(a *app.App) (err error) {
	s.netService = app.MustComponent[net2.Service](a)
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
	cm := newClientManager(s.netService, s.peerStore, s.peerUpdateCh)
	return newStore(cm)
}
