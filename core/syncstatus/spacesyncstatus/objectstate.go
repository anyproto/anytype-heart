package spacesyncstatus

import "github.com/anyproto/anytype-heart/core/syncstatus/helpers"

type ObjectState struct {
	objectSyncStatusBySpace map[string]helpers.SpaceSyncStatus
	objectSyncCountBySpace  map[string]int
}

func NewObjectState() *ObjectState {
	return &ObjectState{
		objectSyncCountBySpace:  make(map[string]int, 0),
		objectSyncStatusBySpace: make(map[string]helpers.SpaceSyncStatus, 0),
	}
}

func (o *ObjectState) SetObjectsNumber(status *helpers.SpaceSync) {
	switch status.Status {
	case helpers.Synced, helpers.Error, helpers.Offline:
		o.objectSyncCountBySpace[status.SpaceId] = 0
	case helpers.Syncing:
		o.objectSyncCountBySpace[status.SpaceId] = status.ObjectsNumber
	}
}

func (o *ObjectState) SetSyncStatus(status *helpers.SpaceSync) {
	o.objectSyncStatusBySpace[status.SpaceId] = status.Status
}

func (o *ObjectState) GetSyncStatus(spaceId string) helpers.SpaceSyncStatus {
	return o.objectSyncStatusBySpace[spaceId]
}

func (o *ObjectState) GetSyncObjectCount(spaceId string) int {
	return o.objectSyncCountBySpace[spaceId]
}

func (o *ObjectState) IsSyncFinished(spaceId string) bool {
	status := o.objectSyncStatusBySpace[spaceId]
	count := o.objectSyncCountBySpace[spaceId]
	return count == 0 && status == helpers.Synced
}
