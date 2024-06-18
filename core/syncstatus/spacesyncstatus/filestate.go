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
	filesErrorBySpace     map[string]domain.SyncError

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
	switch status.Status {
	case domain.Synced:
		f.fileSyncStatusBySpace[status.SpaceId] = domain.Synced
		f.setError(status.SpaceId, domain.Null)
		if number := f.fileSyncCountBySpace[status.SpaceId]; number > 0 {
			f.fileSyncStatusBySpace[status.SpaceId] = domain.Syncing
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
	return f.fileSyncStatusBySpace[spaceId]
}

func (f *FileState) GetSyncObjectCount(spaceId string) int {
	return f.fileSyncCountBySpace[spaceId]
}

func (f *FileState) GetSyncErr(spaceId string) domain.SyncError {
	return f.filesErrorBySpace[spaceId]
}
