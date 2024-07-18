package spacesyncstatus

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/nodeconf"
	"github.com/anyproto/any-sync/util/periodicsync"
	"github.com/cheggaaa/mb/v3"
	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/files"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/core/syncstatus/filesyncstatus"
	"github.com/anyproto/anytype-heart/core/syncstatus/nodestatus"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
)

const service = "core.syncstatus.spacesyncstatus"

var log = logging.Logger("anytype-mw-space-status")

// nodeconfservice
// nodestatus
// GetNodeUsage(ctx context.Context) (*NodeUsageResponse, error)

type Updater interface {
	app.ComponentRunnable
	Refresh(spaceId string)
	UpdateMissingIds(spaceId string, ids []string)
}

type NodeUsage interface {
	GetNodeUsage(ctx context.Context) (*files.NodeUsageResponse, error)
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
	nodeStatus    nodestatus.NodeStatus
	nodeConf      nodeconf.Service
	nodeUsage     NodeUsage
	store         objectstore.ObjectStore
	batcher       *mb.MB[*domain.SpaceSync]

	filesState   State
	objectsState State

	ctx           context.Context
	ctxCancel     context.CancelFunc
	spaceIdGetter SpaceIdGetter
	curStatuses   map[string]struct{}
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
	s.nodeStatus = app.MustComponent[nodestatus.NodeStatus](a)
	s.nodeConf = app.MustComponent[nodeconf.Service](a)
	s.nodeUsage = app.MustComponent[NodeUsage](a)
	s.store = app.MustComponent[objectstore.ObjectStore](a)
	s.curStatuses = make(map[string]struct{})
	s.missingIds = make(map[string][]string)
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
	// TODO: use subscriptions inside middleware instead of this
	s.mx.Lock()
	missingIds := lo.MapEntries(s.missingIds, func(key string, value []string) (string, []string) {
		return key, slice.Copy(value)
	})
	statuses := lo.MapToSlice(s.curStatuses, func(key string, value struct{}) string {
		delete(s.curStatuses, key)
		return key
	})
	s.mx.Unlock()
	for _, spaceId := range statuses {
		if spaceId == s.spaceIdGetter.TechSpaceId() {
			continue
		}
		// if the there are too many updates and this hurts performance,
		// we may skip some iterations and not do the updates for example
		s.updateSpaceSyncStatus(spaceId, missingIds[spaceId])
	}
	return nil
}

func (s *spaceSyncStatus) sendEventToSession(spaceId, token string) {
	s.mx.Lock()
	missingObjects := s.missingIds[spaceId]
	s.mx.Unlock()
	params := syncParams{
		bytesLeftPercentage: s.getBytesLeftPercentage(spaceId),
		connectionStatus:    s.nodeStatus.GetNodeStatus(spaceId),
		compatibility:       s.nodeConf.NetworkCompatibilityStatus(),
		filesSyncingCount:   s.getFileSyncingObjectsCount(spaceId),
		objectsSyncingCount: s.getObjectSyncingObjectsCount(spaceId, missingObjects),
	}
	s.eventSender.SendToSession(token, &pb.Event{
		Messages: []*pb.EventMessage{{
			Value: &pb.EventMessageValueOfSpaceSyncStatusUpdate{
				SpaceSyncStatusUpdate: s.makeSyncEvent(spaceId, params),
			},
		}},
	})
}

func (s *spaceSyncStatus) sendStartEvent(spaceIds []string) {
	for _, id := range spaceIds {
		s.mx.Lock()
		missingObjects := s.missingIds[id]
		s.mx.Unlock()
		s.updateSpaceSyncStatus(id, missingObjects)
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

func (s *spaceSyncStatus) Refresh(spaceId string) {
	s.mx.Lock()
	defer s.mx.Unlock()
	s.curStatuses[spaceId] = struct{}{}
}

func (s *spaceSyncStatus) getObjectSyncingObjectsCount(spaceId string, missingObjects []string) int {
	ids, _, err := s.store.QueryObjectIDs(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeySyncStatus.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Int64(int64(domain.Syncing)),
			},
			{
				RelationKey: bundle.RelationKeyLayout.String(),
				Condition:   model.BlockContentDataviewFilter_NotIn,
				Value: pbtypes.IntList(
					int(model.ObjectType_file),
					int(model.ObjectType_image),
					int(model.ObjectType_video),
					int(model.ObjectType_audio),
				),
			},
			{
				RelationKey: bundle.RelationKeySpaceId.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(spaceId),
			},
		},
	})
	if err != nil {
		log.Errorf("failed to query file status: %s", err)
	}
	_, added := slice.DifferenceRemovedAdded(ids, missingObjects)
	return len(ids) + len(added)
}

