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

type collectionServiceMock struct {
	*MockCollectionService
}

func (c *collectionServiceMock) Name() string {
	return "collectionService"
}

func (c *collectionServiceMock) Init(a *app.App) error { return nil }
