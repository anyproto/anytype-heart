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
	WaitResult(ctx context.Context) (TaskResult, error)
	GetResult() (TaskResult, bool)
	WaitIfPaused(ctx context.Context) error

	markDoneWithError(err error)
	setStartTime(startTime time.Time)
	resume()
	pause()
	isDone() bool
}

type TaskResultWithId struct {
	Id string
	TaskResult
}

type TaskResult struct {
	Err        error
	StartTime  time.Time
	FinishTime time.Time
	WorkTime   time.Duration
}

// TaskBase provides common functionality for tasks, such as pause and resume mechanisms.
type taskBase struct {
	id            string
	pauseChan     chan struct{}
	done          chan struct{}
	mu            sync.RWMutex
	result        TaskResult
	resultMu      sync.Mutex
	totalWorkTime time.Duration
	lastResumed   time.Time
	startTime     time.Time
}

func NewTaskBase(id string) TaskBase {
	return &taskBase{
		id:        id,
		done:      make(chan struct{}),
		pauseChan: make(chan struct{}), // by default, the task is paused
	}
}

func (t *taskBase) ID() string {
	return t.id
}

func (t *taskBase) GetResult() (TaskResult, bool) {
	select {
	case <-t.done:
		t.resultMu.Lock()
		defer t.resultMu.Unlock()
		return t.result, true
	default:
		return TaskResult{}, false
	}
}

func (t *taskBase) WaitResult(ctx context.Context) (TaskResult, error) {
	select {
	case <-t.done:
		t.resultMu.Lock()
		defer t.resultMu.Unlock()
		return t.result, nil
	case <-ctx.Done():
		return TaskResult{}, ctx.Err()
	}
}

func (t *taskBase) isDone() bool {
	select {
	case <-t.done:
		return true
	default:
		return false
	}
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
	t.resultMu.Lock()
	defer t.resultMu.Unlock()
	if !t.result.FinishTime.IsZero() {
		return
	}

	close(t.done)
	t.totalWorkTime += time.Since(t.lastResumed)
	t.result = TaskResult{
		Err:        err,
		StartTime:  t.startTime,
		FinishTime: time.Now(),
		WorkTime:   t.totalWorkTime,
	}
}

func (t *taskBase) setStartTime(startTime time.Time) {
	t.startTime = startTime
}

type taskWrapper struct {
	Task
	taskFinishedCh chan string
}

func (t *taskWrapper) Run(ctx context.Context) {
	t.setStartTime(time.Now())
	err := t.Task.Run(ctx)
	t.markDoneWithError(err)
	t.taskFinishedCh <- t.ID()
}
