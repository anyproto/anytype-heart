package filequeue

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-store/query"
)

var ErrNotFound = fmt.Errorf("not found")
var ErrNoRows = fmt.Errorf("no rows")

type marshalFunc[T any] func(arena *anyenc.Arena, val T) *anyenc.Value
type unmarshalFunc[T any] func(v *anyenc.Value) (T, error)

type Storage[T any] struct {
	arena     *anyenc.Arena
	coll      anystore.Collection
	marshal   marshalFunc[T]
	unmarshal unmarshalFunc[T]
}

func NewStorage[T any](coll anystore.Collection, marshal marshalFunc[T], unmarshal unmarshalFunc[T]) *Storage[T] {
	return &Storage[T]{
		arena:     &anyenc.Arena{},
		coll:      coll,
		marshal:   marshal,
		unmarshal: unmarshal,
	}
}

func (s *Storage[T]) get(ctx context.Context, objectId string) (T, error) {
	doc, err := s.coll.FindId(ctx, objectId)
	if errors.Is(err, anystore.ErrDocNotFound) {
		var defVal T
		return defVal, ErrNotFound
	}

	return s.unmarshal(doc.Value())
}

func (s *Storage[T]) set(ctx context.Context, objectId string, file T) error {
	defer s.arena.Reset()

	val := s.marshal(s.arena, file)
	val.Set("id", s.arena.NewString(objectId))
	return s.coll.UpsertOne(ctx, val)
}

func (s *Storage[T]) delete(ctx context.Context, objectId string) error {
	return s.coll.DeleteId(ctx, objectId)
}

func (s *Storage[T]) query(ctx context.Context, filter query.Filter, order query.Sort, inMemoryFilter func(T) bool) (T, error) {
	var defVal T

	var sortArgs []any
	if order != nil {
		sortArgs = []any{order}
	}

	// Unfortunately, we can't use limit as we need to check row locks on the application level
	// TODO Maybe query items by some batch, for example 10 items at once
	iter, err := s.coll.Find(filter).Sort(sortArgs...).Iter(ctx)
	if err != nil {
		return defVal, fmt.Errorf("iter: %w", err)
	}
	defer iter.Close()

	for iter.Next() {
		doc, err := iter.Doc()
		if err != nil {
			return defVal, fmt.Errorf("read doc: %w", err)
		}

		val, err := s.unmarshal(doc.Value())
		if err != nil {
			return defVal, fmt.Errorf("unmarshal: %w", err)
		}

		if inMemoryFilter(val) {
			return val, nil
		}
	}

	return defVal, ErrNoRows
}

type getByIdRequest[T any] struct {
	objectId   string
	responseCh chan itemResponse[T]
}

type itemResponse[T any] struct {
	item T
	err  error
}

type getNextRequest[T any] struct {
	subscribe   bool
	storeFilter query.Filter
	storeOrder  query.Sort
	filter      func(T) bool
	scheduledAt func(T) time.Time

	responseCh chan itemResponse[T]
}

type scheduledItem[T any] struct {
	timer         *time.Timer
	cancelTimerCh chan struct{}
	item          T

	request    getNextRequest[T]
	responseCh chan itemResponse[T]
}

type releaseAction int

const (
	releaseActionNone = releaseAction(iota)
	releaseActionUpdate
	releaseActionDelete
)

type releaseRequest[T any] struct {
	objectId   string
	item       T
	action     releaseAction
	responseCh chan error
}

type Queue[T any] struct {
	ctx       context.Context
	ctxCancel context.CancelFunc

	store *Storage[T]

	getId func(T) string

	closeCh            chan struct{}
	getByIdCh          chan getByIdRequest[T]
	getNextCh          chan getNextRequest[T]
	getNextScheduledCh chan getNextRequest[T]
	releaseCh          chan releaseRequest[T]
	scheduledCh        chan scheduledItem[T]

	getByIdWaiters map[string][]chan itemResponse[T]
	dueWaiters     map[string][]scheduledItem[T]

	getNextWaiters          []getNextRequest[T]
	getNextScheduledWaiters []getNextRequest[T]

	taskLocked map[string]struct{}
	scheduled  map[string]scheduledItem[T]
}

