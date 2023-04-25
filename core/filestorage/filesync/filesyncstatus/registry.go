package filesyncstatus

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/anytypeio/any-sync/commonspace/syncstatus"

	"github.com/anytypeio/go-anytype-middleware/core/filestorage/filesync"
)

type Registry interface {
	GetFileStatus(ctx context.Context, spaceID string, fileID string) (syncstatus.SyncStatus, error)
}

type registry struct {
	fileSyncService filesync.FileSync

	sync.Mutex

	files          map[fileWithSpace]fileStatus
	updateInterval time.Duration
}

func NewRegistry(fileSyncService filesync.FileSync, updateInterval time.Duration) Registry {
	return &registry{
		fileSyncService: fileSyncService,
		files:           map[fileWithSpace]fileStatus{},
		updateInterval:  updateInterval,
	}
}

func (r *registry) GetFileStatus(ctx context.Context, spaceID string, fileID string) (syncstatus.SyncStatus, error) {
	r.Lock()
	defer r.Unlock()

	key := fileWithSpace{
		spaceID: spaceID,
		fileID:  fileID,
	}
	status, err := r.getFileStatus(ctx, key)
	r.files[key] = status

	return status.status, err
}

func (r *registry) getFileStatus(ctx context.Context, key fileWithSpace) (fileStatus, error) {
	now := time.Now()
	status, ok := r.files[key]
	if !ok {
		status = fileStatus{
			status: syncstatus.StatusNotSynced,
		}
	}

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
