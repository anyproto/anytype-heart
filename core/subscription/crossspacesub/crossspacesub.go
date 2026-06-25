package crossspacesub

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/cheggaaa/mb/v3"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	subscriptionservice "github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/pb"
)

type Predicate func(details *domain.Details) bool

func NoOpPredicate() Predicate {
	return func(details *domain.Details) bool {
		return true
	}
}

type crossSpaceSubscription struct {
	subId string

	request subscriptionservice.SubscribeRequest

	eventSender         event.Sender
	subscriptionService subscriptionservice.Service

	spacePredicate Predicate

	ctx       context.Context
	ctxCancel context.CancelFunc
	queue     *mb.MB[*pb.EventMessage]

	clk          clock
	createdAt    time.Time
	initialGrace time.Duration
	window       time.Duration

	lock sync.Mutex
	// spaceId => subId (only finalized, real subscriptions)
	perSpaceSubscriptions map[string]string
	// spaceId => reservation token of an in-flight subscribe (see reserve /
	// completeReservation). The token identifies the owning call so a
	// concurrent RemoveSpace/AddSpace cannot be mistaken for our own slot.
	inflightSpaceIds map[string]uint64
	reservationSeq   uint64
	// spaces matched by predicate whose objectstore is not opened yet;
	// promoted to perSpaceSubscriptions when the objectstore opens.
	pendingSpaceIds map[string]struct{}
	// internal sub id (bson id) => total count
	totalCounts map[string]int64
	// internal sub ids whose counters may still be aggregated. A per-space sub
	// is added when finalized and removed when its space is removed/rolled
	// back, so a counter still queued for a removed space cannot resurrect its
	// total (GO-7337).
	activeInternalSubs map[string]struct{}
}

func newCrossSpaceSubscription(subId string, request subscriptionservice.SubscribeRequest, eventSender event.Sender, subscriptionService subscriptionservice.Service, loadedSpaceIds []string, pendingSpaceIds []string, predicate Predicate, clk clock, grace, window time.Duration) (*crossSpaceSubscription, *subscriptionservice.SubscribeResponse, error) {
	ctx, ctxCancel := context.WithCancel(context.Background())
	s := &crossSpaceSubscription{
		ctx:                   ctx,
		ctxCancel:             ctxCancel,
		subId:                 subId,
		request:               request,
		eventSender:           eventSender,
		spacePredicate:        predicate,
		subscriptionService:   subscriptionService,
		perSpaceSubscriptions: make(map[string]string),
		inflightSpaceIds:      make(map[string]uint64),
		pendingSpaceIds:       make(map[string]struct{}, len(pendingSpaceIds)),
		totalCounts:           map[string]int64{},
		activeInternalSubs:    map[string]struct{}{},
		queue:                 mb.New[*pb.EventMessage](0),
		clk:                   clk,
		initialGrace:          grace,
		window:                window,
	}
	for _, spaceId := range pendingSpaceIds {
		s.pendingSpaceIds[spaceId] = struct{}{}
	}
	aggregatedResp := &subscriptionservice.SubscribeResponse{
		SubId:    subId,
		Counters: &pb.EventObjectSubscriptionCounters{},
	}

	var wg sync.WaitGroup
	var resErr error

	for _, spaceId := range loadedSpaceIds {
		wg.Add(1)
		go func() {
			defer wg.Done()

			resp, err := s.subscribe(spaceId, false)
			if err != nil {
				// resErr is shared across the per-space goroutines; guard it
				// with the same lock as the aggregation below and join so a
				// concurrent failure is not lost to last-writer-wins.
				s.lock.Lock()
				resErr = errors.Join(resErr, err)
				s.lock.Unlock()
				return
			}

			s.lock.Lock()
			s.perSpaceSubscriptions[spaceId] = resp.SubId
			s.activeInternalSubs[resp.SubId] = struct{}{}
			aggregatedResp.Records = append(aggregatedResp.Records, resp.Records...)
			aggregatedResp.Dependencies = append(aggregatedResp.Dependencies, resp.Dependencies...)
			aggregatedResp.Counters.Total += resp.Counters.Total
			s.lock.Unlock()

			s.updateTotalCount(resp.SubId, resp.Counters.Total)
		}()
	}
	wg.Wait()

	// anchor the grace as late as the server can — close to when the response
	// is sent — so the initial-grace budget is not eaten by the synchronous
	// loaded-space subscribes above.
	s.createdAt = s.clk.now()
	return s, aggregatedResp, resErr
}

func (s *crossSpaceSubscription) run(internalQueue *mb.MB[*pb.EventMessage]) {
	if internalQueue != nil {
		s.runNested(internalQueue)
		return
	}
	s.runBroadcast()
}

