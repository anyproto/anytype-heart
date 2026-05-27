package crossspacesub

import (
	"context"
	"errors"
	"fmt"
	"sync"

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

	lock sync.Mutex
	// spaceId => subId (only finalized, real subscriptions)
	perSpaceSubscriptions map[string]string
	// spaceId => reservation token of an in-flight subscribe (see
	// ensureSpaceSubscribed). The token identifies the owning call so a
	// concurrent RemoveSpace/AddSpace cannot be mistaken for our own slot.
	inflightSpaceIds map[string]uint64
	reservationSeq   uint64
	// spaces matched by predicate whose objectstore is not opened yet;
	// promoted to perSpaceSubscriptions when the objectstore opens.
	pendingSpaceIds map[string]struct{}
	// internal sub id (bson id) => total count
	totalCounts map[string]int64
}

func newCrossSpaceSubscription(subId string, request subscriptionservice.SubscribeRequest, eventSender event.Sender, subscriptionService subscriptionservice.Service, loadedSpaceIds []string, pendingSpaceIds []string, predicate Predicate) (*crossSpaceSubscription, *subscriptionservice.SubscribeResponse, error) {
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
		queue:                 mb.New[*pb.EventMessage](0),
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
			aggregatedResp.Records = append(aggregatedResp.Records, resp.Records...)
			aggregatedResp.Dependencies = append(aggregatedResp.Dependencies, resp.Dependencies...)
			aggregatedResp.Counters.Total += resp.Counters.Total
			s.lock.Unlock()

			s.updateTotalCount(resp.SubId, resp.Counters.Total)
		}()
	}
	wg.Wait()

	return s, aggregatedResp, resErr
}

func (s *crossSpaceSubscription) run(internalQueue *mb.MB[*pb.EventMessage]) {
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
			if internalQueue != nil {
				err = internalQueue.Add(s.ctx, msg)
				if err != nil {
					log.Error("add to internal queue", zap.Error(err), zap.String("subId", s.subId))
				}
			}
		}

		if internalQueue == nil {
			s.eventSender.Broadcast(&pb.Event{
				Messages: msgs,
			})
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
	s.lock.Lock()
	delete(s.pendingSpaceIds, spaceId)
	s.lock.Unlock()
	if err := s.ensureSpaceSubscribed(spaceId); err != nil {
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
	s.lock.Lock()
	if _, ok := s.pendingSpaceIds[spaceId]; !ok {
		s.lock.Unlock()
		return nil
	}
	delete(s.pendingSpaceIds, spaceId)
	s.lock.Unlock()

	if err := s.ensureSpaceSubscribed(spaceId); err != nil {
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

// ensureSpaceSubscribed subscribes spaceId unless it is already subscribed or
// a subscribe is in flight.
//
// subscribe() re-enters the subscription service (Search), which can call
// back into this subscription (objectstore open -> PromotePending). Holding
// s.lock across that call is the GO-7288 ABBA deadlock (subscription-service
// lock <-> s.lock), so the Search runs with s.lock released.
//
// Concurrency is serialized by a per-call reservation token in
// inflightSpaceIds (claimed under s.lock); perSpaceSubscriptions only ever
// holds finalized real subIds. After Search returns we finalize only if our
// exact token is still the inflight reservation. A RemoveSpace (or a
// superseding reservation) that races an in-flight subscribe deletes/replaces
// the token; we detect token mismatch and roll back the just-created sub
// instead of overwriting the live one or leaking it.
func (s *crossSpaceSubscription) ensureSpaceSubscribed(spaceId string) error {
	s.lock.Lock()
	if _, ok := s.perSpaceSubscriptions[spaceId]; ok { // already subscribed
		s.lock.Unlock()
		return nil
	}
	if _, ok := s.inflightSpaceIds[spaceId]; ok { // another subscribe in flight
		s.lock.Unlock()
		return nil
	}
	s.reservationSeq++
	token := s.reservationSeq
	s.inflightSpaceIds[spaceId] = token
	s.lock.Unlock()

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
	if !ours {
		// RemoveSpace cancelled this reservation (or it was superseded)
		// while we were subscribing: roll back the freshly created sub
		// instead of leaving it dangling/untracked.
		s.lock.Unlock()
		if _, uerr := s.subscriptionService.UnsubscribeAndReturnIds(spaceId, resp.SubId); uerr != nil {
			log.Error("rollback subscription after concurrent remove",
				zap.String("subId", s.subId), zap.String("spaceId", spaceId), zap.Error(uerr))
		}
		return nil
	}
	s.perSpaceSubscriptions[spaceId] = resp.SubId
	s.lock.Unlock()
	return nil
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

		total := s.removeTotalCount(subId)
		err = s.queue.Add(s.ctx, event.NewMessage(spaceId, &pb.EventMessageValueOfSubscriptionCounters{
			SubscriptionCounters: &pb.EventObjectSubscriptionCounters{
				SubId: subId,
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
