package spacesyncstatus

/*
AI generated

Name: Space Sync Status Event Broadcaster
Scope: global

## Responsibility
- Aggregates sync state from multiple sources (node status, storage usage, sync subscriptions)
- Computes overall space sync status and broadcasts EventSpaceSyncStatusUpdate to clients
- Deduplicates identical events before broadcasting

## Background Tasks
- periodicCall: processes queued Refresh requests every 1s (update)

## Documentation
Status computation priority (highest wins):
1. IncompatibleVersion - network protocol incompatible
2. NetworkNeedsUpdate - network needs update
3. ConnectionError -> Offline
4. StorageLimitExceed (< 10% storage left)
5. Syncing (objects pending sync)
6. Synced (default)
*/

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/nodeconf"
	"github.com/anyproto/any-sync/util/periodicsync"
	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/anytype/account"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/files/filespaceusage"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/core/syncstatus/nodestatus"
	"github.com/anyproto/anytype-heart/core/syncstatus/syncsubscriptions"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/util/slice"
)

const CName = "core.syncstatus.spacesyncstatus"

var log = logging.Logger(CName)

type Updater interface {
	app.ComponentRunnable
	Refresh(spaceId string)
	UpdateMissingIds(spaceId string, ids []string)
	// SpaceStatus computes the space's sync status now: the value the app is
	// sent for it, except the storage warning. That warning reads the
	// account's usage, which can wait on the backup node, and judges the
	// caller's own account whatever the space. The counts are read from the
	// store by the filters of the app's subscriptions, without creating any:
	// a read must not leave live subscriptions behind for every space it
	// touches. connectionChecked is false while no connection to the space's
	// node has been tried yet: the app then shows the connection as fine. It
	// broadcasts nothing.
	SpaceStatus(spaceId string) (status *pb.EventSpaceSyncStatusUpdate, connectionChecked bool, err error)
}

type NodeUsage interface {
	app.Component
	GetNodeUsage(ctx context.Context) (*filespaceusage.NodeUsageResponse, error)
}

type SpaceIdGetter interface {
	app.Component
	TechSpaceId() string
	AllLoadedSpaceIds() []string
}

type accountIdGetter interface {
	AccountID() string
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
	// objectStore and accountService serve SpaceStatus, which counts from the
	// store instead of the subscriptions
	objectStore    objectstore.ObjectStore
	accountService accountIdGetter

	spaceIdGetter  SpaceIdGetter
	curStatuses    map[string]struct{}
	missingIds     map[string][]string
	lastSentEvents map[string]pb.EventSpaceSyncStatusUpdate
	mx             sync.Mutex
	periodicCall   periodicsync.PeriodicSync
	loopInterval   time.Duration
	startDelay     time.Duration // delay before starting sync, can be overridden in tests
	isLocal        bool
	closeCh        chan struct{}
}