// runNested forwards each patched message to the parent queue (unchanged
// behavior for nested cross-space subscriptions).
func (s *crossSpaceSubscription) runNested(internalQueue *mb.MB[*pb.EventMessage]) {
	for {
		msgs, err := s.queue.Wait(s.ctx)
		if errors.Is(err, context.Canceled) {
			return
		}
		if err != nil {
			log.Error("wait messages", zap.Error(err), zap.String("subId", s.subId))
		}
		for _, msg := range msgs {
			s.patchEvent(msg)
			if aerr := internalQueue.Add(s.ctx, msg); aerr != nil {
				log.Error("add to internal queue", zap.Error(aerr), zap.String("subId", s.subId))
			}
		}
	}
}

// runBroadcast coalesces patched messages and broadcasts them, holding the
// first emission for the initial grace. A drainer goroutine converts the
// blocking queue into a channel so the loop can select message-vs-timer.
func (s *crossSpaceSubscription) runBroadcast() {
	msgCh := make(chan []*pb.EventMessage)
	go s.drain(msgCh)

	c := newCoalescer(s.createdAt, s.initialGrace, s.window, maxFlushSize)
	flush := func() {
		for _, batch := range c.ready(s.clk.now()) {
			log.Debug("crossspacesub broadcast flush",
				zap.String("subId", s.subId), zap.Int("messages", len(batch)))
			s.eventSender.Broadcast(&pb.Event{Messages: batch})
		}
	}
	for {
		var timerC <-chan time.Time
		if d := c.nextDeadline(); !d.IsZero() {
			timerC = s.clk.after(d.Sub(s.clk.now()))
		}
		select {
		case <-s.ctx.Done():
			return
		case msgs, ok := <-msgCh:
			if !ok {
				return
			}
			for _, m := range msgs {
				s.patchEvent(m)
			}
			c.push(s.clk.now(), msgs)
			flush()
		case <-timerC:
			flush()
		}
	}
}

// drain reads bounded batches from the queue and hands them to runBroadcast,
// stopping when the queue closes or the subscription is canceled.
func (s *crossSpaceSubscription) drain(out chan<- []*pb.EventMessage) {
	defer close(out)
	cond := s.queue.NewCond().WithMax(maxFlushSize)
	for {
		msgs, err := s.queue.WaitCond(s.ctx, cond)
		if err != nil {
			return
		}
		select {
		case out <- msgs:
		case <-s.ctx.Done():
			return
		}
	}
}

func (s *crossSpaceSubscription) patchEvent(msg *pb.EventMessage) {
	matcher := subscriptionservice.EventMatcher{
		OnAdd: func(spaceId string, add *pb.EventObjectSubscriptionAdd) {
			add.SubId = s.subId
			add.AfterId = ""
		},
		OnRemove: func(spaceId string, remove *pb.EventObjectSubscriptionRemove) {
			remove.SubId = s.subId
		},
		OnPosition: func(spaceId string, position *pb.EventObjectSubscriptionPosition) {
			position.SubId = s.subId
			position.AfterId = ""
		},
		OnSet: func(spaceId string, set *pb.EventObjectDetailsSet) {
			set.SubIds = []string{s.subId}
		},
		OnUnset: func(spaceId string, unset *pb.EventObjectDetailsUnset) {
			unset.SubIds = []string{s.subId}
		},
		OnAmend: func(spaceId string, amend *pb.EventObjectDetailsAmend) {
			amend.SubIds = []string{s.subId}
		},
		OnCounters: func(spaceId string, counters *pb.EventObjectSubscriptionCounters) {
			total := s.updateTotalCount(counters.SubId, counters.Total)
			counters.Total = total
			counters.SubId = s.subId
		},
		OnGroups: func(spaceId string, groups *pb.EventObjectSubscriptionGroups) {
			groups.SubId = s.subId
		},
	}
	matcher.Match(msg)
}

func (s *crossSpaceSubscription) close() error {
	s.ctxCancel()
	return s.queue.Close()
}

func (s *crossSpaceSubscription) AddSpace(spaceId string) error {
	token, ok := s.reserve(spaceId, false)
	if !ok {
		return nil
	}
	if err := s.completeReservation(spaceId, token); err != nil {
		return fmt.Errorf("add space: %w", err)
	}
	return nil
}

// AddPending records spaceId as a pending space whose objectstore is not yet
// opened. When the objectstore opens, PromotePending should be called to
// upgrade it to a real per-space subscription.
func (s *crossSpaceSubscription) AddPending(spaceId string) {
	s.lock.Lock()
	defer s.lock.Unlock()
	if _, ok := s.perSpaceSubscriptions[spaceId]; ok {
		return
	}
	if _, ok := s.inflightSpaceIds[spaceId]; ok {
		// A subscribe is in flight; it will establish the subscription.
		// Re-pending here would cause a duplicate promote later.
		return
	}
	s.pendingSpaceIds[spaceId] = struct{}{}
}

