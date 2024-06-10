package spacesyncstatus

import (
	"context"

	"github.com/anyproto/any-sync/app"
	"github.com/cheggaaa/mb/v3"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

const service = "core.syncstatus.spacesyncstatus"

var log = logging.Logger("anytype-mw-space-status")

type Updater interface {
	app.ComponentRunnable
	SendUpdate(spaceSync *domain.SpaceSync)
}

type SpaceIdGetter interface {
	app.Component
	TechSpaceId() string
	PersonalSpaceId() string
}

type State interface {
	SetObjectsNumber(status *domain.SpaceSync)
	SetSyncStatusAndErr(status *domain.SpaceSync)
	GetSyncStatus(spaceId string) domain.SyncStatus
	GetSyncObjectCount(spaceId string) int
	GetSyncErr(spaceId string) domain.SyncError
}

type NetworkConfig interface {
	GetNetworkMode() pb.RpcAccountNetworkMode
}

type spaceSyncStatus struct {
	eventSender   event.Sender
	networkConfig NetworkConfig
	batcher       *mb.MB[*domain.SpaceSync]

	filesState   State
	objectsState State

	ctx           context.Context
	ctxCancel     context.CancelFunc
	spaceIdGetter SpaceIdGetter

	finish chan struct{}
}

func NewSpaceSyncStatus() Updater {
	return &spaceSyncStatus{batcher: mb.New[*domain.SpaceSync](0), finish: make(chan struct{})}
}

func (s *spaceSyncStatus) Init(a *app.App) (err error) {
	s.eventSender = app.MustComponent[event.Sender](a)
	s.networkConfig = app.MustComponent[NetworkConfig](a)
	store := app.MustComponent[objectstore.ObjectStore](a)
	s.filesState = NewFileState(store)
	s.objectsState = NewObjectState(store)
	s.spaceIdGetter = app.MustComponent[SpaceIdGetter](a)
	return
}

func (s *spaceSyncStatus) Name() (name string) {
	return service
}

func (s *spaceSyncStatus) Run(ctx context.Context) (err error) {
	if s.networkConfig.GetNetworkMode() == pb.RpcAccount_LocalOnly {
		s.sendLocalOnlyEvent()
		close(s.finish)
		return
	} else {
		s.sendStartEvent()
	}
	s.ctx, s.ctxCancel = context.WithCancel(context.Background())
	go s.processEvents()
	return
}

func (s *spaceSyncStatus) sendStartEvent() {
	s.eventSender.Broadcast(&pb.Event{
		Messages: []*pb.EventMessage{{
			Value: &pb.EventMessageValueOfSpaceSyncStatusUpdate{
				SpaceSyncStatusUpdate: s.makeSpaceSyncEvent(s.spaceIdGetter.PersonalSpaceId()),
			},
		}},
	})
}

func (s *spaceSyncStatus) sendLocalOnlyEvent() {
	s.eventSender.Broadcast(&pb.Event{
		Messages: []*pb.EventMessage{{
			Value: &pb.EventMessageValueOfSpaceSyncStatusUpdate{
				SpaceSyncStatusUpdate: &pb.EventSpaceSyncStatusUpdate{
					Status:  pb.EventSpace_Offline,
					Network: pb.EventSpace_LocalOnly,
				},
			},
		}},
	})
}

func (s *spaceSyncStatus) SendUpdate(status *domain.SpaceSync) {
	e := s.batcher.Add(context.Background(), status)
	if e != nil {
		log.Errorf("failed to add space sync event to queue %s", e)
	}
}

func (s *spaceSyncStatus) processEvents() {
	defer close(s.finish)
	for {
		status, err := s.batcher.WaitOne(s.ctx)
		if err != nil {
			log.Errorf("failed to get event from batcher: %s", err)
			return
		}
		if status.SpaceId == s.spaceIdGetter.TechSpaceId() {
			continue
		}
		s.updateSpaceSyncStatus(status)
	}
}

func (s *spaceSyncStatus) updateSpaceSyncStatus(status *domain.SpaceSync) {
	state := s.getCurrentState(status)
	state.SetObjectsNumber(status)
	state.SetSyncStatusAndErr(status)

	// send synced event only if files and objects are all synced
	if !s.needToSendEvent(status) {
		return
	}

	s.eventSender.Broadcast(&pb.Event{
		Messages: []*pb.EventMessage{{
			Value: &pb.EventMessageValueOfSpaceSyncStatusUpdate{
				SpaceSyncStatusUpdate: s.makeSpaceSyncEvent(status.SpaceId),
			},
		}},
	})
}

func (s *spaceSyncStatus) needToSendEvent(status *domain.SpaceSync) bool {
	if status.Status != domain.Synced {
		return true
	}
	return s.getSpaceSyncStatus(status.SpaceId) == domain.Synced && status.Status == domain.Synced
}

func (s *spaceSyncStatus) Close(ctx context.Context) (err error) {
	if s.ctxCancel != nil {
		s.ctxCancel()
	}
	<-s.finish
	return s.batcher.Close()
}

func (s *spaceSyncStatus) makeSpaceSyncEvent(spaceId string) *pb.EventSpaceSyncStatusUpdate {
	return &pb.EventSpaceSyncStatusUpdate{
		Id:                    spaceId,
		Status:                mapStatus(s.getSpaceSyncStatus(spaceId)),
		Network:               mapNetworkMode(s.networkConfig.GetNetworkMode()),
		Error:                 s.getError(spaceId),
		SyncingObjectsCounter: int64(s.filesState.GetSyncObjectCount(spaceId) + s.objectsState.GetSyncObjectCount(spaceId)),
	}
}

func (s *spaceSyncStatus) getSpaceSyncStatus(spaceId string) domain.SyncStatus {
	filesStatus := s.filesState.GetSyncStatus(spaceId)
	objectsStatus := s.objectsState.GetSyncStatus(spaceId)

	if s.isOfflineStatus(filesStatus, objectsStatus) {
		return domain.Offline
	}

	if s.isSyncedStatus(filesStatus, objectsStatus) {
		return domain.Synced
	}

	if s.isErrorStatus(filesStatus, objectsStatus) {
		return domain.Error
	}

	if s.isSyncingStatus(filesStatus, objectsStatus) {
		return domain.Syncing
	}
	return domain.Synced
}

func (s *spaceSyncStatus) isSyncingStatus(filesStatus domain.SyncStatus, objectsStatus domain.SyncStatus) bool {
	return filesStatus == domain.Syncing || objectsStatus == domain.Syncing
}

func (s *spaceSyncStatus) isErrorStatus(filesStatus domain.SyncStatus, objectsStatus domain.SyncStatus) bool {
	return filesStatus == domain.Error || objectsStatus == domain.Error
}

func (s *spaceSyncStatus) isSyncedStatus(filesStatus domain.SyncStatus, objectsStatus domain.SyncStatus) bool {
	return filesStatus == domain.Synced && objectsStatus == domain.Synced
}

func (s *spaceSyncStatus) isOfflineStatus(filesStatus domain.SyncStatus, objectsStatus domain.SyncStatus) bool {
	return filesStatus == domain.Offline || objectsStatus == domain.Offline
}

func (s *spaceSyncStatus) getCurrentState(status *domain.SpaceSync) State {
	if status.SyncType == domain.Files {
		return s.filesState
	}
	return s.objectsState
}

func (s *spaceSyncStatus) getError(spaceId string) pb.EventSpaceSyncError {
	syncErr := s.filesState.GetSyncErr(spaceId)
	if syncErr != domain.Null {
		return mapError(syncErr)
	}

	syncErr = s.objectsState.GetSyncErr(spaceId)
	if syncErr != domain.Null {
		return mapError(syncErr)
	}

	return pb.EventSpace_Null
}

func mapNetworkMode(mode pb.RpcAccountNetworkMode) pb.EventSpaceNetwork {
	switch mode {
	case pb.RpcAccount_LocalOnly:
		return pb.EventSpace_LocalOnly
	case pb.RpcAccount_CustomConfig:
		return pb.EventSpace_SelfHost
	default:
		return pb.EventSpace_Anytype
	}
}

func mapStatus(status domain.SyncStatus) pb.EventSpaceStatus {
	switch status {
	case domain.Syncing:
		return pb.EventSpace_Syncing
	case domain.Offline:
		return pb.EventSpace_Offline
	case domain.Error:
		return pb.EventSpace_Error
	default:
		return pb.EventSpace_Synced
	}
}

func mapError(err domain.SyncError) pb.EventSpaceSyncError {
	switch err {
	case domain.NetworkError:
		return pb.EventSpace_NetworkError
	case domain.IncompatibleVersion:
		return pb.EventSpace_IncompatibleVersion
	case domain.StorageLimitExceed:
		return pb.EventSpace_StorageLimitExceed
	default:
		return pb.EventSpace_Null
	}
}
