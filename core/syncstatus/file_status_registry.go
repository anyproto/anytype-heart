package syncstatus

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/getblock"
	"github.com/anyproto/anytype-heart/core/event"
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
	picker          getblock.Picker
	eventSender     event.Sender

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

func (r *fileStatusRegistry) hasFileInStore(fileID string) (bool, error) {
	roots, err := r.fileStore.ListByTarget(fileID)
	if err != localstore.ErrNotFound && err != nil {
		return false, err
	}
	return len(roots) > 0, nil
}

func (r *fileStatusRegistry) GetFileStatus(ctx context.Context, spaceID string, fileID string) (FileStatus, error) {
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
		return FileStatusUnknown, err
	}

	return r.setFileStatus(key, status)
}

func (r *fileStatusRegistry) setFileStatus(key fileWithSpace, status fileStatus) (FileStatus, error) {
	r.Lock()
	defer r.Unlock()

	prevStatus := r.files[key]
	if validStatusTransition(prevStatus.status, status.status) {
		err := r.fileStore.SetSyncStatus(key.fileID, int(status.status))
		if err != nil {
			return FileStatusUnknown, fmt.Errorf("failed to set file sync status: %w", err)
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

var errFileNotFound = fmt.Errorf("file is not found")

func (r *fileStatusRegistry) updateFileStatus(ctx context.Context, status fileStatus, key fileWithSpace) (fileStatus, error) {
	now := time.Now()
	if status.status == FileStatusSynced {
		return status, nil
	}

	if time.Since(status.updatedAt) < r.updateInterval {
		return status, nil
	}
	status.updatedAt = now

	isLimited, err := r.fileSyncService.IsFileUploadLimited(key.spaceID, key.fileID)
	if err != nil {
		return status, fmt.Errorf("check that file upload is limited: %w", err)
	}
	if isLimited {
		status.status = FileStatusLimited
		return status, nil
	}

	isUploading, err := r.fileSyncService.HasUpload(key.spaceID, key.fileID)
	if err != nil {
		return status, fmt.Errorf("check queue: %w", err)
	}
	if isUploading {
		status.status = FileStatusSyncing
		return status, nil
	}

	ok, err := r.hasFileInStore(key.fileID)
	if err != nil {
		return status, fmt.Errorf("check that file is in store: %w", err)
	}
	if !ok {
		return status, errFileNotFound
	}
	fstat, err := r.fileSyncService.FileStat(ctx, key.spaceID, key.fileID)
	if err != nil {
		return status, fmt.Errorf("file stat: %w", err)
	}
	if fstat.UploadedChunksCount == fstat.TotalChunksCount {
		status.status = FileStatusSynced
	}

	return status, nil
}

func (r *fileStatusRegistry) indexFileSyncStatus(fileID string, status FileStatus) {
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
