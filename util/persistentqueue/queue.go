package persistentqueue

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cheggaaa/mb/v3"
	"go.uber.org/zap"
)

var errRemoved = fmt.Errorf("removed from queue")

type Item interface {
	Key() string
}

type OrderedItem interface {
	Item
	Less(other OrderedItem) bool
}

// HandlerFunc is a function that processes an item from the queue.
// Input context will be canceled if queue is closed.
// Action result specifies if queue needs to retry item processing or mark it as done.
// Error is just logged.
type HandlerFunc[T Item] func(context.Context, T) (Action, error)

type Action int

func (a Action) String() string {
	switch a {
	case ActionRetry:
		return "retry"
	case ActionDone:
		return "done"
	default:
		return "unknown"
	}
}

const (
	ActionRetry Action = iota
	ActionDone
)

type handledWaiter struct {
	waitCh chan struct{}
}

// Queue represents a queue with persistent on-disk storage. Items handled one-by-one with one worker
type Queue[T Item] struct {
	storage Storage[T]
	logger  *zap.Logger

	batcher      *mb.MB[T]
	handler      HandlerFunc[T]
	options      options
	handledItems uint32

	lock sync.Mutex
	// set is used to keep track of queued items. If item has been added to queue and removed without processing
	// it will be still in batcher, so we need a separate variable to track removed items
	set map[string]struct{}

	currentProcessingKey *string
	waiters              []handledWaiter

	ctx       context.Context
	ctxCancel context.CancelFunc

	isStarted bool
	closedCh  chan struct{}
}

type options struct {
	retryPauseDuration time.Duration
	ctx                context.Context
}

type Option func(*options)

// WithRetryPause adds delay between handling items on ActionRetry
func WithRetryPause(duration time.Duration) Option {
	return func(o *options) {
		o.retryPauseDuration = duration
	}
}

func WithContext(ctx context.Context) Option {
	return func(o *options) {
		o.ctx = ctx
	}
}

func New[T Item](
	storage Storage[T],
	logger *zap.Logger,
	handler HandlerFunc[T],
	opts ...Option,
) *Queue[T] {
	q := &Queue[T]{
		storage:  storage,
		logger:   logger,
		batcher:  mb.New[T](0),
		handler:  handler,
		set:      make(map[string]struct{}),
		options:  options{},
		closedCh: make(chan struct{}),
	}
	for _, opt := range opts {
		opt(&q.options)
	}
	rootCtx := context.Background()
	if q.options.ctx != nil {
		rootCtx = q.options.ctx
	}
	q.ctx, q.ctxCancel = context.WithCancel(rootCtx)
	err := q.restore()
	if err != nil {
		q.logger.Error("can't restore queue", zap.Error(err))
	}
	return q
}

// Run starts queue processing. It will start only once
func (q *Queue[T]) Run() {
	q.lock.Lock()
	defer q.lock.Unlock()
	if q.isStarted {
		return
	}
	q.isStarted = true

	go q.loop()
}

func (q *Queue[T]) loop() {
	defer func() {
		close(q.closedCh)
	}()

	for {
		select {
		case <-q.ctx.Done():
			return
		default:
		}
		err := q.handleNext()
		if errors.Is(err, context.Canceled) {
			return
		}
		if errors.Is(err, errRemoved) {
			continue
		}
		if err != nil {
			q.logger.Error("handle next", zap.Error(err))
		}
	}

}

func (q *Queue[T]) handleNext() error {
	it, err := q.batcher.WaitOne(q.ctx)
	if err != nil {
		return fmt.Errorf("wait one: %w", err)
	}
	ok := q.checkExistsAndMarkAsProcessing(it.Key())
	if !ok {
		return errRemoved
	}

	action, err := q.handler(q.ctx, it)
	atomic.AddUint32(&q.handledItems, 1)
	switch action {
	case ActionDone:
		removeErr := q.removeAndNotifyWaiters(it.Key())
		if removeErr != nil {
			return fmt.Errorf("remove from queue: %w", removeErr)
		}
	case ActionRetry:
		q.lock.Lock()
		// We don't need to check that the item has been removed from queue here, it will be checked on next iteration
		// So just notify waiters that the item has been processed
		q.notifyWaiters()
		q.lock.Unlock()
		addErr := q.batcher.Add(q.ctx, it)
		if addErr != nil {
			return fmt.Errorf("add to queue: %w", addErr)
		}
		if q.options.retryPauseDuration > 0 {
			select {
			case <-time.After(q.options.retryPauseDuration):
			case <-q.ctx.Done():
				return context.Canceled
			}
		}
	}
	if err != nil {
		return fmt.Errorf("handler: %w", err)
	}
	return nil
}

