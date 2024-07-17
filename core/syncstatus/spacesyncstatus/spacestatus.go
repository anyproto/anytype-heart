package spacesyncstatus

import (
	"context"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/util/periodicsync"
	"github.com/cheggaaa/mb/v3"
	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/util/slice"
)

const service = "core.syncstatus.spacesyncstatus"

var log = logging.Logger("anytype-mw-space-status")

type Updater interface {
	app.ComponentRunnable
	SendUpdate(spaceSync *domain.SpaceSync)
	UpdateMissingIds(spaceId string, ids []string)
}

type SpaceIdGetter interface {
	app.Component
	TechSpaceId() string
	AllSpaceIds() []string
}

type State interface {
	SetObjectsNumber(status *domain.SpaceSync)
	SetSyncStatusAndErr(status domain.SpaceSyncStatus, syncError domain.SyncError, spaceId string)
	GetSyncStatus(spaceId string) domain.SpaceSyncStatus
	GetSyncObjectCount(spaceId string) int
	GetSyncErr(spaceId string) domain.SyncError
	ResetSpaceErrorStatus(spaceId string, syncError domain.SyncError)
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
	curStatuses   map[string]*domain.SpaceSync
	missingIds    map[string][]string
	mx            sync.Mutex
	periodicCall  periodicsync.PeriodicSync
	finish        chan struct{}
}

func NewSpaceSyncStatus() Updater {
	return &spaceSyncStatus{batcher: mb.New[*domain.SpaceSync](0), finish: make(chan struct{})}
}

func (s *spaceSyncStatus) Init(a *app.App) (err error) {
	s.eventSender = app.MustComponent[event.Sender](a)
	s.networkConfig = app.MustComponent[NetworkConfig](a)
	store := app.MustComponent[objectstore.ObjectStore](a)
	s.curStatuses = make(map[string]*domain.SpaceSync)
	s.missingIds = make(map[string][]string)
	s.filesState = NewFileState(store)
	s.objectsState = NewObjectState(store)
	s.spaceIdGetter = app.MustComponent[SpaceIdGetter](a)
	sessionHookRunner := app.MustComponent[session.HookRunner](a)
	sessionHookRunner.RegisterHook(s.sendSyncEventForNewSession)
	s.periodicCall = periodicsync.NewPeriodicSync(1, time.Second*5, s.update, logger.CtxLogger{Logger: log.Desugar()})
	return
}

func (s *spaceSyncStatus) Name() (name string) {
	return service
}

func (s *spaceSyncStatus) sendSyncEventForNewSession(ctx session.Context) error {
	ids := s.spaceIdGetter.AllSpaceIds()
	for _, id := range ids {
		s.sendEventToSession(id, ctx.ID())
	}
	return nil
}

func (s *spaceSyncStatus) UpdateMissingIds(spaceId string, ids []string) {
	s.mx.Lock()
	defer s.mx.Unlock()
	s.missingIds[spaceId] = ids
}

func (s *spaceSyncStatus) Run(ctx context.Context) (err error) {
	if s.networkConfig.GetNetworkMode() == pb.RpcAccount_LocalOnly {
		s.sendLocalOnlyEvent()
		close(s.finish)
		return
	} else {
		s.sendStartEvent(s.spaceIdGetter.AllSpaceIds())
	}
	s.ctx, s.ctxCancel = context.WithCancel(context.Background())
	s.periodicCall.Run()
	return
}

func (s *spaceSyncStatus) update(ctx context.Context) error {
	s.mx.Lock()
	missingIds := lo.MapEntries(s.missingIds, func(key string, value []string) (string, []string) {
		return key, slice.Copy(value)
	})
	statuses := lo.MapToSlice(s.curStatuses, func(key string, value *domain.SpaceSync) *domain.SpaceSync {
		delete(s.curStatuses, key)
		return value
	})
	s.mx.Unlock()
	for _, st := range statuses {
		if st.SpaceId == s.spaceIdGetter.TechSpaceId() {
			continue
		}
		st.MissingObjects = missingIds[st.SpaceId]
		// if the there are too many updates and this hurts performance,
		// we may skip some iterations and not do the updates for example
		s.updateSpaceSyncStatus(st)
	}
	return nil
}

func (s *spaceSyncStatus) sendEventToSession(spaceId, token string) {
	s.eventSender.SendToSession(token, &pb.Event{
		Messages: []*pb.EventMessage{{
			Value: &pb.EventMessageValueOfSpaceSyncStatusUpdate{
				SpaceSyncStatusUpdate: s.makeSpaceSyncEvent(spaceId),
			},
		}},
	})
}

func (s *spaceSyncStatus) sendStartEvent(spaceIds []string) {
	for _, id := range spaceIds {
		s.eventSender.Broadcast(&pb.Event{
			Messages: []*pb.EventMessage{{
				Value: &pb.EventMessageValueOfSpaceSyncStatusUpdate{
					SpaceSyncStatusUpdate: s.makeSpaceSyncEvent(id),
				},
			}},
		})
	}
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
	s.mx.Lock()
	defer s.mx.Unlock()
	s.curStatuses[status.SpaceId] = status
}

