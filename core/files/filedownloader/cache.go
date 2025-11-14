package filedownloader

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/domain"
)

type cacheWarmer struct {
	ctx context.Context

	downloadFn func(ctx context.Context, spaceId string, cid domain.FileId) error

	requestBufferSize int
	timeout           time.Duration
	workersCount      int

	queue *queue[warmupTask]
}

func newCacheWarmer(ctx context.Context, downloadFn func(ctx context.Context, spaceId string, cid domain.FileId) error) (*cacheWarmer, error) {
	w := &cacheWarmer{
		ctx:               ctx,
		downloadFn:        downloadFn,
		requestBufferSize: 20,
		timeout:           2 * time.Minute,
		workersCount:      5,
	}
	queue, err := newQueue[warmupTask](w.requestBufferSize)
	if err != nil {
		return nil, fmt.Errorf("create queue: %w", err)
	}
	queue.onCancel = func(task warmupTask) {
		task.ctxCancel()
	}

	return w, nil
}

func (s *cacheWarmer) Run(ctx context.Context) error {
	for range s.workersCount {
		go s.runDownloader()
	}
	return nil
}

func (s *cacheWarmer) Close(ctx context.Context) error {
	s.queue.close()
	return nil
}

func (s *cacheWarmer) runDownloader() {
	for {
		task := s.queue.getNext()
		if task == nil {
			return
		}

		err := s.downloadFn(task.ctx, task.spaceId, task.cid)
		if err != nil {
			log.Error("cache file", zap.Error(err))
		}
	}
}

func (s *cacheWarmer) CacheFile(ctx context.Context, spaceId string, fileId domain.FileId, blocksLimit int) {
	// Task will be canceled along with service context
	// nolint: lostcancel
	taskCtx, _ := context.WithTimeout(s.ctx, s.timeout)

	s.queue.push(warmupTask{
		spaceId:     spaceId,
		cid:         fileId,
		ctx:         taskCtx,
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

	closed bool
	// tasks is LIFO circular buffer
	tasks []*T
	// currentIdx points at the current position in the circular buffer
	currentIdx int

	// onCancel is called when a task is removed from the circular buffer
	onCancel func(T)
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
