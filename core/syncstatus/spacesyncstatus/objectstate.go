package spacesyncstatus

import (
	"github.com/anyproto/anytype-heart/core/domain"
)

type ObjectState struct {
	objectSyncStatusBySpace map[string]domain.SyncStatus
	objectSyncCountBySpace  map[string]int
	objectSyncErrBySpace    map[string]domain.SyncError
}

func NewObjectState() *ObjectState {
	return &ObjectState{
		objectSyncCountBySpace:  make(map[string]int, 0),
		objectSyncStatusBySpace: make(map[string]domain.SyncStatus, 0),
		objectSyncErrBySpace:    make(map[string]domain.SyncError, 0),
	}
}

func (o *ObjectState) SetObjectsNumber(status *domain.SpaceSync) {
	switch status.Status {
	case domain.Synced, domain.Error, domain.Offline:
		o.objectSyncCountBySpace[status.SpaceId] = 0
	case domain.Syncing:
		o.objectSyncCountBySpace[status.SpaceId] = status.ObjectsNumber
	}
}

func (o *ObjectState) SetSyncStatusAndErr(status *domain.SpaceSync) {
	o.objectSyncStatusBySpace[status.SpaceId] = status.Status
	o.objectSyncErrBySpace[status.SpaceId] = status.SyncError
}

func (o *ObjectState) GetSyncStatus(spaceId string) domain.SyncStatus {
	return o.objectSyncStatusBySpace[spaceId]
}

func (o *ObjectState) GetSyncObjectCount(spaceId string) int {
	return o.objectSyncCountBySpace[spaceId]
}

func (o *ObjectState) GetSyncErr(spaceId string) domain.SyncError {
	return o.objectSyncErrBySpace[spaceId]
}
