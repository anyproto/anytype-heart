package crossspacesub

/*
AI generated

Name: Cross-Space Object Subscription Aggregator
Scope: global

## Responsibility
- Aggregates object subscriptions across multiple spaces into a single unified subscription
- Monitors space views to dynamically include/exclude spaces from cross-space subscriptions
- Supports predicates to filter spaces based on space view details (creator, status, etc.)
- Re-maps per-space subscription events to unified cross-space subscription ID

## Background Tasks
- monitorSpaceViewSub: Watches space view changes to add/remove spaces from active subscriptions (monitorSpaceViewSub)
- crossSpaceSubscription.run: Processes per-space events, aggregates counters, broadcasts unified events (run)

## Documentation
On Run, subscribes to all space views in tech space. When Subscribe is called, creates per-space
subscriptions for spaces matching the predicate. Space view changes trigger dynamic add/remove of
spaces from active subscriptions, with events re-mapped to the unified subscription ID.
*/

import (
	"context"
	"fmt"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/globalsign/mgo/bson"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event"
	subscriptionservice "github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/space"
)

var log = logging.Logger(CName).Desugar()

const CName = "core.subscription.crossspacesub"

var (
	ErrSubscriptionNotFound = fmt.Errorf("subscription not found")
)

type Service interface {
	app.ComponentRunnable
	Subscribe(req subscriptionservice.SubscribeRequest, predicate Predicate) (*subscriptionservice.SubscribeResponse, error)
	Unsubscribe(subId string) error
}

type service struct {
	spaceService        space.Service
	subscriptionService subscriptionservice.Service
	eventSender         event.Sender
	objectStore         objectstore.ObjectStore

	componentCtx       context.Context
	componentCtxCancel context.CancelFunc

	lock             sync.Mutex
	spaceViewsSubId  string
	spaceViewDetails map[string]*domain.Details
	// spaceViewId => targetSpaceId
	spaceViewTargetIds map[string]string
	spaceIds           []string
	subscriptions      map[string]*crossSpaceSubscription
	// spaces whose objectstore DB has been opened
	openedSpaceIds map[string]struct{}
}

func New() Service {
	return &service{}
}

func (s *service) Init(a *app.App) error {
	s.componentCtx, s.componentCtxCancel = context.WithCancel(context.Background())
	s.spaceService = app.MustComponent[space.Service](a)
	s.subscriptionService = app.MustComponent[subscriptionservice.Service](a)
	s.eventSender = app.MustComponent[event.Sender](a)
	s.objectStore = app.MustComponent[objectstore.ObjectStore](a)
	s.subscriptions = map[string]*crossSpaceSubscription{}
	s.spaceViewTargetIds = map[string]string{}
	s.spaceViewDetails = map[string]*domain.Details{}
	s.openedSpaceIds = map[string]struct{}{}

	return nil
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) Run(ctx context.Context) error {
	// Register before runSpaceViewSub so that the tech-space open (and any
	// already-opened spaces) replay through onSpaceIndexOpened into our
	// openedSpaceIds map before Subscribe can be called.
	s.objectStore.OnSpaceIndexOpened(s.onSpaceIndexOpened)
	return s.runSpaceViewSub()
}

func (s *service) onSpaceIndexOpened(spaceId string) {
	s.lock.Lock()
	if _, ok := s.openedSpaceIds[spaceId]; ok {
		s.lock.Unlock()
		return
	}
	s.openedSpaceIds[spaceId] = struct{}{}
	subs := make([]*crossSpaceSubscription, 0, len(s.subscriptions))
	for _, sub := range s.subscriptions {
		subs = append(subs, sub)
	}
	s.lock.Unlock()
	for _, sub := range subs {
		if err := sub.PromotePending(spaceId); err != nil {
			log.Error("promote pending space",
				zap.String("subId", sub.subId),
				zap.String("spaceId", spaceId),
				zap.Error(err))
		}
	}
}

func (s *service) Close(ctx context.Context) error {
	s.componentCtxCancel()
	s.lock.Lock()
	err := s.subscriptionService.Unsubscribe(s.spaceViewsSubId)
	s.lock.Unlock()
	for subId := range s.subscriptions {
		_ = s.Unsubscribe(subId)
	}
	return err
}

func (s *service) Subscribe(req subscriptionservice.SubscribeRequest, spaceViewPredicate Predicate) (*subscriptionservice.SubscribeResponse, error) {
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
	if req.AsyncInit {
		return nil, fmt.Errorf("async init is not supported")
	}

	s.lock.Lock()
	defer s.lock.Unlock()
	var loadedIds, pendingIds []string
	for spaceViewId, details := range s.spaceViewDetails {
		if spaceViewPredicate(details) {
			targetSpaceId, ok := s.spaceViewTargetIds[spaceViewId]
			if !ok {
				continue
			}
			if _, opened := s.openedSpaceIds[targetSpaceId]; opened {
				loadedIds = append(loadedIds, targetSpaceId)
			} else {
				pendingIds = append(pendingIds, targetSpaceId)
			}
		}
	}
	spaceSub, resp, err := newCrossSpaceSubscription(req.SubId, req, s.eventSender, s.subscriptionService, loadedIds, pendingIds, spaceViewPredicate)
	if err != nil {
		return nil, fmt.Errorf("new cross space subscription: %w", err)
	}
	s.subscriptions[req.SubId] = spaceSub
	go spaceSub.run(req.InternalQueue)
	return resp, nil
}

func (s *service) Unsubscribe(subId string) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	sub, ok := s.subscriptions[subId]
	if !ok {
		return ErrSubscriptionNotFound
	}

	err := sub.close()
	if err != nil {
		return fmt.Errorf("close subscription: %w", err)
	}
	delete(s.subscriptions, subId)

	return nil
}
