/*
Package taskmanager implements a priority-based task queue that supports pausing, resuming, and dynamically adjusting task priorities.

This package provides a flexible and efficient way to manage tasks that need to be processed in a specific order based on their priority. Tasks can be added to the queue with an assigned priority, and higher-priority tasks are processed first. Additionally, tasks can be paused and resumed when their priority changes, ensuring that resources are allocated appropriately.

Features:
- Add tasks to the queue
- Automatically reorders tasks based on their priority.
- Pause and resume tasks dynamically based on priority changes.
*/
package taskmanager

import (
	"context"
	"sync"

	"github.com/anyproto/any-sync/app/logger"
	"go.uber.org/zap"
	"golang.org/x/exp/maps"
)

var log = logger.NewNamed("taskmanager")

func PrioritySorterAsIs(ids []string) []string {
	return ids
}

// TasksManager manages tasks based on a dynamic priority list and configurable concurrency.
type TasksManager struct {
	tasks                map[string]taskWrapper
	prioritySorterFilter func(ids []string) []string
	priorityList         []string
	priorityRefreshed    chan struct{}
	taskAddCh            chan taskWrapper
	mu                   sync.Mutex
	currentTasks         map[string]taskWrapper
	maxConcurrent        int
	wg                   sync.WaitGroup
	closed               chan struct{}
	started              chan struct{}
	taskFinishedCh       chan string
}

func NewTasksManager(maxConcurrent int, prioritySorterFilter func(ids []string) []string) *TasksManager {
	return &TasksManager{
		tasks:                make(map[string]taskWrapper),
		prioritySorterFilter: prioritySorterFilter,
		priorityList:         []string{},
		priorityRefreshed:    make(chan struct{}),
		taskAddCh:            make(chan taskWrapper),
		currentTasks:         make(map[string]taskWrapper),
		maxConcurrent:        maxConcurrent,
		taskFinishedCh:       make(chan string),
		closed:               make(chan struct{}),
		started:              make(chan struct{}),
	}
}

// AddTask adds a task to the manager.
// If the manager is closed, it panics.
func (qm *TasksManager) AddTask(task Task) {
	taskWrapped := taskWrapper{Task: task, taskFinishedCh: qm.taskFinishedCh}
managerStateSelect:
	select {
	case <-qm.closed:
		panic("cannot add task to closed TasksManager")
	case <-qm.started:

	default:
		qm.mu.Lock()
		// double check under the lock to protect from race conditions
		select {
		case <-qm.started:
			qm.mu.Unlock()
			break managerStateSelect
		default:
		}
		qm.wg.Add(1)
		qm.tasks[task.ID()] = taskWrapped
		qm.priorityList = qm.prioritySorterFilter(maps.Keys(qm.tasks))
		qm.mu.Unlock()
		return
	}
	qm.wg.Add(1)
	// by default, tasks are paused until the manager starts it
	qm.taskAddCh <- taskWrapped
}

// RefreshPriority updates the priority list of the manager
// it should be used when prioritySorter should be re-run for the existing list, e.g. when the priority function is based on external data
func (qm *TasksManager) RefreshPriority() {
managerStateSelect:
	select {
	case <-qm.closed:
		panic("cannot update priority of closed TasksManager")
	case <-qm.started:
		break
	default:
		qm.mu.Lock()
		// double check under the lock to protect from race cond
		select {
		case <-qm.started:
			qm.mu.Unlock()
			break managerStateSelect
		default:

		}

		qm.priorityList = qm.prioritySorterFilter(maps.Keys(qm.tasks))
		qm.mu.Unlock()
		return
	}
	qm.priorityRefreshed <- struct{}{}
}

// WaitAndClose waits for all tasks to finish and then closes the manager.
// MUST be called once, otherwise panics
func (qm *TasksManager) WaitAndClose() {
	qm.wg.Wait()
	close(qm.closed)
}

// Run starts the task manager's queue main loop.
// Should be called only once, next calls are no-ops.
// returns when the manager is closed(WaitAndClose is called)
func (qm *TasksManager) Run(ctx context.Context) {
	select {
	case <-qm.closed:
		panic("called Run on closed TasksManager")
	case <-qm.started:
		return
	default:
	}
	qm.mu.Lock()
	for _, task := range qm.tasks {
		go task.Run(ctx)
	}
	close(qm.started)
	qm.manageTasks()
	qm.mu.Unlock()

	for {
		select {
		case <-qm.priorityRefreshed:
			qm.mu.Lock()
			qm.priorityList = qm.prioritySorterFilter(maps.Keys(qm.tasks))
			qm.manageTasks()
			qm.mu.Unlock()
		case task := <-qm.taskAddCh:
			qm.mu.Lock()
			qm.tasks[task.ID()] = task
			qm.priorityList = qm.prioritySorterFilter(maps.Keys(qm.tasks))
			go task.Run(ctx)
			qm.manageTasks()
			qm.mu.Unlock()
		case finishedTaskID := <-qm.taskFinishedCh:
			qm.mu.Lock()
			delete(qm.currentTasks, finishedTaskID)
			qm.manageTasks()
			qm.mu.Unlock()
			qm.wg.Done() // closed chan closed only after wg.Wait in WaitAndClose()
		case <-qm.closed:
			return
		}
	}
}

// manageTasks MUST be run under the lock
func (qm *TasksManager) manageTasks() {
	desiredTasks := make(map[string]taskWrapper)
	runningCount := 0

	if len(qm.priorityList) != len(qm.tasks) {
		var tasksWithMissingPriority []string
		for taskId := range qm.tasks {
			if _, exists := desiredTasks[taskId]; !exists {
				tasksWithMissingPriority = append(tasksWithMissingPriority, taskId)
			}
		}
		log.With(zap.Int("count", len(tasksWithMissingPriority))).With(zap.Strings("ids", tasksWithMissingPriority)).Warn("priority list inconsistency detected, some tasks will not be started")
	}
	// Determine which tasks should be running based on priority and max concurrency
	for _, taskID := range qm.priorityList {
		if runningCount >= qm.maxConcurrent {
			break
		}
		task, exists := qm.tasks[taskID]
		if exists && !task.isDone() {
			desiredTasks[taskID] = task
			runningCount++
		}
	}

	// Pause tasks that are running but no longer desired
	for taskID, task := range qm.currentTasks {
		if _, shouldRun := desiredTasks[taskID]; !shouldRun {
			task.pause()
			delete(qm.currentTasks, taskID)
		}
	}

	// Resume tasks that are desired but not currently running
	for taskID, task := range desiredTasks {
		if _, isRunning := qm.currentTasks[taskID]; !isRunning {
			task.resume()
			qm.currentTasks[taskID] = task
		}
	}
}
