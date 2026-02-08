package chats

import (
	"context"
	"fmt"
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
	"github.com/anyproto/anytype-heart/core/block/chats/chatpush"
	"github.com/anyproto/anytype-heart/core/block/chats/chatsubscription"
	"github.com/anyproto/anytype-heart/core/block/chats/chatsubscription/mock_chatsubscription"
	"github.com/anyproto/anytype-heart/core/block/detailservice/mock_detailservice"
	"github.com/anyproto/anytype-heart/core/block/editor/chatobject/mock_chatobject"
	"github.com/anyproto/anytype-heart/core/block/object/idresolver/mock_idresolver"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/subscription/crossspacesub/mock_crossspacesub"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/ftsearch/mock_ftsearch"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

const techSpaceId = "techSpaceId"

type pushServiceDummy struct {
}

func (s *pushServiceDummy) Notify(ctx context.Context, spaceId, groupId string, topic []string, payload []byte) (err error) {
	return nil
}

func (s *pushServiceDummy) NotifyRead(ctx context.Context, spaceId, groupId string) (err error) {
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

type fileGCDummy struct{}

func (s *fileGCDummy) Name() string { return "fileGCDummy" }
func (s *fileGCDummy) Init(a *app.App) error { return nil }
func (s *fileGCDummy) Run(ctx context.Context) error { return nil }
func (s *fileGCDummy) Close(ctx context.Context) error { return nil }
func (s *fileGCDummy) CheckFilesOnLinksRemoval(spaceId, contextId string, removedLinks []string, skipBin bool, onlyBlockIds []string) error {
	return nil
}
func (s *fileGCDummy) CheckFilesOnContextArchived(spaceId, contextId string, isArchived bool) error {
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
	ftSearch             *mock_ftsearch.MockFTSearch
	objectStore          *objectstore.StoreFixture

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
	eventSender := mock_event.NewMockSender(t)
	eventSender.EXPECT().Broadcast(mock.Anything).Maybe()
	detailService := mock_detailservice.NewMockService(t)
	ftSearch := mock_ftsearch.NewMockFTSearch(t)

	fx := &fixture{
		service:              New().(*service),
		crossSpaceSubService: crossSpaceSubService,
		subscriptionService:  subscriptionService,
		objectGetter:         objectGetter,
		ftSearch:             ftSearch,
		objectStore:          objectStore,
		actions:              map[string][]recordedAction{},
	}

	ctx := context.Background()
	a := new(app.App)
	a.Register(objectStore)
	a.Register(testutil.PrepareMock(ctx, a, objectGetter))
	a.Register(testutil.PrepareMock(ctx, a, crossSpaceSubService))
	a.Register(testutil.PrepareMock(ctx, a, subscriptionService))
	a.Register(testutil.PrepareMock(ctx, a, idResolver))
	a.Register(testutil.PrepareMock(ctx, a, eventSender))
	a.Register(testutil.PrepareMock(ctx, a, ftSearch))
	a.Register(&pushServiceDummy{})
	a.Register(&accountServiceDummy{})
	a.Register(testutil.PrepareMock(ctx, a, detailService))
	a.Register(&fileGCDummy{})
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
	manager.EXPECT().GetChatState().Return(&model.ChatState{}).Maybe()
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
			name: "with cyrillic symbol",
			text: "ц ",
			marks: []*model.BlockContentTextMark{
				{
					Type: model.BlockContentTextMark_Emoji,
					Range: &model.Range{
						From: 1,
						To:   2,
					},
					Param: "👍",
				},
			},
			want: "ц👍",
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

func TestBuildPushPayload(t *testing.T) {
	t.Run("basic message with space and sender names", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.crossSpaceSubService.EXPECT().Subscribe(mock.Anything, mock.Anything).Return(&subscription.SubscribeResponse{
			Records: []*domain.Details{},
		}, nil).Maybe()

		spaceId := "space1"
		spaceName := "My Workspace"
		senderName := "John Doe"
		participantId := domain.NewParticipantId(spaceId, "testAccountId")

		// Set up space name
		fx.objectStore.AddObjects(t, objectstore.TestTechSpaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:             domain.String("spaceView1"),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_spaceView)),
				bundle.RelationKeyTargetSpaceId:  domain.String(spaceId),
				bundle.RelationKeyName:           domain.String(spaceName),
			},
		})

		// Set up sender name (participant)
		fx.objectStore.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:   domain.String(participantId),
				bundle.RelationKeyName: domain.String(senderName),
			},
		})

		fx.start(t)

		req := pushNotificationRequest{
			spaceId:      spaceId,
			chatObjectId: "chat1",
			chatName:     "General",
			messageId:    "msg1",
			message: &chatmodel.Message{
				ChatMessage: &model.ChatMessage{
					Message: &model.ChatMessageMessageContent{
						Text: "Hello world",
					},
				},
			},
		}
		want := &chatpush.Payload{
			SpaceId:  spaceId,
			SenderId: "testAccountId",
			Type:     chatpush.ChatMessage,
			NewMessagePayload: &chatpush.NewMessagePayload{
				ChatId:         "chat1",
				MsgId:          "msg1",
				SpaceName:      spaceName,
				ChatName:       "General",
				SenderName:     senderName,
				Text:           "Hello world",
				HasAttachments: false,
				Attachments:    nil,
			},
		}

		// when
		got, err := fx.buildPushPayload(req)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("message with attachments", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.crossSpaceSubService.EXPECT().Subscribe(mock.Anything, mock.Anything).Return(&subscription.SubscribeResponse{
			Records: []*domain.Details{},
		}, nil).Maybe()

		spaceId := "space1"

		// Set up attachment details
		fx.objectStore.AddObjects(t, spaceId, []objectstore.TestObject{
			{
				bundle.RelationKeyId:             domain.String("file1"),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_file)),
			},
		})

		fx.start(t)

		req := pushNotificationRequest{
			spaceId:      spaceId,
			chatObjectId: "chat1",
			chatName:     "Team Chat",
			messageId:    "msg1",
			message: &chatmodel.Message{
				ChatMessage: &model.ChatMessage{
					Message: &model.ChatMessageMessageContent{
						Text: "Check this file",
					},
					Attachments: []*model.ChatMessageAttachment{
						{Target: "file1"},
					},
				},
			},
		}
		want := &chatpush.Payload{
			SpaceId:  spaceId,
			SenderId: "testAccountId",
			Type:     chatpush.ChatMessage,
			NewMessagePayload: &chatpush.NewMessagePayload{
				ChatId:         "chat1",
				MsgId:          "msg1",
				SpaceName:      "",
				ChatName:       "Team Chat",
				SenderName:     "",
				Text:           "Check this file",
				HasAttachments: true,
				Attachments: []*chatpush.Attachment{
					{Layout: int(model.ObjectType_file)},
				},
			},
		}

		// when
		got, err := fx.buildPushPayload(req)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("message with emoji marks", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.crossSpaceSubService.EXPECT().Subscribe(mock.Anything, mock.Anything).Return(&subscription.SubscribeResponse{
			Records: []*domain.Details{},
		}, nil).Maybe()
		fx.start(t)

		req := pushNotificationRequest{
			spaceId:      "space1",
			chatObjectId: "chat1",
			chatName:     "Fun Chat",
			messageId:    "msg1",
			message: &chatmodel.Message{
				ChatMessage: &model.ChatMessage{
					Message: &model.ChatMessageMessageContent{
						Text: "Great  ",
						Marks: []*model.BlockContentTextMark{
							{
								Type:  model.BlockContentTextMark_Emoji,
								Range: &model.Range{From: 6, To: 7},
								Param: "👍",
							},
						},
					},
				},
			},
		}
		want := &chatpush.Payload{
			SpaceId:  "space1",
			SenderId: "testAccountId",
			Type:     chatpush.ChatMessage,
			NewMessagePayload: &chatpush.NewMessagePayload{
				ChatId:         "chat1",
				MsgId:          "msg1",
				SpaceName:      "",
				ChatName:       "Fun Chat",
				SenderName:     "",
				Text:           "Great 👍",
				HasAttachments: false,
				Attachments:    nil,
			},
		}

		// when
		got, err := fx.buildPushPayload(req)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("empty chat name", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.crossSpaceSubService.EXPECT().Subscribe(mock.Anything, mock.Anything).Return(&subscription.SubscribeResponse{
			Records: []*domain.Details{},
		}, nil).Maybe()
		fx.start(t)

		req := pushNotificationRequest{
			spaceId:      "space1",
			chatObjectId: "chat1",
			chatName:     "",
			messageId:    "msg1",
			message: &chatmodel.Message{
				ChatMessage: &model.ChatMessage{
					Message: &model.ChatMessageMessageContent{
						Text: "Hello",
					},
				},
			},
		}
		want := &chatpush.Payload{
			SpaceId:  "space1",
			SenderId: "testAccountId",
			Type:     chatpush.ChatMessage,
			NewMessagePayload: &chatpush.NewMessagePayload{
				ChatId:         "chat1",
				MsgId:          "msg1",
				SpaceName:      "",
				ChatName:       "",
				SenderName:     "",
				Text:           "Hello",
				HasAttachments: false,
				Attachments:    nil,
			},
		}

		// when
		got, err := fx.buildPushPayload(req)

		// then
		require.NoError(t, err)
		assert.Equal(t, want, got)
	})
}

