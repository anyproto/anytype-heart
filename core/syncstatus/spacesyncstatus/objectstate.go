package spacesyncstatus

import "github.com/anyproto/any-sync/commonspace/syncstatus"

type ObjectState struct {
	objectSyncStatusBySpace map[string]syncstatus.SpaceSyncStatus
	objectSyncCountBySpace  map[string]int
}

func NewObjectState() *ObjectState {
	return &ObjectState{
		objectSyncCountBySpace:  make(map[string]int, 0),
		objectSyncStatusBySpace: make(map[string]syncstatus.SpaceSyncStatus, 0),
	}
}

func (o *ObjectState) SetObjectsNumber(status *syncstatus.SpaceSync) {
	switch status.Status {
	case syncstatus.Synced, syncstatus.Error, syncstatus.Offline:
		o.objectSyncCountBySpace[status.SpaceId] = 0
	case syncstatus.Syncing:
		o.objectSyncCountBySpace[status.SpaceId] = status.ObjectsNumber
	}
}

func (o *ObjectState) SetSyncStatus(status *syncstatus.SpaceSync) {
	o.objectSyncStatusBySpace[status.SpaceId] = status.Status
}

func (o *ObjectState) GetSyncStatus(spaceId string) syncstatus.SpaceSyncStatus {
	return o.objectSyncStatusBySpace[spaceId]
}

func (o *ObjectState) GetSyncObjectCount(spaceId string) int {
	return o.objectSyncCountBySpace[spaceId]
}

func (o *ObjectState) IsSyncFinished(spaceId string) bool {
	status := o.objectSyncStatusBySpace[spaceId]
	count := o.objectSyncCountBySpace[spaceId]
	return count == 0 && status == syncstatus.Synced
}
