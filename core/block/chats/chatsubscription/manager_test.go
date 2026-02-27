package chatsubscription

import (
	"context"
	"fmt"
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
	repo                  chatrepository.Service
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
		repo:                  repo,
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

		// Setup
		repo, err := fx.repo.Repository(testSpaceId, chatId)
		require.NoError(t, err)
		err = repo.AddTestMessage(ctx, givenSimpleMessage("msg2", "world!", "o2"))
		require.NoError(t, err)
		err = repo.AddTestMessage(ctx, givenSimpleMessage("msg3", "with reactions", "o3"))
		require.NoError(t, err)
		err = repo.AddTestMessage(ctx, givenSimpleMessage("msg4", "text", "o4"))
		require.NoError(t, err)
		err = repo.AddTestMessage(ctx, givenSimpleMessage("msg5", "text", "o5"))
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

		message := givenSimpleMessage("msg1", "hello!", "o1")
		updatedMessage := givenSimpleMessage("msg2", "world!", "o2")
		messageWithReactions := givenComplexMessage("msg3", "with reactions", "o3")

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
		mngr.Flush(true)
		t.Run("flush again, expect no extra events", func(t *testing.T) {
			mngr.Flush(true)
		})

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

func TestFlushReloadStateFlag(t *testing.T) {
	t.Run("flush false skips state reload, flush true performs it", func(t *testing.T) {
		fx := newFixture(t)
		ctx := context.Background()

		const chatId = "chatId1"

		mngr, err := fx.GetManager(testSpaceId, chatId)
		require.NoError(t, err)

		// given
		repo, err := fx.repo.Repository(testSpaceId, chatId)
		require.NoError(t, err)
		err = repo.AddTestMessage(ctx, givenSimpleMessage("msg1", "hello", "o1"))
		require.NoError(t, err)

		_, err = fx.SubscribeLastMessages(ctx, SubscribeLastMessagesRequest{
			ChatObjectId: chatId,
			SubId:        "sub1",
		})
		require.NoError(t, err)

		// when: Delete sets needReloadState, then Flush(false) skips the reload
		mngr.Delete("msg1")
		mngr.Flush(false)

		// then: delete event is sent, but no ChatStateUpdate (state reload was skipped)
		require.Len(t, fx.events, 1)
		require.Len(t, fx.events[0].Messages, 1)
		assert.NotNil(t, fx.events[0].Messages[0].GetChatDelete())

		// when: reset captured events and call Flush(true) which should reload state
		fx.lock.Lock()
		fx.events = nil
		fx.lock.Unlock()
		mngr.Flush(true)

		// then: ChatStateUpdate is sent from the deferred state reload
		require.Len(t, fx.events, 1)
		require.Len(t, fx.events[0].Messages, 1)
		stateUpdate := fx.events[0].Messages[0].GetChatStateUpdate()
		require.NotNil(t, stateUpdate, "expected ChatStateUpdate event after Flush(true)")

		// when: flush again, no more events (needReloadState was reset)
		fx.lock.Lock()
		fx.events = nil
		fx.lock.Unlock()
		mngr.Flush(true)

		// then
		assert.Empty(t, fx.events)
	})
}

func TestOutOfWindowEvents(t *testing.T) {
	t.Run("update full", func(t *testing.T) {
		fx := newFixture(t)
		ctx := context.Background()

		chatId := "chatId1"
		subId := "subId1"

		mngr, err := fx.GetManager(testSpaceId, chatId)
		require.NoError(t, err)

		_, err = fx.SubscribeLastMessages(ctx, SubscribeLastMessagesRequest{
			ChatObjectId: chatId,
			SubId:        subId,
		})

		updatedMessage := givenComplexMessage("msg1", "with reactions", "o1")
		mngr.UpdateFull(updatedMessage)
		mngr.Flush(true)
		t.Run("flush again, expect no extra events", func(t *testing.T) {
			mngr.Flush(true)
		})

		want := []*pb.Event{
			{
				ContextId: chatId,
				Messages: []*pb.EventMessage{
					{
						SpaceId: testSpaceId,
						Value: &pb.EventMessageValueOfChatUpdate{
							ChatUpdate: &pb.EventChatUpdate{
								Id: "msg1",
								SubIds: []string{
									subId,
								},
								Message: updatedMessage.ChatMessage,
							},
						},
					},
				},
			},
		}
		assert.Equal(t, want, fx.events)
	})

	t.Run("update reactions", func(t *testing.T) {
		fx := newFixture(t)
		ctx := context.Background()

		chatId := "chatId1"
		subId := "subId1"

		mngr, err := fx.GetManager(testSpaceId, chatId)
		require.NoError(t, err)

		_, err = fx.SubscribeLastMessages(ctx, SubscribeLastMessagesRequest{
			ChatObjectId: chatId,
			SubId:        subId,
		})

		mngr.UpdateReactions(givenComplexMessage("msg1", "", "o1"))
		mngr.Flush(true)
		t.Run("flush again, expect no extra events", func(t *testing.T) {
			mngr.Flush(true)
		})

		want := []*pb.Event{
			{
				ContextId: chatId,
				Messages: []*pb.EventMessage{
					{
						SpaceId: testSpaceId,
						Value: &pb.EventMessageValueOfChatUpdateReactions{
							ChatUpdateReactions: &pb.EventChatUpdateReactions{
								Id: "msg1",
								SubIds: []string{
									subId,
								},
								Reactions: givenReactions(),
							},
						},
					},
				},
			},
		}
		assert.Equal(t, want, fx.events)
	})
}

func TestGetLastMessage(t *testing.T) {
	fx := newFixture(t)
	ctx := context.Background()

	chatId := "chatId1"
	subId := "subId1"

	mngr, err := fx.GetManager(testSpaceId, chatId)
	require.NoError(t, err)

	t.Run("with no subscriptions and no messages in db", func(t *testing.T) {
		_, ok, err := mngr.GetLastMessage()
		require.NoError(t, err)
		assert.False(t, ok)
	})

	t.Run("with no subscriptions, fallback to repository", func(t *testing.T) {
		repo, err := fx.repo.Repository(testSpaceId, chatId)
		require.NoError(t, err)
		dbMsg := givenSimpleMessage("dbMsg1", "from db", "o0")
		err = repo.AddTestMessage(ctx, dbMsg)
		require.NoError(t, err)

		got, ok, err := mngr.GetLastMessage()
		require.NoError(t, err)
		assert.True(t, ok)
		assert.Equal(t, dbMsg.Id, got.Id)
		assert.Equal(t, dbMsg.ChatMessage.Message.Text, got.Message.Text)
	})

	// Subscribe after messages are already in db — subscription will load them
	_, err = fx.SubscribeLastMessages(ctx, SubscribeLastMessagesRequest{
		ChatObjectId: chatId,
		SubId:        subId,
	})
	require.NoError(t, err)

	msg := givenComplexMessage("msg1", "text", "o1")
	mngr.Add("", msg)

	t.Run("with only one message", func(t *testing.T) {
		got, ok, err := mngr.GetLastMessage()
		require.NoError(t, err)
		assert.True(t, ok)
		assert.Equal(t, msg.ChatMessage, got)
	})

	msg2 := givenComplexMessage("msg2", "text 2", "o2")
	mngr.Add("o1", msg2)

	t.Run("with multiple messages", func(t *testing.T) {
		got, ok, err := mngr.GetLastMessage()
		require.NoError(t, err)
		assert.True(t, ok)
		assert.Equal(t, msg2.ChatMessage, got)
	})
}

func TestMultiSubEventMerge(t *testing.T) {
	// These tests verify that when two subscriptions with different window sizes
	// coexist (e.g. preview Limit=1 and chat Limit=50), the full message is always
	// used in EventChatAdd, even when an out-of-window subscription with a minimal
	// message is merged first due to non-deterministic map iteration.
	//
	// The scenario:
	// 1. Both subs receive Add(msg0), Add(msg1 — newer)
	// 2. Preview (Limit=1) evicts msg0, keeping only msg1
	// 3. A partial update (reactions/pinned) for msg0 creates a minimal out-of-window
	//    entry in the preview sub ({Id: "msg0"} with only reactions/pinned set)
	// 4. Chat (Limit=50) keeps msg0 in window with full content
	// 5. During Flush, if the preview sub is iterated first, the minimal message
	//    would be placed in the events buffer and never overwritten by the chat sub's
	//    full message — resulting in an EventChatAdd with empty Creator/text/content.
	//
	// Multiple iterations are used because Go map iteration order is non-deterministic,
	// so a single run may not trigger the problematic ordering.

	t.Run("add event preserves full message when out-of-window reactions update exists", func(t *testing.T) {
		fx := newFixture(t)
		ctx := context.Background()

		for i := 0; i < 10; i++ {
			chatId := fmt.Sprintf("chat_reactions_%d", i)

			mngr, err := fx.GetManager(testSpaceId, chatId)
			require.NoError(t, err)

			// given: preview (Limit=1) and chat (Limit=50) subscriptions
			_, err = fx.SubscribeLastMessages(ctx, SubscribeLastMessagesRequest{
				ChatObjectId: chatId,
				SubId:        "preview",
				Limit:        1,
			})
			require.NoError(t, err)
			_, err = fx.SubscribeLastMessages(ctx, SubscribeLastMessagesRequest{
				ChatObjectId: chatId,
				SubId:        "chat",
				Limit:        50,
			})
			require.NoError(t, err)

			msg0 := givenSimpleMessage("msg0", "hello", "o0")
			msg1 := givenSimpleMessage("msg1", "world", "o1")

			// when: msg0 added, then msg1 (newer, evicts msg0 from preview),
			// then reactions update for msg0 (out-of-window in preview, in-window in chat)
			mngr.Add("", msg0)
			mngr.Add("o0", msg1)
			mngr.UpdateReactions(&chatmodel.Message{
				ChatMessage: &model.ChatMessage{
					Id:        "msg0",
					Reactions: givenReactions(),
				},
			})

			fx.lock.Lock()
			fx.events = nil
			fx.lock.Unlock()

			mngr.Flush(true)

			// then: EventChatAdd for msg0 must carry the full message
			require.NotEmpty(t, fx.events, "iteration %d", i)
			var found bool
			for _, ev := range fx.events[0].Messages {
				if add := ev.GetChatAdd(); add != nil && add.Id == "msg0" {
					found = true
					require.NotNil(t, add.Message.Message, "iteration %d: message content is nil", i)
					assert.Equal(t, "hello", add.Message.Message.Text, "iteration %d", i)
					assert.Equal(t, testCreator, add.Message.Creator, "iteration %d", i)
					assert.Equal(t, "o0", add.Message.OrderId, "iteration %d", i)
				}
			}
			assert.True(t, found, "iteration %d: expected EventChatAdd for msg0", i)
		}
	})
}

func givenSimpleMessage(id string, text string, orderId string) *chatmodel.Message {
	return &chatmodel.Message{
		ChatMessage: &model.ChatMessage{
			Id:          id,
			OrderId:     orderId,
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
func givenComplexMessage(id string, text string, orderId string) *chatmodel.Message {
	return &chatmodel.Message{
		ChatMessage: &model.ChatMessage{
			Id:               id,
			OrderId:          orderId,
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
