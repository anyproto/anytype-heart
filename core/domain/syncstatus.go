package domain

import "github.com/anyproto/anytype-heart/pkg/lib/pb/model"

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
	ObjectSyncStatusSynced  = ObjectSyncStatus(model.SyncStatus_SyncStatusSynced)
	ObjectSyncStatusSyncing = ObjectSyncStatus(model.SyncStatus_SyncStatusSyncing)
	ObjectSyncStatusError   = ObjectSyncStatus(model.SyncStatus_SyncStatusError)
	ObjectSyncStatusQueued  = ObjectSyncStatus(model.SyncStatus_SyncStatusQueued)
)

type SyncError int32

const (
	SyncErrorNull                = SyncError(model.SyncError_SyncErrorNull)
	SyncErrorIncompatibleVersion = SyncError(model.SyncError_SyncErrorIncompatibleVersion)
	SyncErrorNetworkError        = SyncError(model.SyncError_SyncErrorNetworkError)
	SyncErrorOversized           = SyncError(model.SyncError_SyncErrorOversized)
)
