package chatobject

import (
	"context"
	"fmt"
	"testing"

	"github.com/anyproto/any-sync/app"
	"github.com/anyproto/any-sync/app/debugstat"
	"github.com/anyproto/any-sync/commonspace/object/accountdata"
	"github.com/anyproto/any-sync/util/crypto"
	"github.com/globalsign/mgo/bson"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/cache/mock_cache"
	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
	"github.com/anyproto/anytype-heart/core/block/chats/chatrepository"
	"github.com/anyproto/anytype-heart/core/block/chats/chatsubscription"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/storestate"
	"github.com/anyproto/anytype-heart/core/block/object/idresolver/mock_idresolver"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/block/source/mock_source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/datastore/anystoreprovider"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/tests/testutil"
)

const (
	testSpaceId = "spaceId1"
)

type accountServiceStub struct {
	accountId string
	signKey   crypto.PrivKey
}

func (a *accountServiceStub) AccountID() string {
	return a.accountId
}

func (a *accountServiceStub) Keys() *accountdata.AccountKeys {
	return &accountdata.AccountKeys{
		SignKey: a.signKey,
	}
}

func (a *accountServiceStub) Name() string { return "accountServiceStub" }

func (a *accountServiceStub) Init(ap *app.App) error {
	signKey, _, _ := crypto.GenerateRandomEd25519KeyPair()
	a.signKey = signKey
	return nil
}

type stubSeenHeadsCollector struct {
	heads []string
}

func (c *stubSeenHeadsCollector) collectSeenHeads(ctx context.Context, afterOrderId string) ([]string, error) {
	return c.heads, nil
}

type fixture struct {
	*storeObject
	source             *mock_source.MockStore
	accountServiceStub *accountServiceStub
	sourceCreator      string
	eventSender        *mock_event.MockSender
	events             []*pb.EventMessage
	spaceIndex         spaceindex.Store

	generateOrderIdFunc func(tx *storestate.StoreStateTx) string
}

const testCreator = "accountId1"

