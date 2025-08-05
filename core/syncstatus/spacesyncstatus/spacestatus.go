package spacesyncstatus

import (
	"context"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/nodeconf"
	"github.com/anyproto/any-sync/util/periodicsync"
	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/files/filespaceusage"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/core/syncstatus/nodestatus"
	"github.com/anyproto/anytype-heart/core/syncstatus/syncsubscriptions"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/util/slice"
)

const CName = "core.syncstatus.spacesyncstatus"

var log = logging.Logger(CName)

type Updater interface {
	app.ComponentRunnable
	Refresh(spaceId string)
	UpdateMissingIds(spaceId string, ids []string)
}

type NodeUsage interface {
	app.Component
	GetNodeUsage(ctx context.Context) (*filespaceusage.NodeUsageResponse, error)
}

type SpaceIdGetter interface {
	app.Component
	TechSpaceId() string
	AllSpaceIds() []string
}

type NetworkConfig interface {
	app.Component
	GetNetworkMode() pb.RpcAccountNetworkMode
}

type spaceSyncStatus struct {
	eventSender   event.Sender
	networkConfig NetworkConfig
	nodeStatus    nodestatus.NodeStatus
	nodeConf      nodeconf.Service
	nodeUsage     NodeUsage
	subs          syncsubscriptions.SyncSubscriptions

	spaceIdGetter  SpaceIdGetter
	curStatuses    map[string]struct{}
	missingIds     map[string][]string
	lastSentEvents map[string]pb.EventSpaceSyncStatusUpdate
	mx             sync.Mutex
	periodicCall   periodicsync.PeriodicSync
	loopInterval   time.Duration
	isLocal        bool
}

func NewSpaceSyncStatus() Updater {
	return &spaceSyncStatus{
		loopInterval: time.Second * 1,
	}
}

func (s *spaceSyncStatus) Init(a *app.App) (err error) {
	s.eventSender = app.MustComponent[event.Sender](a)
	s.networkConfig = app.MustComponent[NetworkConfig](a)
	s.nodeStatus = app.MustComponent[nodestatus.NodeStatus](a)
	s.nodeConf = app.MustComponent[nodeconf.Service](a)
	s.nodeUsage = app.MustComponent[NodeUsage](a)
	s.curStatuses = make(map[string]struct{})
	s.subs = app.MustComponent[syncsubscriptions.SyncSubscriptions](a)
	s.missingIds = make(map[string][]string)
	s.lastSentEvents = make(map[string]pb.EventSpaceSyncStatusUpdate)
	s.spaceIdGetter = app.MustComponent[SpaceIdGetter](a)
	s.isLocal = s.networkConfig.GetNetworkMode() == pb.RpcAccount_LocalOnly
	sessionHookRunner := app.MustComponent[session.HookRunner](a)
	sessionHookRunner.RegisterHook(s.sendSyncEventForNewSession)
	s.periodicCall = periodicsync.NewPeriodicSyncDuration(s.loopInterval, time.Second*5, s.update, logger.CtxLogger{Logger: log.Desugar()})
	return
}

func (s *spaceSyncStatus) Name() (name string) {
	return CName
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
	s.sendStartEvent(s.spaceIdGetter.AllSpaceIds())
	s.periodicCall.Run()
	return
}

func (s *spaceSyncStatus) getMissingIds(spaceId string) []string {
	s.mx.Lock()
	defer s.mx.Unlock()
	return slice.Copy(s.missingIds[spaceId])
}

func (s *spaceSyncStatus) update(ctx context.Context) error {
	s.mx.Lock()
	statuses := lo.MapToSlice(s.curStatuses, func(key string, value struct{}) string {
		delete(s.curStatuses, key)
		return key
	})
	s.mx.Unlock()
	for _, spaceId := range statuses {
		// if the there are too many updates and this hurts performance,
		// we may skip some iterations and not do the updates for example
		s.updateSpaceSyncStatus(spaceId)
	}
	return nil
}

func (s *spaceSyncStatus) sendEventToSession(spaceId, token string) {
	if s.isLocal {
		s.sendLocalOnlyEventToSession(spaceId, token)
		return
	}
	params := syncParams{
		bytesLeftPercentage: s.getBytesLeftPercentage(spaceId),
		connectionStatus:    s.nodeStatus.GetNodeStatus(spaceId),
		compatibility:       s.nodeConf.NetworkCompatibilityStatus(),
		objectsSyncingCount: s.getObjectSyncingObjectsCount(spaceId, s.getMissingIds(spaceId)),
		notSyncedFilesCount: s.getNotSyncedFilesCount(spaceId),
	}
	s.eventSender.SendToSession(token, event.NewEventSingleMessage(spaceId, &pb.EventMessageValueOfSpaceSyncStatusUpdate{
		SpaceSyncStatusUpdate: s.makeSyncEvent(spaceId, params),
	}))
}

func (s *spaceSyncStatus) sendStartEvent(spaceIds []string) {
	for _, id := range spaceIds {
		s.updateSpaceSyncStatus(id)
	}
}

