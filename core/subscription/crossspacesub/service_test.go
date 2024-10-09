package crossspacesub

import (
	"context"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/core/kanban/mock_kanban"
	subscriptionservice "github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/space/mock_space"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

type fixture struct {
	*service

	objectStore  *objectstore.StoreFixture
	spaceService *mock_space.MockService
}

func newFixture(t *testing.T) *fixture {
	ctx := context.Background()
	a := &app.App{}

	// Deps for subscription service
	kanbanService := mock_kanban.NewMockService(t)
	eventSender := mock_event.NewMockSender(t)
	objectStore := objectstore.NewStoreFixture(t)
	collService := &dummyCollectionService{}
	// Own deps
	subscriptionService := subscriptionservice.New()
	spaceService := mock_space.NewMockService(t)

	a.Register(testutil.PrepareMock(ctx, a, kanbanService))
	a.Register(testutil.PrepareMock(ctx, a, eventSender))
	a.Register(objectStore)
	a.Register(collService)
	a.Register(subscriptionService)
	a.Register(testutil.PrepareMock(ctx, a, spaceService))

	s := New()
	a.Register(s)
	err := a.Start(ctx)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = a.Close(context.Background())
	})

	return &fixture{
		service:      s.(*service),
		objectStore:  objectStore,
		spaceService: spaceService,
	}
}

func xTestSubscribe(t *testing.T) {
	fx := newFixture(t)

	resp, err := fx.Subscribe(subscriptionservice.SubscribeRequest{})
	require.NoError(t, err)
	require.NotNil(t, resp)
}

type dummyCollectionService struct{}

func (d *dummyCollectionService) Init(a *app.App) (err error) {
	return nil
}

func (d *dummyCollectionService) Name() (name string) {
	return "dummyCollectionService"
}

func (d *dummyCollectionService) SubscribeForCollection(collectionID string, subscriptionID string) ([]string, <-chan []string, error) {
	return nil, nil, nil
}

func (d *dummyCollectionService) UnsubscribeFromCollection(collectionID string, subscriptionID string) {
}