// PromotePending upgrades a pending space to a real per-space subscription
// with asyncInit=true, so initial records flow as events through the internal
// queue. A no-op if spaceId is not pending.
func (s *crossSpaceSubscription) PromotePending(spaceId string) error {
	token, ok := s.reserve(spaceId, true)
	if !ok {
		return nil
	}
	if err := s.completeReservation(spaceId, token); err != nil {
		// Restore pending so a later objectstore open retries the promote,
		// unless the space is already subscribed or another subscribe is in
		// flight (that one will establish it).
		s.lock.Lock()
		_, subscribed := s.perSpaceSubscriptions[spaceId]
		_, inflight := s.inflightSpaceIds[spaceId]
		if !subscribed && !inflight {
			s.pendingSpaceIds[spaceId] = struct{}{}
		}
		s.lock.Unlock()
		return fmt.Errorf("promote pending space: %w", err)
	}
	return nil
}

// reserve atomically claims the right to subscribe spaceId, consuming its
// pending entry in the same critical section. The atomicity matters: if the
// pending-consume and the claim were separate critical sections, a
// RemoveSpace between them would find nothing to cancel (no pending entry,
// no reservation, no sub) and the subscribe would then resurrect the
// just-removed space. With fromPending, the claim is only made if spaceId is
// still pending. ok is false when there is nothing to do: not pending (with
// fromPending), already subscribed, or another subscribe in flight.
func (s *crossSpaceSubscription) reserve(spaceId string, fromPending bool) (token uint64, ok bool) {
	s.lock.Lock()
	defer s.lock.Unlock()
	if fromPending {
		if _, pending := s.pendingSpaceIds[spaceId]; !pending {
			return 0, false
		}
	}
	delete(s.pendingSpaceIds, spaceId)
	if _, subscribed := s.perSpaceSubscriptions[spaceId]; subscribed {
		return 0, false
	}
	if _, inflight := s.inflightSpaceIds[spaceId]; inflight {
		return 0, false
	}
	s.reservationSeq++
	s.inflightSpaceIds[spaceId] = s.reservationSeq
	return s.reservationSeq, true
}

// completeReservation subscribes spaceId and finalizes the reservation
// claimed by reserve().
//
// subscribe() re-enters the subscription service (Search), which can call
// back into this subscription (objectstore open -> PromotePending). Holding
// s.lock across that call is the GO-7288 ABBA deadlock (subscription-service
// lock <-> s.lock), so the Search runs with s.lock released.
//
// perSpaceSubscriptions only ever holds finalized real subIds. After Search
// returns we finalize only if our exact token is still the inflight
// reservation and the cross-space subscription is still open. A RemoveSpace
// that races the in-flight subscribe deletes the token, and a close() means
// nothing would ever unsubscribe what we just created; in both cases the
// freshly created sub is rolled back instead of overwriting a live one or
// leaking it.
func (s *crossSpaceSubscription) completeReservation(spaceId string, token uint64) error {
	resp, err := s.subscribe(spaceId, true)

	s.lock.Lock()
	cur, stillReserved := s.inflightSpaceIds[spaceId]
	ours := stillReserved && cur == token
	if ours {
		delete(s.inflightSpaceIds, spaceId)
	}
	if err != nil {
		s.lock.Unlock()
		return err
	}
	closed := s.ctx.Err() != nil
	if !ours || closed {
		s.lock.Unlock()
		s.rollbackSubscription(spaceId, resp.SubId)
		return nil
	}
	s.perSpaceSubscriptions[spaceId] = resp.SubId
	s.activeInternalSubs[resp.SubId] = struct{}{}
	s.lock.Unlock()
	return nil
}

