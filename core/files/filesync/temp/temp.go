package temp

import (
	"slices"
	"sort"
	"time"

	"github.com/ipfs/go-cid"

	"github.com/anyproto/anytype-heart/core/domain"
)

type FileState int

const (
	FileStatePendingUpload FileState = iota
	FileStateUploading
	FileStateLimited
	FileStatePendingDeletion
	FileStateDone
	FileStateDeleted
)

type FileInfo struct {
	FileId      domain.FileId
	SpaceId     string
	ObjectId    string
	State       FileState
	ScheduledAt time.Time
	HandledAt   time.Time
	Variants    []domain.FileId
	AddedByUser bool
	Imported    bool

	BytesToUpload int
	CidsToUpload  map[cid.Cid]struct{}
}

type storage[T any] struct {
	files map[string]T
}

func newStorage[T any]() *storage[T] {
	return &storage[T]{
		files: make(map[string]T),
	}
}

func (s *storage[T]) get(objectId string) T {
	return s.files[objectId]
}

func (s *storage[T]) set(objectId string, file T) {
	s.files[objectId] = file
}

func (s *storage[T]) delete(objectId string) {
	delete(s.files, objectId)
}

func (s *storage[T]) query(filter func(T) bool, schedulingOrder func(info T) time.Time) (T, bool) {
	var res []T
	for _, file := range s.files {
		if filter(file) {
			res = append(res, file)
		}
	}
	sort.Slice(res, func(i, j int) bool {
		return schedulingOrder(res[i]).Before(schedulingOrder(res[j]))
	})
	if len(res) == 0 {
		var defValue T
		return defValue, false
	}
	return res[0], true
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
	requestId   string
	filter      func(T) bool
	scheduledAt func(T) time.Time
	orderBy     func(T) time.Time

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
	objectId string
	item     T
	action   releaseAction
}

type queue[T any] struct {
	store *storage[T]

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

func newQueue[T any](store *storage[T], getId func(T) string) *queue[T] {
	return &queue[T]{
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

func (q *queue[T]) run() {
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

func (q *queue[T]) handleGetById(req getByIdRequest[T]) {
	_, isLocked := q.taskLocked[req.objectId]
	if isLocked {
		q.getByIdWaiters[req.objectId] = append(q.getByIdWaiters[req.objectId], req.responseCh)
	} else {
		q.taskLocked[req.objectId] = struct{}{}
		req.responseCh <- itemResponse[T]{item: q.store.get(req.objectId)}
	}
}

func (q *queue[T]) handleScheduledItem(req scheduledItem[T]) {
	// That means that the scheduling of the item was canceled
	if !q.isScheduled(req.item) {
		return
	}

	id := q.getId(req.item)
	delete(q.scheduled, req.request.requestId)
	_, isLocked := q.taskLocked[id]
	if isLocked {
		q.dueWaiters[id] = append(q.dueWaiters[id], req)
	} else {
		q.taskLocked[id] = struct{}{}
		req.responseCh <- itemResponse[T]{item: q.store.get(id)}
	}
}

func (q *queue[T]) handleReleaseItem(req releaseRequest[T]) {
	item := req.item
	delete(q.taskLocked, q.getId(item))
	q.store.set(q.getId(item), item)

	q.checkInSchedule(item)

	responded := q.checkGetByIdWaiters(item, false)
	responded = q.checkDueWaiters(item, responded)
	responded = q.checkNextScheduledWaiters(item, responded)
	responded = q.checkGetNextWaiters(item, responded)
}

func (q *queue[T]) checkGetNextWaiters(item T, responded bool) bool {
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

func (q *queue[T]) checkNextScheduledWaiters(item T, responded bool) bool {
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
func (q *queue[T]) checkInSchedule(item T) {
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

func (q *queue[T]) isScheduled(item T) bool {
	_, ok := q.scheduled[q.getId(item)]
	return ok
}

// checkDueWaiters checks that we can return the item to any waiter.
// If the item no longer satisfies a filter for a given waiter, schedule a next item for the waiter's request
func (q *queue[T]) checkDueWaiters(item T, responded bool) bool {
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

func (q *queue[T]) checkGetByIdWaiters(item T, responded bool) bool {
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

func (q *queue[T]) close() {
	close(q.closeCh)
}

func (q *queue[T]) handleGetNextScheduled(req getNextRequest[T]) {
	next, ok := q.store.query(func(info T) bool {
		if _, ok := q.taskLocked[q.getId(info)]; ok {
			return false
		}
		if _, ok := q.scheduled[q.getId(info)]; ok {
			return false
		}
		return req.filter(info)
	}, req.scheduledAt)
	if !ok {
		q.getNextScheduledWaiters = append(q.getNextScheduledWaiters, req)
		return
	}

	if req.scheduledAt(next).Before(time.Now()) {
		q.taskLocked[q.getId(next)] = struct{}{}
		req.responseCh <- itemResponse[T]{item: next}
		return
	}

	q.scheduleItem(req, next)
}

func (q *queue[T]) handleGetNext(req getNextRequest[T]) {
	next, ok := q.store.query(func(info T) bool {
		if _, ok := q.taskLocked[q.getId(info)]; ok {
			return false
		}
		return req.filter(info)
	}, req.orderBy)
	if !ok {
		q.getNextWaiters = append(q.getNextWaiters, req)
		return
	}

	q.taskLocked[q.getId(next)] = struct{}{}
	req.responseCh <- itemResponse[T]{item: next}
}

func (q *queue[T]) scheduleItem(req getNextRequest[T], next T) {
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

func (q *queue[T]) get(objectId string) T {
	responseCh := make(chan itemResponse[T], 1)

	q.getByIdCh <- getByIdRequest[T]{
		objectId:   objectId,
		responseCh: responseCh,
	}

	task := <-responseCh
	return task.item
}

func (q *queue[T]) getNext(filter func(T) bool, orderBy func(T) time.Time) T {
	responseCh := make(chan itemResponse[T], 1)

	if orderBy == nil {
		orderBy = func(info T) time.Time {
			return time.Time{}
		}
	}
	q.getNextCh <- getNextRequest[T]{
		filter:  filter,
		orderBy: orderBy,

		responseCh: responseCh,
	}

	task := <-responseCh
	return task.item
}

func (q *queue[T]) getNextScheduled(filter func(T) bool, scheduledAt func(T) time.Time) T {
	responseCh := make(chan itemResponse[T], 1)

	q.getNextScheduledCh <- getNextRequest[T]{
		filter:      filter,
		scheduledAt: scheduledAt,

		responseCh: responseCh,
	}

	task := <-responseCh
	return task.item
}

func (q *queue[T]) release(task T) {
	q.releaseCh <- releaseRequest[T]{
		action: releaseActionUpdate,
		item:   task,
	}
}
