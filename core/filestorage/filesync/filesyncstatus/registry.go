package filesyncstatus

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/anytypeio/any-sync/commonspace/syncstatus"

	"github.com/anytypeio/go-anytype-middleware/core/filestorage/filesync"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/filestore"
)

type Registry interface {
	GetFileStatus(ctx context.Context, spaceID string, fileID string) (syncstatus.SyncStatus, error)
}

type registry struct {
	fileSyncService   filesync.FileSync
	fileStore         filestore.FileStore
	syncStatusIndexer SyncStatusIndexer

	sync.Mutex

	files          map[fileWithSpace]fileStatus
	updateInterval time.Duration
}

func NewRegistry(
	fileSyncService filesync.FileSync,
	fileStore filestore.FileStore,
	syncStatusIndexer SyncStatusIndexer,
	updateInterval time.Duration,
) Registry {
	return &registry{
		fileSyncService:   fileSyncService,
		fileStore:         fileStore,
		syncStatusIndexer: syncStatusIndexer,
		files:             map[fileWithSpace]fileStatus{},
		updateInterval:    updateInterval,
	}
}

func (r *registry) GetFileStatus(ctx context.Context, spaceID string, fileID string) (syncstatus.SyncStatus, error) {
	key := fileWithSpace{
		spaceID: spaceID,
		fileID:  fileID,
	}

	status, err := r.getFileStatus(key)
	if err != nil {
		return status.status, err
	}

	status, err = r.updateFileStatus(ctx, status, key)
	if err != nil {
		return syncstatus.StatusUnknown, err
	}

	return r.setFileStatus(key, status)
}

func (r *registry) setFileStatus(key fileWithSpace, status fileStatus) (syncstatus.SyncStatus, error) {
	r.Lock()
	defer r.Unlock()

	prevStatus := r.files[key]
	if validStatusTransition(prevStatus.status, status.status) {
		err := r.fileStore.SetSyncStatus(key.fileID, int(status.status))
		if err != nil {
			return syncstatus.StatusUnknown, fmt.Errorf("failed to set file sync status: %w", err)
		}
		r.files[key] = status
		go r.syncStatusIndexer.Index(key.fileID, status.status)
		return status.status, nil
	}
	return prevStatus.status, nil
}

func (r *registry) getFileStatus(key fileWithSpace) (fileStatus, error) {
	r.Lock()
	defer r.Unlock()
	status, ok := r.files[key]
	if !ok {
		rawStatus, err := r.fileStore.GetSyncStatus(key.fileID)
		if err != nil && err != localstore.ErrNotFound {
			return fileStatus{status: syncstatus.StatusUnknown}, fmt.Errorf("failed to get file sync status: %w", err)
		}
		status = fileStatus{
			status: syncstatus.SyncStatus(rawStatus),
		}
	}
	return status, nil
}

func validStatusTransition(from, to syncstatus.SyncStatus) bool {
	switch from {
	case syncstatus.StatusUnknown:
		return to == syncstatus.StatusNotSynced || to == syncstatus.StatusSynced
	case syncstatus.StatusNotSynced:
		return to == syncstatus.StatusSynced
	default:
		return false
	}
}

func (r *registry) updateFileStatus(ctx context.Context, status fileStatus, key fileWithSpace) (fileStatus, error) {
	now := time.Now()
	if status.status == syncstatus.StatusSynced {
		return status, nil
	}

	if time.Since(status.updatedAt) < r.updateInterval {
		return status, nil
	}
	status.updatedAt = now

	isUploading, err := r.fileSyncService.HasUpload(key.spaceID, key.fileID)
	if err != nil {
		return status, fmt.Errorf("check queue: %w", err)
	}
	if isUploading {
		status.status = syncstatus.StatusNotSynced
		return status, nil
	}

	fstat, err := r.fileSyncService.FileStat(ctx, key.spaceID, key.fileID)
	if err != nil {
		return status, fmt.Errorf("file stat: %w", err)
	}
	if fstat.UploadedChunksCount == fstat.TotalChunksCount {
		status.status = syncstatus.StatusSynced
	}

	return status, nil
}
