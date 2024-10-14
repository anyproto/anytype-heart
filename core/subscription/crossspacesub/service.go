package crossspacesub

import (
	"context"
	"fmt"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/globalsign/mgo/bson"

	"github.com/anyproto/anytype-heart/core/event"
	subscriptionservice "github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/space"
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
	if !req.NoDepSubscription {
		return nil, fmt.Errorf("dependency subscription is not yet supported")
	}
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
	if len(req.Sorts) > 0 {
		return nil, fmt.Errorf("sorting is not supported")
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
	s.lock.Lock()
	defer s.lock.Unlock()

	sub, ok := s.subscriptions[subId]
	if !ok {
		return fmt.Errorf("subscription not found")
	}

	err := sub.close()
	if err != nil {
		return fmt.Errorf("close subscription: %w", err)
	}
	delete(s.subscriptions, subId)

	return nil
}
