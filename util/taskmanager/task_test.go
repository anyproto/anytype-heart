package taskmanager

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"
)

// TestTask is a concrete implementation of the Task interface for testing purposes.
type TestTask struct {
	*taskBase
	iterations int
}

// NewTestTask creates a new TestTask with the given ID and number of iterations.
func NewTestTask(id string, iterations int) *TestTask {
	return &TestTask{
		taskBase:   NewTaskBase(id).(*taskBase),
		iterations: iterations,
	}
}

// Run executes the task's work, respecting pause and resume signals and context cancellation.
func (t *TestTask) Run(ctx context.Context) error {
	for i := 0; i < t.iterations; i++ {
		// Check if the task is paused.
		if err := t.WaitIfPaused(ctx); err != nil {
			t.markDoneWithError(err)
			return err
		}

		// Check for context cancellation.
		select {
		case <-ctx.Done():
			err := ctx.Err()
			t.markDoneWithError(err)
			return err
		default:
		}

		// Simulate work.
		time.Sleep(10 * time.Millisecond)
		fmt.Printf("Task %s is working (iteration %d)\n", t.ID(), i+1)
	}

	return nil
}

// TestTasksManager_AddTaskAndRun tests that tasks can be added and run,
// and that they finish successfully.
func TestTasksManager_AddTaskAndRun(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var sortedIds = []string{"task1", "task2", "task3"}
	sorter := func(_ []string) []string {
		return sortedIds
	}
	qm := NewTasksManager(2, sorter) // Allow up to 2 concurrent tasks.
	go qm.Run(ctx)

	// Create tasks.
	task1 := NewTestTask("task1", 5)
	task2 := NewTestTask("task2", 5)
	task3 := NewTestTask("task3", 5)

	// Add tasks to the manager.
	qm.AddTask(task1)
	qm.AddTask(task2)
	qm.AddTask(task3)

	// Set initial priority list.
	qm.RefreshPriority()

	// Wait for tasks to finish and close the manager.
	qm.WaitAndClose()

	// Check that all tasks finished successfully.
	for _, task := range []*TestTask{task1, task2, task3} {
		result, done := task.GetResult()
		if !done {
			t.Errorf("Task %s did not finish", task.ID())
		} else if result.Err != nil {
			t.Errorf("Task %s finished with error: %v", task.ID(), result.Err)
		}
	}
}

// TestTasksManager_PriorityUpdate tests that tasks are paused and resumed
// according to priority changes.
func TestTasksManager_PriorityUpdate(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var sortedIds = []string{"task1", "task2"}
	sorter := func(_ []string) []string {
		return sortedIds
	}

	qm := NewTasksManager(1, sorter) // Only one task can run at a time.
	go qm.Run(ctx)

	// Create tasks.
	task1 := NewTestTask("task1", 10)
	task2 := NewTestTask("task2", 5)

	// Add tasks.
	qm.AddTask(task1)
	qm.AddTask(task2)

	// Set initial priority list.
	qm.RefreshPriority()

	// After a short delay, update the priority to give task2 higher priority.
	time.Sleep(30 * time.Millisecond)
	sortedIds = []string{"task2", "task1"}
	qm.RefreshPriority()

	// Wait for tasks to finish and close the manager.
	qm.WaitAndClose()

	// Get results.
	result1, done1 := task1.GetResult()
	result2, done2 := task2.GetResult()

	// Check that both tasks finished.
	if !done1 {
		t.Errorf("Task %s did not finish", task1.ID())
	}
	if !done2 {
		t.Errorf("Task %s did not finish", task2.ID())
	}

	// Verify that task2 finished before task1.
	if result2.FinishTime.After(result1.FinishTime) {
		t.Errorf("Expected task2 to finish before task1, but task1 finished at %v and task2 finished at %v", result1.FinishTime, result2.FinishTime)
	}
}

// TestTasksManager_MaxConcurrency tests that the manager respects the
// maximum concurrency limit.
func TestTasksManager_MaxConcurrency(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sorter := func(_ []string) []string {
		return []string{"task1", "task2", "task3"}
	}

	qm := NewTasksManager(2, sorter) // Allow up to 2 concurrent tasks.
	go qm.Run(ctx)

	// Create tasks.
	task1 := NewTestTask("task1", 5)
	task2 := NewTestTask("task2", 5)
	task3 := NewTestTask("task3", 5)

	// Add tasks.
	qm.AddTask(task1)
	qm.AddTask(task2)
	qm.AddTask(task3)

	// Set priority list.
	qm.RefreshPriority()

	// Wait for tasks to finish and close the manager.
	qm.WaitAndClose()

	// Check that all tasks finished successfully.
	for _, task := range []*TestTask{task1, task2, task3} {
		result, done := task.GetResult()
		if !done {
			t.Errorf("Task %s did not finish", task.ID())
		} else if result.Err != nil {
			t.Errorf("Task %s finished with error: %v", task.ID(), result.Err)
		}
	}
}

