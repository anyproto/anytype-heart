package chats

import (
	"context"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/anytype-heart/core/block/cache/mock_cache"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/subscription/crossspacesub/mock_crossspacesub"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const techSpaceId = "techSpaceId"

type fixture struct {
	Service

	objectStore *objectstore.StoreFixture
}

type pushServiceDummy struct {
}

func (s *pushServiceDummy) Notify(ctx context.Context, spaceId string, topic []string, payload []byte) (err error) {
	return nil
}

func (s *pushServiceDummy) Name() string { return "pushServiceDummy" }

func (s *pushServiceDummy) Init(a *app.App) error { return nil }

type accountServiceDummy struct {
}

func (s *accountServiceDummy) AccountID() string {
	return "testAccountId"
}

func (s *accountServiceDummy) Name() string {
	return "accountServiceDummy"
}

func (s *accountServiceDummy) Init(a *app.App) error {
	return nil
}

func newFixture(t *testing.T) *fixture {
	objectStore := objectstore.NewStoreFixture(t)
	objectGetter := mock_cache.NewMockObjectGetterComponent(t)
	crossSpaceSubService := mock_crossspacesub.NewMockService(t)
	crossSpaceSubService.EXPECT().Subscribe(mock.Anything).Return(&subscription.SubscribeResponse{}, nil).Maybe()

	fx := &fixture{
		Service:     New(),
		objectStore: objectStore,
	}

	ctx := context.Background()
	a := new(app.App)
	a.Register(objectStore)
	a.Register(testutil.PrepareMock(ctx, a, objectGetter))
	a.Register(testutil.PrepareMock(ctx, a, crossSpaceSubService))
	a.Register(&pushServiceDummy{})
	a.Register(&accountServiceDummy{})
	a.Register(fx)

	err := a.Start(ctx)
	require.NoError(t, err)

	return fx
}

func TestSubscribeToMessagePreviews(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()

	// TODO Add initial chats
	// TODO Add chats via subscription
	// TODO Delete chats via subscription
	// TODO Subscribe multiple times and make sure that Subscribe is called again and again
	resp, err := fx.SubscribeToMessagePreviews(ctx, "previewSub1")
	require.NoError(t, err)
	assert.NotNil(t, resp)
}
