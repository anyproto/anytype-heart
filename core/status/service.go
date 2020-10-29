package status

import (
	"sync"
	"time"

	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	ct "github.com/dgtony/collections/time"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/textileio/go-threads/core/net"
	"github.com/textileio/go-threads/core/thread"
)

const threadStatusUpdatePeriod = 2 * time.Second

type Service interface {
	Watch(tid thread.ID, eventCtx string)
	Unwatch(tid thread.ID)
	ThreadSummary() net.SyncSummary

	// TODO extend with specific requests e.g:
	//  - peer connectivity map
	//  - thread status
	//  - file status

	Start() error
	Stop()
}

var _ Service = (*service)(nil)

type service struct {
	cafe     peer.ID
	threads  net.SyncInfo
	files    core.FileInfo
	watchers map[thread.ID]func()
	connMap  map[peer.ID]bool
	emitter  func(event *pb.Event)
	mu       sync.Mutex
}

func NewService(
	ts net.SyncInfo,
	fs core.FileInfo,
	emitter func(event *pb.Event),
	cafe peer.ID,
) *service {
	return &service{
		cafe:     cafe,
		threads:  ts,
		files:    fs,
		watchers: make(map[thread.ID]func()),
		connMap:  make(map[peer.ID]bool),
		emitter:  emitter,
	}
}

func (r *service) Watch(tid thread.ID, eventCtx string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exist := r.watchers[tid]; exist {
		// practically unreachable as we don't expect two
		// processes simultaneously watching the same thread
		return
	}

	var (
		ticker = ct.NewRightAwayTicker(threadStatusUpdatePeriod)
		stop   = make(chan struct{})
		closer = func() { close(stop); ticker.Stop() }
	)

	r.watchers[tid] = closer

	go func() {
		for {
			select {
			case <-ticker.C:
			case <-stop:
				return
			}

			// get sync status with cafe
			s, _ := r.threads.Status(tid, r.cafe)
			var msg = pb.EventStatusThreadSync{LastPull: s.LastPull}

			// Interpret sync status: we are interested in download stats mostly
			switch s.Down {
			case net.Unknown:
				msg.Status = pb.EventStatusThreadSync_Unknown
			case net.InProgress:
				msg.Status = pb.EventStatusThreadSync_InProgress
			case net.Success:
				msg.Status = pb.EventStatusThreadSync_Success
			case net.Failure:
				msg.Status = pb.EventStatusThreadSync_Failure
			}

			r.sendEvent(eventCtx, &pb.EventMessageValueOfThreadStatus{ThreadStatus: &msg})
		}
	}()
}

func (r *service) Unwatch(tid thread.ID) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if stop, found := r.watchers[tid]; found {
		delete(r.watchers, tid)
		stop()
	}
}

func (r *service) ThreadSummary() net.SyncSummary {
	ps, _ := r.threads.PeerSummary(r.cafe)
	return ps
}

func (r *service) Start() error {
	connEvents, err := r.threads.Connectivity()
	if err != nil {
		return err
	}

	go func() {
		for event := range connEvents {
			r.mu.Lock()
			r.connMap[event.Peer] = event.Connected
			r.mu.Unlock()

			if event.Peer == r.cafe {
				r.sendEvent("", &pb.EventMessageValueOfConnStatus{
					ConnStatus: &pb.EventStatusCafeConnect{Connected: event.Connected},
				})
			}
		}
	}()

	return nil
}

func (r *service) Stop() {
	r.mu.Lock()
	defer r.mu.Unlock()

	// just shutdown all thread status watchers, connectivity tracking
	// will be stopped automatically on closing the network layer
	for tid, stop := range r.watchers {
		delete(r.watchers, tid)
		stop()
	}
}

func (r *service) sendEvent(ctx string, event pb.IsEventMessageValue) {
	r.emitter(&pb.Event{
		Messages:  []*pb.EventMessage{{Value: event}},
		ContextId: ctx,
	})
}
