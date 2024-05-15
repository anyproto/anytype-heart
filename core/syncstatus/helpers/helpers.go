package helpers

type SpaceSyncType int32

const (
	Objects SpaceSyncType = 0
	Files   SpaceSyncType = 1
)

type SpaceSyncStatus int32

const (
	Synced  SpaceSyncStatus = 0
	Syncing SpaceSyncStatus = 1
	Error   SpaceSyncStatus = 2
	Offline SpaceSyncStatus = 3
)

type SpaceSyncError int32

const (
	Null                SpaceSyncError = 0
	StorageLimitExceed  SpaceSyncError = 1
	IncompatibleVersion SpaceSyncError = 2
	NetworkError        SpaceSyncError = 3
)

type SpaceSync struct {
	SpaceId       string
	Status        SpaceSyncStatus
	ObjectsNumber int
	SyncError     SpaceSyncError
	SyncType      SpaceSyncType
}

func MakeSyncStatus(spaceId string, status SpaceSyncStatus, objectsNumber int, syncError SpaceSyncError, syncType SpaceSyncType) *SpaceSync {
	return &SpaceSync{
		SpaceId:       spaceId,
		Status:        status,
		ObjectsNumber: objectsNumber,
		SyncError:     syncError,
		SyncType:      syncType,
	}
}
