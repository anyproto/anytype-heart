package subscription

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/core/kanban"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/space/spacecore/typeprovider/mock_typeprovider"
	"github.com/anyproto/anytype-heart/util/testMock"
	"github.com/anyproto/anytype-heart/util/testMock/mockKanban"
)

type fixture struct {
	*service
	a                 *app.App
	ctrl              *gomock.Controller
	store             *objectstore.StoreFixture
	sender            *mock_event.MockSender
	events            []*pb.Event
	collectionService *collectionServiceMock
	kanban            *mockKanban.MockService
}

func newFixture(t *testing.T) *fixture {
	ctrl := gomock.NewController(t)
	a := new(app.App)
	store := objectstore.NewStoreFixture(t)
	a.Register(store)
	kanban := testMock.RegisterMockKanban(ctrl, a)
	sbtProvider := mock_typeprovider.NewMockSmartBlockTypeProvider(t)
	sbtProvider.EXPECT().Name().Return("smartBlockTypeProvider")
	sbtProvider.EXPECT().Init(mock.Anything).Return(nil)
	a.Register(sbtProvider)

	collectionService := &collectionServiceMock{MockCollectionService: NewMockCollectionService(t)}
	a.Register(collectionService)

	fx := &fixture{
		service:           New().(*service),
		a:                 a,
		ctrl:              ctrl,
		store:             store,
		collectionService: collectionService,
		kanban:            kanban,
	}
	sender := mock_event.NewMockSender(t)
	sender.EXPECT().Init(mock.Anything).Return(nil)
	sender.EXPECT().Name().Return(event.CName)
	sender.EXPECT().Broadcast(mock.Anything).Run(func(e *pb.Event) {
		fx.events = append(fx.events, e)
	}).Maybe()
	fx.sender = sender
	a.Register(fx.service)
	a.Register(fx.sender)

	require.NoError(t, a.Start(context.Background()))
	return fx
}

type fixtureRealStore struct {
	Service
	a                 *app.App
	ctrl              *gomock.Controller
	store             *objectstore.StoreFixture
	sender            *mock_event.MockSender
	eventsLock        sync.Mutex
	events            []pb.IsEventMessageValue
	collectionService *collectionServiceMock
	kanban            kanban.Service
}

func newFixtureWithRealObjectStore(t *testing.T) *fixtureRealStore {
	ctrl := gomock.NewController(t)
	a := new(app.App)
	store := objectstore.NewStoreFixture(t)
	a.Register(store)

	kanbanService := kanban.New()
	a.Register(kanbanService)

	collectionService := &collectionServiceMock{MockCollectionService: NewMockCollectionService(t)}
	a.Register(collectionService)

	sbtProvider := mock_typeprovider.NewMockSmartBlockTypeProvider(t)
	sbtProvider.EXPECT().Name().Return("smartBlockTypeProvider")
	sbtProvider.EXPECT().Init(mock.Anything).Return(nil)
	a.Register(sbtProvider)
	fx := &fixtureRealStore{
		Service:           New(),
		a:                 a,
		ctrl:              ctrl,
		store:             store,
		kanban:            kanbanService,
		collectionService: collectionService,
	}
	sender := mock_event.NewMockSender(t)
	sender.EXPECT().Init(mock.Anything).Return(nil)
	sender.EXPECT().Name().Return(event.CName)
	sender.EXPECT().Broadcast(mock.Anything).Run(func(e *pb.Event) {
		fx.eventsLock.Lock()
		defer fx.eventsLock.Unlock()
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
			fx.eventsLock.Lock()
			assert.Equal(t, ev, fx.events)
			fx.eventsLock.Unlock()
			return
		case <-ticker.C:
		}

		fx.eventsLock.Lock()
		if reflect.DeepEqual(fx.events, ev) {
			fx.events = nil
			fx.eventsLock.Unlock()
			return
		}
		fx.eventsLock.Unlock()
	}
}