func (s *spaceSyncStatus) sendLocalOnlyEvent(spaceId string) {
	s.broadcast(event.NewEventSingleMessage(spaceId, &pb.EventMessageValueOfSpaceSyncStatusUpdate{
		SpaceSyncStatusUpdate: &pb.EventSpaceSyncStatusUpdate{
			Id:      spaceId,
			Status:  pb.EventSpace_Offline,
			Network: pb.EventSpace_LocalOnly,
		},
	}))
}

func eventsEqual(a, b pb.EventSpaceSyncStatusUpdate) bool {
	return a.Id == b.Id &&
		a.Status == b.Status &&
		a.Network == b.Network &&
		a.Error == b.Error &&
		a.SyncingObjectsCounter == b.SyncingObjectsCounter &&
		a.NotSyncedFilesCounter == b.NotSyncedFilesCounter
}

func (s *spaceSyncStatus) broadcast(event *pb.Event) {
	s.mx.Lock()
	val := *event.Messages[0].Value.(*pb.EventMessageValueOfSpaceSyncStatusUpdate).SpaceSyncStatusUpdate
	ev, ok := s.lastSentEvents[val.Id]
	s.lastSentEvents[val.Id] = val
	s.mx.Unlock()
	if ok && eventsEqual(ev, val) {
		return
	}
	s.eventSender.Broadcast(event)
}

func (s *spaceSyncStatus) sendLocalOnlyEventToSession(spaceId, token string) {
	s.eventSender.SendToSession(token, event.NewEventSingleMessage(spaceId, &pb.EventMessageValueOfSpaceSyncStatusUpdate{
		SpaceSyncStatusUpdate: &pb.EventSpaceSyncStatusUpdate{
			Id:      spaceId,
			Status:  pb.EventSpace_Offline,
			Network: pb.EventSpace_LocalOnly,
		},
	}))
}

func (s *spaceSyncStatus) Refresh(spaceId string) {
	s.mx.Lock()
	defer s.mx.Unlock()
	s.curStatuses[spaceId] = struct{}{}
}

func (s *spaceSyncStatus) getObjectSyncingObjectsCount(spaceId string, missingObjects []string) int {
	curSub, err := s.subs.GetSubscription(spaceId)
	if err != nil {
		log.Errorf("failed to get subscription: %v", err)
		return 0
	}
	return curSub.SyncingObjectsCount(missingObjects)
}

func (s *spaceSyncStatus) getNotSyncedFilesCount(spaceId string) int {
	curSub, err := s.subs.GetSubscription(spaceId)
	if err != nil {
		log.Errorf("get not synced files count: failed to get subscription: %v", err)
		return 0
	}
	return curSub.LimitedFilesCount()
}

func (s *spaceSyncStatus) getBytesLeftPercentage(spaceId string) float64 {
	nodeUsage, err := s.nodeUsage.GetNodeUsage(context.Background())
	if err != nil {
		log.Errorf("failed to get node usage: %s", err)
		return 0
	}
	return float64(nodeUsage.Usage.BytesLeft) / float64(nodeUsage.Usage.AccountBytesLimit)
}

func (s *spaceSyncStatus) updateSpaceSyncStatus(spaceId string) {
	if s.isLocal {
		s.sendLocalOnlyEvent(spaceId)
		return
	}
	missingObjects := s.getMissingIds(spaceId)
	params := syncParams{
		bytesLeftPercentage: s.getBytesLeftPercentage(spaceId),
		connectionStatus:    s.nodeStatus.GetNodeStatus(spaceId),
		compatibility:       s.nodeConf.NetworkCompatibilityStatus(),
		objectsSyncingCount: s.getObjectSyncingObjectsCount(spaceId, missingObjects),
		notSyncedFilesCount: s.getNotSyncedFilesCount(spaceId),
	}
	s.broadcast(event.NewEventSingleMessage(spaceId, &pb.EventMessageValueOfSpaceSyncStatusUpdate{
		SpaceSyncStatusUpdate: s.makeSyncEvent(spaceId, params),
	}))
}

func (s *spaceSyncStatus) Close(ctx context.Context) (err error) {
	s.periodicCall.Close()
	return
}

type syncParams struct {
	bytesLeftPercentage float64
	connectionStatus    nodestatus.ConnectionStatus
	compatibility       nodeconf.NetworkCompatibilityStatus
	objectsSyncingCount int
	notSyncedFilesCount int
}

func (s *spaceSyncStatus) makeSyncEvent(spaceId string, params syncParams) *pb.EventSpaceSyncStatusUpdate {
	status := pb.EventSpace_Synced
	err := pb.EventSpace_Null
	syncingObjectsCount := int64(params.objectsSyncingCount)
	if syncingObjectsCount > 0 {
		status = pb.EventSpace_Syncing
	}
	if params.bytesLeftPercentage < 0.1 {
		status = pb.EventSpace_Error
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
	if params.compatibility == nodeconf.NetworkCompatibilityStatusNeedsUpdate {
		status = pb.EventSpace_NetworkNeedsUpdate
	}
	return &pb.EventSpaceSyncStatusUpdate{
		Id:                    spaceId,
		Status:                status,
		Network:               mapNetworkMode(s.networkConfig.GetNetworkMode()),
		Error:                 err,
		SyncingObjectsCounter: syncingObjectsCount,
		NotSyncedFilesCounter: int64(params.notSyncedFilesCount),
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
