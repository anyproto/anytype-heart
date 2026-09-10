package export

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/process"
)

type canceledExportQueue struct {
	process.Queue
	started chan struct{}
	tasks   []process.Task
}

func (q *canceledExportQueue) Wait(tasks ...process.Task) error {
	q.tasks = tasks
	go tasks[0]()
	<-q.started
	return process.ErrQueueCanceled
}

func TestCanceledExportDrainsActiveTasks(t *testing.T) {
	q := &canceledExportQueue{started: make(chan struct{})}
	release := make(chan struct{})
	var finished, ranLate atomic.Bool
	done := make(chan error, 1)
	go func() {
		done <- waitExportTasks(q, func() {
			close(q.started)
			<-release
			finished.Store(true)
		}, func() { ranLate.Store(true) })
	}()
	<-q.started
	select {
	case <-done:
		close(release)
		t.Fatal("export returned while a worker was still writing")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	require.ErrorIs(t, <-done, context.Canceled)
	assert.True(t, finished.Load())
	// A queued task dispatched after cancellation must not touch the output
	// or change the report that has already been returned.
	q.tasks[1]()
	assert.False(t, ranLate.Load())
}
