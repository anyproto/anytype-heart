package filesync

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/anytypeio/any-sync/commonspace/syncstatus"
	"go.uber.org/zap"
)

type StatusService interface {
	UpdateTree(ctx context.Context, objId string, status syncstatus.SyncStatus) (err error)
}

type fileWithSpace struct {
	fileID, spaceID string
}

type fileStatus struct {
	status    syncstatus.SyncStatus
	updatedAt time.Time
}

type StatusWatcher struct {
	filesToWatchLock *sync.Mutex
	filesToWatch     map[fileWithSpace]struct{}

	filesLock *sync.Mutex
	files     map[fileWithSpace]fileStatus
	updateCh  chan fileWithSpace

	updateInterval  time.Duration
	statusService   StatusService
	fileSyncService *fileSync
}

func (f *fileSync) NewStatusWatcher(statusService StatusService, updateInterval time.Duration) *StatusWatcher {
	return &StatusWatcher{
		filesLock:        &sync.Mutex{},
		files:            map[fileWithSpace]fileStatus{},
		filesToWatchLock: &sync.Mutex{},
		filesToWatch:     map[fileWithSpace]struct{}{},
		updateCh:         make(chan fileWithSpace),
		statusService:    statusService,
		fileSyncService:  f,
		updateInterval:   updateInterval,
	}
}

func (s *StatusWatcher) Run() {
	go s.run()
}

func (s *StatusWatcher) run() {
	ctx := context.Background()

	go func() {
		for key := range s.updateCh {
			if err := s.updateFileStatus(ctx, key); err != nil {
				log.Error("check file",
					zap.String("spaceID", key.spaceID),
					zap.String("fileID", key.fileID),
					zap.Error(err),
				)
			}
		}
	}()

	s.checkFiles(ctx)
	t := time.NewTicker(s.updateInterval)
	for range t.C {
		s.checkFiles(ctx)
	}
}

func (s *StatusWatcher) checkFiles(ctx context.Context) {
	s.filesToWatchLock.Lock()
	defer s.filesToWatchLock.Unlock()

	for key := range s.filesToWatch {
		s.updateCh <- key
	}
}

func (s *StatusWatcher) GetFileStatus(ctx context.Context, spaceID string, fileID string) (syncstatus.SyncStatus, error) {
	s.filesLock.Lock()
	defer s.filesLock.Unlock()

	key := fileWithSpace{
		spaceID: spaceID,
		fileID:  fileID,
	}
	status, err := s.getFileStatus(ctx, key)
	s.files[key] = status

	return status.status, err
}

func (s *StatusWatcher) updateFileStatus(ctx context.Context, key fileWithSpace) error {
	s.filesLock.Lock()
	defer s.filesLock.Unlock()

	status, err := s.getFileStatus(ctx, key)
	if err != nil {
		return fmt.Errorf("get file status: %w", err)
	}
	s.files[key] = status
	return s.statusService.UpdateTree(context.Background(), key.fileID, status.status)
}

func (s *StatusWatcher) getFileStatus(ctx context.Context, key fileWithSpace) (fileStatus, error) {
	now := time.Now()
	status, ok := s.files[key]
	if !ok {
		status = fileStatus{
			status: syncstatus.StatusNotSynced,
		}
	}

	if status.status == syncstatus.StatusSynced {
		return status, nil
	}

	if time.Since(status.updatedAt) < s.updateInterval {
		return status, nil
	}
	status.updatedAt = now

	isUploading, err := s.fileSyncService.queue.HasUpload(key.spaceID, key.fileID)
	if err != nil {
		return status, fmt.Errorf("check queue: %w", err)
	}
	if isUploading {
		status.status = syncstatus.StatusNotSynced
		return status, nil
	}

	fstat, err := s.fileSyncService.FileStat(ctx, key.spaceID, key.fileID)
	if err != nil {
		return status, fmt.Errorf("file stat: %w", err)
	}
	if fstat.UploadedChunksCount == fstat.TotalChunksCount {
		status.status = syncstatus.StatusSynced
	}

	return status, nil
}

func (s *StatusWatcher) Watch(spaceID, fileID string) {
	s.filesToWatchLock.Lock()
	defer s.filesToWatchLock.Unlock()

	key := fileWithSpace{spaceID: spaceID, fileID: fileID}
	if _, ok := s.filesToWatch[key]; !ok {
		s.filesToWatch[key] = struct{}{}
	}

	s.updateCh <- key
}

func (s *StatusWatcher) Unwatch(spaceID, fileID string) {
	s.filesToWatchLock.Lock()
	defer s.filesToWatchLock.Unlock()
	delete(s.filesToWatch, fileWithSpace{spaceID: spaceID, fileID: fileID})
}
