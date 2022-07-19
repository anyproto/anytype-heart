package threads

import (
	"container/heap"
)

const (
	DefaultPriority = iota
	HighPriority
	MaxPriority
)

type Operation interface {
	Type() string
	Id() string
	IsRetriable() bool
	Run() error
	OnFinish(err error)
}

func newOperationPriorityQueue() *operationPriorityQueue {
	return &operationPriorityQueue{
		pq: priorityQueue{},
	}
}

type operationPriorityQueue struct {
	pq priorityQueue
}

func (o *operationPriorityQueue) Size() int {
	return len(o.pq)
}

func (o *operationPriorityQueue) Push(item *Item) {
	heap.Push(&o.pq, item)
}

func (o *operationPriorityQueue) Pop() *Item {
	return heap.Pop(&o.pq).(*Item)
}

func (o *operationPriorityQueue) UpdatePriority(item *Item, priority int) {
	o.pq.update(item, priority)
}

type Item struct {
	value     Operation
	priority  int
	index     int
	attempt   int
	isRunning bool
}

type priorityQueue []*Item

func (pq priorityQueue) Len() int { return len(pq) }

func (pq priorityQueue) Less(i, j int) bool {
	return pq[i].priority > pq[j].priority
}

func (pq priorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}

func (pq *priorityQueue) Push(x interface{}) {
	n := len(*pq)
	item := x.(*Item)
	item.index = n
	*pq = append(*pq, item)
}

func (pq *priorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.index = -1
	*pq = old[0 : n-1]
	return item
}

func (pq *priorityQueue) update(item *Item, priority int) {
	item.priority = priority
	heap.Fix(pq, item.index)
}
