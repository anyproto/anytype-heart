package filesyncstatus

import (
	"github.com/anyproto/anytype-heart/core/syncstatus/objectsyncstatus"
)

type Status int

const (
	Unknown Status = iota
	// SyncedLegacy is not used anymore. We have to use another constant to fix bug with migration, where
	// we accidentally set FileBackupStatus to Synced for all files, even not synced
	SyncedLegacy
	Syncing
	Limited
	Synced
	Queued
)

func (s Status) ToSyncStatus() objectsyncstatus.SyncStatus {
	switch s {
	case Unknown, SyncedLegacy:
		return objectsyncstatus.StatusUnknown
	case Synced:
		return objectsyncstatus.StatusSynced
	case Syncing, Limited, Queued:
		return objectsyncstatus.StatusNotSynced
	default:
		return objectsyncstatus.StatusUnknown
	}
}
