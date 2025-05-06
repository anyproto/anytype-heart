package chats

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"

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
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/tests/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const techSpaceId = "techSpaceId"

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

type actionType int

const (
	actionTypeSubscribe = actionType(iota + 1)
	actionTypeUnsubscribe
)

type recordedAction struct {
	actionType actionType
	subId      string
}

type fixture struct {
	*service

	objectGetter         *mock_cache.MockObjectGetterComponent
	app                  *app.App
	crossSpaceSubService *mock_crossspacesub.MockService

	lock sync.Mutex
	// recorded actions (subscribe/unsubscribe) per chat object, in temporal order
	actions map[string][]recordedAction
}

func (fx *fixture) recordAction(chatObjectId string, a recordedAction) {
	fx.lock.Lock()
	defer fx.lock.Unlock()

	fx.actions[chatObjectId] = append(fx.actions[chatObjectId], a)
}

func (fx *fixture) waitForActions(t *testing.T, want map[string][]recordedAction) {
	timer := time.NewTimer(time.Second)
	ticker := time.NewTicker(2 * time.Millisecond)

	for {
		select {
		case <-timer.C:
			t.Fatal("wait for actions: timeout")
		case <-ticker.C:
		}

		fx.lock.Lock()
		if reflect.DeepEqual(want, fx.actions) {
			fx.lock.Unlock()
			return
		}
		fx.lock.Unlock()
	}
}

