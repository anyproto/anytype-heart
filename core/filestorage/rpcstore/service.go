package rpcstore

import (
	"github.com/anytypeio/any-sync/app"
	"github.com/anytypeio/any-sync/app/logger"
	"github.com/anytypeio/any-sync/net/pool"
	"github.com/anytypeio/any-sync/nodeconf"
	"golang.org/x/exp/slices"
	"sync"
)

const CName = "common.commonfile.rpcstore"

var log = logger.NewNamed(CName)

func New() Service {
	return &service{peerUpdateCh: make(chan struct{}, 1)}
}

type Service interface {
	NewStore() RpcStore
	AddLocalPeer(peerId string, spaceIds []string)
	app.Component
}

type service struct {
	pool                pool.Pool
	nodeconf            nodeconf.Service
	nodePeerIds         []string
	localPeerIdsBySpace map[string][]string
	allPeerIds          []string
	mx                  sync.Mutex
	peerUpdateCh        chan struct{}
}

func (s *service) Init(a *app.App) (err error) {
	s.pool = a.MustComponent(pool.CName).(pool.Pool)
	s.nodeconf = a.MustComponent(nodeconf.CName).(nodeconf.Service)
	s.nodePeerIds = s.nodeconf.GetLast().FilePeers()
	s.localPeerIdsBySpace = map[string][]string{}
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
	return s.nodePeerIds
}

func (s *service) localPeers(spaceIds []string) (res []string) {
	// TODO: check if needed
	s.mx.Lock()
	defer s.mx.Unlock()
	unique := map[string]struct{}{}
	for _, spaceId := range spaceIds {
		spacePeers := s.localPeerIdsBySpace[spaceId]
		for _, peerId := range spacePeers {
			unique[peerId] = struct{}{}
		}
	}
	for peerId := range unique {
		res = append(res, peerId)
	}
	return
}

func (s *service) allLocalPeers() []string {
	s.mx.Lock()
	defer s.mx.Unlock()
	return s.allPeerIds
}

func (s *service) AddLocalPeer(peerId string, spaceIds []string) {
	s.mx.Lock()
	defer s.mx.Unlock()
	if !slices.Contains(s.allPeerIds, peerId) {
		return
	}
	s.allPeerIds = append(s.allPeerIds, peerId)
	for _, id := range spaceIds {
		spacePeerIds := s.localPeerIdsBySpace[id]
		spacePeerIds = append(spacePeerIds, peerId)
		s.localPeerIdsBySpace[id] = spacePeerIds
	}
	select {
	case s.peerUpdateCh <- struct{}{}:
	default:
	}
}
