package temp

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	anystore "github.com/anyproto/any-store"
	"github.com/anyproto/any-store/anyenc"
	"github.com/anyproto/any-store/query"
	"github.com/ipfs/go-cid"

	"github.com/anyproto/anytype-heart/core/domain"
)

var ErrNotFound = fmt.Errorf("not found")
var ErrNoRows = fmt.Errorf("no rows")

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

func marshalFileInfo(arena *anyenc.Arena, info FileInfo) *anyenc.Value {
	obj := arena.NewObject()
	obj.Set("fileId", arena.NewString(info.FileId.String()))
	obj.Set("spaceId", arena.NewString(info.SpaceId))
	obj.Set("id", arena.NewString(info.ObjectId))
	obj.Set("state", arena.NewNumberInt(int(info.State)))
	obj.Set("addedAt", arena.NewNumberInt(int(info.ScheduledAt.UTC().Unix())))
	obj.Set("handledAt", arena.NewNumberInt(int(info.HandledAt.UTC().Unix())))
	variants := arena.NewArray()
	for i, variant := range info.Variants {
		variants.SetArrayItem(i, arena.NewString(variant.String()))
	}
	obj.Set("variants", variants)
	obj.Set("addedByUser", newBool(arena, info.AddedByUser))
	obj.Set("imported", newBool(arena, info.Imported))
	obj.Set("bytesToUpload", arena.NewNumberInt(info.BytesToUpload))
	cidsToUpload := arena.NewArray()
	var i int
	for c := range info.CidsToUpload {
		cidsToUpload.SetArrayItem(i, arena.NewString(c.String()))
	}
	obj.Set("cidsToUpload", cidsToUpload)
	return obj
}

func newBool(arena *anyenc.Arena, val bool) *anyenc.Value {
	if val {
		return arena.NewTrue()
	}
	return arena.NewFalse()
}

func unmarshalFileInfo(doc *anyenc.Value) (FileInfo, error) {
	rawVariants := doc.GetArray("variants")
	var variants []domain.FileId
	if len(rawVariants) > 0 {
		variants = make([]domain.FileId, 0, len(rawVariants))
		for _, v := range rawVariants {
			variants = append(variants, domain.FileId(v.GetString()))
		}
	}
	var cidsToUpload map[cid.Cid]struct{}
	rawCidsToUpload := doc.GetArray("cidsToUpload")
	if len(rawCidsToUpload) > 0 {
		cidsToUpload = make(map[cid.Cid]struct{}, len(rawCidsToUpload))
		for _, raw := range rawCidsToUpload {
			c, err := cid.Parse(raw.GetString())
			if err != nil {
				return FileInfo{}, fmt.Errorf("parse cid: %w", err)
			}
			cidsToUpload[c] = struct{}{}
		}
	}
	fileId := domain.FileId(doc.GetString("fileId"))
	return FileInfo{
		FileId:        fileId,
		SpaceId:       doc.GetString("spaceId"),
		ObjectId:      doc.GetString("id"),
		State:         FileState(doc.GetInt("state")),
		ScheduledAt:   time.Unix(int64(doc.GetInt("addedAt")), 0).UTC(),
		HandledAt:     time.Unix(int64(doc.GetInt("handledAt")), 0).UTC(),
		Variants:      variants,
		AddedByUser:   doc.GetBool("addedByUser"),
		Imported:      doc.GetBool("imported"),
		BytesToUpload: doc.GetInt("bytesToUpload"),
		CidsToUpload:  cidsToUpload,
	}, nil
}

type marshalFunc[T any] func(arena *anyenc.Arena, val T) *anyenc.Value
type unmarshalFunc[T any] func(v *anyenc.Value) (T, error)

type storage[T any] struct {
	arena     *anyenc.Arena
	coll      anystore.Collection
	marshal   marshalFunc[T]
	unmarshal unmarshalFunc[T]
}

func newStorage[T any](coll anystore.Collection, marshal marshalFunc[T], unmarshal unmarshalFunc[T]) *storage[T] {
	return &storage[T]{
		arena:     &anyenc.Arena{},
		coll:      coll,
		marshal:   marshal,
		unmarshal: unmarshal,
	}
}

func (s *storage[T]) get(ctx context.Context, objectId string) (T, error) {
	doc, err := s.coll.FindId(ctx, objectId)
	if errors.Is(err, anystore.ErrDocNotFound) {
		var defVal T
		return defVal, ErrNotFound
	}

	return s.unmarshal(doc.Value())
}

func (s *storage[T]) set(ctx context.Context, objectId string, file T) error {
	defer s.arena.Reset()

	val := s.marshal(s.arena, file)
	val.Set("id", s.arena.NewString(objectId))
	return s.coll.UpsertOne(ctx, val)
}

func (s *storage[T]) delete(ctx context.Context, objectId string) error {
	return s.coll.DeleteId(ctx, objectId)
}

func (s *storage[T]) query(ctx context.Context, filter query.Filter, order query.Sort, inMemoryFilter func(T) bool) (T, error) {
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

type queue[T any] struct {
	ctx       context.Context
	ctxCancel context.CancelFunc

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
	ctx, ctxCancel := context.WithCancel(context.Background())
	return &queue[T]{
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
		item, err := q.store.get(q.ctx, req.objectId)

		q.taskLocked[req.objectId] = struct{}{}
		req.responseCh <- itemResponse[T]{item: item, err: err}
	}
}

func (q *queue[T]) handleScheduledItem(req scheduledItem[T]) {
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

func (q *queue[T]) handleReleaseItem(req releaseRequest[T]) {
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
	if q.ctxCancel != nil {
		q.ctxCancel()
	}
	close(q.closeCh)
}

func (q *queue[T]) handleGetNextScheduled(req getNextRequest[T]) {
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
		q.getNextScheduledWaiters = append(q.getNextScheduledWaiters, req)
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

func (q *queue[T]) handleGetNext(req getNextRequest[T]) {
	next, err := q.store.query(q.ctx, req.storeFilter, req.storeOrder, func(info T) bool {
		if _, ok := q.taskLocked[q.getId(info)]; ok {
			return false
		}
		return req.filter(info)
	})
	if errors.Is(err, ErrNoRows) {
		q.getNextWaiters = append(q.getNextWaiters, req)
		return
	}
	if err != nil {
		req.responseCh <- itemResponse[T]{err: err}
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

func (q *queue[T]) GetById(objectId string) (T, error) {
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

func (q *queue[T]) GetNext(req GetNextRequest[T]) (T, error) {
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

func (q *queue[T]) GetNextScheduled(req GetNextScheduledRequest[T]) (T, error) {
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

func (q *queue[T]) Upsert(id string, modifier func(exists bool, prev T) T) error {
	it, err := q.GetById(id)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	exists := !errors.Is(err, ErrNotFound)

	next := modifier(exists, it)

	return q.Release(next)
}

func (q *queue[T]) Release(task T) error {
	responseCh := make(chan error, 1)

	q.releaseCh <- releaseRequest[T]{
		action:     releaseActionUpdate,
		item:       task,
		responseCh: responseCh,
	}

	return <-responseCh
}
