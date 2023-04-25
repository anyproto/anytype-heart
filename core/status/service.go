package status

import (
	"context"
	"github.com/anytypeio/any-sync/app"
	"github.com/anytypeio/any-sync/commonspace/syncstatus"
	"github.com/anytypeio/go-anytype-middleware/core/anytype/config"
	"github.com/anytypeio/go-anytype-middleware/core/event"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pin"
	"github.com/anytypeio/go-anytype-middleware/space"
	"sync"
)

var log = logging.Logger("anytype-mw-status")

const CName = "status"

type LogTime struct {
	AccountID string
	DeviceID  string
	LastEdit  int64
}

type Service interface {
	Watch(id string, fileFunc func() []string) (new bool)
	Unwatch(id string)
	app.ComponentRunnable
}

var _ Service = (*service)(nil)

type service struct {
	fInfo        pin.FilePinService
	profile      core.ProfileInfo
	ownDeviceID  string
	cafeID       string
	emitter      func(event *pb.Event)
	spaceService space.Service
	watcher      syncstatus.StatusWatcher

	nodeConnected bool

	isRunning bool
	sync.Mutex
}

func (s *service) UpdateTree(ctx context.Context, treeId string, status syncstatus.SyncStatus) (err error) {
	var (
		nodeConnected bool
		evStatus      pb.EventStatusThreadSyncStatus
		cafeStatus    pb.EventStatusThreadSyncStatus
	)
	s.Lock()
	nodeConnected = s.nodeConnected
	s.Unlock()
	switch status {
	case syncstatus.StatusUnknown:
		evStatus = pb.EventStatusThread_Unknown
	case syncstatus.StatusSynced:
		evStatus = pb.EventStatusThread_Synced
	case syncstatus.StatusNotSynced:
		evStatus = pb.EventStatusThread_Unknown
	}
	if !nodeConnected {
		evStatus = pb.EventStatusThread_Offline
	}
	cafeStatus = evStatus

	s.sendEvent(treeId, &pb.EventMessageValueOfThreadStatus{ThreadStatus: &pb.EventStatusThread{
		Summary: &pb.EventStatusThreadSummary{Status: evStatus},
		Cafe: &pb.EventStatusThreadCafe{
			Status: cafeStatus,
			Files:  &pb.EventStatusThreadCafePinStatus{},
		},
	}})
	return
}

func (s *service) UpdateNodeConnection(online bool) {
	s.Lock()
	defer s.Unlock()
	s.nodeConnected = online
}

func New() Service {
	return new(service)
}

func (s *service) Init(a *app.App) (err error) {
	disableEvents := a.MustComponent(config.CName).(*config.Config).DisableThreadsSyncEvents
	if !disableEvents {
		s.emitter = a.MustComponent(event.CName).(event.Sender).Send
	}
	s.spaceService = a.MustComponent(space.CName).(space.Service)
	return
}

func (s *service) Run(ctx context.Context) (err error) {
	s.Lock()
	defer s.Unlock()
	res, err := s.spaceService.AccountSpace(ctx)
	if err != nil {
		return
	}

	s.watcher = res.SyncStatus().(syncstatus.StatusWatcher)
	s.watcher.SetUpdateReceiver(s)
	s.isRunning = true
	return nil
}

func (s *service) Name() string {
	return CName
}

func (s *service) Watch(id string, fileFunc func() []string) (new bool) {
	s.Lock()
	defer s.Unlock()
	if !s.isRunning {
		return false
	}
	s.watcher.Watch(id)
	return true
}

func (s *service) Unwatch(id string) {
	s.Lock()
	defer s.Unlock()
	if !s.isRunning {
		return
	}
	s.watcher.Unwatch(id)
}

func (s *service) Close(ctx context.Context) (err error) {
	s.Lock()
	defer s.Unlock()
	s.isRunning = false
	return nil
}

func (s *service) sendEvent(ctx string, event pb.IsEventMessageValue) {
	if s.emitter == nil {
		return
	}
	s.emitter(&pb.Event{
		Messages:  []*pb.EventMessage{{Value: event}},
		ContextId: ctx,
	})
}
