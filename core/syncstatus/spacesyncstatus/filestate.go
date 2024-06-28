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
	fileSyncCountBySpace  map[string]int
	fileSyncStatusBySpace map[string]domain.SpaceSyncStatus
	filesErrorBySpace     map[string]domain.SyncError
	sync.Mutex

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
	f.Lock()
	defer f.Unlock()
	switch status.Status {
	case domain.Error, domain.Offline:
		f.fileSyncCountBySpace[status.SpaceId] = 0
	default:
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
}

func (f *FileState) SetSyncStatusAndErr(status domain.SpaceSyncStatus, syncErr domain.SyncError, spaceId string) {
	f.Lock()
	defer f.Unlock()
	switch status {
	case domain.Synced:
		f.fileSyncStatusBySpace[spaceId] = domain.Synced
		f.filesErrorBySpace[spaceId] = syncErr
		if number := f.fileSyncCountBySpace[spaceId]; number > 0 {
			f.fileSyncStatusBySpace[spaceId] = domain.Syncing
			return
		}
	case domain.Error, domain.Syncing, domain.Offline:
		f.fileSyncStatusBySpace[spaceId] = status
		f.filesErrorBySpace[spaceId] = syncErr
	}
}

func (f *FileState) GetSyncStatus(spaceId string) domain.SpaceSyncStatus {
	f.Lock()
	defer f.Unlock()
	return f.fileSyncStatusBySpace[spaceId]
}

func (f *FileState) GetSyncObjectCount(spaceId string) int {
	f.Lock()
	defer f.Unlock()
	return f.fileSyncCountBySpace[spaceId]
}

func (f *FileState) ResetSpaceErrorStatus(spaceId string, syncError domain.SyncError) {
	// show StorageLimitExceed only once
	if syncError == domain.StorageLimitExceed {
		f.SetSyncStatusAndErr(domain.Synced, domain.Null, spaceId)
	}
}

func (f *FileState) GetSyncErr(spaceId string) domain.SyncError {
	f.Lock()
	defer f.Unlock()
	return f.filesErrorBySpace[spaceId]
}