func NewQueue[T any](store *Storage[T], getId func(T) string) *Queue[T] {
	ctx, ctxCancel := context.WithCancel(context.Background())
	return &Queue[T]{
		ctx:                ctx,
		ctxCancel:          ctxCancel,
		store:              store,
		closeCh:            make(chan struct{}),
		getByIdCh:          make(chan getByIdRequest[T]),
		getNextCh:          make(chan getNextRequest[T]),
		getNextScheduledCh: make(chan getNextRequest[T]),
		scheduledCh:        make(chan scheduledItem[T]),
		releaseCh:          make(chan releaseRequest[T]),
		getByIdWaiters:     make(map[string][]chan itemResponse[T]),
		taskLocked:         make(map[string]struct{}),
		scheduled:          make(map[string]scheduledItem[T]),
		dueWaiters:         map[string][]scheduledItem[T]{},
		getId:              getId,
	}
}

func (q *Queue[T]) run() {
	// TODO Think about deletion
	for {
		select {
		case <-q.closeCh:
			// TODO Close all waiters
			return
		case req := <-q.getByIdCh:
			q.handleGetById(req)
		case req := <-q.getNextScheduledCh:
			q.handleGetNextScheduled(req)
		case req := <-q.getNextCh:
			q.handleGetNext(req)
		case req := <-q.releaseCh:
			q.handleReleaseItem(req)
		case req := <-q.scheduledCh:
			q.handleScheduledItem(req)
		}
	}
}

func (q *Queue[T]) handleGetById(req getByIdRequest[T]) {
	_, isLocked := q.taskLocked[req.objectId]
	if isLocked {
		q.getByIdWaiters[req.objectId] = append(q.getByIdWaiters[req.objectId], req.responseCh)
	} else {
		item, err := q.store.get(q.ctx, req.objectId)

		q.taskLocked[req.objectId] = struct{}{}
		req.responseCh <- itemResponse[T]{item: item, err: err}
	}
}

func (q *Queue[T]) handleScheduledItem(req scheduledItem[T]) {
	// That means that the scheduling of the item was canceled
	if !q.isScheduled(req.item) {
		return
	}

	id := q.getId(req.item)
	delete(q.scheduled, q.getId(req.item))
	_, isLocked := q.taskLocked[id]
	if isLocked {
		q.dueWaiters[id] = append(q.dueWaiters[id], req)
	} else {
		item, err := q.store.get(q.ctx, id)

		q.taskLocked[id] = struct{}{}
		req.responseCh <- itemResponse[T]{item: item, err: err}
	}
}

func (q *Queue[T]) handleReleaseItem(req releaseRequest[T]) {
	item := req.item
	if _, ok := q.taskLocked[q.getId(item)]; !ok {
		req.responseCh <- fmt.Errorf("item is not locked")
		return
	}
	delete(q.taskLocked, q.getId(item))
	err := q.store.set(q.ctx, q.getId(item), item)
	if err != nil {
		req.responseCh <- err
		return
	}

	q.checkInSchedule(item)

	responded := q.checkGetByIdWaiters(item, false)
	responded = q.checkDueWaiters(item, responded)
	responded = q.checkNextScheduledWaiters(item, responded)
	responded = q.checkGetNextWaiters(item, responded)

	req.responseCh <- nil
}

func (q *Queue[T]) checkGetNextWaiters(item T, responded bool) bool {
	if responded {
		return responded
	}
	for i, waiter := range q.getNextWaiters {
		if waiter.filter(item) {
			q.getNextWaiters = slices.Delete(q.getNextWaiters, i, i+1)

			q.taskLocked[q.getId(item)] = struct{}{}
			waiter.responseCh <- itemResponse[T]{item: item}

			return true
		}
	}
	return responded
}

func (q *Queue[T]) checkNextScheduledWaiters(item T, responded bool) bool {
	if responded {
		return responded
	}

	for i, waiter := range q.getNextScheduledWaiters {
		if waiter.filter(item) && !q.isScheduled(item) {
			q.getNextScheduledWaiters = slices.Delete(q.getNextScheduledWaiters, i, i+1)

			// Respond immediately
			if waiter.scheduledAt(item).Before(time.Now()) {
				q.taskLocked[q.getId(item)] = struct{}{}
				waiter.responseCh <- itemResponse[T]{item: item}
				return true
			}

			// Schedule
			q.scheduleItem(waiter, item)
			break
		}
	}
	return responded
}

