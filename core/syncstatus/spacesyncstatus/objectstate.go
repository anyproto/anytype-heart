package spacesyncstatus

import (
	"sync"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type ObjectState struct {
	objectSyncStatusBySpace map[string]domain.SpaceSyncStatus
	statusMu                sync.RWMutex

	objectSyncCountBySpace map[string]int
	objectSyncErrBySpace   map[string]domain.SyncError
	countMu                sync.RWMutex

	store objectstore.ObjectStore
}

func NewObjectState(store objectstore.ObjectStore) *ObjectState {
	return &ObjectState{
		objectSyncCountBySpace:  make(map[string]int, 0),
		objectSyncStatusBySpace: make(map[string]domain.SpaceSyncStatus, 0),
		objectSyncErrBySpace:    make(map[string]domain.SyncError, 0),
		store:                   store,
	}
}

func (o *ObjectState) SetObjectsNumber(status *domain.SpaceSync) {
	o.countMu.Lock()
	defer o.countMu.Unlock()
	switch status.Status {
	case domain.Error, domain.Offline:
		o.objectSyncCountBySpace[status.SpaceId] = 0
	case domain.Syncing, domain.Synced:
		records := o.getSyncingObjects(status)
		o.objectSyncCountBySpace[status.SpaceId] = len(records)
	}
}

func (o *ObjectState) getSyncingObjects(status *domain.SpaceSync) []database.Record {
	records, err := o.store.Query(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeySyncStatus.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Int64(int64(domain.Syncing)),
			},
			{
				RelationKey: bundle.RelationKeyLayout.String(),
				Condition:   model.BlockContentDataviewFilter_NotIn,
				Value: pbtypes.IntList(
					int(model.ObjectType_file),
					int(model.ObjectType_image),
					int(model.ObjectType_video),
					int(model.ObjectType_audio),
				),
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
	return records
}

func (o *ObjectState) SetSyncStatusAndErr(status *domain.SpaceSync) {
	o.statusMu.Lock()
	defer o.statusMu.Unlock()

	if objectNumber, ok := o.objectSyncCountBySpace[status.SpaceId]; ok && objectNumber > 0 {
		o.objectSyncStatusBySpace[status.SpaceId] = domain.Syncing
		o.objectSyncErrBySpace[status.SpaceId] = domain.Null
		return
	}
	o.objectSyncStatusBySpace[status.SpaceId] = status.Status
	o.objectSyncErrBySpace[status.SpaceId] = status.SyncError
}

func (o *ObjectState) GetSyncStatus(spaceId string) domain.SpaceSyncStatus {
	o.statusMu.RLock()
	defer o.statusMu.RUnlock()
	return o.objectSyncStatusBySpace[spaceId]
}

func (o *ObjectState) GetSyncObjectCount(spaceId string) int {
	o.countMu.RLock()
	defer o.countMu.RUnlock()
	return o.objectSyncCountBySpace[spaceId]
}

func (o *ObjectState) GetSyncErr(spaceId string) domain.SyncError {
	o.statusMu.RLock()
	defer o.statusMu.RUnlock()
	return o.objectSyncErrBySpace[spaceId]
}
