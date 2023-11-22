package syncstatus

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/anyproto/any-sync/commonspace/syncstatus"
	"github.com/dgraph-io/badger/v3"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore"
)

type fileEntry struct {
	fileId          string
	fileHash        string
	spaceId         string
	isUploadLimited bool
}

type fileStatus struct {
	status    FileStatus
	updatedAt time.Time
}

type personalIDProvider interface {
	PersonalSpaceID() string
}

type fileWatcher struct {
	filesToWatchLock *sync.Mutex
	// fileId -> entry
	filesToWatch map[string]fileEntry

	dbProvider datastore.Datastore
	badger     *badger.DB
	provider   personalIDProvider
	registry   *fileStatusRegistry
	updateCh   chan fileEntry
	closeCh    chan struct{}

	updateReceiver syncstatus.UpdateReceiver

	updateInterval time.Duration
}

func newFileWatcher(
	provider personalIDProvider,
	dbProvider datastore.Datastore,
	registry *fileStatusRegistry,
	updateReceiver syncstatus.UpdateReceiver,
	updateInterval time.Duration,
) *fileWatcher {
	watcher := &fileWatcher{
		filesToWatchLock: &sync.Mutex{},
		filesToWatch:     map[string]fileEntry{},
		updateCh:         make(chan fileEntry),
		closeCh:          make(chan struct{}),
		updateInterval:   updateInterval,
		updateReceiver:   updateReceiver,
		registry:         registry,
		dbProvider:       dbProvider,
		provider:         provider,
	}
	return watcher
}

const filesToWatchPrefix = "/files_to_watch/"

func (s *fileWatcher) loadFilesToWatch() error {
	s.filesToWatchLock.Lock()
	defer s.filesToWatchLock.Unlock()

	//return s.badger.View(func(txn *badger.Txn) error {
	//	defaultSpaceID := s.provider.PersonalSpaceID()
	//	iter := txn.NewIterator(badger.IteratorOptions{
	//		Prefix: []byte(filesToWatchPrefix),
	//	})
	//	defer iter.Close()
	//
	//	for iter.Rewind(); iter.Valid(); iter.Next() {
	//		it := iter.Item()
	//		fileID := bytes.TrimPrefix(it.Key(), []byte(filesToWatchPrefix))
	//		spaceID, err := it.ValueCopy(nil)
	//		if err != nil {
	//			return fmt.Errorf("failed to copy spaceId value from badger for '%s'", fileID)
	//		}
	//		if len(spaceID) != 0 {
	//			entry := fileEntry{fileHash: string(fileID), spaceId: string(spaceID)}
	//			s.filesToWatch[] = struct{}{}
	//		} else {
	//			err = s.Watch(defaultSpaceID, string(fileID))
	//			if err != nil {
	//				log.Errorf("failed to migrate files in space store: %v", err)
	//			}
	//		}
	//	}
	//
	//	return nil
	//})
	return nil
}

func (s *fileWatcher) init() error {
	// Init badger here because some services will call Watch before file watcher started
	// and Watch writes fileHash to badger
	db, err := s.dbProvider.SpaceStorage()
	if err != nil {
		return fmt.Errorf("get badger from provider: %w", err)
	}
	s.badger = db
	return nil
}

func (s *fileWatcher) run() error {
	if err := s.loadFilesToWatch(); err != nil {
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
					log.With("spaceId", key.spaceId, "fileHash", key.fileHash).Errorf("check file: %s", err)
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

func (s *fileWatcher) list() []fileEntry {
	s.filesToWatchLock.Lock()
	defer s.filesToWatchLock.Unlock()

	result := make([]fileEntry, 0, len(s.filesToWatch))
	for _, entry := range s.filesToWatch {
		result = append(result, entry)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].fileHash < result[j].fileHash
	})
	return result
}

func (s *fileWatcher) updateFileStatus(ctx context.Context, entry fileEntry) error {
	status, err := s.registry.GetFileStatus(ctx, entry.spaceId, entry.fileId, entry.fileHash)
	if errors.Is(err, domain.ErrFileNotFound) {
		s.Unwatch(entry.fileId)
		return err
	}
	if err != nil {
		return fmt.Errorf("get file status: %w", err)
	}
	// Files are immutable, so we can stop watching status updates after file is synced
	if status == FileStatusSynced {
		s.Unwatch(entry.fileId)
	}
	if !entry.isUploadLimited && status == FileStatusLimited {
		go s.moveToLimitedQueue(entry.fileId)
	}
	go func() {
		err = s.updateReceiver.UpdateTree(context.Background(), entry.fileId, fileStatusToSyncStatus(status))
		if err != nil {
			log.Error("send sync status update", zap.String("fileHash", entry.fileHash), zap.Error(err))
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

	for _, entry := range s.filesToWatch {
		if !entry.isUploadLimited {
			s.requestUpdate(entry)
		}
	}
}

func (s *fileWatcher) checkLimitedFiles() {
	s.filesToWatchLock.Lock()
	defer s.filesToWatchLock.Unlock()

	for _, entry := range s.filesToWatch {
		if entry.isUploadLimited {
			s.requestUpdate(entry)
		}
	}
}

func (s *fileWatcher) requestUpdate(key fileEntry) {
	select {
	case <-s.closeCh:
		return
	case s.updateCh <- key:
	}
}

func (s *fileWatcher) Watch(spaceId string, fileId string, fileHash string) error {
	s.filesToWatchLock.Lock()
	defer s.filesToWatchLock.Unlock()

	entry := fileEntry{
		spaceId:  spaceId,
		fileId:   fileId,
		fileHash: fileHash,
	}
	if _, ok := s.filesToWatch[fileId]; !ok {
		s.filesToWatch[fileId] = entry
		err := s.badger.Update(func(txn *badger.Txn) error {
			return txn.Set([]byte(filesToWatchPrefix+fileId), []byte(spaceId))
		})
		if err != nil {
			return fmt.Errorf("add file to watch store: %w", err)
		}
	}
	go s.requestUpdate(entry)
	return nil
}

func (s *fileWatcher) Unwatch(fileId string) {
	go func() {
		err := s.unwatch(fileId)
		if err != nil {
			log.Error("unwatching file", zap.String("fileId", fileId), zap.Error(err))
		}
	}()
}

func (s *fileWatcher) unwatch(fileId string) error {
	s.filesToWatchLock.Lock()
	defer s.filesToWatchLock.Unlock()

	err := s.badger.Update(func(txn *badger.Txn) error {
		return txn.Delete([]byte(filesToWatchPrefix + fileId))
	})
	if err != nil {
		return fmt.Errorf("delete file from watch store: %w", err)
	}
	delete(s.filesToWatch, fileId)

	return nil
}

func (s *fileWatcher) moveToLimitedQueue(fileId string) {
	s.filesToWatchLock.Lock()
	defer s.filesToWatchLock.Unlock()

	entry := s.filesToWatch[fileId]
	entry.isUploadLimited = true
	s.filesToWatch[fileId] = entry
}
