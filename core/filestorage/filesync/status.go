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
}

type Status struct {
	lock  *sync.Mutex
	files map[fileWithSpace]fileStatus

	statusService   StatusService
	dagService      ipld.DAGService
	fileSyncService FileSync
}

func NewStatus(statusService StatusService, dagService ipld.DAGService, fileSyncService FileSync) *Status {
	return &Status{
		lock:            &sync.Mutex{},
		files:           map[fileWithSpace]fileStatus{},
		statusService:   statusService,
		dagService:      dagService,
		fileSyncService: fileSyncService,
	}
}

func (s *Status) Run() {
	go s.run()
}

func (s *Status) run() {
	s.check()
	t := time.NewTicker(time.Second)
	for range t.C {
		s.check()
	}
}

func (s *Status) check() {
	s.lock.Lock()
	defer s.lock.Unlock()

	for key := range s.files {
		status, err := s.checkFile(context.Background(), key)
		if err != nil {
			log.Error("check file", zap.Error(err))
			continue
		}

		err = s.statusService.UpdateTree(context.Background(), key.fileID, status.status)
		if err != nil {
			log.Error("update tree", zap.Error(err))
		}
	}
}

func (s *Status) checkFile(ctx context.Context, key fileWithSpace) (fileStatus, error) {
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

// TODO share chunks count with uploader queue
func (s *Status) countFileChunks(ctx context.Context, id string) (int, error) {
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

func (s *Status) Watch(spaceID, fileID string) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.files[fileWithSpace{spaceID: spaceID, fileID: fileID}] = fileStatus{}
}

func (s *Status) Unwatch(spaceID, fileID string) {
	s.lock.Lock()
	defer s.lock.Unlock()
	delete(s.files, fileWithSpace{spaceID: spaceID, fileID: fileID})
}
