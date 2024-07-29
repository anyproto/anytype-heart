package subscription

import (
	"context"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/core/kanban"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

type InternalTestService struct {
	Service
	*objectstore.StoreFixture
}

func (s *InternalTestService) Init(a *app.App) error {
	return s.Service.Init(a)
}

func (s *InternalTestService) Run(ctx context.Context) error {
	err := s.StoreFixture.Run(ctx)
	if err != nil {
		return err
	}
	return s.Service.Run(ctx)
}

func (s *InternalTestService) Close(ctx context.Context) (err error) {
	_ = s.Service.Close(ctx)
	return s.StoreFixture.Close(ctx)
}

func NewInternalTestService(t *testing.T) *InternalTestService {
	s := New()
	ctx := context.Background()

	objectStore := objectstore.NewStoreFixture(t)

	a := &app.App{}
	a.Register(objectStore)
	a.Register(kanban.New())
	a.Register(&collectionServiceMock{MockCollectionService: NewMockCollectionService(t)})
	a.Register(testutil.PrepareMock(ctx, a, mock_event.NewMockSender(t)))
	a.Register(s)
	err := a.Start(ctx)
	require.NoError(t, err)
	return &InternalTestService{Service: s, StoreFixture: objectStore}
}

func RegisterSubscriptionService(t *testing.T, a *app.App) *InternalTestService {
	s := New()
	ctx := context.Background()
	objectStore := objectstore.NewStoreFixture(t)
	a.Register(objectStore).
		Register(kanban.New()).
		Register(&collectionServiceMock{MockCollectionService: NewMockCollectionService(t)}).
		Register(testutil.PrepareMock(ctx, a, mock_event.NewMockSender(t))).
		Register(s)
	return &InternalTestService{Service: s, StoreFixture: objectStore}
}

type collectionServiceMock struct {
	*MockCollectionService
}

func (c *collectionServiceMock) Name() string {
	return "collectionService"
}

func (c *collectionServiceMock) Init(a *app.App) error { return nil }