func newFixture(t *testing.T) *fixture {
	ctx := context.Background()

	a := &app.App{}

	idResolver := mock_idresolver.NewMockResolver(t)
	idResolver.EXPECT().ResolveSpaceID(mock.Anything).Return(testSpaceId, nil).Maybe()
	idResolver.EXPECT().ResolveSpaceIdWithRetry(mock.Anything, mock.Anything).Return(testSpaceId, nil).Maybe()

	accountService := &accountServiceStub{accountId: testCreator}

	eventSender := mock_event.NewMockSender(t)

	sb := smarttest.New("chatId1")

	objectStore := objectstore.NewStoreFixture(t)
	spaceIndex := objectStore.SpaceIndex(testSpaceId)

	repo := chatrepository.New()
	subscriptions := chatsubscription.New()

	provider, err := anystoreprovider.NewInPath(t.TempDir())
	require.NoError(t, err)

	objectGetter := mock_cache.NewMockObjectWaitGetterComponent(t)
	objectGetter.EXPECT().WaitAndGetObject(mock.Anything, mock.Anything).Return(nil, nil).Maybe()

	a.Register(accountService)
	a.Register(testutil.PrepareMock(ctx, a, eventSender))
	a.Register(testutil.PrepareMock(ctx, a, idResolver))
	a.Register(testutil.PrepareMock(ctx, a, objectGetter))
	a.Register(objectStore)
	a.Register(repo)
	a.Register(subscriptions)
	a.Register(provider)

	err = a.Start(ctx)
	require.NoError(t, err)
	db, err := provider.GetCrdtDb(testSpaceId).Wait()
	require.NoError(t, err)

	object := New(sb, accountService, db, repo, subscriptions, nil, nil, nil, debugstat.NewNoOp())
	rawObject := object.(*storeObject)

	fx := &fixture{
		storeObject:        rawObject,
		accountServiceStub: accountService,
		sourceCreator:      testCreator,
		eventSender:        eventSender,
		spaceIndex:         spaceIndex,
	}
	eventSender.EXPECT().Broadcast(mock.Anything).Run(func(event *pb.Event) {
		for _, msg := range event.Messages {
			fx.events = append(fx.events, msg)
		}
	}).Return().Maybe()

	source := mock_source.NewMockStore(t)
	source.EXPECT().Id().Return("chatId1")
	source.EXPECT().SpaceID().Return(testSpaceId)
	source.EXPECT().ReadStoreDoc(ctx, mock.Anything, mock.Anything).Return(nil)
	source.EXPECT().PushStoreChange(mock.Anything, mock.Anything).RunAndReturn(fx.applyToStore).Maybe()

	onSeenHooks := map[string]func([]string){}
	source.EXPECT().RegisterDiffManager(mock.Anything, mock.Anything).Run(func(name string, hook func([]string)) {
		onSeenHooks[name] = hook
	}).Return()

	source.EXPECT().InitDiffManager(mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	source.EXPECT().StoreSeenHeads(mock.Anything, mock.Anything).Return(nil).Maybe()

	// Imitate diff manager
	source.EXPECT().MarkSeenHeads(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(func(ctx context.Context, name string, seenHeads []string) error {
		allMessagesResp, err := fx.GetMessages(ctx, chatrepository.GetMessagesRequest{
			AfterOrderId:    "",
			IncludeBoundary: true,
		})
		if err != nil {
			return fmt.Errorf("get messages: %w", err)
		}

		var collectedHeads []string

		for _, msg := range allMessagesResp.Messages {
			for _, seen := range seenHeads {
				if msg.Id <= seen {
					collectedHeads = append(collectedHeads, msg.Id)
					break
				}
			}
		}

		onSeenHooks[name](collectedHeads)

		return nil
	}).Maybe()

	fx.source = source

	err = object.Init(&smartblock.InitContext{
		Ctx:    ctx,
		Source: source,
	})
	require.NoError(t, err)

	rawObject.seenHeadsCollector = &stubSeenHeadsCollector{heads: []string{}}

	return fx
}

func TestAddMessage(t *testing.T) {
	t.Run("add own messages", func(t *testing.T) {
		ctx := context.Background()
		sessionCtx := session.NewContext()

		fx := newFixture(t)
		fx.eventSender.EXPECT().BroadcastToOtherSessions(sessionCtx.ID(), mock.Anything).Return().Maybe()

		_, err := fx.chatSubscriptionService.SubscribeLastMessages(ctx, chatsubscription.SubscribeLastMessagesRequest{
			ChatObjectId:           fx.Id(),
			SubId:                  "sub",
			CouldUseSessionContext: true,
		})
		require.NoError(t, err)

		inputMessage := givenComplexMessage()
		messageId, err := fx.AddMessage(ctx, sessionCtx, inputMessage)
		require.NoError(t, err)
		assert.NotEmpty(t, messageId)
		assert.NotEmpty(t, sessionCtx.GetMessages())

		messagesResp, err := fx.GetMessages(ctx, chatrepository.GetMessagesRequest{})
		require.NoError(t, err)
		require.Len(t, messagesResp.Messages, 1)

		want := givenComplexMessage()
		want.Id = messageId
		want.Creator = testCreator

		got := messagesResp.Messages[0]
		assertMessagesEqual(t, want, got)
	})

	t.Run("imitate adding other's messages", func(t *testing.T) {
		ctx := context.Background()
		sessionCtx := session.NewContext()

		fx := newFixture(t)
		fx.eventSender.EXPECT().BroadcastToOtherSessions(sessionCtx.ID(), mock.Anything).Return().Maybe()

		_, err := fx.chatSubscriptionService.SubscribeLastMessages(ctx, chatsubscription.SubscribeLastMessagesRequest{
			ChatObjectId:           fx.Id(),
			SubId:                  "sub",
			CouldUseSessionContext: true,
		})
		require.NoError(t, err)

		// Force all messages as not read
		fx.chatHandler.forceNotRead = true

		inputMessage := givenComplexMessage()
		messageId, err := fx.AddMessage(ctx, sessionCtx, inputMessage)
		require.NoError(t, err)
		assert.NotEmpty(t, messageId)
		assert.NotEmpty(t, sessionCtx.GetMessages())

		messagesResp, err := fx.GetMessages(ctx, chatrepository.GetMessagesRequest{})
		require.NoError(t, err)
		require.Len(t, messagesResp.Messages, 1)
		assert.Equal(t, messagesResp.ChatState.LastStateId, messagesResp.Messages[0].StateId)

		want := givenComplexMessage()
		want.Id = messageId
		want.Creator = testCreator
		want.Read = false
		want.MentionRead = false

		got := messagesResp.Messages[0]
		assertMessagesEqual(t, want, got)
	})
}

func TestGetMessages(t *testing.T) {
	ctx := context.Background()
	fx := newFixture(t)

	for i := 0; i < 10; i++ {
		inputMessage := givenComplexMessage()
		inputMessage.Message.Text = fmt.Sprintf("text %d", i+1)
		messageId, err := fx.AddMessage(ctx, nil, inputMessage)
		require.NoError(t, err)
		assert.NotEmpty(t, messageId)
	}

	messagesResp, err := fx.GetMessages(ctx, chatrepository.GetMessagesRequest{Limit: 5})
	require.NoError(t, err)

	lastMessage := messagesResp.Messages[4]
	assert.Equal(t, messagesResp.ChatState.LastStateId, lastMessage.StateId)

	wantTexts := []string{"text 6", "text 7", "text 8", "text 9", "text 10"}
	for i, msg := range messagesResp.Messages {
		assert.Equal(t, wantTexts[i], msg.Message.Text)
	}

	t.Run("with requested BeforeOrderId", func(t *testing.T) {
		lastOrderId := messagesResp.Messages[0].OrderId // text 6
		gotMessages, err := fx.GetMessages(ctx, chatrepository.GetMessagesRequest{BeforeOrderId: lastOrderId, Limit: 5})
		require.NoError(t, err)
		wantTexts = []string{"text 1", "text 2", "text 3", "text 4", "text 5"}
		for i, msg := range gotMessages.Messages {
			assert.Equal(t, wantTexts[i], msg.Message.Text)
		}
	})

	t.Run("with requested AfterOrderId", func(t *testing.T) {
		lastOrderId := messagesResp.Messages[0].OrderId // text 6
		gotMessages, err := fx.GetMessages(ctx, chatrepository.GetMessagesRequest{AfterOrderId: lastOrderId, Limit: 2})
		require.NoError(t, err)
		wantTexts = []string{"text 7", "text 8"}
		for i, msg := range gotMessages.Messages {
			assert.Equal(t, wantTexts[i], msg.Message.Text)
		}
	})
}

func TestGetMessagesByIds(t *testing.T) {
	ctx := context.Background()

	fx := newFixture(t)

	inputMessage := givenComplexMessage()
	messageId, err := fx.AddMessage(ctx, nil, inputMessage)
	require.NoError(t, err)

	messages, err := fx.GetMessagesByIds(ctx, []string{messageId, "wrongId"})
	require.NoError(t, err)
	require.Len(t, messages, 1)

	want := givenComplexMessage()
	want.Id = messageId
	want.Creator = testCreator
	got := messages[0]
	assertMessagesEqual(t, want, got)
}

func TestEditMessage(t *testing.T) {
	t.Run("edit own message", func(t *testing.T) {
		ctx := context.Background()
		fx := newFixture(t)

		// Add
		inputMessage := givenComplexMessage()

		messageId, err := fx.AddMessage(ctx, nil, inputMessage)
		require.NoError(t, err)
		assert.NotEmpty(t, messageId)

		// Edit
		editedMessage := givenComplexMessage()
		editedMessage.Message.Text = "edited text!"

		err = fx.EditMessage(ctx, messageId, editedMessage)
		require.NoError(t, err)

		messagesResp, err := fx.GetMessages(ctx, chatrepository.GetMessagesRequest{})
		require.NoError(t, err)
		require.Len(t, messagesResp.Messages, 1)

		want := editedMessage
		want.Id = messageId
		want.Creator = testCreator

		got := messagesResp.Messages[0]
		assert.True(t, got.ModifiedAt > 0)
		got.ModifiedAt = 0
		assertMessagesEqual(t, want, got)
	})

	t.Run("edit other's message", func(t *testing.T) {
		ctx := context.Background()
		fx := newFixture(t)

		// Add
		inputMessage := givenComplexMessage()

		messageId, err := fx.AddMessage(ctx, nil, inputMessage)
		require.NoError(t, err)
		assert.NotEmpty(t, messageId)

		// Edit
		editedMessage := givenComplexMessage()
		editedMessage.Message.Text = "edited text!"

		fx.sourceCreator = "maliciousPerson"

		err = fx.EditMessage(ctx, messageId, editedMessage)
		require.Error(t, err)

		// Check that nothing is changed
		messagesResp, err := fx.GetMessages(ctx, chatrepository.GetMessagesRequest{})
		require.NoError(t, err)
		require.Len(t, messagesResp.Messages, 1)

		want := givenComplexMessage()
		want.Id = messageId
		want.Creator = testCreator

		got := messagesResp.Messages[0]
		assertMessagesEqual(t, want, got)
	})

}

func TestToggleReaction(t *testing.T) {
	ctx := context.Background()
	fx := newFixture(t)

	// Add
	inputMessage := givenComplexMessage()
	inputMessage.Reactions = nil

	messageId, err := fx.AddMessage(ctx, nil, inputMessage)
	require.NoError(t, err)
	assert.NotEmpty(t, messageId)

	// Edit
	editedMessage := givenComplexMessage()
	editedMessage.Message.Text = "edited text!"

	t.Run("can toggle own reactions", func(t *testing.T) {
		added, err := fx.ToggleMessageReaction(ctx, messageId, "👻")
		require.NoError(t, err)
		assert.True(t, added)

		added, err = fx.ToggleMessageReaction(ctx, messageId, "🐻")
		require.NoError(t, err)
		assert.True(t, added)

		added, err = fx.ToggleMessageReaction(ctx, messageId, "👺")
		require.NoError(t, err)
		assert.True(t, added)

		added, err = fx.ToggleMessageReaction(ctx, messageId, "👺")
		require.NoError(t, err)
		assert.False(t, added)
	})

	anotherPerson := "anotherPerson"

	t.Run("can't toggle someone else's reactions", func(t *testing.T) {
		fx.sourceCreator = testCreator
		fx.accountServiceStub.accountId = anotherPerson
		added, err := fx.ToggleMessageReaction(ctx, messageId, "🐻")
		require.Error(t, err)
		assert.False(t, added)
	})
	t.Run("can toggle reactions on someone else's messages", func(t *testing.T) {
		fx.sourceCreator = anotherPerson
		fx.accountServiceStub.accountId = anotherPerson
		added, err := fx.ToggleMessageReaction(ctx, messageId, "🐻")
		require.NoError(t, err)
		assert.True(t, added)
	})

	messagesResp, err := fx.GetMessages(ctx, chatrepository.GetMessagesRequest{})
	require.NoError(t, err)
	require.Len(t, messagesResp.Messages, 1)

	got := messagesResp.Messages[0].Reactions

	want := &model.ChatMessageReactions{
		Reactions: map[string]*model.ChatMessageReactionsIdentityList{
			"👻": {
				Ids: []string{testCreator},
			},
			"🐻": {
				Ids: []string{testCreator, anotherPerson},
			},
		},
	}
	assert.Equal(t, want, got)
}

func (fx *fixture) assertReadStatus(t *testing.T, ctx context.Context, afterOrderId string, beforeOrderId string, isRead bool, isMentionRead bool) *GetMessagesResponse {
	messageResp, err := fx.GetMessages(ctx, chatrepository.GetMessagesRequest{
		AfterOrderId:    afterOrderId,
		BeforeOrderId:   beforeOrderId,
		IncludeBoundary: true,
		Limit:           1000,
	})
	require.NoError(t, err)

	for _, m := range messageResp.Messages {
		assert.Equal(t, isRead, m.Read)
		assert.Equal(t, isMentionRead, m.MentionRead)
	}
	return messageResp
}

func (fx *fixture) generateOrderId(tx *storestate.StoreStateTx) string {
	if fx.generateOrderIdFunc != nil {
		return fx.generateOrderIdFunc(tx)
	}
	return tx.NextOrder(tx.GetMaxOrder())
}

func (fx *fixture) applyToStore(ctx context.Context, params source.PushStoreChangeParams) (string, error) {
	changeId := bson.NewObjectId().Hex()
	tx, err := params.State.NewTx(ctx)
	if err != nil {
		return "", fmt.Errorf("new tx: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()
	order := fx.generateOrderId(tx)
	err = tx.ApplyChangeSet(storestate.ChangeSet{
		Id:        changeId,
		Order:     order,
		Changes:   params.Changes,
		Creator:   fx.sourceCreator,
		Timestamp: params.Time.Unix(),
	})
	if err != nil {
		return "", fmt.Errorf("apply change set: %w", err)
	}
	err = tx.Commit()
	if err != nil {
		return "", err
	}
	fx.onUpdate()
	return changeId, nil
}

func givenSimpleMessage(text string) *chatmodel.Message {
	return &chatmodel.Message{
		ChatMessage: &model.ChatMessage{
			Id:          "",
			OrderId:     "",
			Creator:     "",
			Read:        true,
			MentionRead: true,
			Message: &model.ChatMessageMessageContent{
				Text:  text,
				Style: model.BlockContentText_Paragraph,
			},
		},
	}
}

func givenMessageWithMention(text string) *chatmodel.Message {
	return &chatmodel.Message{
		ChatMessage: &model.ChatMessage{
			Id:          "",
			OrderId:     "",
			Creator:     "",
			Read:        true,
			MentionRead: true,
			Message: &model.ChatMessageMessageContent{
				Text:  text,
				Style: model.BlockContentText_Paragraph,
				Marks: []*model.BlockContentTextMark{
					{
						Type:  model.BlockContentTextMark_Mention,
						Param: domain.NewParticipantId(testSpaceId, testCreator),
						Range: &model.Range{From: 0, To: 1},
					},
				},
			},
		},
	}
}

func givenComplexMessage() *chatmodel.Message {
	return &chatmodel.Message{
		ChatMessage: &model.ChatMessage{
			Id:               "",
			OrderId:          "",
			Creator:          "",
			Read:             true,
			MentionRead:      true,
			ReplyToMessageId: "replyToMessageId1",
			Message: &model.ChatMessageMessageContent{
				Text:  "text!",
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
			Reactions: &model.ChatMessageReactions{
				Reactions: map[string]*model.ChatMessageReactionsIdentityList{
					"🥰": {
						Ids: []string{"identity1", "identity2"},
					},
					"🤔": {
						Ids: []string{"identity3"},
					},
				},
			},
		},
	}
}

func assertMessagesEqual(t *testing.T, want, got *chatmodel.Message) {
	// Cleanup order id
	assert.NotEmpty(t, got.OrderId)
	got.OrderId = ""
	// Cleanup timestamp
	assert.NotZero(t, got.CreatedAt)
	got.CreatedAt = 0

	assert.NotEmpty(t, got.StateId)
	got.StateId = ""

	assert.Equal(t, want, got)
}
