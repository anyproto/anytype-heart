package rpcstore

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
