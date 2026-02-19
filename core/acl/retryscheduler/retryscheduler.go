package retryscheduler

import (
	"container/heap"
	"context"
	"sync"
	"time"
)

type TimeProvider interface {
	Now() time.Time
	NewTimer(d time.Duration) Timer
}

type Timer interface {
	C() <-chan time.Time
	Stop() bool
	Reset(d time.Duration) bool
}

type realTimeProvider struct{}

func (realTimeProvider) Now() time.Time {
	return time.Now()
}

func (realTimeProvider) NewTimer(d time.Duration) Timer {
	return &realTimer{timer: time.NewTimer(d)}
}

type realTimer struct {
	timer *time.Timer
}

func (t *realTimer) C() <-chan time.Time {
	return t.timer.C
}

func (t *realTimer) Stop() bool {
	return t.timer.Stop()
}

func (t *realTimer) Reset(d time.Duration) bool {
	return t.timer.Reset(d)
}

type Item[T any] struct {
	ID         string
	Value      T
	ExpiryTime time.Time
	Timeout    time.Duration
	index      int
}

type itemHeap[T any] []*Item[T]

func (h itemHeap[T]) Len() int { return len(h) }

func (h itemHeap[T]) Less(i, j int) bool {
	return h[i].ExpiryTime.Before(h[j].ExpiryTime)
}

func (h itemHeap[T]) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *itemHeap[T]) Push(x interface{}) {
	n := len(*h)
	item := x.(*Item[T])
	item.index = n
	*h = append(*h, item)
}

func (h *itemHeap[T]) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	old[n-1] = nil // avoid memory leak
	item.index = -1
	*h = old[0 : n-1]
	return item
}

type RetryScheduler[T any] struct {
	heap           itemHeap[T]
	items          map[string]*Item[T] // for O(1) lookup
	process        func(ctx context.Context, msg T) error
	shouldRetry    func(msg T, err error) bool
	defaultTimeout time.Duration
	maxTimeout     time.Duration
	timeProvider   TimeProvider

	mu     sync.Mutex
	ctx    context.Context
	cancel context.CancelFunc
	notify chan struct{}
	closed bool
}

type Config struct {
	DefaultTimeout time.Duration
	MaxTimeout     time.Duration
	TimeProvider   TimeProvider // Optional, defaults to real time
}

func NewRetryScheduler[T any](
	process func(ctx context.Context, msg T) error,
	shouldRetry func(msg T, err error) bool,
	config Config,
) *RetryScheduler[T] {
	ctx, cancel := context.WithCancel(context.Background())

	if config.DefaultTimeout == 0 {
		config.DefaultTimeout = 100 * time.Millisecond
	}
	if config.MaxTimeout == 0 {
		config.MaxTimeout = 30 * time.Second
	}
	if config.TimeProvider == nil {
		config.TimeProvider = realTimeProvider{}
	}

	return &RetryScheduler[T]{
		heap:           make(itemHeap[T], 0),
		items:          make(map[string]*Item[T]),
		process:        process,
		shouldRetry:    shouldRetry,
		defaultTimeout: config.DefaultTimeout,
		maxTimeout:     config.MaxTimeout,
		timeProvider:   config.TimeProvider,
		ctx:            ctx,
		cancel:         cancel,
		notify:         make(chan struct{}, 1),
	}
}

func (q *RetryScheduler[T]) Schedule(id string, value T, timeout time.Duration) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.closed {
		return context.Canceled
	}

	now := q.timeProvider.Now()
	expiryTime := now.Add(timeout)

	if existing, ok := q.items[id]; ok {
		existing.Value = value
		existing.ExpiryTime = expiryTime
		existing.Timeout = timeout
		heap.Fix(&q.heap, existing.index)
	} else {
		item := &Item[T]{
			ID:         id,
			Value:      value,
			ExpiryTime: expiryTime,
			Timeout:    timeout,
		}
		heap.Push(&q.heap, item)
		q.items[id] = item
	}

	select {
	case q.notify <- struct{}{}:
	default:
	}

	return nil
}

func (q *RetryScheduler[T]) Remove(id string) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if item, ok := q.items[id]; ok {
		delete(q.items, id)
		if item.index >= 0 && item.index < len(q.heap) {
			heap.Remove(&q.heap, item.index)
		}
	}
}

func (q *RetryScheduler[T]) Run() {
	go q.run()
}

func (q *RetryScheduler[T]) run() {
	var timer Timer

	for {
		q.mu.Lock()

		if len(q.heap) == 0 {
			q.mu.Unlock()

			select {
			case <-q.ctx.Done():
				return
			case <-q.notify:
				continue
			}
		}

		nextItem := q.heap[0]
		now := q.timeProvider.Now()
		waitDuration := nextItem.ExpiryTime.Sub(now)

		if waitDuration < 0 {
			waitDuration = 0
		}

		q.mu.Unlock()

		if timer == nil {
			timer = q.timeProvider.NewTimer(waitDuration)
		} else {
			timer.Reset(waitDuration)
		}

		select {
		case <-q.ctx.Done():
			timer.Stop()
			return

		case <-q.notify:
			if !timer.Stop() {
				select {
				case <-timer.C():
				default:
				}
			}
			continue

		case <-timer.C():
			q.processNextItem()
		}
	}
}

func (q *RetryScheduler[T]) processNextItem() {
	q.mu.Lock()

	if len(q.heap) == 0 {
		q.mu.Unlock()
		return
	}

	item := heap.Pop(&q.heap).(*Item[T])
	delete(q.items, item.ID)

	q.mu.Unlock()
	// nolint: nestif
	if err := q.process(q.ctx, item.Value); err != nil {
		if q.shouldRetry(item.Value, err) {
			originalTimeout := item.Timeout
			if originalTimeout == 0 {
				originalTimeout = q.defaultTimeout
			}

			newTimeout := time.Duration(float64(originalTimeout) * 1.5)
			if newTimeout > q.maxTimeout {
				newTimeout = q.maxTimeout
			}
			// nolint: errcheck
			q.Schedule(item.ID, item.Value, newTimeout)
		}
	}
}

func (q *RetryScheduler[T]) Close() error {
	q.mu.Lock()
	q.closed = true
	q.mu.Unlock()

	q.cancel()
	return nil
}

func (q *RetryScheduler[T]) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}
