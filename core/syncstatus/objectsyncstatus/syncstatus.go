package objectsyncstatus

import (
	"context"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/anyproto/any-sync/commonspace/spacestate"
	"github.com/anyproto/any-sync/commonspace/syncstatus"

	"github.com/anyproto/any-sync/commonspace/spacestorage"
	"github.com/anyproto/any-sync/nodeconf"
	"github.com/anyproto/any-sync/util/periodicsync"
	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/syncstatus/nodestatus"
	"github.com/anyproto/anytype-heart/util/slice"
)

const (
	syncUpdateInterval = 3
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

type treeHeadsEntry struct {
	heads      []string
	syncStatus SyncStatus
}

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
	periodicSync   periodicsync.PeriodicSync
	updateReceiver UpdateReceiver
	storage        spacestorage.SpaceStorage

	spaceId    string
	synced     []string
	tempSynced map[string]struct{}
	treeHeads  map[string]treeHeadsEntry
	watchers   map[string]struct{}

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
		treeHeads:  map[string]treeHeadsEntry{},
		watchers:   map[string]struct{}{},
	}
}

func (s *syncStatusService) Init(a *app.App) (err error) {
	sharedState := a.MustComponent(spacestate.CName).(*spacestate.SpaceState)
	s.updateIntervalSecs = syncUpdateInterval
	s.updateTimeout = syncTimeout
	s.spaceId = sharedState.SpaceId
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
	s.Lock()
	s.treeHeads[treeId] = treeHeadsEntry{heads: heads, syncStatus: StatusNotSynced}
	s.Unlock()
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
	if !allAdded {
		return
	}
	s.synced = append(s.synced, treeId)
	if curTreeHeads, ok := s.treeHeads[treeId]; ok {
		// checking if we received the head that we are interested in
		for _, head := range heads {
			if idx, found := slices.BinarySearch(curTreeHeads.heads, head); found {
				curTreeHeads.heads[idx] = ""
			}
		}
		curTreeHeads.heads = slice.RemoveMut(curTreeHeads.heads, "")
		if len(curTreeHeads.heads) == 0 {
			curTreeHeads.syncStatus = StatusSynced
		}
		s.treeHeads[treeId] = curTreeHeads
	}
}

func (s *syncStatusService) update(ctx context.Context) (err error) {
	var (
		updateDetailsStatuses []treeStatus
		updateThreadStatuses  []treeStatus
	)
	s.Lock()
	if s.updateReceiver == nil {
		s.Unlock()
		return
	}
	for _, treeId := range s.synced {
		updateDetailsStatuses = append(updateDetailsStatuses, treeStatus{treeId, StatusSynced})
	}
	for treeId := range s.watchers {
		treeHeads, exists := s.treeHeads[treeId]
		if !exists {
			continue
		}
		updateThreadStatuses = append(updateThreadStatuses, treeStatus{treeId, treeHeads.syncStatus})
	}
	s.synced = s.synced[:0]
	s.Unlock()
	s.updateReceiver.UpdateNodeStatus()
	for _, entry := range updateDetailsStatuses {
		s.updateDetails(entry.treeId, mapStatus(entry.status))
	}
	for _, entry := range updateThreadStatuses {
		err = s.updateReceiver.UpdateTree(ctx, entry.treeId, entry.status)
		if err != nil {
			return
		}
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
	s.Lock()
	defer s.Unlock()
	_, ok := s.treeHeads[treeId]
	if !ok {
		var (
			st    treestorage.TreeStorage
			heads []string
		)
		st, err = s.storage.TreeStorage(treeId)
		if err != nil {
			return
		}
		heads, err = st.Heads()
		if err != nil {
			return
		}
		slices.Sort(heads)
		s.treeHeads[treeId] = treeHeadsEntry{
			heads:      heads,
			syncStatus: StatusUnknown,
		}
	}

	s.watchers[treeId] = struct{}{}
	return
}

func (s *syncStatusService) Unwatch(treeId string) {
	s.Lock()
	defer s.Unlock()
	delete(s.watchers, treeId)
}

func (s *syncStatusService) RemoveAllExcept(senderId string, differentRemoteIds []string) {
	// if sender is not a responsible node, then this should have no effect
	if !s.isSenderResponsible(senderId) {
		return
	}

	s.Lock()
	defer s.Unlock()

	slices.Sort(differentRemoteIds)
	for treeId, entry := range s.treeHeads {
		if _, found := slices.BinarySearch(differentRemoteIds, treeId); !found {
			if entry.syncStatus != StatusSynced {
				entry.syncStatus = StatusSynced
				s.treeHeads[treeId] = entry
			}
		}
	}
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
	return slices.Contains(s.nodeConfService.NodeIds(s.spaceId), senderId)
}

func (s *syncStatusService) updateDetails(treeId string, status domain.ObjectSyncStatus) {
	s.syncDetailsUpdater.UpdateDetails(treeId, status, s.spaceId)
}