// checkInSchedule checks multiple things for each item in the schedule:
// 1. If the item was scheduled and still satisfies the filter, reschedule it
// 2. If the item was scheduled and no longer satisfies the filter, cancel the timer and schedule the next item for the request
// 3. If the item wasn't scheduled, but it's before the other scheduled item, schedule it instead
func (q *Queue[T]) checkInSchedule(item T) {
	for _, sch := range q.scheduled {
		if q.getId(sch.item) == q.getId(item) {
			close(sch.cancelTimerCh)
			delete(q.scheduled, q.getId(item))

			if sch.request.filter(item) {
				q.scheduleItem(sch.request, item)
			} else {
				q.handleGetNextScheduled(sch.request)
			}
		} else if sch.request.filter(item) && sch.request.scheduledAt(item).Before(sch.request.scheduledAt(sch.item)) && !q.isScheduled(item) {
			close(sch.cancelTimerCh)
			delete(q.scheduled, q.getId(item))

			q.scheduleItem(sch.request, item)
		}
	}
}

func (q *Queue[T]) isScheduled(item T) bool {
	_, ok := q.scheduled[q.getId(item)]
	return ok
}

// checkDueWaiters checks that we can return the item to any waiter.
// If the item no longer satisfies a filter for a given waiter, schedule a next item for the waiter's request
func (q *Queue[T]) checkDueWaiters(item T, responded bool) bool {
	dueWaiters := q.dueWaiters[q.getId(item)]
	if len(dueWaiters) == 0 {
		return responded
	}

	filtered := dueWaiters[:0]
	for _, waiter := range dueWaiters {
		// Still OK
		if waiter.request.filter(item) {
			if !responded {
				dueWaiters = dueWaiters[1:]

				q.taskLocked[q.getId(item)] = struct{}{}
				waiter.responseCh <- itemResponse[T]{item: item}

				responded = true
				continue
			} else {
				filtered = append(filtered, waiter)
			}
			// Item no longer satisfies filter
		} else {
			q.handleGetNextScheduled(waiter.request)
		}
	}

	dueWaiters = filtered
	if len(dueWaiters) == 0 {
		delete(q.dueWaiters, q.getId(item))
	} else {
		q.dueWaiters[q.getId(item)] = dueWaiters
	}

	return responded
}

func (q *Queue[T]) checkGetByIdWaiters(item T, responded bool) bool {
	if responded {
		return responded
	}

	waiters := q.getByIdWaiters[q.getId(item)]
	if len(waiters) > 0 {
		nextResponseCh := waiters[0]
		waiters = waiters[1:]
		q.taskLocked[q.getId(item)] = struct{}{}
		nextResponseCh <- itemResponse[T]{item: item}

		responded = true

		if len(waiters) == 0 {
			delete(q.getByIdWaiters, q.getId(item))
		} else {
			q.getByIdWaiters[q.getId(item)] = waiters
		}
	}
	return responded
}

func (q *Queue[T]) close() {
	if q.ctxCancel != nil {
		q.ctxCancel()
	}
	close(q.closeCh)
}

func (q *Queue[T]) handleGetNextScheduled(req getNextRequest[T]) {
	next, err := q.store.query(q.ctx, req.storeFilter, req.storeOrder, func(info T) bool {
		if _, ok := q.taskLocked[q.getId(info)]; ok {
			return false
		}
		if _, ok := q.scheduled[q.getId(info)]; ok {
			return false
		}
		return req.filter(info)
	})
	if errors.Is(err, ErrNoRows) {
		if req.subscribe {
			q.getNextScheduledWaiters = append(q.getNextScheduledWaiters, req)
		} else {
			req.responseCh <- itemResponse[T]{err: ErrNoRows}
		}
		return
	}
	if err != nil {
		req.responseCh <- itemResponse[T]{err: err}
		return
	}

	if req.scheduledAt(next).Before(time.Now()) {
		q.taskLocked[q.getId(next)] = struct{}{}
		req.responseCh <- itemResponse[T]{item: next}
		return
	}

	q.scheduleItem(req, next)
}

