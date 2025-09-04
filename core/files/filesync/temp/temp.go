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

type storage struct {
	files map[string]FileInfo
}

func (s *storage) get(objectId string) FileInfo {
	return s.files[objectId]
}

func (s *storage) set(objectId string, file FileInfo) {
	s.files[objectId] = file
}

func (s *storage) query(filter func(FileInfo) bool, schedulingOrder func(info FileInfo) time.Time) (FileInfo, bool) {
	var res []FileInfo
	for _, file := range s.files {
		if filter(file) {
			res = append(res, file)
		}
	}
	sort.Slice(res, func(i, j int) bool {
		return schedulingOrder(res[i]).Before(schedulingOrder(res[j]))
	})
	if len(res) == 0 {
		return FileInfo{}, false
	}
	return res[0], true
}

type getByIdRequest struct {
	objectId   string
	responseCh chan getByIdResponse
}

type getByIdResponse struct {
	info FileInfo
}

type getNextRequest struct {
	requestId   string
	filter      func(FileInfo) bool
	scheduledAt func(FileInfo) time.Time
	orderBy     func(FileInfo) time.Time

	responseCh chan getByIdResponse
}

type scheduledItem struct {
	timer         *time.Timer
	cancelTimerCh chan struct{}
	item          FileInfo

	request    getNextRequest
	responseCh chan getByIdResponse
}

type queue struct {
	store *storage

	closeCh            chan struct{}
	getByIdCh          chan getByIdRequest
	getNextCh          chan getNextRequest
	getNextScheduledCh chan getNextRequest
	releaseCh          chan FileInfo
	scheduledCh        chan scheduledItem

	getByIdWaiters   map[string][]chan getByIdResponse
	scheduledWaiters map[string][]scheduledItem
	getNextWaiters   []getNextRequest

	taskLocked map[string]struct{}
	scheduled  map[string]scheduledItem
}

func newQueue(store *storage) *queue {
	return &queue{
		store:              store,
		closeCh:            make(chan struct{}),
		getByIdCh:          make(chan getByIdRequest),
		getNextCh:          make(chan getNextRequest),
		getNextScheduledCh: make(chan getNextRequest),
		scheduledCh:        make(chan scheduledItem),
		releaseCh:          make(chan FileInfo),
		getByIdWaiters:     make(map[string][]chan getByIdResponse),
		taskLocked:         make(map[string]struct{}),
		scheduled:          make(map[string]scheduledItem),
		scheduledWaiters:   map[string][]scheduledItem{},
	}
}

