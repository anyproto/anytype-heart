package persistentqueue

import (
	"fmt"
	"sync"
)

var errClosed = fmt.Errorf("closed")

type messageQueue[T any] struct {
	closed bool
	cond   *sync.Cond
	items  []T
}

func newMessageQueue[T any](bufSize int) *messageQueue[T] {
	q := &messageQueue[T]{
		cond: &sync.Cond{
			L: &sync.Mutex{},
		},
		items: make([]T, 0, bufSize),
	}
	return q
}

func (q *messageQueue[T]) add(item T) error {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()

	if q.closed {
		return errClosed
	}
	q.items = append(q.items, item)
	q.cond.Signal()
	return nil
}

func (q *messageQueue[T]) close() {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()

	if q.closed {
		return
	}
	q.closed = true
	q.cond.Broadcast()
}

func (q *messageQueue[T]) waitOne() (T, error) {
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
