package domain

type SyncType int32

const (
	Objects SyncType = 0
	Files   SyncType = 1
)

type SpaceSyncStatus int32

const (
	Synced  SpaceSyncStatus = 0
	Syncing SpaceSyncStatus = 1
	Error   SpaceSyncStatus = 2
	Offline SpaceSyncStatus = 3
)

type ObjectSyncStatus int32

const (
	ObjectSynced  ObjectSyncStatus = 0
	ObjectSyncing ObjectSyncStatus = 1
	ObjectError   ObjectSyncStatus = 2
	ObjectQueued  ObjectSyncStatus = 3
)

type SyncError int32

const (
	Null                SyncError = 0
	StorageLimitExceed  SyncError = 1
	IncompatibleVersion SyncError = 2
	NetworkError        SyncError = 3
)

type SpaceSync struct {
	SpaceId   string
	Status    SpaceSyncStatus
	SyncError SyncError
	SyncType  SyncType
}

func MakeSyncStatus(spaceId string, status SpaceSyncStatus, syncError SyncError, syncType SyncType) *SpaceSync {
	return &SpaceSync{
		SpaceId:   spaceId,
		Status:    status,
		SyncError: syncError,
		SyncType:  syncType,
	}
}
