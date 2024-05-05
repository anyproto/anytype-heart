package filesyncstatus

import "github.com/anyproto/any-sync/commonspace/syncstatus"

type Status int

const (
	Unknown Status = iota
	// SyncedLegacy is not used anymore. We have to use another constant to fix bug with migration, where
	// we accidentally set FileBackupStatus to Synced for all files, even not synced
	SyncedLegacy
	Syncing
	Limited
	Synced
)

func (s Status) ToSyncStatus() syncstatus.SyncStatus {
	switch s {
	case Unknown, SyncedLegacy:
		return syncstatus.StatusUnknown
	case Synced:
		return syncstatus.StatusSynced
	case Syncing, Limited:
		return syncstatus.StatusNotSynced
	default:
		return syncstatus.StatusUnknown
	}
}
