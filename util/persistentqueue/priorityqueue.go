package persistentqueue

import "container/heap"

type itemWithPriority[T any] struct {
	item     T
	priority int
}

type priorityQueue[T any] struct {
	items []itemWithPriority[T]
}

func (q *priorityQueue[T]) Len() int {
	return len(q.items)
}

func (q *priorityQueue[T]) Less(i, j int) bool {
	return q.items[i].priority > q.items[j].priority
}

func (q *priorityQueue[T]) Swap(i, j int) {
	q.items[i], q.items[j] = q.items[j], q.items[i]
}

func (q *priorityQueue[T]) Push(x any) {
	item := x.(itemWithPriority[T])
	q.items = append(q.items, item)
}

func (q *priorityQueue[T]) Pop() any {
	item := q.items[len(q.items)-1]
	q.items = q.items[0 : len(q.items)-1]
	return item
}

func newPriorityQueue[T any]() *priorityQueue[T] {
	return &priorityQueue[T]{}
}

func (q *priorityQueue[T]) push(item T, priority int) {
	heap.Push(q, itemWithPriority[T]{item: item, priority: priority})
}

func (q *priorityQueue[T]) pop() (T, bool) {
	if q.Len() == 0 {
		var defaultValue T
		return defaultValue, false
	}
	it := heap.Pop(q).(itemWithPriority[T])
	return it.item, true
}
