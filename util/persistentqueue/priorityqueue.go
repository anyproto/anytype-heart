package persistentqueue

import "container/heap"

type priorityQueue[T any] struct {
	items    []T
	lessFunc func(one, other T) bool
}

func newPriorityQueue[T any](lessFunc func(one, other T) bool) *priorityQueue[T] {
	return &priorityQueue[T]{
		lessFunc: lessFunc,
	}
}

func (q *priorityQueue[T]) push(item T) {
	heap.Push(q, item)
}

func (q *priorityQueue[T]) initWith(items []T) {
	q.items = append(q.items, items...)
	heap.Init(q)
}

func (q *priorityQueue[T]) pop() (T, bool) {
	if q.Len() == 0 {
		var defaultValue T
		return defaultValue, false
	}
	it := heap.Pop(q).(T)
	return it, true
}

func (q *priorityQueue[T]) Len() int {
	return len(q.items)
}

func (q *priorityQueue[T]) Less(i, j int) bool {
	return q.lessFunc(q.items[i], q.items[j])
}

func (q *priorityQueue[T]) Swap(i, j int) {
	q.items[i], q.items[j] = q.items[j], q.items[i]
}

func (q *priorityQueue[T]) Push(x any) {
	item := x.(T)
	q.items = append(q.items, item)
}

func (q *priorityQueue[T]) Pop() any {
	item := q.items[len(q.items)-1]
	q.items = q.items[0 : len(q.items)-1]
	return item
}