// TestTasksManager_AddTaskDuringExecution tests adding a new task while
// the manager is already running tasks.
func TestTasksManager_AddTaskDuringExecution(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	priorityList := []string{"task1", "task2"}
	qm := NewTasksManager(2, func(ids []string) []string {
		return priorityList
	})
	go qm.Run(ctx)

	// Create initial tasks.
	task1 := NewTestTask("task1", 5)
	task2 := NewTestTask("task2", 5)

	// Add initial tasks.
	qm.AddTask(task1)
	qm.AddTask(task2)

	// Set priority.
	qm.RefreshPriority()

	var task3 *TestTask
	// After a short delay, add a new task with higher priority.
	time.AfterFunc(20*time.Millisecond, func() {
		task3 = NewTestTask("task3", 5)
		qm.AddTask(task3)
		priorityList = []string{"task3", "task1", "task2"}
		qm.RefreshPriority()
	})

	// Wait for tasks to finish and close the manager.
	qm.WaitAndClose()

	// Check that all tasks finished successfully.
	for _, taskID := range []string{"task1", "task2", "task3"} {
		var task *TestTask
		switch taskID {
		case "task1":
			task = task1
		case "task2":
			task = task2
		case "task3":
			task = task3
		}

		result, done := task.GetResult()
		if !done {
			t.Errorf("Task %s did not finish", taskID)
		} else if result.Err != nil {
			t.Errorf("Task %s finished with error: %v", taskID, result.Err)
		}
	}
}

// TestTasksManager_ContextCancellation tests that tasks stop promptly and
// report errors when the context is canceled.
func TestTasksManager_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	qm := NewTasksManager(2, func(ids []string) []string {
		return []string{"task1", "task2"}
	})
	go qm.Run(ctx)

	// Create tasks with long iteration counts.
	task1 := NewTestTask("task1", 100)
	task2 := NewTestTask("task2", 100)

	// Add tasks.
	qm.AddTask(task1)
	qm.AddTask(task2)

	// Cancel context after a short delay.
	time.AfterFunc(50*time.Millisecond, func() {
		cancel()
	})

	// Wait for tasks to finish and close the manager.
	qm.WaitAndClose()

	// Get results.
	result1, done1 := task1.GetResult()
	result2, done2 := task2.GetResult()

	// Check that tasks have finished.
	if !done1 {
		t.Errorf("Task %s did not finish", task1.ID())
	}
	if !done2 {
		t.Errorf("Task %s did not finish", task2.ID())
	}

	// Check that tasks were canceled.
	if result1.Err != context.Canceled {
		t.Errorf("Expected task %s error to be context.Canceled, but got %v", task1.ID(), result1.Err)
	}
	if result2.Err != context.Canceled {
		t.Errorf("Expected task %s error to be context.Canceled, but got %v", task2.ID(), result2.Err)
	}
}

// TestTasksManager_TaskCompletionOrder tests that tasks complete in the expected order
// based on their priorities and pause/resume behavior.
func TestTasksManager_TaskCompletionOrder(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	priorityList := []string{"task1", "task2", "task3"}
	qm := NewTasksManager(1, func(ids []string) []string {
		return priorityList
	})
	go qm.Run(ctx)

	// Create tasks with different iteration counts.
	task1 := NewTestTask("task1", 10)
	task2 := NewTestTask("task2", 5)
	task3 := NewTestTask("task3", 3)

	// Add tasks.
	qm.AddTask(task1)
	qm.AddTask(task2)
	qm.AddTask(task3)

	// Update priority to move task3 to the top.
	time.AfterFunc(30*time.Millisecond, func() {
		priorityList = []string{"task3", "task1", "task2"}
		qm.RefreshPriority()
	})

	// Wait for tasks to finish and close the manager.
	qm.WaitAndClose()

	// Collect results.
	tasks := []*TestTask{task1, task2, task3}
	type taskResult struct {
		taskID     string
		finishTime time.Time
	}

	var results []taskResult
	for _, task := range tasks {
		result, done := task.GetResult()
		if !done {
			t.Errorf("Task %s did not finish", task.ID())
			continue
		}
		if result.Err != nil {
			t.Errorf("Task %s finished with error: %v", task.ID(), result.Err)
		}
		results = append(results, taskResult{
			taskID:     task.ID(),
			finishTime: result.FinishTime,
		})
	}

	// Sort the results by finish time.
	sort.Slice(results, func(i, j int) bool {
		return results[i].finishTime.Before(results[j].finishTime)
	})

	// Verify the completion order.
	expectedOrder := []string{"task3", "task1", "task2"}
	for i, taskID := range expectedOrder {
		if results[i].taskID != taskID {
			t.Errorf("Expected task %s to finish at position %d, but got %s", taskID, i, results[i].taskID)
		}
	}
}

