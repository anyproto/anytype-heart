package spacesyncstatus

import (
	"context"

	"github.com/anyproto/any-sync/app"
	"github.com/cheggaaa/mb/v3"

	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/syncstatus/helpers"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

const service = "common.commonspace.spaceSyncStatusUpdater"

var log = logging.Logger("anytype-mw-space-status")

type Updater interface {
	app.ComponentRunnable
	SendUpdate(spaceSync *helpers.SpaceSync)
}

type State interface {
	SetObjectsNumber(status *helpers.SpaceSync)
	SetSyncStatus(status *helpers.SpaceSync)
	GetSyncStatus(spaceId string) helpers.SpaceSyncStatus
	GetSyncObjectCount(spaceId string) int
	IsSyncFinished(spaceId string) bool
}

type NetworkConfig interface {
	GetNetworkMode() pb.RpcAccountNetworkMode
}

type spaceSyncStatus struct {
	eventSender   event.Sender
	networkConfig NetworkConfig
	batcher       *mb.MB[*helpers.SpaceSync]

	filesState   State
	objectsState State

	ctx       context.Context
	ctxCancel context.CancelFunc

	finish chan struct{}
}

func NewSpaceSyncStatus() Updater {
	return &spaceSyncStatus{batcher: mb.New[*helpers.SpaceSync](0), finish: make(chan struct{})}
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
	return service
}

func (s *spaceSyncStatus) Run(ctx context.Context) (err error) {
	if s.networkConfig.GetNetworkMode() == pb.RpcAccount_LocalOnly {
		s.sendLocalOnlyEvent()
		close(s.finish)
		return
	}
	s.ctx, s.ctxCancel = context.WithCancel(context.Background())
	go s.processEvents()
	return
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

func (s *spaceSyncStatus) SendUpdate(status *helpers.SpaceSync) {
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

func (s *spaceSyncStatus) updateSpaceSyncStatus(status *helpers.SpaceSync) {
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

func (s *spaceSyncStatus) needToSendEvent(status *helpers.SpaceSync) bool {
	if status.Status != helpers.Synced {
		return true
	}
	return s.getSpaceSyncStatus(status) == helpers.Synced && status.Status == helpers.Synced
}

func (s *spaceSyncStatus) Close(ctx context.Context) (err error) {
	if s.ctxCancel != nil {
		s.ctxCancel()
	}
	<-s.finish
	return s.batcher.Close()
}

func (s *spaceSyncStatus) isSyncFinished(status *helpers.SpaceSync) bool {
	return status.Status == helpers.Synced && s.filesState.IsSyncFinished(status.SpaceId) && s.objectsState.IsSyncFinished(status.SpaceId)
}

func (s *spaceSyncStatus) makeSpaceSyncEvent(status *helpers.SpaceSync) *pb.EventSpaceSyncStatusUpdate {
	return &pb.EventSpaceSyncStatusUpdate{
		Id:                    status.SpaceId,
		Status:                mapStatus(s.getSpaceSyncStatus(status)),
		Network:               mapNetworkMode(s.networkConfig.GetNetworkMode()),
		Error:                 mapError(status.SyncError),
		SyncingObjectsCounter: int64(s.filesState.GetSyncObjectCount(status.SpaceId) + s.objectsState.GetSyncObjectCount(status.SpaceId)),
	}
}

func (s *spaceSyncStatus) getSpaceSyncStatus(status *helpers.SpaceSync) helpers.SpaceSyncStatus {
	filesStatus := s.filesState.GetSyncStatus(status.SpaceId)
	objectsStatus := s.objectsState.GetSyncStatus(status.SpaceId)

	if s.isOfflineStatus(filesStatus, objectsStatus) {
		return helpers.Offline
	}

	if s.isSyncedStatus(filesStatus, objectsStatus) {
		return helpers.Synced
	}

	if s.isErrorStatus(filesStatus, objectsStatus) {
		return helpers.Error
	}

	if s.isSyncingStatus(filesStatus, objectsStatus) {
		return helpers.Syncing
	}
	return helpers.Synced
}

func (s *spaceSyncStatus) isSyncingStatus(filesStatus helpers.SpaceSyncStatus, objectsStatus helpers.SpaceSyncStatus) bool {
	return filesStatus == helpers.Syncing || objectsStatus == helpers.Syncing
}

func (s *spaceSyncStatus) isErrorStatus(filesStatus helpers.SpaceSyncStatus, objectsStatus helpers.SpaceSyncStatus) bool {
	return filesStatus == helpers.Error || objectsStatus == helpers.Error
}

func (s *spaceSyncStatus) isSyncedStatus(filesStatus helpers.SpaceSyncStatus, objectsStatus helpers.SpaceSyncStatus) bool {
	return filesStatus == helpers.Synced && objectsStatus == helpers.Synced
}

func (s *spaceSyncStatus) isOfflineStatus(filesStatus helpers.SpaceSyncStatus, objectsStatus helpers.SpaceSyncStatus) bool {
	return filesStatus == helpers.Offline || objectsStatus == helpers.Offline
}

func (s *spaceSyncStatus) getCurrentState(status *helpers.SpaceSync) State {
	if status.SyncType == helpers.Files {
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

func mapStatus(status helpers.SpaceSyncStatus) pb.EventSpaceStatus {
	switch status {
	case helpers.Syncing:
		return pb.EventSpace_Syncing
	case helpers.Offline:
		return pb.EventSpace_Offline
	case helpers.Error:
		return pb.EventSpace_Error
	default:
		return pb.EventSpace_Synced
	}
}

func mapError(err helpers.SpaceSyncError) pb.EventSpaceSyncError {
	switch err {
	case helpers.NetworkError:
		return pb.EventSpace_NetworkError
	case helpers.IncompatibleVersion:
		return pb.EventSpace_IncompatibleVersion
	case helpers.StorageLimitExceed:
		return pb.EventSpace_StorageLimitExceed
	default:
		return pb.EventSpace_Null
	}
}