func (s *spaceSyncStatus) getFileSyncingObjectsCount(spaceId string) int {
	recs, _, err := s.store.QueryObjectIDs(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyFileBackupStatus.String(),
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       pbtypes.IntList(int(filesyncstatus.Syncing), int(filesyncstatus.Queued)),
			},
			{
				RelationKey: bundle.RelationKeySpaceId.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(spaceId),
			},
		},
	})
	if err != nil {
		log.Errorf("failed to query file status: %s", err)
	}
	return len(recs)
}

func (s *spaceSyncStatus) getBytesLeftPercentage(spaceId string) float64 {
	nodeUsage, err := s.nodeUsage.GetNodeUsage(context.Background())
	if err != nil {
		log.Errorf("failed to get node usage: %s", err)
		return 0
	}
	return float64(nodeUsage.Usage.BytesLeft) / float64(nodeUsage.Usage.AccountBytesLimit)
}

func (s *spaceSyncStatus) updateSpaceSyncStatus(spaceId string, missingObjects []string) {
	params := syncParams{
		bytesLeftPercentage: s.getBytesLeftPercentage(spaceId),
		connectionStatus:    s.nodeStatus.GetNodeStatus(spaceId),
		compatibility:       s.nodeConf.NetworkCompatibilityStatus(),
		filesSyncingCount:   s.getFileSyncingObjectsCount(spaceId),
		objectsSyncingCount: s.getObjectSyncingObjectsCount(spaceId, missingObjects),
	}
	// fmt.Println("[x]: space status", event.Status, "space id", receivedStatus.SpaceId, "network", event.Network, "error", event.Error, "object number", event.SyncingObjectsCounter, "isFile", isFileState)
	s.eventSender.Broadcast(&pb.Event{
		Messages: []*pb.EventMessage{{
			Value: &pb.EventMessageValueOfSpaceSyncStatusUpdate{
				SpaceSyncStatusUpdate: s.makeSyncEvent(spaceId, params),
			},
		}},
	})
}

func (s *spaceSyncStatus) Close(ctx context.Context) (err error) {
	s.periodicCall.Close()
	return
}

type syncParams struct {
	bytesLeftPercentage float64
	connectionStatus    nodestatus.ConnectionStatus
	compatibility       nodeconf.NetworkCompatibilityStatus
	filesSyncingCount   int
	objectsSyncingCount int
}

func (s *spaceSyncStatus) makeSyncEvent(spaceId string, params syncParams) *pb.EventSpaceSyncStatusUpdate {
	status := pb.EventSpace_Synced
	err := pb.EventSpace_Null
	syncingObjectsCount := int64(params.objectsSyncingCount + params.filesSyncingCount)
	if syncingObjectsCount > 0 {
		status = pb.EventSpace_Syncing
	}
	if params.bytesLeftPercentage < 0.1 {
		err = pb.EventSpace_StorageLimitExceed
	}
	if params.connectionStatus == nodestatus.ConnectionError {
		status = pb.EventSpace_Offline
		err = pb.EventSpace_NetworkError
	}
	if params.compatibility == nodeconf.NetworkCompatibilityStatusIncompatible {
		status = pb.EventSpace_Error
		err = pb.EventSpace_IncompatibleVersion
	}
	fmt.Println("[x]: status: connection", params.connectionStatus, ", space id", spaceId, ", compatibility", params.compatibility, ", object number", syncingObjectsCount, ", bytes left", params.bytesLeftPercentage)
	return &pb.EventSpaceSyncStatusUpdate{
		Id:                    spaceId,
		Status:                status,
		Network:               mapNetworkMode(s.networkConfig.GetNetworkMode()),
		Error:                 err,
		SyncingObjectsCounter: syncingObjectsCount,
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
