package domain

type SyncType int32

const (
	Objects SyncType = 0
	Files   SyncType = 1
)

type SyncStatus int32

const (
	Synced  SyncStatus = 0
	Syncing SyncStatus = 1
	Error   SyncStatus = 2
	Offline SyncStatus = 3
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
	Status    SyncStatus
	SyncError SyncError
	SyncType  SyncType
}

func MakeSyncStatus(spaceId string, status SyncStatus, syncError SyncError, syncType SyncType) *SpaceSync {
	return &SpaceSync{
		SpaceId:   spaceId,
		Status:    status,
		SyncError: syncError,
		SyncType:  syncType,
	}
}
