package objectsyncstatus

import (
	"context"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonspace/spacestate"
	"github.com/anyproto/any-sync/commonspace/spacestorage"
	"github.com/anyproto/any-sync/commonspace/syncstatus"

	"github.com/anyproto/any-sync/nodeconf"
	"golang.org/x/exp/slices"

	"github.com/anyproto/anytype-heart/core/anytype/config"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/util/slice"
)

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

type StatusService interface {
	app.ComponentRunnable
	StatusUpdater
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

	spaceId         string
	spaceSettingsId string
	tempSynced      map[string]struct{}
	treeHeads       map[string]treeHeadsEntry

	updateIntervalSecs int
	updateTimeout      time.Duration

	syncDetailsUpdater Updater
	config             *config.Config
	nodeConfService    nodeconf.Service
}

func NewSyncStatusService() StatusService {
	return &syncStatusService{
		tempSynced: map[string]struct{}{},
		treeHeads:  map[string]treeHeadsEntry{},
	}
}

func (s *syncStatusService) Init(a *app.App) (err error) {
	sharedState := app.MustComponent[*spacestate.SpaceState](a)
	spaceStorage := app.MustComponent[spacestorage.SpaceStorage](a)
	s.spaceId = sharedState.SpaceId
	s.spaceSettingsId = spaceStorage.StateStorage().SettingsId()
	s.syncDetailsUpdater = app.MustComponent[Updater](a)
	s.config = app.MustComponent[*config.Config](a)
	s.nodeConfService = app.MustComponent[nodeconf.Service](a)
	return
}

func (s *syncStatusService) Name() (name string) {
	return syncstatus.CName
}

func (s *syncStatusService) Run(ctx context.Context) error {
	return nil
}

func (s *syncStatusService) HeadsChange(treeId string, heads []string) {
	s.Lock()
	s.addTreeHead(treeId, heads, StatusNotSynced)
	s.Unlock()

	s.updateDetails(treeId, domain.ObjectSyncStatusSyncing)
}

func (s *syncStatusService) ObjectReceive(senderId, treeId string, heads []string) {
	s.Lock()
	defer s.Unlock()
	if len(heads) == 0 || !s.isSenderResponsible(senderId) {
		s.tempSynced[treeId] = struct{}{}
		return
	}

	s.updateDetails(treeId, domain.ObjectSyncStatusSynced)
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
	s.updateDetails(treeId, domain.ObjectSyncStatusSynced)

	if curTreeHeads, ok := s.treeHeads[treeId]; ok {
		// checking if we received the head that we are interested in
		for _, head := range heads {
			if idx, found := slices.BinarySearch(curTreeHeads.heads, head); found {
				curTreeHeads.heads = slice.RemoveIndex(curTreeHeads.heads, idx)
			}
		}
		if len(curTreeHeads.heads) == 0 {
			curTreeHeads.syncStatus = StatusSynced
		}
		s.treeHeads[treeId] = curTreeHeads
	}
}

func (s *syncStatusService) HeadsReceive(senderId, treeId string, heads []string) {
}

func (s *syncStatusService) addTreeHead(treeId string, heads []string, status SyncStatus) {
	headsCopy := slice.Copy(heads)
	slices.Sort(headsCopy)
	s.treeHeads[treeId] = treeHeadsEntry{
		heads:      headsCopy,
		syncStatus: status,
	}
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
			s.updateDetails(treeId, domain.ObjectSyncStatusSynced)
		}
	}
}

func (s *syncStatusService) Close(ctx context.Context) error {
	return nil
}

func (s *syncStatusService) isSenderResponsible(senderId string) bool {
	return slices.Contains(s.nodeConfService.NodeIds(s.spaceId), senderId)
}

func (s *syncStatusService) updateDetails(treeId string, status domain.ObjectSyncStatus) {
	if treeId != s.spaceSettingsId {
		s.syncDetailsUpdater.UpdateDetails(treeId, status, s.spaceId)
	}
}
