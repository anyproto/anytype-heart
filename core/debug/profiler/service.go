package profiler

import (
	"context"
	"encoding/json"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/debug/debugreporter"
	"github.com/anyproto/anytype-heart/core/debug/debugsnapshot"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/initialparams"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

var log = logging.Logger("profiler")

// Compile-time assertion: the component must satisfy debugreporter.Reporter
// so bootstrap can fetch it via app.MustComponent[debugreporter.Reporter].
var _ debugreporter.Reporter = (*service)(nil)

type Service interface {
	app.ComponentRunnable
	debugreporter.Reporter
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
	return nil
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

// Report implements debugreporter.Reporter. It marshals extra to JSON,
// captures an artifact according to capture.Kind, and broadcasts
// Event.Debug.ProfileCreated. Artifact creation failures are logged but do
// not stop the event — clients still see that an incident happened, just
// without a local archive to upload.
func (s *service) Report(reason string, extra map[string]any, capture debugreporter.Capture) {
	desc := marshalExtra(reason, extra)

	var path string
	full := false
	switch capture.Kind {
	case debugreporter.KindNone:
		// event only — no on-disk artifact
	case debugreporter.KindTimedFull:
		// Timed capture is not yet factored out of core/application.RunProfiler;
		// fall back to a heap snapshot so callers still get something usable.
		log.Warnw("Report: KindTimedFull not implemented yet, falling back to KindHeap", "reason", reason)
		path = s.saveHeapSnapshot(reason, desc)
	case debugreporter.KindHeap:
		path = s.saveHeapSnapshot(reason, desc)
	default:
		log.Warnw("Report: unknown Kind", "reason", reason, "kind", capture.Kind)
	}

	s.broadcastProfileCreated(reason, desc, path, full)
}

// saveHeapSnapshot writes a heap+goroutines snapshot zip into the profiles
// dir and returns its path. Returns "" on failure (error is logged).
func (s *service) saveHeapSnapshot(reason, reasonDesc string) string {
	paths := initialparams.Get().Paths
	path, err := debugsnapshot.Save(paths.ProfilesDir, reason, reasonDesc, debugsnapshot.Meta{
		RootPath: paths.Workdir,
	})
	if err != nil {
		log.Warnw("Report: save heap snapshot failed", "reason", reason, "error", err)
		return ""
	}
	return path
}

// broadcastProfileCreated emits Event.Debug.ProfileCreated. Safe to call
// with path == "" (KindNone or failed capture).
func (s *service) broadcastProfileCreated(reason, reasonDesc, path string, full bool) {
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

// marshalExtra renders the caller-supplied extras as a JSON string. Empty
// input and marshal failures both yield "" so Event.Debug.ProfileCreated
// gets an omitted (not "null") reasonDesc.
func marshalExtra(reason string, extra map[string]any) string {
	if len(extra) == 0 {
		return ""
	}
	b, err := json.Marshal(extra)
	if err != nil {
		log.Warnw("Report: marshal extra failed", "reason", reason, "error", err)
		return ""
	}
	return string(b)
}
