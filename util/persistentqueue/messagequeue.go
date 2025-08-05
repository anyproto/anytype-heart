package persistentqueue

import (
	"context"
	"fmt"
	"sync"

	"github.com/cheggaaa/mb/v3"
)

var errClosed = fmt.Errorf("closed")

type messageQueue[T any] interface {
	add(item T) error
	initWith(items []T) error
	waitOne() (T, error)
	close() error
}

type simpleMessageQueue[T any] struct {
	ctx     context.Context
	batcher *mb.MB[T]
}

var _ messageQueue[any] = &simpleMessageQueue[any]{}

func newSimpleMessageQueue[T any](ctx context.Context) *simpleMessageQueue[T] {
	return &simpleMessageQueue[T]{
		ctx:     ctx,
		batcher: mb.New[T](0),
	}
}

func (s *simpleMessageQueue[T]) add(item T) error {
	return s.batcher.Add(s.ctx, item)
}

func (s *simpleMessageQueue[T]) initWith(items []T) error {
	for _, it := range items {
		err := s.add(it)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *simpleMessageQueue[T]) waitOne() (T, error) {
	return s.batcher.WaitOne(s.ctx)
}

func (s *simpleMessageQueue[T]) close() error {
	return s.batcher.Close()
}

type priorityMessageQueue[T any] struct {
	closed bool
	cond   *sync.Cond
	items  *priorityQueue[T]
}

var _ messageQueue[any] = &priorityMessageQueue[any]{}

func newPriorityMessageQueue[T any](lessFunc func(one, other T) bool) *priorityMessageQueue[T] {
	q := &priorityMessageQueue[T]{
		cond: &sync.Cond{
			L: &sync.Mutex{},
		},
		items: newPriorityQueue[T](lessFunc),
	}
	return q
}

func (q *priorityMessageQueue[T]) add(item T) error {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()

	if q.closed {
		return errClosed
	}
	q.items.push(item)
	q.cond.Signal()
	return nil
}

func (q *priorityMessageQueue[T]) initWith(items []T) error {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()

	q.items.initWith(items)
	return nil
}

func (q *priorityMessageQueue[T]) close() error {
	q.cond.L.Lock()
	defer q.cond.L.Unlock()

	if q.closed {
		return nil
	}
	q.closed = true
	q.cond.Broadcast()
	return nil
}

func (q *priorityMessageQueue[T]) waitOne() (T, error) {
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
