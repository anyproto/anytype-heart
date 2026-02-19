package filequeue

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/anyproto/any-store/query"
	"github.com/anyproto/any-sync/app/logger"
	"github.com/globalsign/mgo/bson"
)

var ErrClosed = fmt.Errorf("closed")
var ErrNotFound = fmt.Errorf("not found")
var ErrNoRows = fmt.Errorf("no rows")

var log = logger.NewNamed("filequeue")

type Queue[T any] struct {
	ctx       context.Context
	ctxCancel context.CancelFunc

	store *Storage[T]

	getId func(T) string
	setId func(T, string) T

	closed bool

	// request channels
	getByIdCh          chan getByIdRequest[T]
	getNextCh          chan getNextRequest[T]
	getNextScheduledCh chan getNextRequest[T]
	releaseCh          chan releaseRequest[T]
	dueItemCh          chan scheduledItem[T]
	cancelRequestCh    chan string

	// If a requested item is locked we need means to signal the next blocked goroutine that item is unlocked
	getByIdWaiters map[string][]chan itemResponse[T]

	// If time of a scheduled item has come, but it's locked, we need a way to signal a waiting goroutine (GetNextScheduled) when item is unlocked
	dueWaiters map[string][]scheduledItem[T]

	// When a suitable item appears, we need to return this item to a waiting goroutine
	getNextWaiters []getNextRequest[T]

	// When a suitable scheduled item appears, we need to return this item to a waiting goroutine if the time has come,
	// or to schedule item if it hasn't been scheduled yet
	getNextScheduledWaiters []getNextRequest[T]

	itemLocked map[string]struct{}         // itemLocked indicates that item is being processed and can't be accessed by others
	scheduled  map[string]scheduledItem[T] // scheduled contains a set of items scheduled for specific time
}

func NewQueue[T any](store *Storage[T], getId func(T) string, setId func(T, string) T) *Queue[T] {
	ctx, ctxCancel := context.WithCancel(context.Background())
	return &Queue[T]{
		ctx:       ctx,
		ctxCancel: ctxCancel,
		store:     store,

		// Request channels
		getByIdCh:          make(chan getByIdRequest[T]),
		getNextScheduledCh: make(chan getNextRequest[T]),
		getNextCh:          make(chan getNextRequest[T]),
		releaseCh:          make(chan releaseRequest[T]),
		dueItemCh:          make(chan scheduledItem[T]),
		cancelRequestCh:    make(chan string),

		getByIdWaiters: make(map[string][]chan itemResponse[T]),
		itemLocked:     make(map[string]struct{}),
		scheduled:      make(map[string]scheduledItem[T]),
		dueWaiters:     map[string][]scheduledItem[T]{},
		getId:          getId,
		setId:          setId,
	}
}

func (q *Queue[T]) Run() {
	for {
		select {
		case <-q.ctx.Done():
			q.handleClose()
			return
		case req := <-q.getByIdCh:
			q.handleGetById(req)
		case req := <-q.getNextScheduledCh:
			q.handleGetNextScheduled(req)
		case req := <-q.getNextCh:
			q.handleGetNext(req)
		case req := <-q.releaseCh:
			q.handleReleaseItem(req)
		case req := <-q.dueItemCh:
			q.handleScheduledItem(req)
		case req := <-q.cancelRequestCh:
			q.handleCancelRequest(req)
		}
	}
}

func (q *Queue[T]) handleClose() {
	if q.closed {
		return
	}

	for _, waiters := range q.getByIdWaiters {
		for _, w := range waiters {
			w <- itemResponse[T]{
				err: ErrClosed,
			}
		}
	}
	q.getByIdWaiters = nil

	for _, waiters := range q.dueWaiters {
		for _, scheduled := range waiters {
			scheduled.timer.Stop()
			scheduled.responseCh <- itemResponse[T]{
				err: ErrClosed,
			}
		}
	}
	q.dueWaiters = nil

	for _, scheduled := range q.scheduled {
		scheduled.timer.Stop()
		scheduled.responseCh <- itemResponse[T]{
			err: ErrClosed,
		}
	}
	q.scheduled = nil

	for _, waiter := range q.getNextWaiters {
		waiter.responseCh <- itemResponse[T]{
			err: ErrClosed,
		}
	}
	q.getNextWaiters = nil

	for _, waiter := range q.getNextScheduledWaiters {
		waiter.responseCh <- itemResponse[T]{
			err: ErrClosed,
		}
	}
	q.getNextScheduledWaiters = nil
}