// rollbackSubscription unsubscribes a freshly created per-space subscription
// that lost its reservation (concurrent RemoveSpace) or finalized after
// close(). Like removeSpace, it emits SubscriptionRemove for the ids the sub
// had already delivered through its async-init events — without them the
// client keeps ghost records of a removed space — plus a zeroing counters
// event. After close() the queue is gone and the client unsubscribed, so
// failing to enqueue is fine.
func (s *crossSpaceSubscription) rollbackSubscription(spaceId string, subId string) {
	ids, err := s.subscriptionService.UnsubscribeAndReturnIds(spaceId, subId)
	if err != nil {
		log.Error("rollback per-space subscription",
			zap.String("subId", s.subId), zap.String("spaceId", spaceId), zap.Error(err))
		return
	}
	msgs := make([]*pb.EventMessage, 0, len(ids)+1)
	for _, id := range ids {
		msgs = append(msgs, event.NewMessage(spaceId, &pb.EventMessageValueOfSubscriptionRemove{
			SubscriptionRemove: &pb.EventObjectSubscriptionRemove{
				SubId: s.subId,
				Id:    id,
			},
		}))
	}
	msgs = append(msgs, event.NewMessage(spaceId, &pb.EventMessageValueOfSubscriptionCounters{
		SubscriptionCounters: &pb.EventObjectSubscriptionCounters{
			// the cross-space subId, not the internal one: patchEvent feeds
			// counters through updateTotalCount keyed by SubId, and the
			// internal id would re-insert the per-space total just removed
			SubId: s.subId,
			Total: 0,
		},
	}))
	if aerr := s.queue.Add(s.ctx, msgs...); aerr != nil &&
		!errors.Is(aerr, mb.ErrClosed) && !errors.Is(aerr, context.Canceled) {
		log.Error("rollback subscription: send removal events",
			zap.String("subId", s.subId), zap.String("spaceId", spaceId), zap.Error(aerr))
	}
}

func (s *crossSpaceSubscription) subscribe(spaceId string, asyncInit bool) (*subscriptionservice.SubscribeResponse, error) {
	req := s.request
	// Will be generated automatically
	req.SubId = ""
	req.Internal = true
	req.InternalQueue = s.queue
	req.SpaceId = spaceId
	req.AsyncInit = asyncInit

	return s.subscriptionService.Search(req)
}

func (s *crossSpaceSubscription) RemoveSpace(spaceId string) {
	s.lock.Lock()
	defer s.lock.Unlock()
	delete(s.pendingSpaceIds, spaceId)
	err := s.removeSpace(spaceId)
	if err != nil {
		log.Error("remove space", zap.Error(err), zap.String("subId", s.subId), zap.String("spaceId", spaceId))
	}
}

func (s *crossSpaceSubscription) removeSpace(spaceId string) error {
	// Cancel any in-flight subscribe: clearing its reservation token makes
	// the subscriber roll back the sub it is creating instead of finalizing
	// an untracked one.
	delete(s.inflightSpaceIds, spaceId)
	subId, ok := s.perSpaceSubscriptions[spaceId]
	if ok {
		ids, err := s.subscriptionService.UnsubscribeAndReturnIds(spaceId, subId)
		if err != nil {
			return err
		}
		for _, id := range ids {
			err = s.queue.Add(s.ctx, event.NewMessage(spaceId, &pb.EventMessageValueOfSubscriptionRemove{
				SubscriptionRemove: &pb.EventObjectSubscriptionRemove{
					SubId: s.subId,
					Id:    id,
				},
			},
			))
			if err != nil {
				return fmt.Errorf("send remove event to queue: %w", err)
			}
		}

		// mark inactive before draining its total so any counter for this
		// internal sub still queued ahead is ignored when patched (GO-7337)
		delete(s.activeInternalSubs, subId)
		total := s.removeTotalCount(subId)
		err = s.queue.Add(s.ctx, event.NewMessage(spaceId, &pb.EventMessageValueOfSubscriptionCounters{
			SubscriptionCounters: &pb.EventObjectSubscriptionCounters{
				// the cross-space subId: keyed by the internal id, patchEvent's
				// updateTotalCount would re-insert the per-space total this
				// removal just deleted, double-counting every later total
				SubId: s.subId,
				Total: total,
			},
		},
		))
		if err != nil {
			return fmt.Errorf("send counters event to queue: %w", err)
		}
		delete(s.perSpaceSubscriptions, spaceId)
	}
	return nil
}

func (s *crossSpaceSubscription) updateTotalCount(internalSubId string, perSpaceTotal int64) int64 {
	s.lock.Lock()
	defer s.lock.Unlock()

	if internalSubId == s.subId {
		// a synthesized counters event (space removal/rollback) — already
		// aggregated, nothing per-space to record
		return s.getTotalCount()
	}
	if _, active := s.activeInternalSubs[internalSubId]; !active {
		// counter for a removed/rolled-back per-space sub: ignore so a stale
		// queued counter cannot resurrect the removed space's total (GO-7337)
		return s.getTotalCount()
	}
	s.totalCounts[internalSubId] = perSpaceTotal

	return s.getTotalCount()
}

// removeTotalCount should be only called under s.lock
func (s *crossSpaceSubscription) removeTotalCount(internalSubId string) int64 {
	delete(s.totalCounts, internalSubId)

	return s.getTotalCount()
}

// getTotalCount should be only called under s.lock
func (s *crossSpaceSubscription) getTotalCount() int64 {
	var total int64
	for _, t := range s.totalCounts {
		total += t
	}
	return total
}
