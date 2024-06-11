package objectsyncstatus

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/commonspace/spacestate"
	"github.com/anyproto/any-sync/commonspace/syncstatus"

	"github.com/anyproto/any-sync/commonspace/object/tree/treestorage"
	"github.com/anyproto/any-sync/commonspace/spacestorage"
	"github.com/anyproto/any-sync/nodeconf"
	"github.com/anyproto/any-sync/util/periodicsync"
	"github.com/anyproto/any-sync/util/slice"
	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/syncstatus/nodestatus"
)

const (
	syncUpdateInterval = 5
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

type treeHeadsEntry struct {
	heads        []string
	stateCounter uint64
	syncStatus   SyncStatus
}

type treeStatus struct {
	treeId string
	status SyncStatus
}

type Updater interface {
	app.Component
	UpdateDetails(objectId []string, status domain.SyncStatus, syncError domain.SyncError, spaceId string)
}

type syncStatusService struct {
	sync.Mutex
	configuration  nodeconf.NodeConf
	periodicSync   periodicsync.PeriodicSync
	updateReceiver UpdateReceiver
	storage        spacestorage.SpaceStorage

	spaceId      string
	treeHeads    map[string]treeHeadsEntry
	watchers     map[string]struct{}
	stateCounter uint64

	treeStatusBuf []treeStatus

	updateIntervalSecs int
	updateTimeout      time.Duration

	syncDetailsUpdater Updater
	nodeStatus         nodestatus.NodeStatus
	config             *config.Config
	nodeConfService    nodeconf.Service
}

func NewSyncStatusService() StatusService {
	return &syncStatusService{
		treeHeads: map[string]treeHeadsEntry{},
		watchers:  map[string]struct{}{},
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
	s.Lock()
	defer s.Unlock()

	var headsCopy []string
	headsCopy = append(headsCopy, heads...)

	s.treeHeads[treeId] = treeHeadsEntry{
		heads:        headsCopy,
		stateCounter: s.stateCounter,
		syncStatus:   StatusNotSynced,
	}
	s.stateCounter++
	s.updateDetails(treeId, domain.Syncing)
}

func (s *syncStatusService) update(ctx context.Context) (err error) {
	s.treeStatusBuf = s.treeStatusBuf[:0]

	s.Lock()
	if s.updateReceiver == nil {
		s.Unlock()
		return
	}
	for treeId := range s.watchers {
		// that means that we haven't yet got the status update
		treeHeads, exists := s.treeHeads[treeId]
		if !exists {
			err = fmt.Errorf("treeHeads should always exist for watchers")
			s.Unlock()
			return
		}
		s.treeStatusBuf = append(s.treeStatusBuf, treeStatus{treeId, treeHeads.syncStatus})
	}
	s.Unlock()
	s.updateReceiver.UpdateNodeStatus()
	for _, entry := range s.treeStatusBuf {
		err = s.updateReceiver.UpdateTree(ctx, entry.treeId, entry.status)
		if err != nil {
			return
		}
	}
	return
}

func (s *syncStatusService) HeadsReceive(senderId, treeId string, heads []string) {
	s.Lock()
	defer s.Unlock()

	curTreeHeads, ok := s.treeHeads[treeId]
	if !ok || curTreeHeads.syncStatus == StatusSynced {
		return
	}

	// checking if other node is responsible
	if len(heads) == 0 || !s.isSenderResponsible(senderId) {
		return
	}

	// checking if we received the head that we are interested in
	for _, head := range heads {
		if idx, found := slices.BinarySearch(curTreeHeads.heads, head); found {
			curTreeHeads.heads[idx] = ""
		}
	}
	curTreeHeads.heads = slice.DiscardFromSlice(curTreeHeads.heads, func(h string) bool {
		return h == ""
	})
	if len(curTreeHeads.heads) == 0 {
		curTreeHeads.syncStatus = StatusSynced
	}
	s.treeHeads[treeId] = curTreeHeads
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
		s.stateCounter++
		s.treeHeads[treeId] = treeHeadsEntry{
			heads:        heads,
			stateCounter: s.stateCounter,
			syncStatus:   StatusUnknown,
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
		// if the current update is outdated
		if entry.stateCounter > s.stateCounter {
			continue
		}
		// if we didn't find our treeId in heads ids which are different from us and node
		if _, found := slices.BinarySearch(differentRemoteIds, treeId); !found {
			entry.syncStatus = StatusSynced
			s.treeHeads[treeId] = entry
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

func (s *syncStatusService) updateDetails(treeId string, status domain.SyncStatus) {
	var syncErr domain.SyncError
	if s.nodeStatus.GetNodeStatus(s.spaceId) != nodestatus.Online {
		syncErr = domain.NetworkError
		status = domain.Error
	}
	if s.config.IsLocalOnlyMode() {
		syncErr = domain.Null
		status = domain.Offline
	}
	if s.nodeConfService.NetworkCompatibilityStatus() == nodeconf.NetworkCompatibilityStatusIncompatible {
		syncErr = domain.IncompatibleVersion
		status = domain.Error
	}
	s.syncDetailsUpdater.UpdateDetails([]string{treeId}, status, syncErr, s.spaceId)
}
