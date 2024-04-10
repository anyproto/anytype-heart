package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cheggaaa/mb/v3"
	"github.com/dgraph-io/badger/v4"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/util/badgerhelper"
)

var errRemoved = fmt.Errorf("removed from queue")

type Item interface {
	Key() string
}

type ItemWithOrder interface {
	Item
	Less(other ItemWithOrder) bool
}

type Action int

const (
	ActionRetry Action = iota
	ActionDone
)

type FactoryFunc[T Item] func() T

type HandlerFunc[T Item] func(context.Context, T) (Action, error)

type Queue[T Item] struct {
	db           *badger.DB
	logger       *zap.Logger
	badgerPrefix []byte
	batcher      *mb.MB[T]
	factoryFunc  FactoryFunc[T]
	handler      HandlerFunc[T]
	options      options
	handledItems uint32

	lock sync.Mutex
	// set is used to keep track of queued items. If item has been added to queue and removed without processing
	// it will be still in batcher, so we need a separate variable to track removed items
	set map[string]struct{}

	ctx       context.Context
	ctxCancel context.CancelFunc
}

type options struct {
	handlerTickPeriod time.Duration
}

type Option func(*options)

func WithHandlerTickPeriod(period time.Duration) Option {
	return func(o *options) {
		o.handlerTickPeriod = period
	}
}

func New[T Item](
	db *badger.DB,
	logger *zap.Logger,
	badgerPrefix []byte,
	factoryFunc FactoryFunc[T],
	handler HandlerFunc[T],
	opts ...Option,
) *Queue[T] {
	q := &Queue[T]{
		logger:       logger,
		db:           db,
		badgerPrefix: badgerPrefix,
		batcher:      mb.New[T](0),
		handler:      handler,
		factoryFunc:  factoryFunc,
		set:          make(map[string]struct{}),
		options:      options{},
	}
	for _, opt := range opts {
		opt(&q.options)
	}
	q.ctx, q.ctxCancel = context.WithCancel(context.Background())
	err := q.restore()
	if err != nil {
		q.logger.Error("can't restore queue", zap.String("prefix", string(q.badgerPrefix)), zap.Error(err))
	}
	return q
}

func (q *Queue[T]) Run() {
	go q.loop()
}

func (q *Queue[T]) loop() {
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
		if q.options.handlerTickPeriod > 0 {
			select {
			case <-time.After(q.options.handlerTickPeriod):
			case <-q.ctx.Done():
				return
			}
		}
	}

}

func (q *Queue[T]) handleNext() error {
	it, err := q.batcher.WaitOne(q.ctx)
	if err != nil {
		return fmt.Errorf("wait one: %w", err)
	}
	ok := q.Has(it.Key())
	if !ok {
		return errRemoved
	}

	action, err := q.handler(q.ctx, it)
	atomic.AddUint32(&q.handledItems, 1)
	switch action {
	case ActionDone:
		removeErr := q.Remove(it.Key())
		if removeErr != nil {
			return fmt.Errorf("remove from queue: %w", removeErr)
		}
	case ActionRetry:
		addErr := q.batcher.Add(q.ctx, it)
		if addErr != nil {
			return fmt.Errorf("add to queue: %w", addErr)
		}
	}
	if err != nil {
		return fmt.Errorf("handler: %w", err)
	}
	return nil
}

func (q *Queue[T]) restore() error {
	items, err := q.listItemsByPrefix()
	if err != nil {
		return fmt.Errorf("list items: %w", err)
	}
	err = q.batcher.Add(q.ctx, items...)
	if err != nil {
		return fmt.Errorf("add to queue: %w", err)
	}
	for _, it := range items {
		q.set[it.Key()] = struct{}{}
	}
	return nil
}

func (q *Queue[T]) listItemsByPrefix() ([]T, error) {
	var items []T
	err := q.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.IteratorOptions{
			PrefetchSize:   100,
			PrefetchValues: true,
			Prefix:         q.badgerPrefix,
		})
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			qItem, err := q.unmarshalItem(item)
			if err != nil {
				return fmt.Errorf("get queue item %s: %w", item.Key(), err)
			}
			items = append(items, qItem)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	if len(items) > 0 {
		var itemIface Item = items[0]
		if _, ok := itemIface.(ItemWithOrder); ok {
			sort.Slice(items, func(i, j int) bool {
				var left Item = items[i]
				var right Item = items[j]
				return left.(ItemWithOrder).Less(right.(ItemWithOrder))
			})
		}
	}

	return items, nil
}

func (q *Queue[T]) unmarshalItem(item *badger.Item) (T, error) {
	it := q.factoryFunc()
	err := item.Value(func(raw []byte) error {
		return json.Unmarshal(raw, it)
	})
	if err != nil {
		return it, err
	}
	return it, nil
}

func (q *Queue[T]) Close() error {
	q.ctxCancel()
	return q.batcher.Close()
}

func (q *Queue[T]) makeKey(itemKey string) []byte {
	return append(q.badgerPrefix, []byte(itemKey)...)
}

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
	return q.store(item)
}

func (q *Queue[T]) Has(key string) bool {
	q.lock.Lock()
	defer q.lock.Unlock()
	_, ok := q.set[key]
	return ok
}

func (q *Queue[T]) store(item T) error {
	raw, err := json.Marshal(item)
	if err != nil {
		return fmt.Errorf("create queue item: %w", err)
	}
	return badgerhelper.SetValue(q.db, q.makeKey(item.Key()), raw)
}

func (q *Queue[T]) Remove(key string) error {
	err := q.checkClosed()
	if err != nil {
		return err
	}
	q.lock.Lock()
	defer q.lock.Unlock()
	delete(q.set, key)
	return badgerhelper.DeleteValue(q.db, q.makeKey(key))
}

func (q *Queue[T]) HandledItems() int {
	return int(atomic.LoadUint32(&q.handledItems))
}

// ListKeys lists queued but not yet handled keys
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
