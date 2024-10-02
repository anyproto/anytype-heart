package taskmanager

import (
	"context"
	"sync"
	"time"
)

// Task interface allows for different task implementations without code duplication.
type Task interface {
	TaskBase
	Run(ctx context.Context) error
}

type TaskBase interface {
	ID() string
	GetResult() (*TaskResult, bool)
	WaitIfPaused(ctx context.Context) error

	markDoneWithError(err error)
	resume()
	pause()
	isDone() bool
}
type TaskResult struct {
	Err        error
	FinishTime time.Time
	WorkTime   time.Duration
}

// TaskBase provides common functionality for tasks, such as pause and resume mechanisms.
type taskBase struct {
	id            string
	pauseChan     chan struct{}
	done          bool
	mu            sync.RWMutex
	result        TaskResult
	resultMu      sync.Mutex
	totalWorkTime time.Duration
	lastResumed   time.Time
}

func NewTaskBase(id string) TaskBase {
	return &taskBase{
		id:        id,
		pauseChan: make(chan struct{}), // by default, the task is paused
	}
}

func (t *taskBase) ID() string {
	return t.id
}

func (t *taskBase) GetResult() (*TaskResult, bool) {
	t.resultMu.Lock()
	defer t.resultMu.Unlock()
	if t.done {
		return &t.result, true
	}
	return &TaskResult{}, false
}

func (t *taskBase) isDone() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.done
}

func (t *taskBase) pause() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.pauseChan == nil {
		t.totalWorkTime += time.Since(t.lastResumed)
		t.pauseChan = make(chan struct{})
	}
}

func (t *taskBase) resume() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.pauseChan != nil {
		close(t.pauseChan)
		t.lastResumed = time.Now()
		t.pauseChan = nil
	}
}

func (t *taskBase) WaitIfPaused(ctx context.Context) error {
	t.mu.RLock()
	pauseChan := t.pauseChan
	t.mu.RUnlock()

	if pauseChan == nil {
		return nil
	}

	select {
	case <-pauseChan:
		// Resumed
	case <-ctx.Done():
		// Context canceled
		return ctx.Err()
	}

	return nil
}

func (t *taskBase) markDoneWithError(err error) {
	t.mu.Lock()
	if !t.done {
		t.done = true
		t.mu.Unlock()
		t.resultMu.Lock()
		t.totalWorkTime += time.Since(t.lastResumed)
		t.result = TaskResult{
			Err:        err,
			FinishTime: time.Now(),
			WorkTime:   t.totalWorkTime,
		}
		t.resultMu.Unlock()
	} else {
		t.mu.Unlock()
	}
}

type taskWrapper struct {
	Task
	taskFinishedCh chan string
}

func (t *taskWrapper) Run(ctx context.Context) {
	err := t.Task.Run(ctx)
	t.markDoneWithError(err)
	t.taskFinishedCh <- t.ID()
}
