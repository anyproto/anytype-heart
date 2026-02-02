package rpcstore

/*
AI generated

Name: Remote File Block Store Factory
Scope: global

## Responsibility
- Factory for creating RpcStore instances that communicate with file node peers
- Manages peer observation for connectivity changes
- Tracks aggregate traffic statistics (inbound/outbound bytes)

## Background Tasks
- checkPeerLoop: manages peer client lifecycle (connect/disconnect, TTL-based GC) [clientManager.checkPeerLoop]
- opLoop: per-client worker loops for read/write task execution [client.opLoop]

## Documentation
Task distribution: Operations are queued as tasks with read/write separation.
Multiple client workers compete to pick tasks from a shared queue based on
EWMA speed scores (faster clients get priority). Failed tasks are retried
on alternative peers via denyPeerIds mechanism.
*/

import (
	"fmt"
	"io"
	"net/http"
	"sync/atomic"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/net/pool"
	"github.com/go-chi/chi/v5"

	"github.com/anyproto/anytype-heart/space/spacecore/peerstore"
	"github.com/anyproto/anytype-heart/util/debug"
)

const CName = "common.commonfile.rpcstore"

var log = logger.NewNamed(CName)

func New() Service {
	return &service{
		peerUpdateCh:      make(chan checkPeersMessage, 1),
		trafficStatistics: &trafficStatistics{},
	}
}

type Service interface {
	NewStore() RpcStore
	app.Component
}

type service struct {
	pool         pool.Pool
	peerStore    peerstore.PeerStore
	peerUpdateCh chan checkPeersMessage

	trafficStatistics *trafficStatistics
}

type trafficStatistics struct {
	inbound  atomic.Int64
	outbound atomic.Int64
}

func (s *service) Init(a *app.App) (err error) {
	s.pool = a.MustComponent(pool.CName).(pool.Pool)
	s.peerStore = a.MustComponent(peerstore.CName).(peerstore.PeerStore)
	s.peerStore.AddObserver(func(peerId string, _, spaceIds []string, peerRemoved bool) {
		select {
		case s.peerUpdateCh <- checkPeersMessage{needClient: false}:
		default:
		}
	})
	return
}

func (s *service) DebugRouter(r chi.Router) {
	r.Get("/traffic", debug.PlaintextHandler(s.debugTraffic))
}

func (s *service) debugTraffic(w io.Writer, req *http.Request) error {
	fmt.Fprintf(w, "inbound= %d\n", s.trafficStatistics.inbound.Load())
	fmt.Fprintf(w, "outbound=%d\n", s.trafficStatistics.outbound.Load())
	return nil
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) NewStore() RpcStore {
	cm := newClientManager(s.pool, s.peerStore, s.peerUpdateCh)
	return newStore(cm, s.trafficStatistics)
}
