package daemon

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// Task is a background function that runs until the given context is canceled.
type Task func(ctx context.Context) error

// TaskManager tracks running background tasks.
type TaskManager struct {
	mu    sync.Mutex
	tasks map[string]context.CancelFunc
}

// defaultTaskManager is the singleton instance.
var defaultTaskManager = NewTaskManager()

// NewTaskManager returns a new task manager.
func NewTaskManager() *TaskManager {
	return &TaskManager{
		tasks: make(map[string]context.CancelFunc),
	}
}

// StartTask starts a new task with a unique ID.
// It returns an error if a task with that ID is already running.
func (tm *TaskManager) StartTask(id string, task Task) error {
	tm.mu.Lock()
	if _, exists := tm.tasks[id]; exists {
		tm.mu.Unlock()
		return errors.New("task already running")
	}
	ctx, cancel := context.WithCancel(context.Background())
	tm.tasks[id] = cancel
	tm.mu.Unlock()

	go func() {
		if err := task(ctx); err != nil {
			fmt.Printf("Task %s exited with error: %v", id, err)
		}
		tm.mu.Lock()
		delete(tm.tasks, id)
		tm.mu.Unlock()
	}()
	return nil
}

// StopTask cancels a running task by its ID.
func (tm *TaskManager) StopTask(id string) error {
	tm.mu.Lock()
	cancel, exists := tm.tasks[id]
	tm.mu.Unlock()
	if !exists {
		return errors.New("task not found")
	}
	cancel()
	return nil
}

// StopAll stops every running task.
func (tm *TaskManager) StopAll() {
	tm.mu.Lock()
	for id, cancel := range tm.tasks {
		cancel()
		delete(tm.tasks, id)
	}
	tm.mu.Unlock()
}

// GetTaskManager returns the singleton instance.
func GetTaskManager() *TaskManager {
	return defaultTaskManager
}
