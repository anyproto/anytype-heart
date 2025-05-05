package chats

import (
	"context"
	"sync"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/anytype-heart/core/block/cache/mock_cache"
	"github.com/anyproto/anytype-heart/core/block/editor/chatobject"
	"github.com/anyproto/anytype-heart/core/block/editor/chatobject/mock_chatobject"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/subscription/crossspacesub/mock_crossspacesub"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const techSpaceId = "techSpaceId"

type fixture struct {
	*service

	objectGetter         *mock_cache.MockObjectGetterComponent
	app                  *app.App
	crossSpaceSubService *mock_crossspacesub.MockService
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

	fx := &fixture{
		service:              New().(*service),
		crossSpaceSubService: crossSpaceSubService,
		objectGetter:         objectGetter,
	}

	ctx := context.Background()
	a := new(app.App)
	a.Register(objectStore)
	a.Register(testutil.PrepareMock(ctx, a, objectGetter))
	a.Register(testutil.PrepareMock(ctx, a, crossSpaceSubService))
	a.Register(&pushServiceDummy{})
	a.Register(&accountServiceDummy{})
	a.Register(fx)

	fx.app = a

	return fx
}

func (fx *fixture) start(t *testing.T) {
	err := fx.app.Start(context.Background())
	require.NoError(t, err)
}

type chatObjectWrapper struct {
	smartblock.SmartBlock
	chatobject.StoreObject
}

func (fx *fixture) expectSubscribe(t *testing.T, chatObjectId string, wg *sync.WaitGroup) {
	wg.Add(1)
	fx.objectGetter.EXPECT().GetObject(mock.Anything, chatObjectId).RunAndReturn(func(ctx context.Context, id string) (smartblock.SmartBlock, error) {
		sb := mock_chatobject.NewMockStoreObject(t)

		sb.EXPECT().Lock().Return()
		sb.EXPECT().Unlock().Return()
		sb.EXPECT().SubscribeLastMessages(mock.Anything, mock.Anything).RunAndReturn(func(context.Context, chatobject.SubscribeLastMessagesRequest) (*chatobject.SubscribeLastMessagesResponse, error) {
			defer wg.Done()
			return &chatobject.SubscribeLastMessagesResponse{}, nil
		})

		return sb, nil
	})

}

func TestSubscribeToMessagePreviews(t *testing.T) {
	// TODO Delete chats via subscription
	// TODO Subscribe multiple times and make sure that Subscribe is called again and again

	t.Run("subscribe on all existing chats", func(t *testing.T) {
		fx := newFixture(t)
		ctx := context.Background()

		fx.crossSpaceSubService.EXPECT().Subscribe(mock.Anything).Return(&subscription.SubscribeResponse{
			Records: []*domain.Details{
				domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
					bundle.RelationKeyId:      domain.String("chat1"),
					bundle.RelationKeySpaceId: domain.String("space1"),
				}),
				domain.NewDetailsFromMap(map[domain.RelationKey]domain.Value{
					bundle.RelationKeyId:      domain.String("chat2"),
					bundle.RelationKeySpaceId: domain.String("space2"),
				}),
			},
		}, nil).Maybe()

		wg := &sync.WaitGroup{}
		fx.expectSubscribe(t, "chat1", wg)
		fx.expectSubscribe(t, "chat2", wg)

		fx.start(t)

		resp, err := fx.SubscribeToMessagePreviews(ctx, "previewSub1")
		require.NoError(t, err)
		assert.NotNil(t, resp)

		wg.Wait()
	})

	t.Run("chats are added via subscription", func(t *testing.T) {
		fx := newFixture(t)
		ctx := context.Background()

		fx.crossSpaceSubService.EXPECT().Subscribe(mock.Anything).Return(&subscription.SubscribeResponse{
			Records: []*domain.Details{},
		}, nil).Maybe()

		wg := &sync.WaitGroup{}
		fx.expectSubscribe(t, "chat1", wg)
		fx.expectSubscribe(t, "chat2", wg)

		fx.start(t)

		fx.chatObjectsSubQueue.Add(ctx, &pb.EventMessage{
			SpaceId: "space1",
			Value: &pb.EventMessageValueOfSubscriptionAdd{
				SubscriptionAdd: &pb.EventObjectSubscriptionAdd{
					Id: "chat1",
				},
			},
		})
		fx.chatObjectsSubQueue.Add(ctx, &pb.EventMessage{
			SpaceId: "space2",
			Value: &pb.EventMessageValueOfSubscriptionAdd{
				SubscriptionAdd: &pb.EventObjectSubscriptionAdd{
					Id: "chat2",
				},
			},
		})

		resp, err := fx.SubscribeToMessagePreviews(ctx, "previewSub1")
		require.NoError(t, err)
		assert.NotNil(t, resp)

		wg.Wait()
	})

}
