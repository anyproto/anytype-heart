package spacesyncstatus

import (
	"sync"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/syncstatus/filesyncstatus"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type FileState struct {
	fileSyncCountBySpace map[string]int
	countMu              sync.RWMutex

	fileSyncStatusBySpace map[string]domain.SpaceSyncStatus
	filesErrorBySpace     map[string]domain.SyncError
	statusMu              sync.RWMutex

	store objectstore.ObjectStore
}

func NewFileState(store objectstore.ObjectStore) *FileState {
	return &FileState{
		fileSyncCountBySpace:  make(map[string]int, 0),
		fileSyncStatusBySpace: make(map[string]domain.SpaceSyncStatus, 0),
		filesErrorBySpace:     make(map[string]domain.SyncError, 0),

		store: store,
	}
}

func (f *FileState) SetObjectsNumber(status *domain.SpaceSync) {
	f.countMu.Lock()
	defer f.countMu.Unlock()
	records, err := f.store.Query(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyFileBackupStatus.String(),
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       pbtypes.IntList(int(filesyncstatus.Syncing), int(filesyncstatus.Queued)),
			},
			{
				RelationKey: bundle.RelationKeySpaceId.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(status.SpaceId),
			},
		},
	})
	if err != nil {
		log.Errorf("failed to query file status: %s", err)
	}
	f.fileSyncCountBySpace[status.SpaceId] = len(records)
}

func (f *FileState) SetSyncStatusAndErr(status *domain.SpaceSync) {
	f.statusMu.RLock()
	defer f.statusMu.RUnlock()
	switch status.Status {
	case domain.Synced:
		f.fileSyncStatusBySpace[status.SpaceId] = domain.Synced
		f.setError(status.SpaceId, domain.Null)
		if number := f.fileSyncCountBySpace[status.SpaceId]; number > 0 {
			f.fileSyncStatusBySpace[status.SpaceId] = domain.Syncing
			return
		}
		if fileLimitedCount := f.getFileLimitedCount(status.SpaceId); fileLimitedCount > 0 {
			f.fileSyncStatusBySpace[status.SpaceId] = domain.Error
			f.setError(status.SpaceId, domain.StorageLimitExceed)
			return
		}
	case domain.Error, domain.Syncing, domain.Offline:
		f.fileSyncStatusBySpace[status.SpaceId] = status.Status
		f.setError(status.SpaceId, status.SyncError)
	}
}

func (f *FileState) setError(spaceId string, syncErr domain.SyncError) {
	f.filesErrorBySpace[spaceId] = syncErr
}

func (f *FileState) GetSyncStatus(spaceId string) domain.SpaceSyncStatus {
	f.statusMu.RLock()
	defer f.statusMu.RUnlock()
	return f.fileSyncStatusBySpace[spaceId]
}

func (f *FileState) GetSyncObjectCount(spaceId string) int {
	f.countMu.RLock()
	defer f.countMu.RUnlock()
	return f.fileSyncCountBySpace[spaceId]
}

func (f *FileState) GetSyncErr(spaceId string) domain.SyncError {
	f.statusMu.RLock()
	defer f.statusMu.RUnlock()
	return f.filesErrorBySpace[spaceId]
}

func (f *FileState) getFileLimitedCount(spaceId string) int {
	records, err := f.store.Query(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyFileBackupStatus.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Int64(int64(filesyncstatus.Limited)),
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
	return len(records)
}