func (q *queue) run() {
	// TODO Think about deletion
	for {
		select {
		case <-q.closeCh:
			return
		case req := <-q.getByIdCh:
			_, isLocked := q.taskLocked[req.objectId]
			if isLocked {
				q.getByIdWaiters[req.objectId] = append(q.getByIdWaiters[req.objectId], req.responseCh)
			} else {
				q.taskLocked[req.objectId] = struct{}{}
				req.responseCh <- getByIdResponse{info: q.store.get(req.objectId)}
			}
		case req := <-q.getNextScheduledCh:
			q.handleGetNextScheduled(req)
		case req := <-q.getNextCh:
			q.handleGetNext(req)
		case req := <-q.releaseCh:
			delete(q.taskLocked, req.ObjectId)
			q.store.set(req.ObjectId, req)

			for _, sch := range q.scheduled {
				if sch.item.ObjectId == req.ObjectId {
					close(sch.cancelTimerCh)
					if sch.request.filter(req) {
						q.scheduleItem(sch.request, req)
					} else {
						q.handleGetNextScheduled(sch.request)
					}
				} else if sch.request.filter(req) && sch.request.scheduledAt(req).Before(sch.request.scheduledAt(sch.item)) {
					close(sch.cancelTimerCh)
					q.scheduleItem(sch.request, req)
				}
			}

			// Prioritize scheduled items
			var responded bool
			scheduledWaiters := q.scheduledWaiters[req.ObjectId]
			if len(scheduledWaiters) > 0 {
				filtered := scheduledWaiters[:0]

				for _, nextScheduled := range scheduledWaiters {
					// Still OK
					if nextScheduled.request.filter(req) {
						if !responded {
							scheduledWaiters = scheduledWaiters[1:]
							q.taskLocked[req.ObjectId] = struct{}{}

							nextScheduled.responseCh <- getByIdResponse{info: req}
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
					delete(q.scheduledWaiters, req.ObjectId)
				} else {
					q.scheduledWaiters[req.ObjectId] = scheduledWaiters
				}
			}

			waiters := q.getByIdWaiters[req.ObjectId]
			if !responded && len(waiters) > 0 {
				nextResponseCh := waiters[0]
				waiters = waiters[1:]
				q.taskLocked[req.ObjectId] = struct{}{}
				nextResponseCh <- getByIdResponse{info: req}

				if len(waiters) == 0 {
					delete(q.getByIdWaiters, req.ObjectId)
				} else {
					q.getByIdWaiters[req.ObjectId] = waiters
				}
			}

			for i, waiter := range q.getNextWaiters {
				if waiter.filter(req) {
					q.getNextWaiters = slices.Delete(q.getNextWaiters, i, i+1)
					q.handleGetNextScheduled(waiter)
					break
				}
			}

		case req := <-q.scheduledCh:
			id := req.item.ObjectId
			delete(q.scheduled, req.request.requestId)
			_, isLocked := q.taskLocked[id]
			if isLocked {
				q.scheduledWaiters[id] = append(q.scheduledWaiters[id], req)
			} else {
				q.taskLocked[id] = struct{}{}
				req.responseCh <- getByIdResponse{info: q.store.get(id)}
			}
		}
	}
}

func (q *queue) close() {
	close(q.closeCh)
}

func (q *queue) handleGetNextScheduled(req getNextRequest) {
	next, ok := q.store.query(func(info FileInfo) bool {
		if _, ok := q.taskLocked[info.ObjectId]; ok {
			return false
		}
		if _, ok := q.scheduled[info.ObjectId]; ok {
			return false
		}
		return req.filter(info)
	}, req.scheduledAt)
	if !ok {
		q.getNextWaiters = append(q.getNextWaiters, req)
		return
	}

	if next.ScheduledAt.Before(time.Now()) {
		q.taskLocked[next.ObjectId] = struct{}{}
		req.responseCh <- getByIdResponse{info: next}
		return
	}

	q.scheduleItem(req, next)
}

func (q *queue) handleGetNext(req getNextRequest) {
	next, ok := q.store.query(func(info FileInfo) bool {
		if _, ok := q.taskLocked[info.ObjectId]; ok {
			return false
		}
		return req.filter(info)
	}, req.orderBy)
	if !ok {
		q.getNextWaiters = append(q.getNextWaiters, req)
		return
	}

	q.taskLocked[next.ObjectId] = struct{}{}
	req.responseCh <- getByIdResponse{info: next}
}

func (q *queue) scheduleItem(req getNextRequest, next FileInfo) {
	timer := time.NewTimer(time.Until(next.ScheduledAt))
	cancelTimerCh := make(chan struct{})
	scheduled := scheduledItem{
		timer:         timer,
		cancelTimerCh: cancelTimerCh,
		item:          next,

		request:    req,
		responseCh: req.responseCh,
	}
	q.scheduled[next.ObjectId] = scheduled

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

func (q *queue) get(objectId string) FileInfo {
	responseCh := make(chan getByIdResponse, 1)

	q.getByIdCh <- getByIdRequest{
		objectId:   objectId,
		responseCh: responseCh,
	}

	task := <-responseCh
	return task.info
}

func (q *queue) getNext(filter func(FileInfo) bool, orderBy func(FileInfo) time.Time) FileInfo {
	responseCh := make(chan getByIdResponse, 1)

	if orderBy == nil {
		orderBy = func(info FileInfo) time.Time {
			return time.Time{}
		}
	}
	q.getNextCh <- getNextRequest{
		filter:  filter,
		orderBy: orderBy,

		responseCh: responseCh,
	}

	task := <-responseCh
	return task.info
}

func (q *queue) getNextScheduled(filter func(FileInfo) bool, scheduledAt func(FileInfo) time.Time) FileInfo {
	responseCh := make(chan getByIdResponse, 1)

	q.getNextScheduledCh <- getNextRequest{
		filter:      filter,
		scheduledAt: scheduledAt,

		responseCh: responseCh,
	}

	task := <-responseCh
	return task.info
}

func (q *queue) release(task FileInfo) {
	q.releaseCh <- task
}
