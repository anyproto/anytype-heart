package spacesyncstatus

import (
	"github.com/anyproto/anytype-heart/core/domain"
)

type ObjectState struct {
	objectSyncStatusBySpace map[string]domain.SyncStatus
	objectSyncCountBySpace  map[string]int
}

func NewObjectState() *ObjectState {
	return &ObjectState{
		objectSyncCountBySpace:  make(map[string]int, 0),
		objectSyncStatusBySpace: make(map[string]domain.SyncStatus, 0),
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

func (o *ObjectState) SetSyncStatus(status *domain.SpaceSync) {
	o.objectSyncStatusBySpace[status.SpaceId] = status.Status
}

func (o *ObjectState) GetSyncStatus(spaceId string) domain.SyncStatus {
	return o.objectSyncStatusBySpace[spaceId]
}

func (o *ObjectState) GetSyncObjectCount(spaceId string) int {
	return o.objectSyncCountBySpace[spaceId]
}

func (o *ObjectState) IsSyncFinished(spaceId string) bool {
	if _, ok := o.objectSyncStatusBySpace[spaceId]; !ok {
		return false
	}
	status := o.objectSyncStatusBySpace[spaceId]
	count := o.objectSyncCountBySpace[spaceId]
	return count == 0 && status == domain.Synced
}
