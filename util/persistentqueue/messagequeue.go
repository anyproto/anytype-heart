package persistentqueue

import (
	"fmt"
	"sync"
)

var errClosed = fmt.Errorf("closed")

type messageQueue[T any] struct {
	closed bool
	cond   *sync.Cond
	items  *priorityQueue[T]
}

func newMessageQueue[T any](lessFunc func(one, other T) bool) *messageQueue[T] {
	q := &messageQueue[T]{
		cond: &sync.Cond{
			L: &sync.Mutex{},
		},
		items: newPriorityQueue[T](lessFunc),
	}
	return q
}

func (q *messageQueue[T]) add(item T) error {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()

	if q.closed {
		return errClosed
	}
	q.items.push(item)
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
	defer q.cond.L.Unlock()

	var defaultVal T

	for {
		if q.closed {
			return defaultVal, errClosed
		}
		if q.items.Len() == 0 {
			q.cond.Wait()
		} else {
			break
		}
	}
	it, ok := q.items.pop()
	if !ok {
		return defaultVal, fmt.Errorf("integrity violation")
	}
	return it, nil
}
