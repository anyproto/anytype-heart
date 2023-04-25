package filesync

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/anytypeio/any-sync/commonspace/syncstatus"
	"github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
	"go.uber.org/zap"
)

type StatusService interface {
	UpdateTree(ctx context.Context, objId string, status syncstatus.SyncStatus) (err error)
}

type fileWithSpace struct {
	fileID, spaceID string
}

type fileStatus struct {
	chunksCount int
	status      syncstatus.SyncStatus
	updatedAt   time.Time
}

type StatusWatcher struct {
	lock         *sync.Mutex
	filesToWatch map[fileWithSpace]struct{}
	files        map[fileWithSpace]fileStatus

	updateInterval  time.Duration
	statusService   StatusService
	dagService      ipld.DAGService
	fileSyncService FileSync
}

func (f *fileSync) NewStatusWatcher(statusService StatusService, updateInterval time.Duration) *StatusWatcher {
	return &StatusWatcher{
		lock:            &sync.Mutex{},
		files:           map[fileWithSpace]fileStatus{},
		filesToWatch:    map[fileWithSpace]struct{}{},
		statusService:   statusService,
		dagService:      f.dagService,
		fileSyncService: f,
		updateInterval:  updateInterval,
	}
}

func (s *StatusWatcher) Run() {
	go s.run()
}

func (s *StatusWatcher) run() {
	ctx := context.Background()

	s.checkFiles(ctx)
	t := time.NewTicker(s.updateInterval)
	for range t.C {
		s.checkFiles(ctx)
	}
}

func (s *StatusWatcher) checkFiles(ctx context.Context) {
	s.lock.Lock()
	defer s.lock.Unlock()

	for key := range s.filesToWatch {
		if err := s.updateFileStatus(ctx, key); err != nil {
			log.Error("check file",
				zap.String("spaceID", key.spaceID),
				zap.String("fileID", key.fileID),
				zap.Error(err),
			)
		}
	}
}

func (s *StatusWatcher) updateFileStatus(ctx context.Context, key fileWithSpace) error {
	status, err := s.getFileStatus(ctx, key)
	if err != nil {
		return fmt.Errorf("get file status: %w", err)
	}

	return s.statusService.UpdateTree(context.Background(), key.fileID, status.status)
}

func (s *StatusWatcher) getFileStatus(ctx context.Context, key fileWithSpace) (fileStatus, error) {
	now := time.Now()
	status, ok := s.files[key]
	if !ok || status.chunksCount == 0 {
		chunksCount, err := s.countFileChunks(ctx, key.fileID)
		if err != nil {
			return fileStatus{}, fmt.Errorf("count file chunks: %w", err)
		}
		status = fileStatus{
			chunksCount: chunksCount,
			status:      syncstatus.StatusNotSynced,
		}
		s.files[key] = status
	}

	if status.status == syncstatus.StatusSynced {
		return status, nil
	}

	if time.Since(status.updatedAt) < s.updateInterval {
		return status, nil
	}
	status.updatedAt = now

	fstat, err := s.fileSyncService.FileStat(ctx, key.spaceID, key.fileID)
	if err != nil {
		return fileStatus{}, fmt.Errorf("file stat: %w", err)
	}
	if fstat.CidCount == status.chunksCount {
		status.status = syncstatus.StatusSynced
	}
	s.files[key] = status
	return status, nil
}

func (s *StatusWatcher) countFileChunks(ctx context.Context, id string) (int, error) {
	fileCid, err := cid.Parse(id)
	if err != nil {
		return 0, err
	}
	node, err := s.dagService.Get(ctx, fileCid)
	if err != nil {
		return 0, err
	}

	var count int
	walker := ipld.NewWalker(ctx, ipld.NewNavigableIPLDNode(node, s.dagService))
	err = walker.Iterate(func(node ipld.NavigableNode) error {
		count++
		return nil
	})
	if err == ipld.EndOfDag {
		err = nil
	}
	return count, err
}

func (s *StatusWatcher) Watch(spaceID, fileID string) {
	s.lock.Lock()
	defer s.lock.Unlock()

	key := fileWithSpace{spaceID: spaceID, fileID: fileID}
	if _, ok := s.filesToWatch[key]; ok {
		return
	}
	s.filesToWatch[key] = struct{}{}

	go func() {
		s.lock.Lock()
		defer s.lock.Unlock()

		if err := s.updateFileStatus(context.Background(), key); err != nil {
			log.Error("watch: check file",
				zap.String("spaceID", key.spaceID),
				zap.String("fileID", key.fileID),
				zap.Error(err),
			)
		}
	}()
}

func (s *StatusWatcher) Unwatch(spaceID, fileID string) {
	s.lock.Lock()
	defer s.lock.Unlock()
	delete(s.filesToWatch, fileWithSpace{spaceID: spaceID, fileID: fileID})
}