func (q *Queue[T]) handleGetById(req getByIdRequest[T]) {
	_, isLocked := q.itemLocked[req.objectId]
	if isLocked {
		q.getByIdWaiters[req.objectId] = append(q.getByIdWaiters[req.objectId], req.responseCh)
	} else {
		item, err := q.store.get(q.ctx, req.objectId)
		if err != nil && !errors.Is(err, ErrNotFound) {
			req.responseCh <- itemResponse[T]{err: err}
			return
		}

		if errors.Is(err, ErrNotFound) {
			item = q.setId(item, req.objectId)
		}

		q.itemLocked[req.objectId] = struct{}{}
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
	_, isLocked := q.itemLocked[id]
	if isLocked {
		q.dueWaiters[id] = append(q.dueWaiters[id], req)
	} else {
		item, err := q.store.get(q.ctx, id)

		q.itemLocked[id] = struct{}{}
		req.responseCh <- itemResponse[T]{item: item, err: err}
	}
}

func (q *Queue[T]) handleReleaseItem(req releaseRequest[T]) {
	item := req.item
	if _, ok := q.itemLocked[req.objectId]; !ok {
		req.responseCh <- fmt.Errorf("item is not locked")
		return
	}
	delete(q.itemLocked, req.objectId)

	if req.update {
		err := q.store.set(q.ctx, req.objectId, item)
		if err != nil {
			req.responseCh <- err
			return
		}
	} else {
		// If we don't want to update an item, request the prev version to broadcast it to all waiting goroutines
		prevItem, err := q.store.get(q.ctx, req.objectId)
		// If there is no item, do nothing
		if errors.Is(err, ErrNotFound) {
			q.releaseGetByIdWaitersWithError(req.objectId, ErrNotFound)
			req.responseCh <- nil
			return
		}

		// Something bad happened
		if err != nil {
			req.responseCh <- err
			return
		}

		item = prevItem
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

			q.itemLocked[q.getId(item)] = struct{}{}
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
				q.itemLocked[q.getId(item)] = struct{}{}
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
	for id, sch := range q.scheduled {
		if id == q.getId(item) {
			close(sch.cancelTimerCh)
			delete(q.scheduled, q.getId(item))

			if sch.request.filter(item) {
				q.scheduleItem(sch.request, item)
			} else {
				q.handleGetNextScheduled(sch.request)
			}

		} else if sch.request.filter(item) && sch.request.scheduledAt(item).Before(sch.request.scheduledAt(sch.item)) && !q.isScheduled(item) {
			close(sch.cancelTimerCh)
			delete(q.scheduled, id)

			q.scheduleItem(sch.request, item)
		}
	}
}

func (q *Queue[T]) isScheduled(item T) bool {
	_, ok := q.scheduled[q.getId(item)]
	return ok
}

// checkDueWaiters checks that we can return the item to any waiting goroutine.
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

				q.itemLocked[q.getId(item)] = struct{}{}
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
		q.itemLocked[q.getId(item)] = struct{}{}
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

func (q *Queue[T]) releaseGetByIdWaitersWithError(id string, err error) {
	waiters := q.getByIdWaiters[id]
	if len(waiters) > 0 {
		nextResponseCh := waiters[0]
		waiters = waiters[1:]
		q.itemLocked[id] = struct{}{}
		nextResponseCh <- itemResponse[T]{err: err}

		if len(waiters) == 0 {
			delete(q.getByIdWaiters, id)
		} else {
			q.getByIdWaiters[id] = waiters
		}
	}
}

func (q *Queue[T]) handleCancelRequest(id string) {
	findAndCancel := func(waiters []getNextRequest[T]) []getNextRequest[T] {
		for i, w := range waiters {
			if w.id == id {
				w.responseCh <- itemResponse[T]{
					err: context.Canceled,
				}
				waiters = slices.Delete(waiters, i, i+1)
				return waiters
			}
		}
		return waiters
	}

	q.getNextWaiters = findAndCancel(q.getNextWaiters)
	q.getNextScheduledWaiters = findAndCancel(q.getNextScheduledWaiters)

	for schId, it := range q.scheduled {
		if it.request.id == id {
			sch := q.scheduled[schId]
			sch.responseCh <- itemResponse[T]{
				err: context.Canceled,
			}
			close(sch.cancelTimerCh)

			delete(q.scheduled, schId)
			break
		}
	}

	checkInDueWaiters := func() {
		for itemId, scheduled := range q.dueWaiters {
			for i, it := range scheduled {
				if it.request.id == id {
					scheduled = slices.Delete(scheduled, i, i+1)
					q.dueWaiters[itemId] = scheduled
					return
				}
			}
		}
	}
	checkInDueWaiters()
}

func (q *Queue[T]) Close() {
	if q.ctxCancel != nil {
		q.ctxCancel()
	}
}

func (q *Queue[T]) handleGetNextScheduled(req getNextRequest[T]) {
	next, err := q.store.query(q.ctx, req.storeFilter, req.storeOrder, func(info T) bool {
		if _, ok := q.itemLocked[q.getId(info)]; ok {
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
			go func() {
				select {
				case <-req.ctx.Done():
					q.cancelRequest(req.id)
				case <-q.ctx.Done():
					return
				}
			}()
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
		q.itemLocked[q.getId(next)] = struct{}{}
		req.responseCh <- itemResponse[T]{item: next}
		return
	}

	q.scheduleItem(req, next)
}

func (q *Queue[T]) handleGetNext(req getNextRequest[T]) {
	next, err := q.store.query(q.ctx, req.storeFilter, req.storeOrder, func(info T) bool {
		if _, ok := q.itemLocked[q.getId(info)]; ok {
			return false
		}
		return req.filter(info)
	})
	if errors.Is(err, ErrNoRows) {
		if req.subscribe {
			q.getNextWaiters = append(q.getNextWaiters, req)
			go func() {
				select {
				case <-req.ctx.Done():
					q.cancelRequest(req.id)
				case <-q.ctx.Done():
					return
				}
			}()
		} else {
			req.responseCh <- itemResponse[T]{err: ErrNoRows}
		}
		return
	}
	if err != nil {
		req.responseCh <- itemResponse[T]{err: err}
		return
	}

	q.itemLocked[q.getId(next)] = struct{}{}
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
		case <-q.ctx.Done():
			return
		case <-timer.C:
			q.dispatchDueItem(scheduled)
		case <-req.ctx.Done():
			q.cancelRequest(req.id)
		case <-cancelTimerCh:
			return
		}
	}()
}

// GetById locks and returns an item by id. It locks an item even if it's not stored in a DB, it's useful to prevent race conditions.
// Typical usage: call GetById, process or initialize an item and store it using ReleaseAndUpdate
func (q *Queue[T]) GetById(objectId string) (T, error) {
	responseCh := make(chan itemResponse[T], 1)

	req := getByIdRequest[T]{
		objectId:   objectId,
		responseCh: responseCh,
	}

	select {
	case q.getByIdCh <- req:
		task := <-responseCh
		return task.item, task.err
	case <-q.ctx.Done():
		var defValue T
		return defValue, ErrClosed
	}

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

func (q *Queue[T]) GetNext(ctx context.Context, req GetNextRequest[T]) (T, error) {
	err := req.Validate()
	if err != nil {
		var defVal T
		return defVal, fmt.Errorf("validate: %w", err)
	}
	responseCh := make(chan itemResponse[T], 1)

	chReq := getNextRequest[T]{
		id:          bson.NewObjectId().Hex(),
		ctx:         ctx,
		subscribe:   req.Subscribe,
		storeFilter: req.StoreFilter,
		storeOrder:  req.StoreOrder,
		filter:      req.Filter,

		responseCh: responseCh,
	}

	select {
	case q.getNextCh <- chReq:
		task := <-responseCh
		return task.item, task.err
	case <-q.ctx.Done():
		var defVal T
		return defVal, ErrClosed
	}
}

func (q *Queue[T]) GetNextScheduled(ctx context.Context, req GetNextScheduledRequest[T]) (T, error) {
	err := req.Validate()
	if err != nil {
		var defVal T
		return defVal, fmt.Errorf("validate: %w", err)
	}
	responseCh := make(chan itemResponse[T], 1)

	chReq := getNextRequest[T]{
		id:          bson.NewObjectId().Hex(),
		ctx:         ctx,
		subscribe:   req.Subscribe,
		storeFilter: req.StoreFilter,
		storeOrder:  req.StoreOrder,
		filter:      req.Filter,
		scheduledAt: req.ScheduledAt,

		responseCh: responseCh,
	}

	select {
	case q.getNextScheduledCh <- chReq:
		task := <-responseCh
		return task.item, task.err
	case <-q.ctx.Done():
		var defVal T
		return defVal, ErrClosed
	}
}

func (q *Queue[T]) Upsert(id string, modifier func(exists bool, prev T) T) error {
	it, err := q.GetById(id)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	exists := !errors.Is(err, ErrNotFound)

	next := modifier(exists, it)

	return q.ReleaseAndUpdate(id, next)
}

func (q *Queue[T]) ReleaseAndUpdate(id string, task T) error {
	task = q.setId(task, id)

	responseCh := make(chan error, 1)
	req := releaseRequest[T]{
		objectId:   id,
		item:       task,
		responseCh: responseCh,
		update:     true,
	}

	select {
	case q.releaseCh <- req:
		return <-responseCh
	case <-q.ctx.Done():
		return ErrClosed
	}
}

func (q *Queue[T]) Release(id string) error {
	responseCh := make(chan error, 1)
	req := releaseRequest[T]{
		objectId:   id,
		responseCh: responseCh,
		update:     false,
	}

	select {
	case q.releaseCh <- req:
		return <-responseCh
	case <-q.ctx.Done():
		return ErrClosed
	}
}

func (q *Queue[T]) List() ([]T, error) {
	return q.store.listAll(q.ctx)
}

func (q *Queue[T]) cancelRequest(reqId string) {
	select {
	case <-q.ctx.Done():
		return
	case q.cancelRequestCh <- reqId:
	}
}

// dispatchDueItem sends a message that item's time has come
func (q *Queue[T]) dispatchDueItem(item scheduledItem[T]) {
	select {
	case <-q.ctx.Done():
		return
	case q.dueItemCh <- item:
	}
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
	id          string
	ctx         context.Context
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

type releaseRequest[T any] struct {
	objectId   string
	item       T
	responseCh chan error
	update     bool
}