func (s *spaceSyncStatus) updateSpaceSyncStatus(receivedStatus *domain.SpaceSync) {
	currSyncStatus := s.getSpaceSyncStatus(receivedStatus.SpaceId)
	if s.isStatusNotChanged(receivedStatus, currSyncStatus) {
		return
	}
	state := s.getCurrentState(receivedStatus)
	prevObjectNumber := s.getObjectNumber(receivedStatus.SpaceId)
	state.SetObjectsNumber(receivedStatus)
	newObjectNumber := s.getObjectNumber(receivedStatus.SpaceId)
	state.SetSyncStatusAndErr(receivedStatus.Status, receivedStatus.SyncError, receivedStatus.SpaceId)

	spaceStatus := s.getSpaceSyncStatus(receivedStatus.SpaceId)

	// send synced event only if files and objects are all synced
	if !s.needToSendEvent(spaceStatus, currSyncStatus, prevObjectNumber, newObjectNumber) {
		return
	}
	s.eventSender.Broadcast(&pb.Event{
		Messages: []*pb.EventMessage{{
			Value: &pb.EventMessageValueOfSpaceSyncStatusUpdate{
				SpaceSyncStatusUpdate: s.makeSpaceSyncEvent(receivedStatus.SpaceId),
			},
		}},
	})
	state.ResetSpaceErrorStatus(receivedStatus.SpaceId, receivedStatus.SyncError)
}

func (s *spaceSyncStatus) isStatusNotChanged(status *domain.SpaceSync, syncStatus domain.SpaceSyncStatus) bool {
	if status.Status == domain.Syncing {
		// we need to check if number of syncing object is changed first
		return false
	}
	syncErrNotChanged := s.getError(status.SpaceId) == mapError(status.SyncError)
	if syncStatus == domain.Unknown {
		return false
	}
	statusNotChanged := syncStatus == status.Status
	if syncErrNotChanged && statusNotChanged {
		return true
	}
	return false
}

func (s *spaceSyncStatus) needToSendEvent(status domain.SpaceSyncStatus, currSyncStatus domain.SpaceSyncStatus, prevObjectNumber int64, newObjectNumber int64) bool {
	// that because we get update on syncing objects count, so we need to send updated object counter to client
	return (status == domain.Syncing && prevObjectNumber != newObjectNumber) || currSyncStatus != status
}

func (s *spaceSyncStatus) Close(ctx context.Context) (err error) {
	s.periodicCall.Close()
	return
}

func (s *spaceSyncStatus) makeSpaceSyncEvent(spaceId string) *pb.EventSpaceSyncStatusUpdate {
	return &pb.EventSpaceSyncStatusUpdate{
		Id:                    spaceId,
		Status:                mapStatus(s.getSpaceSyncStatus(spaceId)),
		Network:               mapNetworkMode(s.networkConfig.GetNetworkMode()),
		Error:                 s.getError(spaceId),
		SyncingObjectsCounter: s.getObjectNumber(spaceId),
	}
}

func (s *spaceSyncStatus) getObjectNumber(spaceId string) int64 {
	return int64(s.filesState.GetSyncObjectCount(spaceId) + s.objectsState.GetSyncObjectCount(spaceId))
}

func (s *spaceSyncStatus) getSpaceSyncStatus(spaceId string) domain.SpaceSyncStatus {
	filesStatus := s.filesState.GetSyncStatus(spaceId)
	objectsStatus := s.objectsState.GetSyncStatus(spaceId)

	if s.isUnknown(filesStatus, objectsStatus) {
		return domain.Unknown
	}
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

func (s *spaceSyncStatus) isSyncingStatus(filesStatus domain.SpaceSyncStatus, objectsStatus domain.SpaceSyncStatus) bool {
	return filesStatus == domain.Syncing || objectsStatus == domain.Syncing
}

func (s *spaceSyncStatus) isErrorStatus(filesStatus domain.SpaceSyncStatus, objectsStatus domain.SpaceSyncStatus) bool {
	return filesStatus == domain.Error || objectsStatus == domain.Error
}

func (s *spaceSyncStatus) isSyncedStatus(filesStatus domain.SpaceSyncStatus, objectsStatus domain.SpaceSyncStatus) bool {
	return filesStatus == domain.Synced && objectsStatus == domain.Synced
}

func (s *spaceSyncStatus) isOfflineStatus(filesStatus domain.SpaceSyncStatus, objectsStatus domain.SpaceSyncStatus) bool {
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

func (s *spaceSyncStatus) isUnknown(filesStatus domain.SpaceSyncStatus, objectsStatus domain.SpaceSyncStatus) bool {
	return filesStatus == domain.Unknown && objectsStatus == domain.Unknown
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

func mapStatus(status domain.SpaceSyncStatus) pb.EventSpaceStatus {
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
