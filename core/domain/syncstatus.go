package domain

type SpaceSyncStatus int32

const (
	SpaceSyncStatusSynced  SpaceSyncStatus = 0
	SpaceSyncStatusSyncing SpaceSyncStatus = 1
	SpaceSyncStatusError   SpaceSyncStatus = 2
	SpaceSyncStatusOffline SpaceSyncStatus = 3
	SpaceSyncStatusUnknown SpaceSyncStatus = 4
)

type ObjectSyncStatus int32

const (
	ObjectSyncStatusSynced  ObjectSyncStatus = 0
	ObjectSyncStatusSyncing ObjectSyncStatus = 1
	ObjectSyncStatusError   ObjectSyncStatus = 2
	ObjectSyncStatusQueued  ObjectSyncStatus = 3
)

type SyncError int32

const (
	SyncErrorNull                SyncError = 0
	SyncErrorIncompatibleVersion SyncError = 2
	SyncErrorNetworkError        SyncError = 3
	SyncErrorOversized           SyncError = 4
)
