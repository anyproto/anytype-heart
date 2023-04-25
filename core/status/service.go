package status

import (
	"context"
	"sync"

	"github.com/anytypeio/any-sync/app"
	"github.com/anytypeio/any-sync/commonfile/fileservice"
	"github.com/anytypeio/any-sync/commonspace/syncstatus"
	"go.uber.org/zap"
	"golang.org/x/exp/slices"

	"github.com/anytypeio/go-anytype-middleware/core/anytype/config"
	"github.com/anytypeio/go-anytype-middleware/core/event"
	"github.com/anytypeio/go-anytype-middleware/core/filestorage/filesync"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/logging"
	"github.com/anytypeio/go-anytype-middleware/space"
	"github.com/anytypeio/go-anytype-middleware/space/typeprovider"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
)

var log = logging.Logger("anytype-mw-status")

const CName = "status"

type Service interface {
	Watch(id string, fileFunc func() []string) (new bool, err error)
	Unwatch(id string)
	app.ComponentRunnable
}

var _ Service = (*service)(nil)

type service struct {
	typeProvider      typeprovider.SmartBlockTypeProvider
	emitter           func(event *pb.Event)
	spaceService      space.Service
	watcher           syncstatus.StatusWatcher
	coreService       core.Service
	fileStatusService *filesync.Status

	nodeConnected bool
	subObjects    []string
	isRunning     bool

	sync.Mutex
}

func (s *service) UpdateTree(ctx context.Context, objId string, status syncstatus.SyncStatus) (err error) {
	var (
		nodeConnected bool
		objStatus     pb.EventStatusThreadSyncStatus
		generalStatus pb.EventStatusThreadSyncStatus
	)
	s.Lock()
	nodeConnected = s.nodeConnected
	s.Unlock()
	switch status {
	case syncstatus.StatusUnknown:
		objStatus = pb.EventStatusThread_Unknown
	case syncstatus.StatusSynced:
		objStatus = pb.EventStatusThread_Synced
	case syncstatus.StatusNotSynced:
		objStatus = pb.EventStatusThread_Syncing
	}
	if !nodeConnected {
		objStatus = pb.EventStatusThread_Offline
	}
	generalStatus = objStatus

	s.notify(objId, objStatus, generalStatus)
	if objId != s.coreService.PredefinedBlocks().Account {
		return
	}
	s.Lock()
	cp := slice.Copy(s.subObjects)
	s.Unlock()

	for _, obj := range cp {
		s.notify(obj, objStatus, generalStatus)
	}
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
	s.typeProvider = a.MustComponent(typeprovider.CName).(typeprovider.SmartBlockTypeProvider)
	s.spaceService = a.MustComponent(space.CName).(space.Service)
	s.coreService = a.MustComponent(core.CName).(core.Service)

	dagService := a.MustComponent(fileservice.CName).(fileservice.FileService).DAGService()
	fileSyncService := app.MustComponent[filesync.FileSync](a)
	s.fileStatusService = filesync.NewStatus(s, dagService, fileSyncService)
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
	_, err = s.watch(s.coreService.PredefinedBlocks().Account)
	s.fileStatusService.Run()
	return
}

func (s *service) Name() string {
	return CName
}

func (s *service) Watch(id string, fileFunc func() []string) (new bool, err error) {
	s.Lock()
	defer s.Unlock()
	return s.watch(id)
}

func (s *service) Unwatch(id string) {
	s.Lock()
	defer s.Unlock()
	s.unwatch(id)
}

func (s *service) watch(id string) (new bool, err error) {
	if !s.isRunning {
		return false, nil
	}
	tp, err := s.typeProvider.Type(id)
	if err != nil {
		log.Debug("failed to get type of", zap.String("objectID", id))
	}
	if tp == smartblock.SmartBlockTypeSubObject {
		s.subObjects = append(s.subObjects, id)
		return true, nil
	}

	if tp == smartblock.SmartBlockTypeFile {
		s.fileStatusService.Watch(s.spaceService.AccountId(), id)
		return false, nil
	}

	if err = s.watcher.Watch(id); err != nil {
		return false, err
	}
	return true, nil
}

func (s *service) unwatch(id string) {
	if !s.isRunning {
		return
	}
	if s.tryUnregister(id) {
		return
	}
	tp, err := s.typeProvider.Type(id)
	if err != nil {
		log.Debug("failed to get type of", zap.String("objectID", id))
	}
	if tp == smartblock.SmartBlockTypeFile {
		s.fileStatusService.Unwatch(s.spaceService.AccountId(), id)
		return
	}
	s.watcher.Unwatch(id)
}

func (s *service) Close(ctx context.Context) (err error) {
	s.Lock()
	defer s.Unlock()
	s.isRunning = false
	s.unwatch(s.coreService.PredefinedBlocks().Account)
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

func (s *service) tryUnregister(id string) bool {
	idx := slices.Index(s.subObjects, id)
	if idx != -1 {
		s.subObjects = slice.RemoveIndex(s.subObjects, idx)
		return true
	}
	return false
}

func (s *service) notify(objId string, objStatus, generalStatus pb.EventStatusThreadSyncStatus) {
	s.sendEvent(objId, &pb.EventMessageValueOfThreadStatus{ThreadStatus: &pb.EventStatusThread{
		Summary: &pb.EventStatusThreadSummary{Status: objStatus},
		Cafe: &pb.EventStatusThreadCafe{
			Status: generalStatus,
			Files:  &pb.EventStatusThreadCafePinStatus{},
		},
	}})
}
