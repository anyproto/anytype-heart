package syncstatus

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/anyproto/any-sync/commonspace/syncstatus"
	"github.com/dgraph-io/badger/v3"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/pkg/lib/datastore"
	"github.com/anyproto/anytype-heart/space"
)

type fileWithSpace struct {
	fileID, spaceID string
	isUploadLimited bool
}

type fileStatus struct {
	status    FileStatus
	updatedAt time.Time
}

type fileWatcher struct {
	filesToWatchLock *sync.Mutex
	filesToWatch     map[fileWithSpace]struct{}

	dbProvider   datastore.Datastore
	badger       *badger.DB
	spaceService space.Service
	registry     *fileStatusRegistry
	updateCh     chan fileWithSpace
	closeCh      chan struct{}

	updateReceiver syncstatus.UpdateReceiver

	updateInterval time.Duration
}

func newFileWatcher(
	spaceService space.Service,
	dbProvider datastore.Datastore,
	registry *fileStatusRegistry,
	updateReceiver syncstatus.UpdateReceiver,
	updateInterval time.Duration,
) *fileWatcher {
	watcher := &fileWatcher{
		filesToWatchLock: &sync.Mutex{},
		filesToWatch:     map[fileWithSpace]struct{}{},
		updateCh:         make(chan fileWithSpace),
		closeCh:          make(chan struct{}),
		updateInterval:   updateInterval,
		updateReceiver:   updateReceiver,
		registry:         registry,
		dbProvider:       dbProvider,
		spaceService:     spaceService,
	}
	return watcher
}

const filesToWatchPrefix = "/files_to_watch/"

func (s *fileWatcher) loadFilesToWatch() error {
	return s.badger.View(func(txn *badger.Txn) error {
		iter := txn.NewIterator(badger.IteratorOptions{
			Prefix: []byte(filesToWatchPrefix),
		})
		defer iter.Close()

		for iter.Rewind(); iter.Valid(); iter.Next() {
			it := iter.Item()
			fileID := bytes.TrimPrefix(it.Key(), []byte(filesToWatchPrefix))
			s.filesToWatch[fileWithSpace{fileID: string(fileID), spaceID: s.spaceService.AccountId()}] = struct{}{}
		}
		return nil
	})
}

func (s *fileWatcher) run() error {
	db, err := s.dbProvider.SpaceStorage()
	if err != nil {
		return fmt.Errorf("get badger from provider: %w", err)
	}
	s.badger = db
	err = s.loadFilesToWatch()
	if err != nil {
		return fmt.Errorf("load files to watch: %w", err)
	}

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

	go func() {
		s.checkFiles()
		t := time.NewTicker(s.updateInterval)
		defer t.Stop()
		for {
			select {
			case <-s.closeCh:
				return
			case <-t.C:
				s.checkFiles()
			}
		}
	}()

	go func() {
		s.checkLimitedFiles()
		t := time.NewTicker(1 * time.Minute)
		defer t.Stop()
		for {
			select {
			case <-s.closeCh:
				return
			case <-t.C:
				s.checkLimitedFiles()
			}
		}
	}()

	return nil
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
	if status == FileStatusSynced {
		go func() {
			err = s.Unwatch(key.spaceID, key.fileID)
			if err != nil {
				log.Error("unwatching file", zap.String("fileID", key.fileID), zap.Error(err))
			}
		}()
	}
	if !key.isUploadLimited && status == FileStatusLimited {
		go s.moveToLimitedQueue(key)
	}
	go func() {
		err = s.updateReceiver.UpdateTree(context.Background(), key.fileID, fileStatusToSyncStatus(status))
		if err != nil {
			log.Error("send sync status update", zap.String("fileID", key.fileID), zap.Error(err))
		}
	}()
	return nil
}

func fileStatusToSyncStatus(fileStatus FileStatus) syncstatus.SyncStatus {
	switch fileStatus {
	case FileStatusUnknown:
		return syncstatus.StatusUnknown
	case FileStatusSynced:
		return syncstatus.StatusSynced
	case FileStatusSyncing, FileStatusLimited:
		return syncstatus.StatusNotSynced

	default:
		return syncstatus.StatusUnknown
	}
}

func (s *fileWatcher) checkFiles() {
	s.filesToWatchLock.Lock()
	defer s.filesToWatchLock.Unlock()

	for key := range s.filesToWatch {
		if !key.isUploadLimited {
			s.requestUpdate(key)
		}
	}
}

func (s *fileWatcher) checkLimitedFiles() {
	s.filesToWatchLock.Lock()
	defer s.filesToWatchLock.Unlock()

	for key := range s.filesToWatch {
		if key.isUploadLimited {
			s.requestUpdate(key)
		}
	}
}

func (s *fileWatcher) requestUpdate(key fileWithSpace) {
	select {
	case <-s.closeCh:
		return
	case s.updateCh <- key:
	}
}

func (s *fileWatcher) Watch(spaceID, fileID string) error {
	s.filesToWatchLock.Lock()
	defer s.filesToWatchLock.Unlock()

	key := fileWithSpace{spaceID: spaceID, fileID: fileID}
	if _, ok := s.filesToWatch[key]; !ok {
		s.filesToWatch[key] = struct{}{}
		err := s.badger.Update(func(txn *badger.Txn) error {
			return txn.Set([]byte(filesToWatchPrefix+key.fileID), nil)
		})
		if err != nil {
			return fmt.Errorf("add file to watch store: %w", err)
		}
	}
	go s.requestUpdate(key)
	return nil
}

func (s *fileWatcher) Unwatch(spaceID, fileID string) error {
	s.filesToWatchLock.Lock()
	defer s.filesToWatchLock.Unlock()

	err := s.badger.Update(func(txn *badger.Txn) error {
		return txn.Delete([]byte(filesToWatchPrefix + fileID))
	})
	if err != nil {
		return fmt.Errorf("delete file from watch store: %w", err)
	}
	delete(s.filesToWatch, fileWithSpace{spaceID: spaceID, fileID: fileID})

	return nil
}

func (s *fileWatcher) moveToLimitedQueue(key fileWithSpace) {
	s.filesToWatchLock.Lock()
	defer s.filesToWatchLock.Unlock()

	delete(s.filesToWatch, key)

	key.isUploadLimited = true
	s.filesToWatch[key] = struct{}{}
}