func (q *Queue[T]) restore() error {
	items, err := q.storage.List()
	if err != nil {
		return fmt.Errorf("list items from storage: %w", err)
	}

	sortItems(items)

	err = q.batcher.Add(q.ctx, items...)
	if err != nil {
		return fmt.Errorf("add to queue: %w", err)
	}
	for _, it := range items {
		q.set[it.Key()] = struct{}{}
	}
	return nil
}

func sortItems[T Item](items []T) {
	if len(items) == 0 {
		return
	}
	var itemIface Item = items[0]
	if _, ok := itemIface.(OrderedItem); ok {
		sort.Slice(items, func(i, j int) bool {
			var left Item = items[i]
			var right Item = items[j]
			return left.(OrderedItem).Less(right.(OrderedItem))
		})
	}
}

// Close stops queue processing and waits for the last in-process item to be processed
func (q *Queue[T]) Close() error {
	q.ctxCancel()
	err := q.batcher.Close()
	if err != nil {
		q.logger.Error("close batcher", zap.Error(err))
	}
	q.lock.Lock()
	isStarted := q.isStarted
	q.lock.Unlock()

	if isStarted {
		<-q.closedCh
	}
	return nil
}

// Add item to If item with the same key already in queue, input item will be ignored
func (q *Queue[T]) Add(item T) error {
	err := q.checkClosed()
	if err != nil {
		return err
	}

	q.lock.Lock()
	if _, ok := q.set[item.Key()]; ok {
		q.lock.Unlock()
		return nil
	}
	q.set[item.Key()] = struct{}{}
	q.lock.Unlock()

	err = q.batcher.Add(q.ctx, item)
	if err != nil {
		return err
	}
	return q.storage.Put(item)
}

// Has returns true if item with specific key is in queue
func (q *Queue[T]) Has(key string) bool {
	q.lock.Lock()
	defer q.lock.Unlock()
	_, ok := q.set[key]
	return ok
}

func (q *Queue[T]) checkExistsAndMarkAsProcessing(key string) bool {
	q.lock.Lock()
	defer q.lock.Unlock()
	_, ok := q.set[key]
	if ok {
		q.currentProcessingKey = &key
	}
	return ok
}

func (q *Queue[T]) removeAndNotifyWaiters(key string) error {
	err := q.checkClosed()
	if err != nil {
		return err
	}
	q.lock.Lock()
	delete(q.set, key)
	q.notifyWaiters()
	q.lock.Unlock()

	return q.storage.Delete(key)
}

func (q *Queue[T]) notifyWaiters() {
	for _, w := range q.waiters {
		close(w.waitCh)
	}
	q.waiters = nil
	q.currentProcessingKey = nil
}

// Remove item with specified key from If this item is already in process, it will be processed.
// If you need to stop processing removable item, you should use own cancellation mechanism
func (q *Queue[T]) Remove(key string) error {
	err := q.checkClosed()
	if err != nil {
		return err
	}
	q.lock.Lock()
	defer q.lock.Unlock()
	delete(q.set, key)
	return q.storage.Delete(key)
}

func (q *Queue[T]) RemoveWait(key string) (chan struct{}, error) {
	err := q.checkClosed()
	if err != nil {
		return nil, err
	}
	q.lock.Lock()
	delete(q.set, key)
	waitCh := make(chan struct{})
	if q.currentProcessingKey != nil && *q.currentProcessingKey == key {
		// Channel will be closed after handling
		q.waiters = append(q.waiters, handledWaiter{
			waitCh: waitCh,
		})
		q.lock.Unlock()
	} else {
		close(waitCh)
		q.lock.Unlock()
	}
	err = q.storage.Delete(key)
	if err != nil {
		// Consume channel
		<-waitCh
		return nil, err
	}
	return waitCh, nil
}

// NumProcessedItems returns number of items processed by handler
func (q *Queue[T]) NumProcessedItems() int {
	return int(atomic.LoadUint32(&q.handledItems))
}

// ListKeys lists queued but not yet processed keys
func (q *Queue[T]) ListKeys() []string {
	q.lock.Lock()
	defer q.lock.Unlock()
	keys := make([]string, 0, len(q.set))
	for key := range q.set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// Len returns number of unprocessed items in queue
func (q *Queue[T]) Len() int {
	q.lock.Lock()
	defer q.lock.Unlock()
	return len(q.set)
}

func (q *Queue[T]) checkClosed() error {
	select {
	case <-q.ctx.Done():
		return q.ctx.Err()
	default:
	}
	return nil
}
