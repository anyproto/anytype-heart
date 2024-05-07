package spacesyncstatus

import (
	"context"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/syncstatus"
	"github.com/cheggaaa/mb/v3"

	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/syncstatus/filesyncstatus"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var log = logging.Logger("anytype-mw-space-status")

type NetworkConfig interface {
	GetNetworkMode() pb.RpcAccountNetworkMode
}

type spaceSyncStatus struct {
	eventSender   event.Sender
	networkConfig NetworkConfig
	batcher       *mb.MB[*syncstatus.SpaceSync]
	store         objectstore.ObjectStore

	fileSyncCountBySpace        map[string]int
	objectSyncCountBySpace      map[string]int
	fileSyncInProgressBySpace   map[string]bool
	objectSyncInProgressBySpace map[string]bool

	spaceSyncStatus map[string]syncstatus.SpaceSyncStatus
	ctx             context.Context
	ctxCancel       context.CancelFunc
}

func NewSpaceSyncStatus() syncstatus.SpaceSyncStatusUpdater {
	return &spaceSyncStatus{
		batcher:                     mb.New[*syncstatus.SpaceSync](0),
		fileSyncCountBySpace:        make(map[string]int, 0),
		objectSyncCountBySpace:      make(map[string]int, 0),
		fileSyncInProgressBySpace:   make(map[string]bool, 0),
		objectSyncInProgressBySpace: make(map[string]bool, 0),
		spaceSyncStatus:             make(map[string]syncstatus.SpaceSyncStatus, 0),
	}
}

func (s *spaceSyncStatus) Init(a *app.App) (err error) {
	s.eventSender = app.MustComponent[event.Sender](a)
	s.networkConfig = app.MustComponent[NetworkConfig](a)
	s.store = app.MustComponent[objectstore.ObjectStore](a)
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

func (s *spaceSyncStatus) processEvents() {
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}
		status, err := s.batcher.WaitOne(context.Background())
		if err != nil {
			log.Errorf("failed to get event from batcher: %s", err)
		}
		s.updateSpaceSyncStatus(status)
	}
}

func (s *spaceSyncStatus) updateSpaceSyncStatus(status *syncstatus.SpaceSync) {
	if s.isSyncFinished(status.Status, status.SpaceId) {
		return
	}

	s.setObjectNumber(status)
	s.setSyncProgress(status)

	if !s.needToSendSyncedEvent(status) {
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

func (s *spaceSyncStatus) needToSendSyncedEvent(status *syncstatus.SpaceSync) bool {
	if status.Status == syncstatus.Synced {
		isFileSyncInProgress := s.fileSyncInProgressBySpace[status.SpaceId]
		isObjectSyncInProgress := s.objectSyncInProgressBySpace[status.SpaceId]
		if isFileSyncInProgress || isObjectSyncInProgress {
			return false
		}
	}
	return true
}

func (s *spaceSyncStatus) Close(ctx context.Context) (err error) {
	if s.ctxCancel != nil {
		s.ctxCancel()
	}
	return s.batcher.Close()
}

func (s *spaceSyncStatus) SendUpdate(status *syncstatus.SpaceSync) {
	e := s.batcher.Add(context.Background(), status)
	if e != nil {
		log.Errorf("failed to add space sync event to queue %s", e)
	}
}

func (s *spaceSyncStatus) setSyncProgress(status *syncstatus.SpaceSync) {
	var syncInProgress bool
	if status.Status == syncstatus.Syncing {
		syncInProgress = true
	}
	if status.IsFilesSync {
		s.fileSyncInProgressBySpace[status.SpaceId] = true
		if !syncInProgress && s.fileSyncCountBySpace[status.SpaceId] == 0 {
			s.fileSyncInProgressBySpace[status.SpaceId] = false
		}
	}
	if status.IsObjectSync {
		s.objectSyncInProgressBySpace[status.SpaceId] = syncInProgress
	}
}

func (s *spaceSyncStatus) isSyncFinished(status syncstatus.SpaceSyncStatus, spaceId string) bool {
	isFileSyncInProgress := s.fileSyncInProgressBySpace[spaceId]
	isObjectSyncInProgress := s.objectSyncInProgressBySpace[spaceId]
	return status == syncstatus.Synced && !(isFileSyncInProgress || isObjectSyncInProgress)
}

func (s *spaceSyncStatus) makeSpaceSyncEvent(status *syncstatus.SpaceSync) *pb.EventSpaceSyncStatusUpdate {
	return &pb.EventSpaceSyncStatusUpdate{
		Status:                mapStatus(status.Status),
		Network:               mapNetworkMode(s.networkConfig.GetNetworkMode()),
		Error:                 mapError(status.SyncError),
		SyncingObjectsCounter: int64(s.objectSyncCountBySpace[status.SpaceId] + s.fileSyncCountBySpace[status.SpaceId]),
	}
}

func (s *spaceSyncStatus) setObjectNumber(status *syncstatus.SpaceSync) {
	if status.IsFilesSync {
		s.setFilesSyncCount(status)
	}
	switch status.Status {
	case syncstatus.Synced, syncstatus.Error, syncstatus.Offline:
		if status.IsObjectSync {
			s.objectSyncCountBySpace[status.SpaceId] = 0
		}
	case syncstatus.Syncing:
		if status.IsObjectSync {
			s.objectSyncCountBySpace[status.SpaceId] = status.ObjectsNumber
		}
	}
}

func (s *spaceSyncStatus) setFilesSyncCount(status *syncstatus.SpaceSync) {
	records, _, err := s.store.Query(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyFileBackupStatus.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Int64(int64(filesyncstatus.Syncing)),
			},
		},
	})
	if err != nil {
		log.Errorf("failed to query file status: %s", err)
	}
	s.fileSyncCountBySpace[status.SpaceId] = len(records)
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
