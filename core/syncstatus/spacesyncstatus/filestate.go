package spacesyncstatus

import (
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
	filesErrorBySpace     map[string]domain.SpaceSyncError

	store objectstore.ObjectStore
}

func NewFileState(store objectstore.ObjectStore) *FileState {
	return &FileState{
		fileSyncCountBySpace:  make(map[string]int, 0),
		fileSyncStatusBySpace: make(map[string]domain.SyncStatus, 0),
		filesErrorBySpace:     make(map[string]domain.SpaceSyncError, 0),

		store: store,
	}
}

func (f *FileState) SetObjectsNumber(status *domain.SpaceSync) {
	records, err := f.store.Query(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyFileBackupStatus.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Int64(int64(filesyncstatus.Syncing)),
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
	switch status.Status {
	case domain.Synced:
		f.fileSyncStatusBySpace[status.SpaceId] = domain.Synced
		if number := f.fileSyncCountBySpace[status.SpaceId]; number > 0 {
			f.fileSyncStatusBySpace[status.SpaceId] = domain.Syncing
			f.setError(status.SpaceId, status.SyncError)
			return
		}
		if fileLimitedCount := f.getFileLimitedCount(status.SpaceId); fileLimitedCount > 0 {
			f.fileSyncStatusBySpace[status.SpaceId] = domain.Error
			f.setError(status.SpaceId, domain.StorageLimitExceed)
		}
	case domain.Error, domain.Syncing, domain.Offline:
		f.fileSyncStatusBySpace[status.SpaceId] = status.Status
		f.setError(status.SpaceId, status.SyncError)
	}
}

func (f *FileState) setError(spaceId string, syncErr domain.SpaceSyncError) {
	f.filesErrorBySpace[spaceId] = syncErr
}

func (f *FileState) GetSyncStatus(spaceId string) domain.SyncStatus {
	return f.fileSyncStatusBySpace[spaceId]
}

func (f *FileState) GetSyncObjectCount(spaceId string) int {
	return f.fileSyncCountBySpace[spaceId]
}

func (f *FileState) GetSyncErr(spaceId string) domain.SpaceSyncError {
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
