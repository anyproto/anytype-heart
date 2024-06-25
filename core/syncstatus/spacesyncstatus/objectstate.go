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
	objectSyncCountBySpace  map[string]int
	sync.Mutex

	store objectstore.ObjectStore
}

func NewObjectState(store objectstore.ObjectStore) *ObjectState {
	return &ObjectState{
		objectSyncCountBySpace:  make(map[string]int, 0),
		objectSyncStatusBySpace: make(map[string]domain.SpaceSyncStatus, 0),
		store:                   store,
	}
}

func (o *ObjectState) SetObjectsNumber(status *domain.SpaceSync) {
	o.Lock()
	defer o.Unlock()
	switch status.Status {
	case domain.Error, domain.Offline, domain.Synced:
		o.objectSyncCountBySpace[status.SpaceId] = 0
	case domain.Syncing:
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

func (o *ObjectState) SetSyncStatus(status domain.SpaceSyncStatus, spaceId string) {
	o.objectSyncStatusBySpace[spaceId] = status
}

func (o *ObjectState) GetSyncStatus(spaceId string) domain.SpaceSyncStatus {
	o.Lock()
	defer o.Unlock()
	return o.objectSyncStatusBySpace[spaceId]
}

func (o *ObjectState) GetSyncObjectCount(spaceId string) int {
	o.Lock()
	defer o.Unlock()
	return o.objectSyncCountBySpace[spaceId]
}

func (o *ObjectState) ResetSpaceErrorStatus(string, domain.SyncError) {}
