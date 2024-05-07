package spacesyncstatus

import (
	"context"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/syncstatus"
	"github.com/cheggaaa/mb/v3"

	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

var log = logging.Logger("anytype-mw-space-status")

type State interface {
	SetObjectsNumber(status *syncstatus.SpaceSync)
	SetSyncStatus(status *syncstatus.SpaceSync)
	GetSyncStatus(spaceId string) syncstatus.SpaceSyncStatus
	GetSyncObjectCount(spaceId string) int
	IsSyncFinished(spaceId string) bool
}

type NetworkConfig interface {
	GetNetworkMode() pb.RpcAccountNetworkMode
}

type spaceSyncStatus struct {
	eventSender   event.Sender
	networkConfig NetworkConfig
	batcher       *mb.MB[*syncstatus.SpaceSync]

	filesState   State
	objectsState State

	ctx       context.Context
	ctxCancel context.CancelFunc

	finish chan struct{}
}

func NewSpaceSyncStatus() syncstatus.SpaceSyncStatusUpdater {
	return &spaceSyncStatus{batcher: mb.New[*syncstatus.SpaceSync](0), finish: make(chan struct{})}
}

func (s *spaceSyncStatus) Init(a *app.App) (err error) {
	s.eventSender = app.MustComponent[event.Sender](a)
	s.networkConfig = app.MustComponent[NetworkConfig](a)
	store := app.MustComponent[objectstore.ObjectStore](a)
	s.filesState = NewFileState(store)
	s.objectsState = NewObjectState()
	return
}

func (s *spaceSyncStatus) Name() (name string) {
	return syncstatus.SpaceSyncStatusService
}

func (s *spaceSyncStatus) Run(ctx context.Context) (err error) {
	s.ctx, s.ctxCancel = context.WithCancel(context.Background())
	go s.processEvents()
	return
}

func (s *spaceSyncStatus) SendUpdate(status *syncstatus.SpaceSync) {
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
		s.updateSpaceSyncStatus(status)
	}
}

func (s *spaceSyncStatus) updateSpaceSyncStatus(status *syncstatus.SpaceSync) {
	if s.networkConfig.GetNetworkMode() == pb.RpcAccount_LocalOnly {
		return
	}
	// don't send unnecessary event
	if s.isSyncFinished(status) {
		return
	}

	state := s.getCurrentState(status)
	state.SetObjectsNumber(status)
	state.SetSyncStatus(status)

	// send synced event only if files and objects are all synced
	if !s.needToSendEvent(status) {
		return
	}

	s.eventSender.Broadcast(&pb.Event{
		Messages: []*pb.EventMessage{{
			Value: &pb.EventMessageValueOfSpaceSyncStatusUpdate{
				SpaceSyncStatusUpdate: s.makeSpaceSyncEvent(status),
			},
		}},
	})
}

func (s *spaceSyncStatus) needToSendEvent(status *syncstatus.SpaceSync) bool {
	if status.Status != syncstatus.Synced {
		return true
	}
	return s.getSpaceSyncStatus(status) == syncstatus.Synced && status.Status == syncstatus.Synced
}

func (s *spaceSyncStatus) Close(ctx context.Context) (err error) {
	if s.ctxCancel != nil {
		s.ctxCancel()
	}
	<-s.finish
	return s.batcher.Close()
}

func (s *spaceSyncStatus) isSyncFinished(status *syncstatus.SpaceSync) bool {
	return status.Status == syncstatus.Synced && s.filesState.IsSyncFinished(status.SpaceId) && s.objectsState.IsSyncFinished(status.SpaceId)
}

func (s *spaceSyncStatus) makeSpaceSyncEvent(status *syncstatus.SpaceSync) *pb.EventSpaceSyncStatusUpdate {
	return &pb.EventSpaceSyncStatusUpdate{
		Status:                mapStatus(s.getSpaceSyncStatus(status)),
		Network:               mapNetworkMode(s.networkConfig.GetNetworkMode()),
		Error:                 mapError(status.SyncError),
		SyncingObjectsCounter: int64(s.filesState.GetSyncObjectCount(status.SpaceId) + s.objectsState.GetSyncObjectCount(status.SpaceId)),
	}
}

func (s *spaceSyncStatus) getSpaceSyncStatus(status *syncstatus.SpaceSync) syncstatus.SpaceSyncStatus {
	filesStatus := s.filesState.GetSyncStatus(status.SpaceId)
	objectsStatus := s.objectsState.GetSyncStatus(status.SpaceId)

	if s.isOfflineStatus(filesStatus, objectsStatus) {
		return syncstatus.Offline
	}

	if s.isSyncedStatus(filesStatus, objectsStatus) {
		return syncstatus.Synced
	}

	if s.isErrorStatus(filesStatus, objectsStatus) {
		return syncstatus.Error
	}

	if s.isSyncingStatus(filesStatus, objectsStatus) {
		return syncstatus.Syncing
	}
	return syncstatus.Synced
}

func (s *spaceSyncStatus) isSyncingStatus(filesStatus syncstatus.SpaceSyncStatus, objectsStatus syncstatus.SpaceSyncStatus) bool {
	return filesStatus == syncstatus.Syncing || objectsStatus == syncstatus.Syncing
}

func (s *spaceSyncStatus) isErrorStatus(filesStatus syncstatus.SpaceSyncStatus, objectsStatus syncstatus.SpaceSyncStatus) bool {
	return filesStatus == syncstatus.Error || objectsStatus == syncstatus.Error
}

func (s *spaceSyncStatus) isSyncedStatus(filesStatus syncstatus.SpaceSyncStatus, objectsStatus syncstatus.SpaceSyncStatus) bool {
	return filesStatus == syncstatus.Synced && objectsStatus == syncstatus.Synced
}

func (s *spaceSyncStatus) isOfflineStatus(filesStatus syncstatus.SpaceSyncStatus, objectsStatus syncstatus.SpaceSyncStatus) bool {
	return filesStatus == syncstatus.Offline && objectsStatus == syncstatus.Offline
}

func (s *spaceSyncStatus) getCurrentState(status *syncstatus.SpaceSync) State {
	if status.SyncType == syncstatus.Files {
		return s.filesState
	}
	return s.objectsState
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

func mapStatus(status syncstatus.SpaceSyncStatus) pb.EventSpaceStatus {
	switch status {
	case syncstatus.Syncing:
		return pb.EventSpace_Syncing
	case syncstatus.Offline:
		return pb.EventSpace_Offline
	case syncstatus.Error:
		return pb.EventSpace_Error
	default:
		return pb.EventSpace_Synced
	}
}

func mapError(err syncstatus.SpaceSyncError) pb.EventSpaceSyncError {
	switch err {
	case syncstatus.NetworkError:
		return pb.EventSpace_NetworkError
	case syncstatus.IncompatibleVersion:
		return pb.EventSpace_IncompatibleVersion
	case syncstatus.StorageLimitExceed:
		return pb.EventSpace_StorageLimitExceed
	default:
		return pb.EventSpace_Null
	}
}
