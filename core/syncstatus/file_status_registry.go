package syncstatus

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/getblock"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/filestorage/filesync"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/filestore"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type FileStatus int

// First constants must repeat syncstatus.SyncStatus constants for
// avoiding inconsistency with data stored in filestore
const (
	FileStatusUnknown FileStatus = iota
	FileStatusSynced
	FileStatusSyncing
	FileStatusLimited
)

type fileStatusRegistry struct {
	fileSyncService filesync.FileSync
	fileStore       filestore.FileStore
	picker          getblock.ObjectGetter

	sync.Mutex

	files          map[string]fileStatus
	updateInterval time.Duration
}

func newFileStatusRegistry(
	fileSyncService filesync.FileSync,
	fileStore filestore.FileStore,
	picker getblock.ObjectGetter,
	updateInterval time.Duration,
) *fileStatusRegistry {
	return &fileStatusRegistry{
		picker:          picker,
		fileSyncService: fileSyncService,
		fileStore:       fileStore,
		files:           map[string]fileStatus{},
		updateInterval:  updateInterval,
	}
}

func (r *fileStatusRegistry) hasFileInStore(fileID string) (bool, error) {
	roots, err := r.fileStore.ListByTarget(fileID)
	if err != localstore.ErrNotFound && err != nil {
		return false, err
	}
	return len(roots) > 0, nil
}

func (r *fileStatusRegistry) GetFileStatus(ctx context.Context, spaceId string, fileId string, fileHash string) (FileStatus, error) {
	entry := fileEntry{
		spaceId:  spaceId,
		fileHash: fileHash,
		fileId:   fileId,
	}

	status, err := r.getFileStatus(entry)
	if err != nil {
		return status.status, err
	}

	status, err = r.updateFileStatus(ctx, status, entry)
	if err != nil {
		return FileStatusUnknown, err
	}

	return r.setFileStatus(entry, status)
}

func (r *fileStatusRegistry) setFileStatus(entry fileEntry, status fileStatus) (FileStatus, error) {
	r.Lock()
	defer r.Unlock()

	prevStatus := r.files[entry.fileId]
	if validStatusTransition(prevStatus.status, status.status) {
		//err := r.fileStore.SetSyncStatus(entry.fileHash, int(status.status))
		//if err != nil {
		//	return FileStatusUnknown, fmt.Errorf("failed to set file sync status: %w", err)
		//}
		r.files[entry.fileId] = status
		go r.indexFileSyncStatus(entry.fileId, status.status)
		return status.status, nil
	}
	return prevStatus.status, nil
}

func (r *fileStatusRegistry) getFileStatus(entry fileEntry) (fileStatus, error) {
	r.Lock()
	defer r.Unlock()
	status, ok := r.files[entry.fileId]
	if !ok {
		var rawStatus int64
		err := getblock.Do(r.picker, entry.fileId, func(sb smartblock.SmartBlock) (err error) {
			rawStatus = pbtypes.GetInt64(sb.Details(), bundle.RelationKeyFileBackupStatus.String())
			return nil
		})
		if err != nil {
			return fileStatus{status: FileStatusUnknown}, fmt.Errorf("failed to get file sync status: %w", err)
		}
		status = fileStatus{
			status: FileStatus(rawStatus),
		}
	}
	return status, nil
}

func validStatusTransition(from, to FileStatus) bool {
	switch from {
	case FileStatusUnknown:
		// To any status expect itself
		return to != FileStatusUnknown
	case FileStatusSyncing:
		return to == FileStatusSynced || to == FileStatusLimited
	case FileStatusLimited:
		return to == FileStatusSynced || to == FileStatusSyncing
	default:
		return from == to
	}
}

func (r *fileStatusRegistry) updateFileStatus(ctx context.Context, status fileStatus, key fileEntry) (fileStatus, error) {
	now := time.Now()
	if status.status == FileStatusSynced {
		return status, nil
	}

	if time.Since(status.updatedAt) < r.updateInterval {
		return status, nil
	}
	status.updatedAt = now

	isLimited, err := r.fileSyncService.IsFileUploadLimited(key.spaceId, key.fileHash)
	if err != nil {
		return status, fmt.Errorf("check that file upload is limited: %w", err)
	}
	if isLimited {
		status.status = FileStatusLimited
		return status, nil
	}

	isUploading, err := r.fileSyncService.HasUpload(key.spaceId, key.fileHash)
	if err != nil {
		return status, fmt.Errorf("check queue: %w", err)
	}
	if isUploading {
		status.status = FileStatusSyncing
		return status, nil
	}

	ok, err := r.hasFileInStore(key.fileHash)
	if err != nil {
		return status, fmt.Errorf("check that file is in store: %w", err)
	}
	if !ok {
		return status, domain.ErrFileNotFound
	}
	fstat, err := r.fileSyncService.FileStat(ctx, key.spaceId, key.fileHash)
	if err != nil {
		return status, fmt.Errorf("file stat: %w", err)
	}
	if fstat.UploadedChunksCount == fstat.TotalChunksCount {
		status.status = FileStatusSynced
	}

	return status, nil
}

func (r *fileStatusRegistry) indexFileSyncStatus(fileId string, status FileStatus) {
	err := getblock.Do(r.picker, fileId, func(sb smartblock.SmartBlock) (err error) {
		prevStatus := pbtypes.GetFloat64(sb.Details(), bundle.RelationKeyFileBackupStatus.String())
		newStatus := float64(status)
		if prevStatus == newStatus {
			return nil
		}

		detailsSetter, ok := sb.(basic.DetailsSettable)
		if !ok {
			return fmt.Errorf("setting of details is not supported for %T", sb)
		}
		return detailsSetter.SetDetails(nil, []*pb.RpcObjectSetDetailsDetail{
			{
				Key:   bundle.RelationKeyFileBackupStatus.String(),
				Value: pbtypes.Float64(newStatus),
			},
		}, true)
	})
	if err != nil && !errors.Is(err, domain.ErrFileNotFound) {
		log.With("fileHash", fileId, "status", status).Errorf("failed to index file sync status: %v", err)
	}
}
