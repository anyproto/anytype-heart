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
	// spaceId => subId
	perSpaceSubscriptions map[string]string
	// internal sub id (bson id) => total count
	totalCounts map[string]int64
}

func newCrossSpaceSubscription(subId string, request subscriptionservice.SubscribeRequest, eventSender event.Sender, subscriptionService subscriptionservice.Service, initialSpaceIds []string, predicate Predicate) (*crossSpaceSubscription, *subscriptionservice.SubscribeResponse, error) {
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
		totalCounts:           map[string]int64{},
		queue:                 mb.New[*pb.EventMessage](0),
	}
	aggregatedResp := &subscriptionservice.SubscribeResponse{
		SubId:    subId,
		Counters: &pb.EventObjectSubscriptionCounters{},
	}
	for _, spaceId := range initialSpaceIds {
		resp, err := s.addSpace(spaceId, false)
		if err != nil {
			return nil, nil, fmt.Errorf("add space: %w", err)
		}
		aggregatedResp.Records = append(aggregatedResp.Records, resp.Records...)
		aggregatedResp.Dependencies = append(aggregatedResp.Dependencies, resp.Dependencies...)
		aggregatedResp.Counters.Total += resp.Counters.Total

		s.updateTotalCount(resp.SubId, resp.Counters.Total)
	}
	return s, aggregatedResp, nil
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
	defer s.lock.Unlock()
	_, err := s.addSpace(spaceId, true)
	if err != nil {
		return fmt.Errorf("add space: %w", err)
	}

	return nil
}

func (s *crossSpaceSubscription) addSpace(spaceId string, asyncInit bool) (*subscriptionservice.SubscribeResponse, error) {
	if _, ok := s.perSpaceSubscriptions[spaceId]; ok {
		return nil, nil
	}

	req := s.request
	// Will be generated automatically
	req.SubId = ""
	req.Internal = true
	req.InternalQueue = s.queue
	req.SpaceId = spaceId
	req.AsyncInit = asyncInit

	resp, err := s.subscriptionService.Search(req)
	if err != nil {
		return nil, err
	}
	s.perSpaceSubscriptions[spaceId] = resp.SubId
	return resp, nil
}

func (s *crossSpaceSubscription) RemoveSpace(spaceId string) {
	s.lock.Lock()
	defer s.lock.Unlock()
	err := s.removeSpace(spaceId)
	if err != nil {
		log.Error("remove space", zap.Error(err), zap.String("subId", s.subId), zap.String("spaceId", spaceId))
	}
}

func (s *crossSpaceSubscription) removeSpace(spaceId string) error {
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
