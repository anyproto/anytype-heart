package objectsyncstatus

import (
	"context"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/commonspace/spacestate"
	"github.com/anyproto/any-sync/commonspace/syncstatus"

	"github.com/anyproto/any-sync/commonspace/spacestorage"
	"github.com/anyproto/any-sync/nodeconf"
	"github.com/anyproto/any-sync/util/periodicsync"
	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/syncstatus/nodestatus"
)

const (
	syncUpdateInterval = 2
	syncTimeout        = time.Second
)

var log = logger.NewNamed(syncstatus.CName)

type UpdateReceiver interface {
	UpdateTree(ctx context.Context, treeId string, status SyncStatus) (err error)
	UpdateNodeStatus()
}

type SyncStatus int

const (
	StatusUnknown SyncStatus = iota
	StatusSynced
	StatusNotSynced
)

type StatusUpdater interface {
	HeadsChange(treeId string, heads []string)
	HeadsReceive(senderId, treeId string, heads []string)
	HeadsApply(senderId, treeId string, heads []string, allAdded bool)
	ObjectReceive(senderId, treeId string, heads []string)
	RemoveAllExcept(senderId string, differentRemoteIds []string)
}

type StatusWatcher interface {
	Watch(treeId string) (err error)
	Unwatch(treeId string)
	SetUpdateReceiver(updater UpdateReceiver)
}

type StatusService interface {
	app.ComponentRunnable
	StatusUpdater
	StatusWatcher
}

type treeStatus struct {
	treeId string
	status SyncStatus
}

type Updater interface {
	app.Component
	UpdateDetails(objectId string, status domain.ObjectSyncStatus, spaceId string)
}

type syncStatusService struct {
	sync.Mutex
	configuration  nodeconf.NodeConf
	periodicSync   periodicsync.PeriodicSync
	updateReceiver UpdateReceiver
	storage        spacestorage.SpaceStorage

	spaceId      string
	synced       []string
	tempSynced   map[string]struct{}
	stateCounter uint64

	updateIntervalSecs int
	updateTimeout      time.Duration

	syncDetailsUpdater Updater
	nodeStatus         nodestatus.NodeStatus
	config             *config.Config
	nodeConfService    nodeconf.Service
}

func NewSyncStatusService() StatusService {
	return &syncStatusService{
		tempSynced: map[string]struct{}{},
	}
}

func (s *syncStatusService) Init(a *app.App) (err error) {
	sharedState := a.MustComponent(spacestate.CName).(*spacestate.SpaceState)
	s.updateIntervalSecs = syncUpdateInterval
	s.updateTimeout = syncTimeout
	s.spaceId = sharedState.SpaceId
	s.configuration = app.MustComponent[nodeconf.NodeConf](a)
	s.storage = app.MustComponent[spacestorage.SpaceStorage](a)
	s.periodicSync = periodicsync.NewPeriodicSync(
		s.updateIntervalSecs,
		s.updateTimeout,
		s.update,
		log)
	s.syncDetailsUpdater = app.MustComponent[Updater](a)
	s.config = app.MustComponent[*config.Config](a)
	s.nodeConfService = app.MustComponent[nodeconf.Service](a)
	s.nodeStatus = app.MustComponent[nodestatus.NodeStatus](a)
	return
}

func (s *syncStatusService) Name() (name string) {
	return syncstatus.CName
}

func (s *syncStatusService) SetUpdateReceiver(updater UpdateReceiver) {
	s.Lock()
	defer s.Unlock()

	s.updateReceiver = updater
}

func (s *syncStatusService) Run(ctx context.Context) error {
	s.periodicSync.Run()
	return nil
}

func (s *syncStatusService) HeadsChange(treeId string, heads []string) {
	s.updateDetails(treeId, domain.ObjectSyncing)
}

func (s *syncStatusService) ObjectReceive(senderId, treeId string, heads []string) {
	s.Lock()
	defer s.Unlock()
	if len(heads) == 0 || !s.isSenderResponsible(senderId) {
		s.tempSynced[treeId] = struct{}{}
		return
	}
	s.synced = append(s.synced, treeId)
}

func (s *syncStatusService) HeadsApply(senderId, treeId string, heads []string, allAdded bool) {
	s.Lock()
	defer s.Unlock()
	if len(heads) == 0 || !s.isSenderResponsible(senderId) {
		if allAdded {
			s.tempSynced[treeId] = struct{}{}
		}
		return
	}
	if allAdded {
		s.synced = append(s.synced, treeId)
	}
}

func (s *syncStatusService) update(ctx context.Context) (err error) {
	var treeStatusBuf []treeStatus
	s.Lock()
	if s.updateReceiver == nil {
		s.Unlock()
		return
	}
	for _, treeId := range s.synced {
		treeStatusBuf = append(treeStatusBuf, treeStatus{treeId, StatusSynced})
	}
	s.synced = s.synced[:0]
	s.Unlock()
	s.updateReceiver.UpdateNodeStatus()
	for _, entry := range treeStatusBuf {
		err = s.updateReceiver.UpdateTree(ctx, entry.treeId, entry.status)
		if err != nil {
			return
		}
		s.updateDetails(entry.treeId, mapStatus(entry.status))
	}
	return
}

func mapStatus(status SyncStatus) domain.ObjectSyncStatus {
	if status == StatusSynced {
		return domain.ObjectSynced
	}
	return domain.ObjectSyncing
}

func (s *syncStatusService) HeadsReceive(senderId, treeId string, heads []string) {
}

func (s *syncStatusService) Watch(treeId string) (err error) {
	return nil
}

func (s *syncStatusService) Unwatch(treeId string) {
}

func (s *syncStatusService) RemoveAllExcept(senderId string, differentRemoteIds []string) {
	// if sender is not a responsible node, then this should have no effect
	if !s.isSenderResponsible(senderId) {
		return
	}

	s.Lock()
	defer s.Unlock()

	slices.Sort(differentRemoteIds)
	for treeId := range s.tempSynced {
		delete(s.tempSynced, treeId)
		if _, found := slices.BinarySearch(differentRemoteIds, treeId); !found {
			s.synced = append(s.synced, treeId)
		}
	}
}

func (s *syncStatusService) Close(ctx context.Context) error {
	s.periodicSync.Close()
	return nil
}

func (s *syncStatusService) isSenderResponsible(senderId string) bool {
	return slices.Contains(s.configuration.NodeIds(s.spaceId), senderId)
}

func (s *syncStatusService) updateDetails(treeId string, status domain.ObjectSyncStatus) {
	s.syncDetailsUpdater.UpdateDetails(treeId, status, s.spaceId)
}
