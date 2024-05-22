package spacesyncstatus

type ObjectState struct {
	objectSyncStatusBySpace map[string]helpers.SyncStatus
	objectSyncCountBySpace  map[string]int
}

func NewObjectState() *ObjectState {
	return &ObjectState{
		objectSyncCountBySpace:  make(map[string]int, 0),
		objectSyncStatusBySpace: make(map[string]helpers.SyncStatus, 0),
	}
}

func (o *ObjectState) SetObjectsNumber(status *SpaceSync) {
	switch status.Status {
	case Synced, Error, Offline:
		o.objectSyncCountBySpace[status.SpaceId] = 0
	case Syncing:
		o.objectSyncCountBySpace[status.SpaceId] = status.ObjectsNumber
	}
}

func (o *ObjectState) SetSyncStatus(status *SpaceSync) {
	o.objectSyncStatusBySpace[status.SpaceId] = status.Status
}

func (o *ObjectState) GetSyncStatus(spaceId string) helpers.SyncStatus {
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
	return count == 0 && status == Synced
}
