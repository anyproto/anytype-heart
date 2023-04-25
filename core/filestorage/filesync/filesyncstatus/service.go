package filesyncstatus

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/anytypeio/any-sync/app"
	"github.com/anytypeio/any-sync/app/logger"
	"github.com/anytypeio/any-sync/commonspace/syncstatus"
	"go.uber.org/zap"
)

var log = logger.NewNamed(CName)

type fileWithSpace struct {
	fileID, spaceID string
}

type fileStatus struct {
	status    syncstatus.SyncStatus
	updatedAt time.Time
}

type StatusWatcher interface {
	Watch(spaceID, fileID string)
	Unwatch(spaceID, fileID string)

	app.ComponentRunnable
}

type statusWatcher struct {
	filesToWatchLock *sync.Mutex
	filesToWatch     map[fileWithSpace]struct{}

	registry Registry
	updateCh chan fileWithSpace
	closeCh  chan struct{}

	updateReceiver syncstatus.UpdateReceiver

	updateInterval time.Duration
}

func New(registry Registry, updateReceiver syncstatus.UpdateReceiver, updateInterval time.Duration) StatusWatcher {
	return &statusWatcher{
		filesToWatchLock: &sync.Mutex{},
		filesToWatch:     map[fileWithSpace]struct{}{},
		updateCh:         make(chan fileWithSpace),
		closeCh:          make(chan struct{}),
		updateInterval:   updateInterval,
		updateReceiver:   updateReceiver,
		registry:         registry,
	}
}

func (s *statusWatcher) Init(_ *app.App) error {
	return nil
}

func (s *statusWatcher) Run(ctx context.Context) error {
	go s.run()
	return nil
}

func (s *statusWatcher) Close(ctx context.Context) error {
	close(s.closeCh)
	return nil
}

const CName = "file_sync_status"

func (s *statusWatcher) Name() string {
	return CName
}

func (s *statusWatcher) run() {
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

func (s *statusWatcher) updateFileStatus(ctx context.Context, key fileWithSpace) error {
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

func (s *statusWatcher) checkFiles(ctx context.Context) {
	s.filesToWatchLock.Lock()
	defer s.filesToWatchLock.Unlock()

	for key := range s.filesToWatch {
		s.requestUpdate(key)
	}
}

func (s *statusWatcher) requestUpdate(key fileWithSpace) {
	select {
	case <-s.closeCh:
		return
	case s.updateCh <- key:
	}
}

func (s *statusWatcher) Watch(spaceID, fileID string) {
	s.filesToWatchLock.Lock()
	defer s.filesToWatchLock.Unlock()

	key := fileWithSpace{spaceID: spaceID, fileID: fileID}
	if _, ok := s.filesToWatch[key]; !ok {
		s.filesToWatch[key] = struct{}{}
	}
	go s.requestUpdate(key)
}

func (s *statusWatcher) Unwatch(spaceID, fileID string) {
	s.filesToWatchLock.Lock()
	defer s.filesToWatchLock.Unlock()
	delete(s.filesToWatch, fileWithSpace{spaceID: spaceID, fileID: fileID})
}