func NewSpaceSyncStatus() Updater {
	return &spaceSyncStatus{
		loopInterval: time.Second * 1,
		startDelay:   time.Second * 3,
		closeCh:      make(chan struct{}),
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
	s.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	s.accountService = app.MustComponent[account.Service](a)
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
	ids := s.spaceIdGetter.AllLoadedSpaceIds()
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
	go func() {
		select {
		case <-time.After(s.startDelay):
		case <-s.closeCh:
			return
		}
		s.sendStartEvent(s.spaceIdGetter.AllLoadedSpaceIds())
		s.periodicCall.Run()
	}()
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
	s.eventSender.SendToSession(token, event.NewEventSingleMessage(spaceId, &pb.EventMessageValueOfSpaceSyncStatusUpdate{
		SpaceSyncStatusUpdate: s.appStatus(spaceId),
	}))
}

func (s *spaceSyncStatus) SpaceStatus(spaceId string) (*pb.EventSpaceSyncStatusUpdate, bool, error) {
	if s.isLocal {
		return localOnlyEvent(spaceId), true, nil
	}
	_, checked := s.nodeStatus.TryGetNodeStatus(spaceId)
	params, err := s.storeParams(spaceId)
	if err != nil {
		return nil, false, fmt.Errorf("sync counts of space %s: %w", spaceId, err)
	}
	return s.makeSyncEvent(spaceId, params), checked, nil
}

// storeParams reads the inputs of the space's status with the counts taken
// from the store at one moment, and no storage warning: its input reads as
// all of the storage left.
func (s *spaceSyncStatus) storeParams(spaceId string) (syncParams, error) {
	index := s.objectStore.SpaceIndex(spaceId)
	pendingIds, _, err := index.QueryObjectIds(database.Query{Filters: syncsubscriptions.SyncingObjectsFilters()})
	if err != nil {
		return syncParams{}, fmt.Errorf("query objects not in sync: %w", err)
	}
	_, refused, err := index.QueryObjectIds(database.Query{Filters: syncsubscriptions.LimitedFilesFilters()})
	if err != nil {
		return syncParams{}, fmt.Errorf("query files refused for the limit: %w", err)
	}
	me := domain.NewParticipantId(spaceId, s.accountService.AccountID())
	_, uploading, err := index.QueryObjectIds(database.Query{Filters: syncsubscriptions.UploadingFilesFilters(me)})
	if err != nil {
		return syncParams{}, fmt.Errorf("query files uploading: %w", err)
	}
	return syncParams{
		bytesLeftPercentage: 1,
		connectionStatus:    s.nodeStatus.GetNodeStatus(spaceId),
		compatibility:       s.nodeConf.NetworkCompatibilityStatus(),
		objectsSyncingCount: syncsubscriptions.SyncingObjectsCount(pendingIds, s.getMissingIds(spaceId)),
		notSyncedFilesCount: refused,
		uploadingFilesCount: uploading,
	}, nil
}

// appStatus computes the status the app is sent, from its subscriptions.
func (s *spaceSyncStatus) appStatus(spaceId string) *pb.EventSpaceSyncStatusUpdate {
	if s.isLocal {
		return localOnlyEvent(spaceId)
	}
	return s.makeSyncEvent(spaceId, s.currentParams(spaceId))
}

// currentParams reads the inputs of the space's status from their sources
func (s *spaceSyncStatus) currentParams(spaceId string) syncParams {
	return syncParams{
		bytesLeftPercentage: s.getBytesLeftPercentage(spaceId),
		connectionStatus:    s.nodeStatus.GetNodeStatus(spaceId),
		compatibility:       s.nodeConf.NetworkCompatibilityStatus(),
		objectsSyncingCount: s.getObjectSyncingObjectsCount(spaceId, s.getMissingIds(spaceId)),
		notSyncedFilesCount: s.getNotSyncedFilesCount(spaceId),
		uploadingFilesCount: s.getUploadingFilesCount(spaceId),
	}
}

func localOnlyEvent(spaceId string) *pb.EventSpaceSyncStatusUpdate {
	return &pb.EventSpaceSyncStatusUpdate{
		Id:      spaceId,
		Status:  pb.EventSpace_Offline,
		Network: pb.EventSpace_LocalOnly,
	}
}

func (s *spaceSyncStatus) sendStartEvent(spaceIds []string) {
	for _, id := range spaceIds {
		s.updateSpaceSyncStatus(id)
	}
}

func eventsEqual(a, b pb.EventSpaceSyncStatusUpdate) bool {
	return a.Id == b.Id &&
		a.Status == b.Status &&
		a.Network == b.Network &&
		a.Error == b.Error &&
		a.SyncingObjectsCounter == b.SyncingObjectsCounter &&
		a.NotSyncedFilesCounter == b.NotSyncedFilesCounter &&
		a.UploadingFilesCounter == b.UploadingFilesCounter
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

func (s *spaceSyncStatus) getUploadingFilesCount(spaceId string) int {
	curSub, err := s.subs.GetSubscription(spaceId)
	if err != nil {
		log.Errorf("get uploading files count: failed to get subscription: %v", err)
		return 0
	}
	return curSub.UploadingFilesCount()
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
	s.broadcast(event.NewEventSingleMessage(spaceId, &pb.EventMessageValueOfSpaceSyncStatusUpdate{
		SpaceSyncStatusUpdate: s.appStatus(spaceId),
	}))
}

func (s *spaceSyncStatus) Close(ctx context.Context) (err error) {
	close(s.closeCh)
	s.periodicCall.Close()
	return
}

type syncParams struct {
	bytesLeftPercentage float64
	connectionStatus    nodestatus.ConnectionStatus
	compatibility       nodeconf.NetworkCompatibilityStatus
	objectsSyncingCount int
	notSyncedFilesCount int
	uploadingFilesCount int
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
		Network:               MapNetworkMode(s.networkConfig.GetNetworkMode()),
		Error:                 err,
		SyncingObjectsCounter: syncingObjectsCount,
		NotSyncedFilesCounter: int64(params.notSyncedFilesCount),
		UploadingFilesCounter: int64(params.uploadingFilesCount),
	}
}

// MapNetworkMode maps the account's network mode to the network a space
// syncs to, as the app is told.
func MapNetworkMode(mode pb.RpcAccountNetworkMode) pb.EventSpaceNetwork {
	switch mode {
	case pb.RpcAccount_LocalOnly:
		return pb.EventSpace_LocalOnly
	case pb.RpcAccount_CustomConfig:
		return pb.EventSpace_SelfHost
	default:
		return pb.EventSpace_Anytype
	}
}
