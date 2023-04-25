package syncstatus

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/anytypeio/any-sync/commonspace/syncstatus"
	"go.uber.org/zap"
)

type fileWithSpace struct {
	fileID, spaceID string
}

type fileStatus struct {
	status    syncstatus.SyncStatus
	updatedAt time.Time
}

type fileWatcher struct {
	filesToWatchLock *sync.Mutex
	filesToWatch     map[fileWithSpace]struct{}

	registry *fileStatusRegistry
	updateCh chan fileWithSpace
	closeCh  chan struct{}

	updateReceiver syncstatus.UpdateReceiver

	updateInterval time.Duration
}

func newFileWatcher(registry *fileStatusRegistry, updateReceiver syncstatus.UpdateReceiver, updateInterval time.Duration) *fileWatcher {
	return &fileWatcher{
		filesToWatchLock: &sync.Mutex{},
		filesToWatch:     map[fileWithSpace]struct{}{},
		updateCh:         make(chan fileWithSpace),
		closeCh:          make(chan struct{}),
		updateInterval:   updateInterval,
		updateReceiver:   updateReceiver,
		registry:         registry,
	}
}

func (s *fileWatcher) run() {
	ctx := context.Background()

	go func() {
		for {
			select {
			case <-s.closeCh:
				return
			case key := <-s.updateCh:
				if err := s.updateFileStatus(ctx, key); err != nil {
					log.Error("check file",
						zap.String("spaceID", key.spaceID),
						zap.String("fileID", key.fileID),
						zap.Error(err),
					)
				}
			}
		}
	}()

	s.checkFiles(ctx)
	t := time.NewTicker(s.updateInterval)
	defer t.Stop()
	for {
		select {
		case <-s.closeCh:
			return
		case <-t.C:
			s.checkFiles(ctx)
		}
	}
}

func (s *fileWatcher) close() {
	close(s.closeCh)
}

func (s *fileWatcher) updateFileStatus(ctx context.Context, key fileWithSpace) error {
	status, err := s.registry.GetFileStatus(ctx, key.spaceID, key.fileID)
	if err != nil {
		return fmt.Errorf("get file status: %w", err)
	}
	// Files are immutable, so we can stop watching status updates after file is synced
	if status == syncstatus.StatusSynced {
		go s.Unwatch(key.spaceID, key.fileID)
	}
	go func() {
		err = s.updateReceiver.UpdateTree(context.Background(), key.fileID, status)
		if err != nil {
			log.Error("send sync status update", zap.Error(err))
		}
	}()
	return nil
}

func (s *fileWatcher) checkFiles(ctx context.Context) {
	s.filesToWatchLock.Lock()
	defer s.filesToWatchLock.Unlock()

	for key := range s.filesToWatch {
		s.requestUpdate(key)
	}
}

func (s *fileWatcher) requestUpdate(key fileWithSpace) {
	select {
	case <-s.closeCh:
		return
	case s.updateCh <- key:
	}
}

func (s *fileWatcher) Watch(spaceID, fileID string) {
	s.filesToWatchLock.Lock()
	defer s.filesToWatchLock.Unlock()

	key := fileWithSpace{spaceID: spaceID, fileID: fileID}
	if _, ok := s.filesToWatch[key]; !ok {
		s.filesToWatch[key] = struct{}{}
	}
	go s.requestUpdate(key)
}

func (s *fileWatcher) Unwatch(spaceID, fileID string) {
	s.filesToWatchLock.Lock()
	defer s.filesToWatchLock.Unlock()
	delete(s.filesToWatch, fileWithSpace{spaceID: spaceID, fileID: fileID})
}
