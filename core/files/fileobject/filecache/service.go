package filecache

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/anyproto/any-sync/app"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/files/filedownloader"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

const CName = "core.filecache"

var log = logging.Logger(CName).Desugar()

type Service interface {
	CacheFile(ctx context.Context, spaceId string, fileId domain.FileId, blocksLimit int)

	app.ComponentRunnable
}

type service struct {
	ctx       context.Context
	ctxCancel context.CancelFunc

	fileDownloaderService filedownloader.Service

	requestBufferSize int
	timeout           time.Duration
	workersCount      int

	queue *queue[warmupTask]
}

func New() Service {
	return &service{
		requestBufferSize: 20,
		timeout:           2 * time.Minute,
		workersCount:      5,
	}
}

func (s *service) Name() string {
	return CName
}

func (s *service) Init(a *app.App) error {
	s.ctx, s.ctxCancel = context.WithCancel(context.Background())

	s.fileDownloaderService = app.MustComponent[filedownloader.Service](a)

	var err error
	s.queue, err = newQueue[warmupTask](s.requestBufferSize)
	if err != nil {
		return fmt.Errorf("create queue: %w", err)
	}
	s.queue.onCancel = func(task warmupTask) {
		task.ctxCancel()
	}
	return nil
}

func (s *service) Run(ctx context.Context) error {
	for range s.workersCount {
		go s.runDownloader()
	}
	return nil
}

func (s *service) Close(ctx context.Context) error {
	if s.ctxCancel != nil {
		s.ctxCancel()
	}
	s.queue.close()
	return nil
}

func (s *service) runDownloader() {
	for {
		task := s.queue.getNext()
		if task == nil {
			return
		}

		err := s.fileDownloaderService.DownloadToLocalStore(task.ctx, task.spaceId, task.cid)
		if err != nil {
			log.Error("cache file", zap.Error(err))
		}
	}
}

func (s *service) CacheFile(ctx context.Context, spaceId string, fileId domain.FileId) {
	// Task will be canceled along with service context
	// nolint: lostcancel
	taskCtx, _ := context.WithTimeout(s.ctx, s.timeout)

	s.queue.push(warmupTask{
		spaceId:     spaceId,
		cid:         fileId,
		ctx:         taskCtx,
		ctxCancel:   taskCtxCancel,
		blocksLimit: blocksLimit,
	})
}

type warmupTask struct {
	spaceId     string
	cid         domain.FileId
	ctx         context.Context
	ctxCancel   context.CancelFunc
	blocksLimit int
}

type queue[T any] struct {
	cond sync.Cond

	onCancel func(T)

	closed bool
	// tasks is LIFO circular buffer
	tasks      []*T
	currentIdx int
}

func newQueue[T any](maxSize int) (*queue[T], error) {
	if maxSize <= 0 {
		return nil, fmt.Errorf("max size must be > 0")
	}
	return &queue[T]{
		cond:  sync.Cond{L: &sync.Mutex{}},
		tasks: make([]*T, maxSize),
	}, nil
}

func (q *queue[T]) push(task T) {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()

	prevTask := q.tasks[q.currentIdx]
	if prevTask != nil && q.onCancel != nil {
		q.onCancel(*prevTask)
	}

	q.tasks[q.currentIdx] = &task
	q.currentIdx++
	if q.currentIdx == len(q.tasks) {
		q.currentIdx = 0
	}

	q.cond.Signal()
}

func (q *queue[T]) getNext() *T {
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

func (q *queue[T]) pop() *T {
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

func (q *queue[T]) close() {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()

	q.closed = true
	q.cond.Broadcast()
}