func (q *Queue[T]) handleGetNext(req getNextRequest[T]) {
	next, err := q.store.query(q.ctx, req.storeFilter, req.storeOrder, func(info T) bool {
		if _, ok := q.taskLocked[q.getId(info)]; ok {
			return false
		}
		return req.filter(info)
	})
	if errors.Is(err, ErrNoRows) {
		if req.subscribe {
			q.getNextWaiters = append(q.getNextWaiters, req)
		} else {
			req.responseCh <- itemResponse[T]{err: ErrNoRows}
		}
		return
	}
	if err != nil {
		req.responseCh <- itemResponse[T]{err: err}
		return
	}

	q.taskLocked[q.getId(next)] = struct{}{}
	req.responseCh <- itemResponse[T]{item: next}
}

func (q *Queue[T]) scheduleItem(req getNextRequest[T], next T) {
	timer := time.NewTimer(time.Until(req.scheduledAt(next)))
	cancelTimerCh := make(chan struct{})
	scheduled := scheduledItem[T]{
		timer:         timer,
		cancelTimerCh: cancelTimerCh,
		item:          next,

		request:    req,
		responseCh: req.responseCh,
	}
	q.scheduled[q.getId(next)] = scheduled

	go func() {
		defer timer.Stop()

		select {
		case <-timer.C:
			q.scheduledCh <- scheduled
		case <-cancelTimerCh:
			return
		}
	}()
}

func (q *Queue[T]) GetById(objectId string) (T, error) {
	responseCh := make(chan itemResponse[T], 1)

	q.getByIdCh <- getByIdRequest[T]{
		objectId:   objectId,
		responseCh: responseCh,
	}

	task := <-responseCh
	return task.item, task.err
}

type GetNextRequest[T any] struct {
	Subscribe   bool
	StoreFilter query.Filter
	StoreOrder  query.Sort

	Filter func(T) bool
}

func (r GetNextRequest[T]) Validate() error {
	if r.Filter == nil {
		return fmt.Errorf("filter is nil")
	}
	return nil
}

type GetNextScheduledRequest[T any] struct {
	Subscribe   bool
	StoreFilter query.Filter
	StoreOrder  query.Sort

	Filter      func(T) bool
	ScheduledAt func(T) time.Time
}

func (r GetNextScheduledRequest[T]) Validate() error {
	if r.Filter == nil {
		return fmt.Errorf("filter is nil")
	}
	if r.ScheduledAt == nil {
		return fmt.Errorf("scheduledAt is nil")
	}
	return nil
}

func (q *Queue[T]) GetNext(req GetNextRequest[T]) (T, error) {
	err := req.Validate()
	if err != nil {
		var defVal T
		return defVal, fmt.Errorf("validate: %w", err)
	}
	responseCh := make(chan itemResponse[T], 1)

	q.getNextCh <- getNextRequest[T]{
		subscribe:   req.Subscribe,
		storeFilter: req.StoreFilter,
		storeOrder:  req.StoreOrder,
		filter:      req.Filter,

		responseCh: responseCh,
	}

	task := <-responseCh
	return task.item, task.err
}

func (q *Queue[T]) GetNextScheduled(req GetNextScheduledRequest[T]) (T, error) {
	err := req.Validate()
	if err != nil {
		var defVal T
		return defVal, fmt.Errorf("validate: %w", err)
	}
	responseCh := make(chan itemResponse[T], 1)

	q.getNextScheduledCh <- getNextRequest[T]{
		subscribe:   req.Subscribe,
		storeFilter: req.StoreFilter,
		storeOrder:  req.StoreOrder,
		filter:      req.Filter,
		scheduledAt: req.ScheduledAt,

		responseCh: responseCh,
	}

	task := <-responseCh
	return task.item, task.err
}

func (q *Queue[T]) Upsert(id string, modifier func(exists bool, prev T) T) error {
	it, err := q.GetById(id)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	exists := !errors.Is(err, ErrNotFound)

	next := modifier(exists, it)

	return q.Release(next)
}

func (q *Queue[T]) Release(task T) error {
	responseCh := make(chan error, 1)

	q.releaseCh <- releaseRequest[T]{
		action:     releaseActionUpdate,
		item:       task,
		responseCh: responseCh,
	}

	return <-responseCh
}
