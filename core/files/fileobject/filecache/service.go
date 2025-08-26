package filecache

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/commonfile/fileservice"
	"github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files/filehelper"
	"github.com/anyproto/anytype-heart/core/files/fileobject"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

const CName = "core.filecache"

var log = logging.Logger(CName).Desugar()

type Service interface {
	CacheFile(ctx context.Context, spaceId string, fileId domain.FileId) error

	app.ComponentRunnable
}

type service struct {
	fileObjectService fileobject.Service
	dagService        ipld.DAGService

	maxSize int
	queue   *lruQueue[warmupTask]
}

func New() Service {
	return &service{
		maxSize: 10 * 1024 * 1024,
	}
}

func (s *service) Name() string {
	return CName
}

func (s *service) Init(a *app.App) error {
	commonFile := app.MustComponent[fileservice.FileService](a)

	s.dagService = commonFile.DAGService()
	s.fileObjectService = app.MustComponent[fileobject.Service](a)

	s.queue = newLruQueue[warmupTask](50)
	return nil
}

func (s *service) Run(ctx context.Context) error {
	go s.runDownloader()
	return nil
}

func (s *service) Close(ctx context.Context) error {
	s.queue.close()
	return nil
}

func (s *service) runDownloader() {
	for {
		task := s.queue.getNext()
		if task == nil {
			return
		}

		err := s.cacheFile(task.ctx, task.spaceId, task.cid)
		if err != nil {
			log.Error("cache file", zap.Error(err))
		}
	}
}

func (s *service) cacheFile(ctx context.Context, spaceId string, rootCid cid.Cid) error {
	dagService := s.dagServiceForSpace(spaceId)
	rootNode, err := dagService.Get(ctx, rootCid)
	if err != nil {
		return fmt.Errorf("get root node: %w", err)
	}

	var totalSize int
	visited := map[cid.Cid]struct{}{}
	walker := ipld.NewWalker(ctx, ipld.NewNavigableIPLDNode(rootNode, dagService))
	err = walker.Iterate(func(navNode ipld.NavigableNode) error {
		node := navNode.GetIPLDNode()
		if _, ok := visited[node.Cid()]; !ok {
			size, err := navNode.GetIPLDNode().Size()
			if err != nil {
				return fmt.Errorf("get size: %w", err)
			}
			totalSize += int(size)
			if totalSize > s.maxSize {
				// TODO Remove cached data (store cached keys collector in context)
				return fmt.Errorf("file is too big")
			}
			visited[node.Cid()] = struct{}{}
		}
		return nil
	})
	if errors.Is(err, ipld.EndOfDag) {
		return nil
	}
	return nil
}

func (s *service) CacheFile(ctx context.Context, spaceId string, fileId domain.FileId) error {
	rootCid, err := fileId.Cid()
	if err != nil {
		return fmt.Errorf("parse cid: %w", err)
	}

	s.queue.push(warmupTask{
		spaceId: spaceId,
		cid:     rootCid,
		// TODO Decide how to add cache warm cancellation
		ctx: context.Background(),
	})

	return err
}

func (s *service) dagServiceForSpace(spaceID string) ipld.DAGService {
	return filehelper.NewDAGServiceWithSpaceID(spaceID, s.dagService)
}

type warmupTask struct {
	spaceId string
	cid     cid.Cid
	ctx     context.Context
}

type lruQueue[T any] struct {
	cond sync.Cond

	closed bool
	// tasks is LIFO circular buffer
	tasks      []*T
	currentIdx int
}

func newLruQueue[T any](maxSize int) *lruQueue[T] {
	return &lruQueue[T]{
		cond:  sync.Cond{L: &sync.Mutex{}},
		tasks: make([]*T, maxSize),
	}
}

func (q *lruQueue[T]) push(task T) {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()

	q.tasks[q.currentIdx] = &task
	q.currentIdx++
	if q.currentIdx == len(q.tasks) {
		q.currentIdx = 0
	}

	q.cond.Signal()
}

func (q *lruQueue[T]) getNext() *T {
	q.cond.L.Lock()
	for {
		if q.closed {
			q.cond.L.Unlock()
			return nil
		}

		task := q.pop()
		if task != nil {
			q.cond.L.Unlock()
			return task
		}
		q.cond.Wait()
	}
}

func (q *lruQueue[T]) pop() *T {
	for range len(q.tasks) {
		task := q.tasks[q.currentIdx]
		// Remove from buffer
		if task != nil {
			q.tasks[q.currentIdx] = nil
		}

		// Move the pointer backwards
		q.currentIdx--
		if q.currentIdx < 0 {
			q.currentIdx = len(q.tasks) - 1
		}

		if task != nil {
			return task
		}
	}
	return nil
}

func (q *lruQueue[T]) close() {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()

	q.closed = true
	q.cond.Broadcast()
}
