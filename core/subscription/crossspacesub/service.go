package crossspacesub

import (
	"context"
	"sync"

	"github.com/anyproto/any-sync/app"
	"github.com/cheggaaa/mb/v3"

	subscriptionservice "github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/space"
)

const CName = "core.subscription.crossspacesub"

type Service interface {
	app.ComponentRunnable
	Subscribe(req subscriptionservice.SubscribeRequest) (resp *subscriptionservice.SubscribeResponse, err error)
}

type service struct {
	spaceService        space.Service
	subscriptionService subscriptionservice.Service

	componentCtx       context.Context
	componentCtxCancel context.CancelFunc

	lock          sync.Mutex
	spaceViewsSub *subscription
	subscriptions map[string]*crossSpaceSubscription
}

func New() Service {
	return &service{}
}

func (s *service) Init(a *app.App) error {
	s.componentCtx, s.componentCtxCancel = context.WithCancel(context.Background())
	s.spaceService = app.MustComponent[space.Service](a)
	s.subscriptionService = app.MustComponent[subscriptionservice.Service](a)
	return nil
}

func (s *service) Name() (name string) {
	return CName
}

func (s *service) Run(ctx context.Context) error {
	return nil
}

func (s *service) monitorSpaceViews() {
	for {
		select {
		case <-s.componentCtx.Done():
		}
	}
}

func (s *service) Close(ctx context.Context) error {
	s.componentCtxCancel()
	return nil
}

func (s *service) Subscribe(req subscriptionservice.SubscribeRequest) (resp *subscriptionservice.SubscribeResponse, err error) {
	return nil, nil
}

type crossSpaceSubscription struct {
	subId string

	perSpaceSubscriptions map[string]*subscription
}

func (s *crossSpaceSubscription) addSpace(spaceId string) {

}

func (s *crossSpaceSubscription) removeSpace(spaceId string) {

}

type subscription struct {
	subId string

	events mb.MB[*pb.EventMessage]
}
