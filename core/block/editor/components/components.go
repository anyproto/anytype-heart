package components

import "github.com/anyproto/anytype-heart/core/domain"

type SyncStatusHandler interface {
	HandleSyncStatusUpdate(heads []string, status domain.ObjectSyncStatus, syncError domain.SyncError)
}
