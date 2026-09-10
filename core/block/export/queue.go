package export

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/anyproto/anytype-heart/core/block/process"
)

// Queue.Wait can return on cancellation while workers are still running.
// Drain started tasks before taking a report snapshot or removing export files.
func waitExportTasks(queue process.Queue, tasks ...process.Task) error {
	var mu sync.Mutex
	var active sync.WaitGroup
	stopped := false
	queued := make([]process.Task, 0, len(tasks))
	for _, task := range tasks {
		queued = append(queued, func() {
			mu.Lock()
			if stopped {
				mu.Unlock()
				return
			}
			active.Add(1)
			mu.Unlock()
			defer active.Done()
			task()
		})
	}
	err := queue.Wait(queued...)
	mu.Lock()
	stopped = true
	mu.Unlock()
	active.Wait()
	if errors.Is(err, process.ErrQueueCanceled) {
		return context.Canceled
	}
	if err != nil {
		return fmt.Errorf("run export tasks: %w", err)
	}
	return nil
}
