package chats

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

	"github.com/anyproto/anytype-heart/core/block/cache/mock_cache"
	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
	"github.com/anyproto/anytype-heart/core/block/chats/chatsubscription"
	"github.com/anyproto/anytype-heart/core/block/chats/chatsubscription/mock_chatsubscription"
	"github.com/anyproto/anytype-heart/core/block/object/idresolver/mock_idresolver"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/subscription/crossspacesub/mock_crossspacesub"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/tests/testutil"
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

	objectGetter         *mock_cache.MockObjectWaitGetterComponent
	subscriptionService  *mock_chatsubscription.MockService
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
	objectGetter := mock_cache.NewMockObjectWaitGetterComponent(t)
	crossSpaceSubService := mock_crossspacesub.NewMockService(t)
	subscriptionService := mock_chatsubscription.NewMockService(t)
	idResolver := mock_idresolver.NewMockResolver(t)
	idResolver.EXPECT().ResolveSpaceID(mock.Anything).Return("", nil).Maybe()

	fx := &fixture{
		service:              New().(*service),
		crossSpaceSubService: crossSpaceSubService,
		subscriptionService:  subscriptionService,
		objectGetter:         objectGetter,
		actions:              map[string][]recordedAction{},
	}

	ctx := context.Background()
	a := new(app.App)
	a.Register(objectStore)
	a.Register(testutil.PrepareMock(ctx, a, objectGetter))
	a.Register(testutil.PrepareMock(ctx, a, crossSpaceSubService))
	a.Register(testutil.PrepareMock(ctx, a, subscriptionService))
	a.Register(testutil.PrepareMock(ctx, a, idResolver))
	a.Register(&pushServiceDummy{})
	a.Register(&accountServiceDummy{})
	a.Register(fx)

	fx.app = a

	fx.expectSubscribe(t)
	return fx
}

func (fx *fixture) start(t *testing.T) {
	err := fx.app.Start(context.Background())
	require.NoError(t, err)
}

func givenLastMessages() []*chatmodel.Message {
	return []*chatmodel.Message{
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

func (fx *fixture) expectSubscribe(t *testing.T) {
	fx.subscriptionService.EXPECT().SubscribeLastMessages(mock.Anything, mock.Anything).RunAndReturn(func(ctx context.Context, req chatsubscription.SubscribeLastMessagesRequest) (*chatsubscription.SubscribeLastMessagesResponse, error) {
		fx.recordAction(req.ChatObjectId, recordedAction{
			actionType: actionTypeSubscribe,
			subId:      req.SubId,
		})
		return &chatsubscription.SubscribeLastMessagesResponse{
			Messages:     givenLastMessages(),
			ChatState:    givenLastState(),
			Dependencies: givenDependencies(),
		}, nil
	}).Maybe()

	fx.subscriptionService.EXPECT().Unsubscribe(mock.Anything, mock.Anything).RunAndReturn(func(chatObjectId string, subId string) error {
		fx.recordAction(chatObjectId, recordedAction{
			actionType: actionTypeUnsubscribe,
			subId:      subId,
		})
		return nil
	}).Maybe()
}

func (fx *fixture) assertSendEvents(t *testing.T, chatIds []string) {
	manager := mock_chatsubscription.NewMockManager(t)
	manager.EXPECT().Lock().Return()
	manager.EXPECT().Add(mock.Anything, mock.Anything).Return().Maybe()
	manager.EXPECT().ForceSendingChatState().Return()
	manager.EXPECT().Flush().Return()
	manager.EXPECT().Unlock().Return()

	for _, chatId := range chatIds {
		fx.subscriptionService.EXPECT().GetManager(mock.Anything, chatId).Return(manager, nil)
	}
}

func TestSubscribeToMessagePreviews(t *testing.T) {
	t.Run("subscribe to all existing chats", func(t *testing.T) {
		fx := newFixture(t)
		ctx := context.Background()

		fx.crossSpaceSubService.EXPECT().Subscribe(mock.Anything, mock.Anything).Return(&subscription.SubscribeResponse{
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

		fx.crossSpaceSubService.EXPECT().Subscribe(mock.Anything, mock.Anything).Return(&subscription.SubscribeResponse{
			Records: []*domain.Details{},
		}, nil).Maybe()

		fx.assertSendEvents(t, []string{"chat1", "chat2"})

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

		fx.crossSpaceSubService.EXPECT().Subscribe(mock.Anything, mock.Anything).Return(&subscription.SubscribeResponse{
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

		fx.crossSpaceSubService.EXPECT().Subscribe(mock.Anything, mock.Anything).Return(&subscription.SubscribeResponse{
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

func TestApplyEmojiMarks(t *testing.T) {
	for _, tc := range []struct {
		name  string
		text  string
		marks []*model.BlockContentTextMark
		want  string
	}{
		{
			name:  "empty text",
			text:  "",
			marks: []*model.BlockContentTextMark{},
			want:  "",
		},
		{
			name:  "no marks",
			text:  "hello",
			marks: []*model.BlockContentTextMark{},
			want:  "hello",
		},
		{
			name: "invalid range",
			text: "hello",
			marks: []*model.BlockContentTextMark{
				{
					Type: model.BlockContentTextMark_Emoji,
					Range: &model.Range{
						From: 100,
						To:   101,
					},
					Param: "👍",
				},
			},
			want: "hello",
		},
		{
			name: "only emoji",
			text: " ",
			marks: []*model.BlockContentTextMark{
				{
					Type: model.BlockContentTextMark_Emoji,
					Range: &model.Range{
						From: 0,
						To:   1,
					},
					Param: "👍",
				},
			},
			want: "👍",
		},
		{
			name: "multiple marks",
			text: " a b ",
			marks: []*model.BlockContentTextMark{
				{
					Type: model.BlockContentTextMark_Emoji,
					Range: &model.Range{
						From: 0,
						To:   1,
					},
					Param: "👍",
				},
				{
					Type: model.BlockContentTextMark_Emoji,
					Range: &model.Range{
						From: 2,
						To:   3,
					},
					Param: "👌",
				},
				{
					Type: model.BlockContentTextMark_Emoji,
					Range: &model.Range{
						From: 4,
						To:   5,
					},
					Param: "😀",
				},
			},
			want: "👍a👌b😀",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := applyEmojiMarks(tc.text, tc.marks)
			assert.Equal(t, tc.want, got)
		})
	}
}
