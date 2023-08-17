package subscription

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/space/typeprovider/mock_typeprovider"
	"github.com/anyproto/anytype-heart/util/testMock"
	"go.uber.org/mock/gomock"
)

type collectionServiceMock struct {
	updateCh chan []string
}

func (c *collectionServiceMock) SubscribeForCollection(collectionID string, subscriptionID string) ([]string, <-chan []string, error) {
	return nil, c.updateCh, nil
}

func (c *collectionServiceMock) UnsubscribeFromCollection(collectionID string, subscriptionID string) {
}

type fixture struct {
	Service
	a      *app.App
	ctrl   *gomock.Controller
	store  *testMock.MockObjectStore
	sender *mock_event.MockSender
	events []*pb.Event
}

func newFixture(t *testing.T) *fixture {
	ctrl := gomock.NewController(t)
	a := new(app.App)
	testMock.RegisterMockObjectStore(ctrl, a)
	testMock.RegisterMockKanban(ctrl, a)
	a.Register(&collectionServiceMock{})
	sbtProvider := mock_typeprovider.NewMockSmartBlockTypeProvider(t)
	sbtProvider.EXPECT().Name().Return("smartBlockTypeProvider")
	sbtProvider.EXPECT().Init(mock.Anything).Return(nil)
	a.Register(sbtProvider)
	fx := &fixture{
		Service: New(),
		a:       a,
		ctrl:    ctrl,
		store:   a.MustComponent(objectstore.CName).(*testMock.MockObjectStore),
	}
	sender := mock_event.NewMockSender(t)
	sender.EXPECT().Init(mock.Anything).Return(nil)
	sender.EXPECT().Name().Return(event.CName)
	sender.EXPECT().Broadcast(mock.Anything).Run(func(e *pb.Event) {
		fx.events = append(fx.events, e)
	}).Maybe()
	fx.sender = sender
	a.Register(fx.Service)
	a.Register(fx.sender)

	fx.store.EXPECT().SubscribeForAll(gomock.Any())
	require.NoError(t, a.Start(context.Background()))
	return fx
}

type fixtureRealStore struct {
	Service
	a      *app.App
	ctrl   *gomock.Controller
	store  *objectstore.StoreFixture
	sender *mock_event.MockSender
	events []pb.IsEventMessageValue
}

func newFixtureWithRealObjectStore(t *testing.T) *fixtureRealStore {
	ctrl := gomock.NewController(t)
	a := new(app.App)
	store := objectstore.NewStoreFixture(t)
	a.Register(store)
	testMock.RegisterMockKanban(ctrl, a)
	a.Register(&collectionServiceMock{})
	sbtProvider := mock_typeprovider.NewMockSmartBlockTypeProvider(t)
	sbtProvider.EXPECT().Name().Return("smartBlockTypeProvider")
	sbtProvider.EXPECT().Init(mock.Anything).Return(nil)
	a.Register(sbtProvider)
	fx := &fixtureRealStore{
		Service: New(),
		a:       a,
		ctrl:    ctrl,
		store:   store,
	}
	sender := mock_event.NewMockSender(t)
	sender.EXPECT().Init(mock.Anything).Return(nil)
	sender.EXPECT().Name().Return(event.CName)
	sender.EXPECT().Broadcast(mock.Anything).Run(func(e *pb.Event) {
		for _, em := range e.Messages {
			fx.events = append(fx.events, em.Value)
		}
	}).Maybe()
	fx.sender = sender
	a.Register(fx.Service)
	a.Register(fx.sender)

	require.NoError(t, a.Start(context.Background()))
	return fx
}

func (fx *fixtureRealStore) waitEvents(t *testing.T, ev ...pb.IsEventMessageValue) {
	timeout := time.NewTimer(1 * time.Second)
	ticker := time.NewTicker(1 * time.Millisecond)
	for {
		select {
		case <-timeout.C:
			require.Equal(t, ev, fx.events)
		case <-ticker.C:
		}

		if reflect.DeepEqual(fx.events, ev) {
			fx.events = nil
			return
		}
	}
}
