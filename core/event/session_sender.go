package event

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/cheggaaa/mb/v3"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

// log is shared by the untagged sessionSender and the build-tagged GrpcSender.
var log = logging.Logger("anytype-grpc")

const (
	// maxSessionQueueMessages bounds a session's outbound buffer by total
	// EventMessage count, NOT by event count: a single *pb.Event can carry many
	// messages (a coalesced cross-space flush up to ~500, a deliverOps batch
	// more), so an event-count bound would not bound memory. A client buffered
	// this far behind is closed; it reconnects and re-subscribes. Comparable to
	// the subscription engine's per-message maxInternalQueueLen.
	maxSessionQueueMessages = 50000
	// maxSendBatch caps how many events one drain iteration pulls.
	maxSendBatch = 1000
)

// sessionSender serializes event delivery to one client session: a single
// goroutine drains a queue and calls send in FIFO order, so the underlying gRPC
// stream's Send is never invoked concurrently. enqueue is non-blocking and the
// buffer is bounded by total EventMessage count; a client that exceeds the
// bound is closed via onClose.
//
// onClose contract: it is invoked at most once (guarded by `once`), but the
// caller must still make it safe to run from the drain goroutine and to not
// block — in particular it must not synchronously acquire a lock that the
// caller of enqueue may hold (enqueue runs under GrpcSender.ServerMutex.RLock).
// close() does not interrupt an in-flight send(); the drain exits once that send
// returns (the stream is independently canceled) or the queue is closed.
type sessionSender struct {
	queue   *mb.MB[*pb.Event]
	send    func(*pb.Event) error
	onClose func()

	maxMsgs int64
	curMsgs atomic.Int64
	once    sync.Once
	done    chan struct{} // closed when run() exits
}

func newSessionSender(send func(*pb.Event) error, onClose func(), maxMsgs int) *sessionSender {
	if maxMsgs <= 0 {
		maxMsgs = maxSessionQueueMessages // guard: 0 would disable the bound
	}
	s := &sessionSender{
		queue:   mb.New[*pb.Event](0), // unbounded in events; bounded by curMsgs/maxMsgs
		send:    send,
		onClose: onClose,
		maxMsgs: int64(maxMsgs),
		done:    make(chan struct{}),
	}
	go s.run()
	return s
}

func (s *sessionSender) run() {
	defer close(s.done)
	cond := s.queue.NewCond().WithMax(maxSendBatch)
	for {
		events, err := s.queue.WaitCond(context.Background(), cond)
		if err != nil {
			return // queue closed
		}
		for _, e := range events {
			serr := s.send(e)
			s.curMsgs.Add(-int64(len(e.Messages)))
			if serr != nil {
				s.requestClose() // a stream Send error is terminal
				return
			}
		}
	}
}

// enqueue queues event for in-order delivery. Non-blocking. If the session's
// buffered message count would exceed maxMsgs, the session is closed (a
// hopelessly-behind client) rather than blocking the broadcaster.
func (s *sessionSender) enqueue(event *pb.Event) {
	n := int64(len(event.Messages))
	if n == 0 {
		return
	}
	if s.curMsgs.Add(n) > s.maxMsgs {
		s.curMsgs.Add(-n)
		s.once.Do(func() {
			log.Warnf("session outbound buffer exceeded %d messages, closing slow session", s.maxMsgs)
			s.onClose()
		})
		return
	}
	if err := s.queue.TryAdd(event); err != nil {
		s.curMsgs.Add(-n) // queue closed: not buffered
	}
}

// requestClose invokes onClose exactly once.
func (s *sessionSender) requestClose() {
	s.once.Do(s.onClose)
}

func (s *sessionSender) close() {
	_ = s.queue.Close()
}
