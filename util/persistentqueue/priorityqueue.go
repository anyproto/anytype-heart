package persistentqueue

import (
	"fmt"
	"sync"
)

var errClosed = fmt.Errorf("closed")

type priorityQueue[T any] struct {
	closed bool
	cond   *sync.Cond
	items  []T
}

func newPriorityQueue[T any](bufSize int) *priorityQueue[T] {
	q := &priorityQueue[T]{
		cond: &sync.Cond{
			L: &sync.Mutex{},
		},
		items: make([]T, 0, bufSize),
	}
	return q
}

func (q *priorityQueue[T]) add(item T) error {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()

	if q.closed {
		return errClosed
	}
	q.items = append(q.items, item)
	q.cond.Signal()
	return nil
}

func (q *priorityQueue[T]) close() {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()

	if q.closed {
		return
	}
	q.closed = true
	q.cond.Broadcast()
}

func (q *priorityQueue[T]) waitOne() (T, error) {
	q.cond.L.Lock()
	for {
		if q.closed {
			q.cond.L.Unlock()
			var defaultVal T
			return defaultVal, errClosed
		}
		if len(q.items) == 0 {
			q.cond.Wait()
		} else {
			break
		}
	}
	it := q.items[len(q.items)-1]
	q.items = q.items[:len(q.items)-1]
	q.cond.L.Unlock()

	return it, nil
}