func TestService_Search(t *testing.T) {
	chatId := "chat1"
	spaceId := "space1"
	t.Run("success", func(t *testing.T) {
		// given
		fx := newFixture(t)
		ctx := context.Background()

		fx.crossSpaceSubService.EXPECT().Subscribe(mock.Anything, mock.Anything).Return(&subscription.SubscribeResponse{
			Records: []*domain.Details{},
		}, nil).Maybe()

		fx.ftSearch.EXPECT().Search(spaceId, "file").Return([]*ftsearch.DocumentMatch{
			{
				Score: 11.11,
				ID:    domain.NewObjectPathWithMessage(chatId, "msg1").String(),
				Fragments: map[string]*ftsearch.Highlight{
					"Text": {
						Ranges: [][]int{{5, 9}, {33, 37}},
						Text:   "This file is called profile.yaml",
					},
				},
			},
			{
				Score: 3.14,
				ID:    domain.NewObjectPathWithMessage(chatId, "msg2").String(),
				Fragments: map[string]*ftsearch.Highlight{
					"Text": {
						Ranges: [][]int{{3, 7}},
						Text:   "QA filed the issue",
					},
				},
			},
		}, nil)

		mockChatObj := mock_chatobject.NewMockStoreObject(t)
		mockChatObj.EXPECT().Lock().Return()
		mockChatObj.EXPECT().Unlock().Return()
		mockChatObj.EXPECT().GetMessagesByIds(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, ids []string) ([]*chatmodel.Message, error) {
			assert.Len(t, ids, 2)
			assert.Contains(t, ids, "msg1")
			assert.Contains(t, ids, "msg2")
			return []*chatmodel.Message{
				{&model.ChatMessage{Id: "msg1"}},
				{&model.ChatMessage{Id: "msg2"}},
			}, nil
		})
		fx.objectGetter.EXPECT().WaitAndGetObject(mock.Anything, chatId).Return(mockChatObj, nil)

		fx.start(t)

		// when
		results, err := fx.Search(ctx, &pb.RpcChatSearchRequest{
			SpaceId:  spaceId,
			ChatId:   chatId,
			FullText: "file",
			Limit:    10,
			Offset:   0,
		})

		// then
		assert.NoError(t, err)
		assert.Len(t, results, 2)
	})

	t.Run("with limit - returns only limited results", func(t *testing.T) {
		// given
		fx := newFixture(t)
		ctx := context.Background()

		fx.crossSpaceSubService.EXPECT().Subscribe(mock.Anything, mock.Anything).Return(&subscription.SubscribeResponse{
			Records: []*domain.Details{},
		}, nil).Maybe()

		fx.ftSearch.EXPECT().Search(spaceId, "test").Return([]*ftsearch.DocumentMatch{
			{
				Score: 10.0,
				ID:    domain.NewObjectPathWithMessage(chatId, "msg1").String(),
				Fragments: map[string]*ftsearch.Highlight{
					"Text": {
						Ranges: [][]int{{0, 4}},
						Text:   "test message 1",
					},
				},
			},
			{
				Score: 9.0,
				ID:    domain.NewObjectPathWithMessage(chatId, "msg2").String(),
				Fragments: map[string]*ftsearch.Highlight{
					"Text": {
						Ranges: [][]int{{0, 4}},
						Text:   "test message 2",
					},
				},
			},
			{
				Score: 8.0,
				ID:    domain.NewObjectPathWithMessage(chatId, "msg3").String(),
				Fragments: map[string]*ftsearch.Highlight{
					"Text": {
						Ranges: [][]int{{0, 4}},
						Text:   "test message 3",
					},
				},
			},
		}, nil)

		mockChatObj := mock_chatobject.NewMockStoreObject(t)
		mockChatObj.EXPECT().Lock().Return()
		mockChatObj.EXPECT().Unlock().Return()
		mockChatObj.EXPECT().GetMessagesByIds(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, ids []string) ([]*chatmodel.Message, error) {
			messages := make([]*chatmodel.Message, len(ids))
			for i, id := range ids {
				messages[i] = &chatmodel.Message{ChatMessage: &model.ChatMessage{Id: id}}
			}
			return messages, nil
		})
		fx.objectGetter.EXPECT().WaitAndGetObject(mock.Anything, chatId).Return(mockChatObj, nil)

		fx.start(t)

		// when
		results, err := fx.Search(ctx, &pb.RpcChatSearchRequest{
			SpaceId:  spaceId,
			ChatId:   chatId,
			FullText: "test",
			Limit:    2, // limit to 2 results
			Offset:   0,
		})

		// then
		assert.NoError(t, err)
		assert.Len(t, results, 2)
	})

	t.Run("with offset - skips first results", func(t *testing.T) {
		// given
		fx := newFixture(t)
		ctx := context.Background()

		fx.crossSpaceSubService.EXPECT().Subscribe(mock.Anything, mock.Anything).Return(&subscription.SubscribeResponse{
			Records: []*domain.Details{},
		}, nil).Maybe()

		fx.ftSearch.EXPECT().Search(spaceId, "query").Return([]*ftsearch.DocumentMatch{
			{
				Score: 10.0,
				ID:    domain.NewObjectPathWithMessage(chatId, "msg1").String(),
				Fragments: map[string]*ftsearch.Highlight{
					"Text": {
						Ranges: [][]int{{0, 5}},
						Text:   "query result 1",
					},
				},
			},
			{
				Score: 9.0,
				ID:    domain.NewObjectPathWithMessage(chatId, "msg2").String(),
				Fragments: map[string]*ftsearch.Highlight{
					"Text": {
						Ranges: [][]int{{0, 5}},
						Text:   "query result 2",
					},
				},
			},
			{
				Score: 8.0,
				ID:    domain.NewObjectPathWithMessage(chatId, "msg3").String(),
				Fragments: map[string]*ftsearch.Highlight{
					"Text": {
						Ranges: [][]int{{0, 5}},
						Text:   "query result 3",
					},
				},
			},
		}, nil)

		mockChatObj := mock_chatobject.NewMockStoreObject(t)
		mockChatObj.EXPECT().Lock().Return()
		mockChatObj.EXPECT().Unlock().Return()
		mockChatObj.EXPECT().GetMessagesByIds(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, ids []string) ([]*chatmodel.Message, error) {
			messages := make([]*chatmodel.Message, len(ids))
			for i, id := range ids {
				messages[i] = &chatmodel.Message{&model.ChatMessage{Id: id}}
			}
			return messages, nil
		})
		fx.objectGetter.EXPECT().WaitAndGetObject(mock.Anything, chatId).Return(mockChatObj, nil)

		fx.start(t)

		// when
		results, err := fx.Search(ctx, &pb.RpcChatSearchRequest{
			SpaceId:  spaceId,
			ChatId:   chatId,
			FullText: "query",
			Limit:    10,
			Offset:   1, // skip first result
		})

		// then
		assert.NoError(t, err)
		assert.Len(t, results, 2) // should return 2 results (msg2 and msg3)
	})

	t.Run("with limit and offset - pagination", func(t *testing.T) {
		// given
		fx := newFixture(t)
		ctx := context.Background()

		fx.crossSpaceSubService.EXPECT().Subscribe(mock.Anything, mock.Anything).Return(&subscription.SubscribeResponse{
			Records: []*domain.Details{},
		}, nil).Maybe()

		ftResults := make([]*ftsearch.DocumentMatch, 0)
		for i := 1; i <= 10; i++ {
			ftResults = append(ftResults, &ftsearch.DocumentMatch{
				Score: float64(11 - i), // descending scores
				ID:    domain.NewObjectPathWithMessage(chatId, fmt.Sprintf("msg%d", i)).String(),
				Fragments: map[string]*ftsearch.Highlight{
					"Text": {
						Ranges: [][]int{{0, 4}},
						Text:   fmt.Sprintf("text message %d", i),
					},
				},
			})
		}
		fx.ftSearch.EXPECT().Search(spaceId, "text").Return(ftResults, nil)

		mockChatObj := mock_chatobject.NewMockStoreObject(t)
		mockChatObj.EXPECT().Lock().Return()
		mockChatObj.EXPECT().Unlock().Return()
		mockChatObj.EXPECT().GetMessagesByIds(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, ids []string) ([]*chatmodel.Message, error) {
			messages := make([]*chatmodel.Message, len(ids))
			for i, id := range ids {
				messages[i] = &chatmodel.Message{&model.ChatMessage{Id: id}}
			}
			return messages, nil
		})
		fx.objectGetter.EXPECT().WaitAndGetObject(mock.Anything, chatId).Return(mockChatObj, nil)

		fx.start(t)

		// when - get second page with 3 items per page
		results, err := fx.Search(ctx, &pb.RpcChatSearchRequest{
			SpaceId:  spaceId,
			ChatId:   chatId,
			FullText: "text",
			Limit:    3,
			Offset:   3, // skip first 3 results
		})

		// then
		assert.NoError(t, err)
		assert.Len(t, results, 3)
		assert.Equal(t, "msg4", results[0].Message.Id)
		assert.Equal(t, "msg5", results[1].Message.Id)
		assert.Equal(t, "msg6", results[2].Message.Id)
	})

	t.Run("with sort by score desc", func(t *testing.T) {
		// given
		fx := newFixture(t)
		ctx := context.Background()

		fx.crossSpaceSubService.EXPECT().Subscribe(mock.Anything, mock.Anything).Return(&subscription.SubscribeResponse{
			Records: []*domain.Details{},
		}, nil).Maybe()

		fx.ftSearch.EXPECT().Search(spaceId, "search").Return([]*ftsearch.DocumentMatch{
			{
				Score: 5.0,
				ID:    domain.NewObjectPathWithMessage(chatId, "msg1").String(),
				Fragments: map[string]*ftsearch.Highlight{
					"Text": {
						Ranges: [][]int{{0, 6}},
						Text:   "search result low score",
					},
				},
			},
			{
				Score: 10.0,
				ID:    domain.NewObjectPathWithMessage(chatId, "msg2").String(),
				Fragments: map[string]*ftsearch.Highlight{
					"Text": {
						Ranges: [][]int{{0, 6}},
						Text:   "search result high score",
					},
				},
			},
			{
				Score: 7.5,
				ID:    domain.NewObjectPathWithMessage(chatId, "msg3").String(),
				Fragments: map[string]*ftsearch.Highlight{
					"Text": {
						Ranges: [][]int{{0, 6}},
						Text:   "search result medium score",
					},
				},
			},
		}, nil)

		mockChatObj := mock_chatobject.NewMockStoreObject(t)
		mockChatObj.EXPECT().Lock().Return()
		mockChatObj.EXPECT().Unlock().Return()
		mockChatObj.EXPECT().GetMessagesByIds(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, ids []string) ([]*chatmodel.Message, error) {
			messages := make([]*chatmodel.Message, len(ids))
			for i, id := range ids {
				messages[i] = &chatmodel.Message{&model.ChatMessage{Id: id}}
			}
			return messages, nil
		})
		fx.objectGetter.EXPECT().WaitAndGetObject(mock.Anything, chatId).Return(mockChatObj, nil)

		fx.start(t)

		// when - sort by score descending
		results, err := fx.Search(ctx, &pb.RpcChatSearchRequest{
			SpaceId:  spaceId,
			ChatId:   chatId,
			FullText: "search",
			Sorts: []*model.SearchMessageSort{
				{
					Key:  model.SearchMessageSort_SCORE,
					Type: model.SearchMessageSort_Desc,
				},
			},
		})

		// then
		assert.NoError(t, err)
		assert.Len(t, results, 3)
		// verify descending score order
		assert.Equal(t, "msg2", results[0].Message.Id) // score 10.0
		assert.Equal(t, "msg3", results[1].Message.Id) // score 7.5
		assert.Equal(t, "msg1", results[2].Message.Id) // score 5.0
	})

	t.Run("with sort by order_id asc", func(t *testing.T) {
		// given
		fx := newFixture(t)
		ctx := context.Background()

		fx.crossSpaceSubService.EXPECT().Subscribe(mock.Anything, mock.Anything).Return(&subscription.SubscribeResponse{
			Records: []*domain.Details{},
		}, nil).Maybe()

		fx.ftSearch.EXPECT().Search(spaceId, "msg").Return([]*ftsearch.DocumentMatch{
			{
				Score: 5.0,
				ID:    domain.NewObjectPathWithMessage(chatId, "msg1").String(),
				Fragments: map[string]*ftsearch.Highlight{
					"Text": {Ranges: [][]int{{0, 3}}, Text: "msg content 1"},
				},
			},
			{
				Score: 6.0,
				ID:    domain.NewObjectPathWithMessage(chatId, "msg2").String(),
				Fragments: map[string]*ftsearch.Highlight{
					"Text": {Ranges: [][]int{{0, 3}}, Text: "msg content 2"},
				},
			},
			{
				Score: 7.0,
				ID:    domain.NewObjectPathWithMessage(chatId, "msg3").String(),
				Fragments: map[string]*ftsearch.Highlight{
					"Text": {Ranges: [][]int{{0, 3}}, Text: "msg content 3"},
				},
			},
		}, nil)

		mockChatObj := mock_chatobject.NewMockStoreObject(t)
		mockChatObj.EXPECT().Lock().Return()
		mockChatObj.EXPECT().Unlock().Return()
		mockChatObj.EXPECT().GetMessagesByIds(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, ids []string) ([]*chatmodel.Message, error) {
			messages := make([]*chatmodel.Message, len(ids))
			for i, id := range ids {
				messages[i] = &chatmodel.Message{&model.ChatMessage{
					Id:      id,
					OrderId: fmt.Sprintf("order_%s", id), // order_msg1, order_msg2, order_msg3
				}}
			}
			return messages, nil
		})
		fx.objectGetter.EXPECT().WaitAndGetObject(mock.Anything, chatId).Return(mockChatObj, nil)

		fx.start(t)

		// when - sort by orderId ascending
		results, err := fx.Search(ctx, &pb.RpcChatSearchRequest{
			SpaceId:  spaceId,
			ChatId:   chatId,
			FullText: "msg",
			Sorts: []*model.SearchMessageSort{
				{
					Key:  model.SearchMessageSort_ORDER_ID,
					Type: model.SearchMessageSort_Asc,
				},
			},
		})

		// then
		assert.NoError(t, err)
		assert.Len(t, results, 3)
		// verify ascending orderId order
		assert.Equal(t, "msg1", results[0].Message.Id)
		assert.Equal(t, "msg2", results[1].Message.Id)
		assert.Equal(t, "msg3", results[2].Message.Id)
	})

	t.Run("with sort by created_at desc", func(t *testing.T) {
		// given
		fx := newFixture(t)
		ctx := context.Background()

		fx.crossSpaceSubService.EXPECT().Subscribe(mock.Anything, mock.Anything).Return(&subscription.SubscribeResponse{
			Records: []*domain.Details{},
		}, nil).Maybe()

		fx.ftSearch.EXPECT().Search(spaceId, "text").Return([]*ftsearch.DocumentMatch{
			{
				Score: 5.0,
				ID:    domain.NewObjectPathWithMessage(chatId, "msg1").String(),
				Fragments: map[string]*ftsearch.Highlight{
					"Text": {Ranges: [][]int{{0, 4}}, Text: "text old"},
				},
			},
			{
				Score: 6.0,
				ID:    domain.NewObjectPathWithMessage(chatId, "msg2").String(),
				Fragments: map[string]*ftsearch.Highlight{
					"Text": {Ranges: [][]int{{0, 4}}, Text: "text new"},
				},
			},
			{
				Score: 7.0,
				ID:    domain.NewObjectPathWithMessage(chatId, "msg3").String(),
				Fragments: map[string]*ftsearch.Highlight{
					"Text": {Ranges: [][]int{{0, 4}}, Text: "text middle"},
				},
			},
		}, nil)

		mockChatObj := mock_chatobject.NewMockStoreObject(t)
		mockChatObj.EXPECT().Lock().Return()
		mockChatObj.EXPECT().Unlock().Return()
		mockChatObj.EXPECT().GetMessagesByIds(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, ids []string) ([]*chatmodel.Message, error) {
			messages := make([]*chatmodel.Message, len(ids))
			for i, id := range ids {
				timestamp := int64(1000 + i) // msg1=1000, msg2=1001, msg3=1002
				messages[i] = &chatmodel.Message{&model.ChatMessage{
					Id:        id,
					CreatedAt: timestamp,
				}}
			}
			return messages, nil
		})
		fx.objectGetter.EXPECT().WaitAndGetObject(mock.Anything, chatId).Return(mockChatObj, nil)

		fx.start(t)

		// when - sort by createdAt descending (newest first)
		results, err := fx.Search(ctx, &pb.RpcChatSearchRequest{
			SpaceId:  spaceId,
			ChatId:   chatId,
			FullText: "text",
			Sorts: []*model.SearchMessageSort{
				{
					Key:  model.SearchMessageSort_CREATED_AT,
					Type: model.SearchMessageSort_Desc,
				},
			},
		})

		// then
		assert.NoError(t, err)
		assert.Len(t, results, 3)
		// verify descending createdAt order (newest first)
		assert.Equal(t, "msg3", results[0].Message.Id) // timestamp 1002
		assert.Equal(t, "msg2", results[1].Message.Id) // timestamp 1001
		assert.Equal(t, "msg1", results[2].Message.Id) // timestamp 1000
	})

	t.Run("with multiple sorts - primary and secondary", func(t *testing.T) {
		// given
		fx := newFixture(t)
		ctx := context.Background()

		fx.crossSpaceSubService.EXPECT().Subscribe(mock.Anything, mock.Anything).Return(&subscription.SubscribeResponse{
			Records: []*domain.Details{},
		}, nil).Maybe()

		fx.ftSearch.EXPECT().Search(spaceId, "test").Return([]*ftsearch.DocumentMatch{
			{
				Score: 5.0,
				ID:    domain.NewObjectPathWithMessage(chatId, "msg1").String(),
				Fragments: map[string]*ftsearch.Highlight{
					"Text": {Ranges: [][]int{{0, 4}}, Text: "test a"},
				},
			},
			{
				Score: 5.0, // same score as msg1
				ID:    domain.NewObjectPathWithMessage(chatId, "msg2").String(),
				Fragments: map[string]*ftsearch.Highlight{
					"Text": {Ranges: [][]int{{0, 4}}, Text: "test b"},
				},
			},
			{
				Score: 7.0,
				ID:    domain.NewObjectPathWithMessage(chatId, "msg3").String(),
				Fragments: map[string]*ftsearch.Highlight{
					"Text": {Ranges: [][]int{{0, 4}}, Text: "test c"},
				},
			},
		}, nil)

		mockChatObj := mock_chatobject.NewMockStoreObject(t)
		mockChatObj.EXPECT().Lock().Return()
		mockChatObj.EXPECT().Unlock().Return()
		mockChatObj.EXPECT().GetMessagesByIds(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, ids []string) ([]*chatmodel.Message, error) {
			orderMap := map[string]string{
				"msg1": "order_002",
				"msg2": "order_001",
				"msg3": "order_003",
			}
			messages := make([]*chatmodel.Message, len(ids))
			for i, id := range ids {
				messages[i] = &chatmodel.Message{&model.ChatMessage{
					Id:      id,
					OrderId: orderMap[id],
				}}
			}
			return messages, nil
		})
		fx.objectGetter.EXPECT().WaitAndGetObject(mock.Anything, chatId).Return(mockChatObj, nil)

		fx.start(t)

		// when - sort by score desc, then by orderId asc
		results, err := fx.Search(ctx, &pb.RpcChatSearchRequest{
			SpaceId:  spaceId,
			ChatId:   chatId,
			FullText: "test",
			Sorts: []*model.SearchMessageSort{
				{
					Key:  model.SearchMessageSort_SCORE,
					Type: model.SearchMessageSort_Desc,
				},
				{
					Key:  model.SearchMessageSort_ORDER_ID,
					Type: model.SearchMessageSort_Asc,
				},
			},
		})

		// then
		assert.NoError(t, err)
		assert.Len(t, results, 3)
		// verify: first by score desc, then by orderId asc
		assert.Equal(t, "msg3", results[0].Message.Id) // score 7.0
		assert.Equal(t, "msg2", results[1].Message.Id) // score 5.0, orderId order_001
		assert.Equal(t, "msg1", results[2].Message.Id) // score 5.0, orderId order_002
	})

	t.Run("filters by chat id - excludes other chats", func(t *testing.T) {
		// given
		fx := newFixture(t)
		ctx := context.Background()

		fx.crossSpaceSubService.EXPECT().Subscribe(mock.Anything, mock.Anything).Return(&subscription.SubscribeResponse{
			Records: []*domain.Details{},
		}, nil).Maybe()

		// FTSearch returns results from multiple chats
		fx.ftSearch.EXPECT().Search(spaceId, "query").Return([]*ftsearch.DocumentMatch{
			{
				Score: 10.0,
				ID:    domain.NewObjectPathWithMessage(chatId, "msg1").String(),
				Fragments: map[string]*ftsearch.Highlight{
					"Text": {Ranges: [][]int{{0, 5}}, Text: "query in chat1"},
				},
			},
			{
				Score: 9.0,
				ID:    domain.NewObjectPathWithMessage("chat2", "msg2").String(), // different chat
				Fragments: map[string]*ftsearch.Highlight{
					"Text": {Ranges: [][]int{{0, 5}}, Text: "query in chat2"},
				},
			},
			{
				Score: 8.0,
				ID:    domain.NewObjectPathWithMessage(chatId, "msg3").String(),
				Fragments: map[string]*ftsearch.Highlight{
					"Text": {Ranges: [][]int{{0, 5}}, Text: "query in chat1 again"},
				},
			},
		}, nil)

		mockChatObj := mock_chatobject.NewMockStoreObject(t)
		mockChatObj.EXPECT().Lock().Return()
		mockChatObj.EXPECT().Unlock().Return()
		mockChatObj.EXPECT().GetMessagesByIds(mock.Anything, mock.Anything).RunAndReturn(func(_ context.Context, ids []string) ([]*chatmodel.Message, error) {
			// Should only receive msg1 and msg3, not msg2 (different chat)
			assert.Len(t, ids, 2)
			assert.Contains(t, ids, "msg1")
			assert.Contains(t, ids, "msg3")
			assert.NotContains(t, ids, "msg2")

			messages := make([]*chatmodel.Message, len(ids))
			for i, id := range ids {
				messages[i] = &chatmodel.Message{&model.ChatMessage{Id: id}}
			}
			return messages, nil
		})
		fx.objectGetter.EXPECT().WaitAndGetObject(mock.Anything, chatId).Return(mockChatObj, nil)

		fx.start(t)

		// when
		results, err := fx.Search(ctx, &pb.RpcChatSearchRequest{
			SpaceId:  spaceId,
			ChatId:   chatId,
			FullText: "query",
		})

		// then
		assert.NoError(t, err)
		assert.Len(t, results, 2) // only messages from chat1
	})

	t.Run("returns error when ft search fails", func(t *testing.T) {
		// given
		fx := newFixture(t)
		ctx := context.Background()

		fx.crossSpaceSubService.EXPECT().Subscribe(mock.Anything, mock.Anything).Return(&subscription.SubscribeResponse{
			Records: []*domain.Details{},
		}, nil).Maybe()

		fx.ftSearch.EXPECT().Search(spaceId, "error").Return(nil, fmt.Errorf("search index error"))

		fx.start(t)

		// when
		results, err := fx.Search(ctx, &pb.RpcChatSearchRequest{
			SpaceId:  spaceId,
			ChatId:   chatId,
			FullText: "error",
		})

		// then
		assert.Error(t, err)
		assert.Nil(t, results)
		assert.Contains(t, err.Error(), "search ft")
	})

	t.Run("empty query returns empty results", func(t *testing.T) {
		// given
		fx := newFixture(t)
		ctx := context.Background()

		fx.crossSpaceSubService.EXPECT().Subscribe(mock.Anything, mock.Anything).Return(&subscription.SubscribeResponse{
			Records: []*domain.Details{},
		}, nil).Maybe()

		fx.ftSearch.EXPECT().Search(spaceId, "").Return([]*ftsearch.DocumentMatch{}, nil)

		mockChatObj := mock_chatobject.NewMockStoreObject(t)
		mockChatObj.EXPECT().Lock().Return()
		mockChatObj.EXPECT().Unlock().Return()
		mockChatObj.EXPECT().GetMessagesByIds(mock.Anything, []string{}).Return([]*chatmodel.Message{}, nil)
		fx.objectGetter.EXPECT().WaitAndGetObject(mock.Anything, chatId).Return(mockChatObj, nil)

		fx.start(t)

		// when
		results, err := fx.Search(ctx, &pb.RpcChatSearchRequest{
			SpaceId:  spaceId,
			ChatId:   chatId,
			FullText: "",
		})

		// then
		assert.NoError(t, err)
		assert.Empty(t, results)
	})
}
