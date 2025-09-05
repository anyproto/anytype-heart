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

	getByIdWaiters          map[string][]chan itemResponse[T]
	scheduledWaiters        map[string][]scheduledItem[T]
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
		scheduledWaiters:   map[string][]scheduledItem[T]{},
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
	id := q.getId(req.item)
	delete(q.scheduled, req.request.requestId)
	_, isLocked := q.taskLocked[id]
	if isLocked {
		q.scheduledWaiters[id] = append(q.scheduledWaiters[id], req)
	} else {
		q.taskLocked[id] = struct{}{}
		req.responseCh <- itemResponse[T]{item: q.store.get(id)}
	}
}

func (q *queue[T]) handleReleaseItem(req releaseRequest[T]) {
	item := req.item
	delete(q.taskLocked, q.getId(item))
	q.store.set(q.getId(item), item)

	for _, sch := range q.scheduled {
		if q.getId(sch.item) == q.getId(item) {
			close(sch.cancelTimerCh)
			if sch.request.filter(item) {
				q.scheduleItem(sch.request, item)
			} else {
				q.handleGetNextScheduled(sch.request)
			}
		} else if sch.request.filter(item) && sch.request.scheduledAt(item).Before(sch.request.scheduledAt(sch.item)) {
			close(sch.cancelTimerCh)
			q.scheduleItem(sch.request, item)
		}
	}

	var responded bool
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

	scheduledWaiters := q.scheduledWaiters[q.getId(item)]
	if len(scheduledWaiters) > 0 {
		filtered := scheduledWaiters[:0]
		for _, nextScheduled := range scheduledWaiters {
			// Still OK
			if nextScheduled.request.filter(item) {
				if !responded {
					scheduledWaiters = scheduledWaiters[1:]
					q.taskLocked[q.getId(item)] = struct{}{}

					nextScheduled.responseCh <- itemResponse[T]{item: item}
					responded = true
				} else {
					filtered = append(filtered, nextScheduled)
				}
			} else {
				q.handleGetNextScheduled(nextScheduled.request)
			}
		}

		scheduledWaiters = filtered
		if len(scheduledWaiters) == 0 {
			delete(q.scheduledWaiters, q.getId(item))
		} else {
			q.scheduledWaiters[q.getId(item)] = scheduledWaiters
		}
	}

	for i, waiter := range q.getNextScheduledWaiters {
		if waiter.filter(item) {
			q.getNextScheduledWaiters = slices.Delete(q.getNextScheduledWaiters, i, i+1)
			// TODO Handle item directly, now it's queried again
			q.handleGetNextScheduled(waiter)
			break
		}
	}

	for i, waiter := range q.getNextWaiters {
		if waiter.filter(item) {
			q.getNextWaiters = slices.Delete(q.getNextWaiters, i, i+1)
			// TODO Handle item directly
			q.handleGetNext(waiter)
			break
		}
	}
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
