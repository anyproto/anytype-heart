package profiler

import (
	"context"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/metrics"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

var log = logging.Logger("profiler")

type Service interface {
	app.ComponentRunnable
}

type service struct {
	closeCh chan struct{}
	sender  event.Sender

	timesHighMemoryUsageDetected int
	previousHighMemoryDetected   uint64
}

func New() Service {
	return &service{
		closeCh: make(chan struct{}),
	}
}

func (s *service) Init(a *app.App) (err error) {
	s.sender = app.MustComponent[event.Sender](a)
	metrics.SetProfileCreatedHook(s.emitProfileCreated)
	return nil
}

// emitProfileCreated broadcasts an Event.Debug.ProfileCreated notification.
// Used by the memory-growth detector and by the metrics interceptor via
// SetProfileCreatedHook.
func (s *service) emitProfileCreated(reason, reasonDesc, path string, full bool) {
	if s.sender == nil {
		return
	}
	s.sender.Broadcast(&pb.Event{
		Messages: []*pb.EventMessage{
			event.NewMessage("", &pb.EventMessageValueOfDebugProfileCreated{
				DebugProfileCreated: &pb.EventDebugProfileCreated{
					Reason:     reason,
					ReasonDesc: reasonDesc,
					Path:       path,
					Full:       full,
				},
			}),
		},
	})
}

func (s *service) Name() (name string) {
	return "profiler"
}

func (s *service) Run(ctx context.Context) (err error) {
	// Desktop-only: run() is a no-op on gomobile (see profiler_mobile.go).
	go s.run()
	return nil
}

func (s *service) Close(ctx context.Context) (err error) {
	close(s.closeCh)
	return nil
}
