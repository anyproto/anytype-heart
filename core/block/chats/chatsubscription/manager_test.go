package chatsubscription

import (
	"context"
	"sync"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/cache/mock_cache"
	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
	"github.com/anyproto/anytype-heart/core/block/chats/chatrepository"
	"github.com/anyproto/anytype-heart/core/block/object/idresolver/mock_idresolver"
	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore/anystoreprovider"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

type accountServiceStub struct {
	accountId string
}

func (a *accountServiceStub) AccountID() string {
	return a.accountId
}

func (a *accountServiceStub) Name() string { return "accountServiceStub" }

func (a *accountServiceStub) Init(ap *app.App) error {
	return nil
}

type fixture struct {
	Service

	lock                  sync.Mutex
	events                []*pb.Event
	eventsToOtherSessions map[string][]*pb.Event
}

const (
	testCreator = "creator1"
	testSpaceId = "space1"
)

func newFixture(t *testing.T) *fixture {
	ctx := context.Background()

	a := &app.App{}

	idResolver := mock_idresolver.NewMockResolver(t)
	idResolver.EXPECT().ResolveSpaceID(mock.Anything).Return(testSpaceId, nil).Maybe()
	idResolver.EXPECT().ResolveSpaceIdWithRetry(mock.Anything, mock.Anything).Return(testSpaceId, nil).Maybe()

	accountService := &accountServiceStub{accountId: testCreator}

	eventSender := mock_event.NewMockSender(t)

	objectStore := objectstore.NewStoreFixture(t)

	objectGetter := mock_cache.NewMockObjectWaitGetterComponent(t)
	objectGetter.EXPECT().WaitAndGetObject(mock.Anything, mock.Anything).Return(nil, nil).Maybe()

	repo := chatrepository.New()

	provider, err := anystoreprovider.NewInPath(t.TempDir())
	require.NoError(t, err)

	a.Register(accountService)
	a.Register(testutil.PrepareMock(ctx, a, eventSender))
	a.Register(testutil.PrepareMock(ctx, a, idResolver))
	a.Register(testutil.PrepareMock(ctx, a, objectGetter))
	a.Register(objectStore)
	a.Register(repo)
	a.Register(provider)
	err = a.Start(ctx)
	require.NoError(t, err)

	fx := &fixture{
		Service:               New(),
		eventsToOtherSessions: make(map[string][]*pb.Event),
	}
	eventSender.EXPECT().Broadcast(mock.Anything).Run(func(ev *pb.Event) {
		fx.lock.Lock()
		defer fx.lock.Unlock()
		fx.events = append(fx.events, ev)
	}).Maybe()

	eventSender.EXPECT().BroadcastToOtherSessions(mock.Anything, mock.Anything).Run(func(sessionId string, ev *pb.Event) {
		fx.lock.Lock()
		defer fx.lock.Unlock()
		fx.eventsToOtherSessions[sessionId] = append(fx.eventsToOtherSessions[sessionId], ev)
	}).Maybe()

	err = fx.Init(a)
	require.NoError(t, err)

	return fx
}

func TestFlush(t *testing.T) {
	t.Run("sync and async events", func(t *testing.T) {
		fx := newFixture(t)
		ctx := context.Background()
		sessionId := "session1"
		sessionCtx := session.NewContext(session.WithSession(sessionId))

		const chatId = "chatId1"

		mngr, err := fx.GetManager(testSpaceId, chatId)
		require.NoError(t, err)

		_, err = fx.SubscribeLastMessages(ctx, SubscribeLastMessagesRequest{
			ChatObjectId:           chatId,
			SubId:                  "sync",
			CouldUseSessionContext: true,
		})
		_, err = fx.SubscribeLastMessages(ctx, SubscribeLastMessagesRequest{
			ChatObjectId: chatId,
			SubId:        "async",
		})

		message := givenSimpleMessage("msg1", "hello!")
		updatedMessage := givenSimpleMessage("msg2", "world!")
		messageWithReactions := givenComplexMessage("msg3", "with reactions")

		mngr.SetSessionContext(sessionCtx)
		mngr.Add("prevOrder1", message)
		mngr.UpdateFull(updatedMessage)
		mngr.UpdateReactions(messageWithReactions)
		mngr.Delete("msg4")
		mngr.UpdateChatState(func(state *model.ChatState) *model.ChatState {
			state.LastStateId = "lastStateId"
			return state
		})
		mngr.ReadMessages("oldestOrderId", []string{"msg5"}, chatmodel.CounterTypeMessage)
		mngr.ReadMessages("oldestOrderId", []string{"msg5"}, chatmodel.CounterTypeMention)
		mngr.Flush()

		generateWantEvents := func(subId string) []*pb.Event {
			return []*pb.Event{
				{
					ContextId: chatId,
					Messages: []*pb.EventMessage{
						{
							SpaceId: testSpaceId,
							Value: &pb.EventMessageValueOfChatAdd{
								ChatAdd: &pb.EventChatAdd{
									Id: message.Id,
									SubIds: []string{
										subId,
									},
									OrderId:      message.OrderId,
									Message:      message.ChatMessage,
									AfterOrderId: "prevOrder1",
								},
							},
						},
						{
							SpaceId: testSpaceId,
							Value: &pb.EventMessageValueOfChatUpdate{
								ChatUpdate: &pb.EventChatUpdate{
									Id: updatedMessage.Id,
									SubIds: []string{
										subId,
									},
									Message: updatedMessage.ChatMessage,
								},
							},
						},
						{
							SpaceId: testSpaceId,
							Value: &pb.EventMessageValueOfChatUpdateReactions{
								ChatUpdateReactions: &pb.EventChatUpdateReactions{
									Id: messageWithReactions.Id,
									SubIds: []string{
										subId,
									},
									Reactions: givenReactions(),
								},
							},
						},
						{
							SpaceId: testSpaceId,
							Value: &pb.EventMessageValueOfChatDelete{
								ChatDelete: &pb.EventChatDelete{
									Id: "msg4",
									SubIds: []string{
										subId,
									},
								},
							},
						},
						{
							SpaceId: testSpaceId,
							Value: &pb.EventMessageValueOfChatUpdateMessageReadStatus{
								ChatUpdateMessageReadStatus: &pb.EventChatUpdateMessageReadStatus{
									Ids:    []string{"msg5"},
									IsRead: true,
									SubIds: []string{
										subId,
									},
								},
							},
						},
						{
							SpaceId: testSpaceId,
							Value: &pb.EventMessageValueOfChatUpdateMentionReadStatus{
								ChatUpdateMentionReadStatus: &pb.EventChatUpdateMentionReadStatus{
									Ids:    []string{"msg5"},
									IsRead: true,
									SubIds: []string{
										subId,
									},
								},
							},
						},
						// ChatState is reloaded from database, because Delete was called
						{
							SpaceId: testSpaceId,
							Value: &pb.EventMessageValueOfChatStateUpdate{
								ChatStateUpdate: &pb.EventChatUpdateState{
									State: &model.ChatState{
										Messages:    &model.ChatStateUnreadState{},
										Mentions:    &model.ChatStateUnreadState{},
										LastStateId: "",
										Order:       6,
									},
									SubIds: []string{
										subId,
									},
								},
							},
						},
					},
				},
			}
		}

		assert.Equal(t, generateWantEvents("async"), fx.events)
		assert.Equal(t, generateWantEvents("sync"), fx.eventsToOtherSessions[sessionId])
	})
}

func givenSimpleMessage(id string, text string) *chatmodel.Message {
	return &chatmodel.Message{
		ChatMessage: &model.ChatMessage{
			Id:          id,
			OrderId:     "order1",
			Creator:     testCreator,
			Read:        true,
			MentionRead: true,
			Message: &model.ChatMessageMessageContent{
				Text:  text,
				Style: model.BlockContentText_Paragraph,
			},
		},
	}
}
func givenComplexMessage(id string, text string) *chatmodel.Message {
	return &chatmodel.Message{
		ChatMessage: &model.ChatMessage{
			Id:               id,
			OrderId:          "order2",
			Creator:          testCreator,
			Read:             true,
			MentionRead:      true,
			ReplyToMessageId: "replyToMessageId1",
			Message: &model.ChatMessageMessageContent{
				Text:  text,
				Style: model.BlockContentText_Quote,
				Marks: []*model.BlockContentTextMark{
					{
						Range: &model.Range{
							From: 0,
							To:   1,
						},
						Type:  model.BlockContentTextMark_Link,
						Param: "https://example.com",
					},
					{
						Range: &model.Range{
							From: 2,
							To:   3,
						},
						Type: model.BlockContentTextMark_Italic,
					},
				},
			},
			Attachments: []*model.ChatMessageAttachment{
				{
					Target: "attachmentId1",
					Type:   model.ChatMessageAttachment_IMAGE,
				},
				{
					Target: "attachmentId2",
					Type:   model.ChatMessageAttachment_LINK,
				},
			},
			Reactions: givenReactions(),
		},
	}
}

func givenReactions() *model.ChatMessageReactions {
	return &model.ChatMessageReactions{
		Reactions: map[string]*model.ChatMessageReactionsIdentityList{
			"🥰": {
				Ids: []string{"identity1", "identity2"},
			},
			"🤔": {
				Ids: []string{"identity3"},
			},
		},
	}
}
