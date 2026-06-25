package event

import (
	"context"
	"errors"

	"github.com/cheggaaa/mb/v3"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

// log is shared by the untagged sessionSender and the build-tagged GrpcSender.
var log = logging.Logger("anytype-grpc")

const (
	// maxSessionQueueLen bounds a session's outbound buffer. A client this far
	// behind is closed rather than allowed to grow the buffer (and goroutines)
	// without bound; it reconnects and re-subscribes.
	maxSessionQueueLen = 50000
	// maxSendBatch caps how many events one drain iteration pulls, so the
	// in-flight slice is bounded too (the rest stays in the bounded queue).
	maxSendBatch = 1000
)

// sessionSender serializes event delivery to one client session. A single
// goroutine drains a bounded queue and calls send in FIFO order, so the
// underlying gRPC stream's Send is never invoked concurrently. enqueue is
// non-blocking; if the client falls maxSessionQueueLen behind, onClose is
// invoked and further events are dropped. A send error also invokes onClose and
// stops the drain (a stream Send error is terminal).
type sessionSender struct {
	queue   *mb.MB[*pb.Event]
	send    func(*pb.Event) error
	onClose func()
}

func newSessionSender(send func(*pb.Event) error, onClose func(), queueSize int) *sessionSender {
	s := &sessionSender{
		queue:   mb.New[*pb.Event](queueSize),
		send:    send,
		onClose: onClose,
	}
	go s.run()
	return s
}

func (s *sessionSender) run() {
	cond := s.queue.NewCond().WithMax(maxSendBatch)
	for {
		events, err := s.queue.WaitCond(context.Background(), cond)
		if err != nil {
			return // queue closed
		}
		for _, e := range events {
			if serr := s.send(e); serr != nil {
				s.onClose()
				return
			}
		}
	}
}

// enqueue queues event for in-order delivery. Non-blocking: on overflow the
// session is closed (a hopelessly-behind client) rather than blocking the
// broadcaster; on a closed queue the event is dropped.
func (s *sessionSender) enqueue(event *pb.Event) {
	switch err := s.queue.TryAdd(event); {
	case err == nil:
	case errors.Is(err, mb.ErrOverflowed):
		log.Warnf("session outbound queue overflow (>%d events), closing slow session", maxSessionQueueLen)
		s.onClose()
	case errors.Is(err, mb.ErrClosed):
		// session already closing; drop
	default:
		log.Errorf("session enqueue: %v", err)
	}
}

func (s *sessionSender) close() {
	_ = s.queue.Close()
}
