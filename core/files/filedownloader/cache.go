package filedownloader

import (
	"context"
	"slices"
	"time"

	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/domain"
)

type cacheWarmer struct {
	ctx context.Context

	getNextCh chan getNextRequest
	enqueueCh chan warmupTask
	cancelCh  chan domain.FileId
	doneCh    chan domain.FileId

	waiters  []getNextRequest
	tasks    []warmupTask
	inflight map[domain.FileId]warmupTask

	blocksLimit int
	tasksLimit  int
	downloadFn  func(ctx context.Context, spaceId string, cid domain.FileId, blocksLimit int) error
}

func newCacheWarmer(ctx context.Context, blocksLimit int, tasksLimit int, timeout time.Duration, downloadFn func(ctx context.Context, spaceId string, cid domain.FileId, blocksLimit int) error) *cacheWarmer {
	return &cacheWarmer{
		ctx:         ctx,
		getNextCh:   make(chan getNextRequest),
		enqueueCh:   make(chan warmupTask),
		cancelCh:    make(chan domain.FileId),
		doneCh:      make(chan domain.FileId),
		inflight:    make(map[domain.FileId]warmupTask),
		blocksLimit: blocksLimit,
		tasksLimit:  tasksLimit,
		downloadFn: func(ctx context.Context, spaceId string, cid domain.FileId, blocksLimit int) error {
			ctx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			return downloadFn(ctx, spaceId, cid, blocksLimit)
		},
	}
}

type getNextRequest struct {
	responseCh chan warmupTask
}

func (w *cacheWarmer) runWorker() {
	for {
		task, err := w.getNext()
		if err != nil {
			return
		}

		err = w.downloadFn(task.ctx, task.spaceId, task.cid, w.blocksLimit)
		if err != nil {
			log.Error("cache warmer: download file", zap.Error(err))
		}

		err = w.markDone(task.cid)
		if err != nil {
			return
		}
	}
}

func (w *cacheWarmer) enqueue(spaceId string, cid domain.FileId) {
	ctx, cancel := context.WithCancel(w.ctx)

	task := warmupTask{
		spaceId: spaceId,
		cid:     cid,

		ctx:    ctx,
		cancel: cancel,
	}

	select {
	case w.enqueueCh <- task:
	case <-w.ctx.Done():
	}
}

func (w *cacheWarmer) getNext() (warmupTask, error) {
	respCh := make(chan warmupTask, 1)
	select {
	case w.getNextCh <- getNextRequest{respCh}:
		select {
		case task := <-respCh:
			return task, nil
		case <-w.ctx.Done():
			return warmupTask{}, w.ctx.Err()
		}
	case <-w.ctx.Done():
		return warmupTask{}, w.ctx.Err()
	}
}

func (w *cacheWarmer) cancelTask(cid domain.FileId) {
	select {
	case w.cancelCh <- cid:
	case <-w.ctx.Done():
	}
}

func (w *cacheWarmer) markDone(cid domain.FileId) error {
	select {
	case w.doneCh <- cid:
		return nil
	case <-w.ctx.Done():
		return w.ctx.Err()
	}
}

func (w *cacheWarmer) run() {
	for {
		select {
		case <-w.ctx.Done():
			return
		case req := <-w.getNextCh:
			w.handleGetNext(req)
		case task := <-w.enqueueCh:
			w.handleEnqueue(task)
		case fileId := <-w.doneCh:
			w.handleDone(fileId)
		case fileId := <-w.cancelCh:
			w.handleCancel(fileId)
		}
	}
}

func (w *cacheWarmer) handleGetNext(req getNextRequest) {
	if len(w.tasks) == 0 {
		w.waiters = append(w.waiters, req)
	} else {
		next := w.tasks[0]
		w.tasks = w.tasks[1:]
		w.respond(req.responseCh, next)
	}
}

func (w *cacheWarmer) handleEnqueue(task warmupTask) {
	if len(w.waiters) == 0 {
		w.pushTask(task)
	} else {
		first := w.waiters[0]
		w.waiters = w.waiters[1:]
		w.respond(first.responseCh, task)
	}
}

func (w *cacheWarmer) pushTask(task warmupTask) {
	if len(w.tasks) == w.tasksLimit && w.tasksLimit > 0 {
		w.tasks = append(w.tasks[1:], task)
	} else {
		w.tasks = append(w.tasks, task)
	}
}

func (w *cacheWarmer) respond(respCh chan<- warmupTask, task warmupTask) {
	w.inflight[task.cid] = task
	respCh <- task
}

func (w *cacheWarmer) handleCancel(fileId domain.FileId) {
	for i, task := range w.tasks {
		if task.cid == fileId {
			w.tasks = slices.Delete(w.tasks, i, i+1)
			break
		}
	}

	w.handleDone(fileId)
}

func (w *cacheWarmer) handleDone(fileId domain.FileId) {
	task, ok := w.inflight[fileId]
	if ok {
		task.cancel()
		delete(w.inflight, fileId)
	}
}

type warmupTask struct {
	spaceId string
	cid     domain.FileId

	ctx    context.Context
	cancel context.CancelFunc
}
