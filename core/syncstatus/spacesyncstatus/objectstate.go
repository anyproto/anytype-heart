package spacesyncstatus

import (
	"fmt"
	"sync"

	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
)

type ObjectState struct {
	objectSyncStatusBySpace map[string]domain.SpaceSyncStatus
	objectSyncCountBySpace  map[string]int
	objectSyncErrBySpace    map[string]domain.SyncError
	sync.Mutex

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
	o.Lock()
	defer o.Unlock()
	switch status.Status {
	case domain.Error, domain.Offline:
		o.objectSyncCountBySpace[status.SpaceId] = 0
	default:
		records := o.getSyncingObjects(status)
		ids := lo.Map(records, func(r database.Record, idx int) string {
			return pbtypes.GetString(r.Details, bundle.RelationKeyId.String())
		})
		_, added := slice.DifferenceRemovedAdded(ids, status.MissingObjects)
		o.objectSyncCountBySpace[status.SpaceId] = len(records) + len(added)
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

func (o *ObjectState) SetSyncStatusAndErr(status domain.SpaceSyncStatus, syncErr domain.SyncError, spaceId string) {
	o.Lock()
	defer o.Unlock()
	if objectNumber, ok := o.objectSyncCountBySpace[spaceId]; ok && objectNumber > 0 {
		o.objectSyncStatusBySpace[spaceId] = domain.Syncing
		o.objectSyncErrBySpace[spaceId] = domain.Null
		return
	} else if ok && objectNumber == 0 && status == domain.Syncing {
		o.objectSyncStatusBySpace[spaceId] = domain.Synced
		o.objectSyncErrBySpace[spaceId] = domain.Null
		return
	}
	o.objectSyncStatusBySpace[spaceId] = status
	o.objectSyncErrBySpace[spaceId] = syncErr
}

func (o *ObjectState) GetSyncStatus(spaceId string) domain.SpaceSyncStatus {
	o.Lock()
	defer o.Unlock()
	if status, ok := o.objectSyncStatusBySpace[spaceId]; ok {
		return status
	}
	return domain.Unknown
}

func (o *ObjectState) GetSyncObjectCount(spaceId string) int {
	o.Lock()
	defer o.Unlock()
	return o.objectSyncCountBySpace[spaceId]
}

func (o *ObjectState) GetSyncErr(spaceId string) domain.SyncError {
	o.Lock()
	defer o.Unlock()
	return o.objectSyncErrBySpace[spaceId]
}

func (o *ObjectState) ResetSpaceErrorStatus(string, domain.SyncError) {}
