package syncstatus

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/anyproto/any-sync/commonspace/syncstatus"

	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/getblock"
	"github.com/anyproto/anytype-heart/core/filestorage/filesync"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/filestore"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type fileStatusRegistry struct {
	fileSyncService filesync.FileSync
	fileStore       filestore.FileStore
	picker          getblock.Picker

	sync.Mutex

	files          map[fileWithSpace]fileStatus
	updateInterval time.Duration
}

func newFileStatusRegistry(
	fileSyncService filesync.FileSync,
	fileStore filestore.FileStore,
	picker getblock.Picker,
	updateInterval time.Duration,
) *fileStatusRegistry {
	return &fileStatusRegistry{
		picker:          picker,
		fileSyncService: fileSyncService,
		fileStore:       fileStore,
		files:           map[fileWithSpace]fileStatus{},
		updateInterval:  updateInterval,
	}
}

func (r *fileStatusRegistry) GetFileStatus(ctx context.Context, spaceID string, fileID string) (syncstatus.SyncStatus, error) {
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

func (r *fileStatusRegistry) setFileStatus(key fileWithSpace, status fileStatus) (syncstatus.SyncStatus, error) {
	r.Lock()
	defer r.Unlock()

	prevStatus := r.files[key]
	if validStatusTransition(prevStatus.status, status.status) {
		err := r.fileStore.SetSyncStatus(key.fileID, int(status.status))
		if err != nil {
			return syncstatus.StatusUnknown, fmt.Errorf("failed to set file sync status: %w", err)
		}
		r.files[key] = status
		go r.indexFileSyncStatus(key.fileID, status.status)
		return status.status, nil
	}
	return prevStatus.status, nil
}

func (r *fileStatusRegistry) getFileStatus(key fileWithSpace) (fileStatus, error) {
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

func (r *fileStatusRegistry) updateFileStatus(ctx context.Context, status fileStatus, key fileWithSpace) (fileStatus, error) {
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

func (r *fileStatusRegistry) indexFileSyncStatus(fileID string, status syncstatus.SyncStatus) {
	err := getblock.Do(r.picker, fileID, func(b basic.DetailsSettable) (err error) {
		return b.SetDetails(nil, []*pb.RpcObjectSetDetailsDetail{
			{
				Key:   bundle.RelationKeyFileSyncStatus.String(),
				Value: pbtypes.Float64(float64(status)),
			},
		}, true)
	})
	if err != nil {
		log.With("fileID", fileID, "status", status).Errorf("failed to index file sync status: %v", err)
	}
}
