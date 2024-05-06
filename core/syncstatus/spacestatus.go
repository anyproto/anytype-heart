package syncstatus

import (
	"context"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/syncstatus"
	"github.com/cheggaaa/mb/v3"

	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"
)

type NetworkConfig interface {
	GetNetworkMode() pb.RpcAccountNetworkMode
}

type UpdateSender interface {
	app.ComponentRunnable
	SendUpdate(status *syncstatus.SpaceSync)
}

type spaceSyncStatus struct {
	eventSender   event.Sender
	networkConfig NetworkConfig
	batcher       *mb.MB[*pb.EventSpaceSyncStatusUpdate]

	isFileSyncInProgress, isObjectSyncInProgress bool
}

func (s *spaceSyncStatus) SendUpdate(status *syncstatus.SpaceSync) {
	s.setSyncProgress(status)

	if s.isSyncFinished(status.Status) {
		return
	}

	e := s.batcher.Add(context.Background(), s.makeSpaceSyncEvent(status))
	if e != nil {
		log.Errorf("failed to add space sync event to queue %s", e)
	}
}

func (s *spaceSyncStatus) setSyncProgress(status *syncstatus.SpaceSync) {
	if s.isSyncFinished(status.Status) {
		if status.IsFilesSync {
			s.isFileSyncInProgress = false
		}
		if status.IsObjectSync {
			s.isObjectSyncInProgress = false
		}
		return
	}
	if status.IsFilesSync {
		s.isFileSyncInProgress = true
	}
	if status.IsObjectSync {
		s.isObjectSyncInProgress = true
	}
}

func (s *spaceSyncStatus) isSyncFinished(status syncstatus.SpaceSyncStatus) bool {
	return status == syncstatus.Synced && (s.isFileSyncInProgress || s.isObjectSyncInProgress)
}

func NewSpaceSyncStatus() UpdateSender {
	return &spaceSyncStatus{batcher: mb.New[*pb.EventSpaceSyncStatusUpdate](0)}
}

func (s *spaceSyncStatus) Init(a *app.App) (err error) {
	s.eventSender = app.MustComponent[event.Sender](a)
	s.networkConfig = app.MustComponent[NetworkConfig](a)
	return
}

func (s *spaceSyncStatus) Name() (name string) {
	return syncstatus.SpaceSyncStatusService
}

func (s *spaceSyncStatus) Run(ctx context.Context) (err error) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		msg, err := s.batcher.WaitOne(context.Background())
		if err != nil {
			return err
		}
		s.eventSender.Broadcast(&pb.Event{
			Messages: []*pb.EventMessage{{
				Value: &pb.EventMessageValueOfSpaceSyncStatusUpdate{
					SpaceSyncStatusUpdate: msg,
				},
			}},
		})
	}
}

func (s *spaceSyncStatus) Close(ctx context.Context) (err error) {
	return s.batcher.Close()
}

func (s *spaceSyncStatus) makeSpaceSyncEvent(status *syncstatus.SpaceSync) *pb.EventSpaceSyncStatusUpdate {
	return &pb.EventSpaceSyncStatusUpdate{
		Status:                mapStatus(status.Status),
		Network:               mapNetworkMode(s.networkConfig.GetNetworkMode()),
		Error:                 mapError(status.SyncError),
		SyncingObjectsCounter: int64(status.ObjectsNumber),
	}
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