func newFixture(t *testing.T) *fixture {
	objectStore := objectstore.NewStoreFixture(t)
	objectGetter := mock_cache.NewMockObjectGetterComponent(t)
	crossSpaceSubService := mock_crossspacesub.NewMockService(t)

	fx := &fixture{
		service:              New().(*service),
		crossSpaceSubService: crossSpaceSubService,
		objectGetter:         objectGetter,
		actions:              map[string][]recordedAction{},
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

func givenLastMessages() []*chatobject.Message {
	return []*chatobject.Message{
		{
			ChatMessage: &model.ChatMessage{
				Id: "messageId1",
			},
		},
	}
}

func givenLastState() *model.ChatState {
	return &model.ChatState{
		Messages: &model.ChatStateUnreadState{
			Counter:       2,
			OldestOrderId: "abc",
		},
		Mentions: &model.ChatStateUnreadState{
			Counter:       1,
			OldestOrderId: "aaa",
		},
	}
}

func givenDependencies() map[string][]*domain.Details {
	return map[string][]*domain.Details{
		"messageId1": {
			domain.NewDetails().SetString(bundle.RelationKeyId, "depId1"),
		},
	}
}

func (fx *fixture) expectChatObject(t *testing.T, chatObjectId string) {
	fx.objectGetter.EXPECT().GetObject(mock.Anything, chatObjectId).RunAndReturn(func(ctx context.Context, id string) (smartblock.SmartBlock, error) {
		sb := mock_chatobject.NewMockStoreObject(t)

		sb.EXPECT().Lock().Return().Maybe()
		sb.EXPECT().Unlock().Return().Maybe()
		sb.EXPECT().SubscribeLastMessages(mock.Anything, mock.Anything).RunAndReturn(func(ctx context.Context, req chatobject.SubscribeLastMessagesRequest) (*chatobject.SubscribeLastMessagesResponse, error) {
			fx.recordAction(chatObjectId, recordedAction{
				actionType: actionTypeSubscribe,
				subId:      req.SubId,
			})
			return &chatobject.SubscribeLastMessagesResponse{
				Messages:     givenLastMessages(),
				ChatState:    givenLastState(),
				Dependencies: givenDependencies(),
			}, nil
		}).Maybe()

		sb.EXPECT().Unsubscribe(mock.Anything).RunAndReturn(func(subId string) error {
			fx.recordAction(chatObjectId, recordedAction{
				actionType: actionTypeUnsubscribe,
				subId:      subId,
			})
			return nil
		}).Maybe()

		return sb, nil
	})
}

func TestSubscribeToMessagePreviews(t *testing.T) {
	t.Run("subscribe to all existing chats", func(t *testing.T) {
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

		fx.expectChatObject(t, "chat1")
		fx.expectChatObject(t, "chat2")

		fx.start(t)

		resp, err := fx.SubscribeToMessagePreviews(ctx, "previewSub1")
		require.NoError(t, err)

		wantPreviews := []*ChatPreview{
			{
				SpaceId:      "space1",
				ChatObjectId: "chat1",
				State:        givenLastState(),
				Message:      givenLastMessages()[0],
				Dependencies: givenDependencies()["messageId1"],
			},
			{
				SpaceId:      "space2",
				ChatObjectId: "chat2",
				State:        givenLastState(),
				Message:      givenLastMessages()[0],
				Dependencies: givenDependencies()["messageId1"],
			},
		}
		assert.ElementsMatch(t, wantPreviews, resp.Previews)

		fx.waitForActions(t, map[string][]recordedAction{
			"chat1": {
				{
					actionType: actionTypeSubscribe,
					subId:      "previewSub1",
				},
			},
			"chat2": {
				{
					actionType: actionTypeSubscribe,
					subId:      "previewSub1",
				},
			},
		})
	})

	t.Run("chats are added via subscription", func(t *testing.T) {
		fx := newFixture(t)
		ctx := context.Background()

		fx.crossSpaceSubService.EXPECT().Subscribe(mock.Anything).Return(&subscription.SubscribeResponse{
			Records: []*domain.Details{},
		}, nil).Maybe()

		fx.expectChatObject(t, "chat1")
		fx.expectChatObject(t, "chat2")

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

		fx.waitForActions(t, map[string][]recordedAction{
			"chat1": {
				{
					actionType: actionTypeSubscribe,
					subId:      "previewSub1",
				},
			},
			"chat2": {
				{
					actionType: actionTypeSubscribe,
					subId:      "previewSub1",
				},
			},
		})
	})

	t.Run("chats removed via subscription", func(t *testing.T) {
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

		fx.expectChatObject(t, "chat1")
		fx.expectChatObject(t, "chat2")

		fx.start(t)

		fx.chatObjectsSubQueue.Add(ctx, &pb.EventMessage{
			SpaceId: "space1",
			Value: &pb.EventMessageValueOfSubscriptionRemove{
				SubscriptionRemove: &pb.EventObjectSubscriptionRemove{
					Id: "chat1",
				},
			},
		})

		resp, err := fx.SubscribeToMessagePreviews(ctx, "previewSub1")
		require.NoError(t, err)
		assert.NotNil(t, resp)

		fx.waitForActions(t, map[string][]recordedAction{
			"chat1": {
				{
					actionType: actionTypeSubscribe,
					subId:      "previewSub1",
				},
				{
					actionType: actionTypeUnsubscribe,
					subId:      "previewSub1",
				},
			},
			"chat2": {
				{
					actionType: actionTypeSubscribe,
					subId:      "previewSub1",
				},
			},
		})
	})

	t.Run("subscribe to all existing chats multiple times", func(t *testing.T) {
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

		fx.expectChatObject(t, "chat1")
		fx.expectChatObject(t, "chat2")

		fx.start(t)

		resp, err := fx.SubscribeToMessagePreviews(ctx, "previewSub1")
		require.NoError(t, err)
		assert.NotNil(t, resp)
		resp, err = fx.SubscribeToMessagePreviews(ctx, "previewSub2")
		require.NoError(t, err)
		assert.NotNil(t, resp)

		fx.waitForActions(t, map[string][]recordedAction{
			"chat1": {
				{
					actionType: actionTypeSubscribe,
					subId:      "previewSub1",
				},
				{
					actionType: actionTypeSubscribe,
					subId:      "previewSub2",
				},
			},
			"chat2": {
				{
					actionType: actionTypeSubscribe,
					subId:      "previewSub1",
				},
				{
					actionType: actionTypeSubscribe,
					subId:      "previewSub2",
				},
			},
		})
	})
}