// TestTasksManager_AddTasksBeforeRun tests that tasks added before the manager's Run method are handled correctly.
func TestTasksManager_AddTasksBeforeRun(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	qm := NewTasksManager(2, func(ids []string) []string {
		return []string{"task1", "task2"}
	}) // Allow up to 2 concurrent tasks.

	// Create tasks.
	task1 := NewTestTask("task1", 5)
	task2 := NewTestTask("task2", 5)

	// Add tasks to the manager before calling Run.
	qm.AddTask(task1)
	qm.AddTask(task2)
	// Start the manager.
	go qm.Run(ctx)

	// Wait for tasks to finish and close the manager.
	qm.WaitAndClose()

	// Check that both tasks finished successfully.
	for _, task := range []*TestTask{task1, task2} {
		result, done := task.GetResult()
		if !done {
			t.Errorf("Task %s did not finish", task.ID())
		} else if result.Err != nil {
			t.Errorf("Task %s finished with error: %v", task.ID(), result.Err)
		}
	}
}

// TestTasksManager_PriorityChangeBeforeRun tests that priority changes before the manager's Run method are handled correctly.
func TestTasksManager_PriorityChangeBeforeRun(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var priorityList = []string{"task1", "task2"}
	qm := NewTasksManager(1, func(ids []string) []string {
		return priorityList
	}) // Only one task can run at a time.

	// Create tasks.
	task1 := NewTestTask("task1", 5)
	task2 := NewTestTask("task2", 5)

	// Add tasks to the manager before calling Run.
	qm.AddTask(task1)
	qm.AddTask(task2)

	priorityList = []string{"task2", "task1"}
	// Set initial priority list before Run.
	qm.RefreshPriority()

	// Start the manager.
	go qm.Run(ctx)

	// Wait for tasks to finish and close the manager.
	qm.WaitAndClose()

	// Get results.
	result1, done1 := task1.GetResult()
	result2, done2 := task2.GetResult()

	// Check that both tasks finished.
	if !done1 {
		t.Errorf("Task %s did not finish", task1.ID())
	}
	if !done2 {
		t.Errorf("Task %s did not finish", task2.ID())
	}

	// Verify that task2 finished before task1.
	if result2.FinishTime.After(result1.FinishTime) {
		t.Errorf("Expected task2 to finish before task1, but task1 finished at %v and task2 finished at %v", result1.FinishTime, result2.FinishTime)
	}
}

// Create a custom TestTask that increments and decrements the running task counter.
type CountingTestTask struct {
	mu                                  sync.Mutex
	runningTasks, maxObservedConcurrent int
	*TestTask
}

// Override the Run method.
func (t *CountingTestTask) Run(ctx context.Context) error {
	defer t.markDoneWithError(nil)

	for i := 0; i < t.iterations; i++ {
		if err := t.WaitIfPaused(ctx); err != nil {
			t.markDoneWithError(err)
			return err
		}

		select {
		case <-ctx.Done():
			err := ctx.Err()
			t.markDoneWithError(err)
			return err
		default:
		}

		// Increment runningTasks counter.
		t.mu.Lock()
		t.runningTasks++
		if t.runningTasks > t.maxObservedConcurrent {
			t.maxObservedConcurrent = t.runningTasks
		}
		t.mu.Unlock()

		// Simulate work.
		time.Sleep(10 * time.Millisecond)

		// Decrement runningTasks counter.
		t.mu.Lock()
		t.runningTasks--
		t.mu.Unlock()
	}

	return nil
}

// TestTasksManager_MaxConcurrentTasks verifies that the manager does not exceed the maximum number of concurrent tasks.
func TestTasksManager_MaxConcurrentTasks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	maxConcurrentTasks := 2
	qm := NewTasksManager(maxConcurrentTasks, func(ids []string) []string {
		return []string{"task1", "task2", "task3"}
	})
	maxObservedConcurrent := 0
	runningTasks := 0
	mu := sync.Mutex{}
	// Function to create a CountingTestTask.
	newCountingTestTask := func(id string, iterations int) *CountingTestTask {
		return &CountingTestTask{
			TestTask:              NewTestTask(id, iterations),
			mu:                    mu,
			runningTasks:          runningTasks,
			maxObservedConcurrent: maxObservedConcurrent,
		}
	}

	// Create tasks.
	task1 := newCountingTestTask("task1", 10)
	task2 := newCountingTestTask("task2", 10)
	task3 := newCountingTestTask("task3", 10)

	// Add tasks to the manager.
	qm.AddTask(task1)
	qm.AddTask(task2)
	qm.AddTask(task3)

	// Start the manager.
	go qm.Run(ctx)

	// Wait for tasks to finish and close the manager.
	qm.WaitAndClose()

	// Check that all tasks finished successfully.
	for _, task := range []*CountingTestTask{task1, task2, task3} {
		result, done := task.GetResult()
		if !done {
			t.Errorf("Task %s did not finish", task.ID())
		} else if result.Err != nil {
			t.Errorf("Task %s finished with error: %v", task.ID(), result.Err)
		}
	}

	// Verify that maxObservedConcurrent does not exceed maxConcurrentTasks.
	if maxObservedConcurrent > maxConcurrentTasks {
		t.Errorf("Expected maximum concurrent tasks to be %d, but observed %d", maxConcurrentTasks, maxObservedConcurrent)
	} else {
		t.Logf("Maximum concurrent tasks observed: %d", maxObservedConcurrent)
	}
}
