package crossspacesub

import (
	"context"
	"fmt"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/cheggaaa/mb/v3"
	"github.com/globalsign/mgo/bson"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/event"
	subscriptionservice "github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var log = logging.Logger(CName).Desugar()

const CName = "core.subscription.crossspacesub"

type Service interface {
	app.ComponentRunnable
	Subscribe(req subscriptionservice.SubscribeRequest) (resp *subscriptionservice.SubscribeResponse, err error)
}

type service struct {
	spaceService        space.Service
	subscriptionService subscriptionservice.Service
	eventSender         event.Sender

	componentCtx       context.Context
	componentCtxCancel context.CancelFunc

	lock               sync.Mutex
	spaceViewsSubId    string
	spaceViewTargetIds map[string]string
	spaceIds           []string
	subscriptions      map[string]*crossSpaceSubscription
}

func New() Service {
	return &service{}
}

func (s *service) Init(a *app.App) error {
	s.componentCtx, s.componentCtxCancel = context.WithCancel(context.Background())
	s.spaceService = app.MustComponent[space.Service](a)
	s.subscriptionService = app.MustComponent[subscriptionservice.Service](a)
	s.eventSender = app.MustComponent[event.Sender](a)
	s.subscriptions = map[string]*crossSpaceSubscription{}
	s.spaceViewTargetIds = map[string]string{}

	return nil
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) Run(ctx context.Context) error {
	return s.runSpaceViewSub()
}

func (s *service) Close(ctx context.Context) error {
	s.componentCtxCancel()
	s.lock.Lock()
	err := s.subscriptionService.Unsubscribe(s.spaceViewsSubId)
	s.lock.Unlock()
	return err
}

func (s *service) Subscribe(req subscriptionservice.SubscribeRequest) (*subscriptionservice.SubscribeResponse, error) {
	if req.Limit != 0 {
		return nil, fmt.Errorf("limit is not supported")
	}
	if req.AfterId != "" || req.BeforeId != "" {
		return nil, fmt.Errorf("pagination is not supported")
	}
	if req.CollectionId != "" {
		return nil, fmt.Errorf("collection is not supported")
	}
	if req.SubId == "" {
		req.SubId = bson.NewObjectId().Hex()
	}
	s.lock.Lock()
	defer s.lock.Unlock()
	spaceSub, resp, err := newCrossSpaceSubscription(req.SubId, req, s.eventSender, s.subscriptionService, s.spaceIds)
	if err != nil {
		return nil, fmt.Errorf("new cross space subscription: %w", err)
	}
	s.subscriptions[req.SubId] = spaceSub
	go spaceSub.run()
	return resp, nil
}

func (s *service) Unsubscribe(subId string) error {
	return fmt.Errorf("not implemented")
}

type crossSpaceSubscription struct {
	subId string

	request subscriptionservice.SubscribeRequest

	eventSender         event.Sender
	subscriptionService subscriptionservice.Service

	ctx       context.Context
	ctxCancel context.CancelFunc
	queue     *mb.MB[*pb.EventMessage]

	lock sync.Mutex
	// spaceId => subId
	perSpaceSubscriptions map[string]string
}

func newCrossSpaceSubscription(subId string, request subscriptionservice.SubscribeRequest, eventSender event.Sender, subscriptionService subscriptionservice.Service, initialSpaceIds []string) (*crossSpaceSubscription, *subscriptionservice.SubscribeResponse, error) {
	ctx, ctxCancel := context.WithCancel(context.Background())
	s := &crossSpaceSubscription{
		ctx:                   ctx,
		ctxCancel:             ctxCancel,
		subId:                 subId,
		request:               request,
		eventSender:           eventSender,
		subscriptionService:   subscriptionService,
		perSpaceSubscriptions: make(map[string]string),
		queue:                 mb.New[*pb.EventMessage](0),
	}
	aggregatedResp := &subscriptionservice.SubscribeResponse{
		SubId:    subId,
		Counters: &pb.EventObjectSubscriptionCounters{},
	}
	for _, spaceId := range initialSpaceIds {
		resp, err := s.addSpace(spaceId)
		if err != nil {
			return nil, nil, fmt.Errorf("add space: %w", err)
		}
		aggregatedResp.Records = append(aggregatedResp.Records, resp.Records...)
		aggregatedResp.Dependencies = append(aggregatedResp.Dependencies, resp.Dependencies...)
		aggregatedResp.Counters.Total += resp.Counters.Total
	}
	return s, aggregatedResp, nil
}

func (s *crossSpaceSubscription) run() {
	for {
		msgs, err := s.queue.Wait(s.ctx)
		if err != nil {
			log.Error("wait messages", zap.Error(err), zap.String("subId", s.subId))
		}
		for _, msg := range msgs {
			s.patchEvent(msg)
		}

		s.eventSender.Broadcast(&pb.Event{
			Messages: msgs,
		})
	}
}

func (s *crossSpaceSubscription) patchEvent(msg *pb.EventMessage) {
	matcher := subscriptionservice.EventMatcher{
		OnAdd: func(add *pb.EventObjectSubscriptionAdd) {
			add.SubId = s.subId
		},
		OnRemove: func(remove *pb.EventObjectSubscriptionRemove) {
			remove.SubId = s.subId
		},
		OnPosition: func(position *pb.EventObjectSubscriptionPosition) {
			position.SubId = s.subId
		},
		OnSet: func(set *pb.EventObjectDetailsSet) {
			set.SubIds = []string{s.subId}
		},
		OnUnset: func(unset *pb.EventObjectDetailsUnset) {
			unset.SubIds = []string{s.subId}
		},
		OnAmend: func(amend *pb.EventObjectDetailsAmend) {
			amend.SubIds = []string{s.subId}
		},
		OnCounters: func(counters *pb.EventObjectSubscriptionCounters) {
			// TODO Fix this: use shared Total
			counters.SubId = s.subId
		},
		OnGroups: func(groups *pb.EventObjectSubscriptionGroups) {
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
	resp, err := s.addSpace(spaceId)
	if err != nil {
		return fmt.Errorf("add space: %w", err)
	}

	for _, rec := range resp.Records {
		id := pbtypes.GetString(rec, bundle.RelationKeyId.String())
		err = s.queue.Add(s.ctx, &pb.EventMessage{
			Value: &pb.EventMessageValueOfObjectDetailsSet{
				ObjectDetailsSet: &pb.EventObjectDetailsSet{
					Id:      id,
					Details: rec,
				},
			},
		})
		return fmt.Errorf("add set event: %w", err)
	}

	var afterId string
	for _, rec := range resp.Records {
		id := pbtypes.GetString(rec, bundle.RelationKeyId.String())
		err = s.queue.Add(s.ctx, &pb.EventMessage{
			Value: &pb.EventMessageValueOfSubscriptionAdd{
				SubscriptionAdd: &pb.EventObjectSubscriptionAdd{
					Id:      id,
					SubId:   s.subId,
					AfterId: afterId,
				},
			},
		})
		if err != nil {
			return fmt.Errorf("add subscription add event: %w", err)
		}
		afterId = id
	}

	err = s.queue.Add(s.ctx, &pb.EventMessage{
		Value: &pb.EventMessageValueOfSubscriptionCounters{
			SubscriptionCounters: &pb.EventObjectSubscriptionCounters{
				SubId: s.subId,
				Total: int64(len(resp.Records)),
			},
		},
	})
	if err != nil {
		return fmt.Errorf("add counters event: %w", err)
	}

	return nil
}

func (s *crossSpaceSubscription) addSpace(spaceId string) (*subscriptionservice.SubscribeResponse, error) {
	// TODO Do I need to check if subscripion already exists?

	req := s.request
	// Will be generated automatically
	req.SubId = ""
	req.Internal = true
	req.InternalQueue = s.queue
	req.SpaceId = spaceId

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
		// TODO Use UnsubscribeInSpace
		err := s.subscriptionService.Unsubscribe(subId)
		if err != nil {
			return err
		}
		delete(s.perSpaceSubscriptions, spaceId)
	}
	return nil
}
