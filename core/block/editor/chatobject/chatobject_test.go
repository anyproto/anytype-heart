package chatobject

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	anystore "github.com/anyproto/any-store"
	"github.com/globalsign/mgo/bson"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/storestate"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/block/source/mock_source"
	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type accountServiceStub struct {
	accountId string
}

func (a *accountServiceStub) AccountID() string {
	return a.accountId
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
}

const testCreator = "accountId1"

func newFixture(t *testing.T) *fixture {
	ctx := context.Background()
	db, err := anystore.Open(ctx, filepath.Join(t.TempDir(), "crdt.db"), nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		err := db.Close()
		require.NoError(t, err)
	})

	accountService := &accountServiceStub{accountId: testCreator}

	eventSender := mock_event.NewMockSender(t)

	sb := smarttest.New("chatId1")

	spaceIndex := spaceindex.NewStoreFixture(t)

	object := New(sb, accountService, eventSender, db, spaceIndex)
	rawObject := object.(*storeObject)

	fx := &fixture{
		storeObject:        rawObject,
		accountServiceStub: accountService,
		sourceCreator:      testCreator,
		eventSender:        eventSender,
	}
	eventSender.EXPECT().Broadcast(mock.Anything).Run(func(event *pb.Event) {
		for _, msg := range event.Messages {
			fx.events = append(fx.events, msg)
		}
	}).Return().Maybe()

	source := mock_source.NewMockStore(t)
	source.EXPECT().Id().Return("chatId1")
	source.EXPECT().SpaceID().Return("space1")
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
		allMessagesResp, err := fx.GetMessages(ctx, GetMessagesRequest{
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
		fx.eventSender.EXPECT().BroadcastToOtherSessions(mock.Anything, mock.Anything).Return().Maybe()

		inputMessage := givenComplexMessage()
		messageId, err := fx.AddMessage(ctx, sessionCtx, inputMessage)
		require.NoError(t, err)
		assert.NotEmpty(t, messageId)
		assert.NotEmpty(t, sessionCtx.GetMessages())

		messagesResp, err := fx.GetMessages(ctx, GetMessagesRequest{})
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
		fx.eventSender.EXPECT().BroadcastToOtherSessions(mock.Anything, mock.Anything).Return()

		// Force all messages as not read
		fx.chatHandler.forceNotRead = true

		inputMessage := givenComplexMessage()
		messageId, err := fx.AddMessage(ctx, sessionCtx, inputMessage)
		require.NoError(t, err)
		assert.NotEmpty(t, messageId)
		assert.NotEmpty(t, sessionCtx.GetMessages())

		messagesResp, err := fx.GetMessages(ctx, GetMessagesRequest{})
		require.NoError(t, err)
		require.Len(t, messagesResp.Messages, 1)
		assert.Equal(t, messagesResp.ChatState.DbTimestamp, messagesResp.Messages[0].AddedAt)

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

	messagesResp, err := fx.GetMessages(ctx, GetMessagesRequest{Limit: 5})
	require.NoError(t, err)

	lastMessage := messagesResp.Messages[4]
	assert.Equal(t, messagesResp.ChatState.DbTimestamp, lastMessage.AddedAt)

	wantTexts := []string{"text 6", "text 7", "text 8", "text 9", "text 10"}
	for i, msg := range messagesResp.Messages {
		assert.Equal(t, wantTexts[i], msg.Message.Text)
	}

	t.Run("with requested BeforeOrderId", func(t *testing.T) {
		lastOrderId := messagesResp.Messages[0].OrderId // text 6
		gotMessages, err := fx.GetMessages(ctx, GetMessagesRequest{BeforeOrderId: lastOrderId, Limit: 5})
		require.NoError(t, err)
		wantTexts = []string{"text 1", "text 2", "text 3", "text 4", "text 5"}
		for i, msg := range gotMessages.Messages {
			assert.Equal(t, wantTexts[i], msg.Message.Text)
		}
	})

	t.Run("with requested AfterOrderId", func(t *testing.T) {
		lastOrderId := messagesResp.Messages[0].OrderId // text 6
		gotMessages, err := fx.GetMessages(ctx, GetMessagesRequest{AfterOrderId: lastOrderId, Limit: 2})
		require.NoError(t, err)
		wantTexts = []string{"text 7", "text 8"}
		for i, msg := range gotMessages.Messages {
			assert.Equal(t, wantTexts[i], msg.Message.Text)
		}
	})
}

func TestGetMessagesByIds(t *testing.T) {
	ctx := context.Background()
	sessionCtx := session.NewContext()

	fx := newFixture(t)
	fx.eventSender.EXPECT().BroadcastToOtherSessions(mock.Anything, mock.Anything).Return()

	inputMessage := givenComplexMessage()
	messageId, err := fx.AddMessage(ctx, sessionCtx, inputMessage)
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

		messagesResp, err := fx.GetMessages(ctx, GetMessagesRequest{})
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
		messagesResp, err := fx.GetMessages(ctx, GetMessagesRequest{})
		require.NoError(t, err)
		require.Len(t, messagesResp.Messages, 1)

		want := inputMessage
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
		err = fx.ToggleMessageReaction(ctx, messageId, "👻")
		require.NoError(t, err)
		err = fx.ToggleMessageReaction(ctx, messageId, "🐻")
		require.NoError(t, err)
		err = fx.ToggleMessageReaction(ctx, messageId, "👺")
		require.NoError(t, err)
		err = fx.ToggleMessageReaction(ctx, messageId, "👺")
		require.NoError(t, err)
	})

	anotherPerson := "anotherPerson"

	t.Run("can't toggle someone else's reactions", func(t *testing.T) {
		fx.sourceCreator = testCreator
		fx.accountServiceStub.accountId = anotherPerson
		err = fx.ToggleMessageReaction(ctx, messageId, "🐻")
		require.Error(t, err)
	})
	t.Run("can toggle reactions on someone else's messages", func(t *testing.T) {
		fx.sourceCreator = anotherPerson
		fx.accountServiceStub.accountId = anotherPerson
		err = fx.ToggleMessageReaction(ctx, messageId, "🐻")
		require.NoError(t, err)
	})

	messagesResp, err := fx.GetMessages(ctx, GetMessagesRequest{})
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

func TestReadMessages(t *testing.T) {
	ctx := context.Background()
	fx := newFixture(t)
	fx.chatHandler.forceNotRead = true
	const n = 10
	for i := 0; i < n; i++ {
		_, err := fx.AddMessage(ctx, nil, givenSimpleMessage(fmt.Sprintf("message %d", i+1)))
		require.NoError(t, err)
	}
	// All messages forced as not read
	messagesResp := fx.assertReadStatus(t, ctx, "", "", false)

	err := fx.MarkReadMessages(ctx, "", messagesResp.Messages[2].OrderId, messagesResp.ChatState.DbTimestamp, CounterTypeMessage)
	require.NoError(t, err)

	fx.assertReadStatus(t, ctx, "", messagesResp.Messages[2].OrderId, true)
	fx.assertReadStatus(t, ctx, messagesResp.Messages[3].OrderId, "", false)
}

func TestMarkMessagesAsNotRead(t *testing.T) {
	ctx := context.Background()
	fx := newFixture(t)

	const n = 10
	for i := 0; i < n; i++ {
		_, err := fx.AddMessage(ctx, nil, givenSimpleMessage(fmt.Sprintf("message %d", i+1)))
		require.NoError(t, err)
	}
	// All messages added by myself are read
	fx.assertReadStatus(t, ctx, "", "", true)

	err := fx.MarkMessagesAsUnread(ctx, "", CounterTypeMessage)
	require.NoError(t, err)

	fx.assertReadStatus(t, ctx, "", "", false)
}

func (fx *fixture) assertReadStatus(t *testing.T, ctx context.Context, afterOrderId string, beforeOrderId string, isRead bool) *GetMessagesResponse {
	messageResp, err := fx.GetMessages(ctx, GetMessagesRequest{
		AfterOrderId:    afterOrderId,
		BeforeOrderId:   beforeOrderId,
		IncludeBoundary: true,
		Limit:           1000,
	})
	require.NoError(t, err)

	for _, m := range messageResp.Messages {
		assert.Equal(t, isRead, m.Read)
	}
	return messageResp
}

func (fx *fixture) applyToStore(ctx context.Context, params source.PushStoreChangeParams) (string, error) {
	changeId := bson.NewObjectId().Hex()
	tx, err := params.State.NewTx(ctx)
	if err != nil {
		return "", fmt.Errorf("new tx: %w", err)
	}
	order := tx.NextOrder(tx.GetMaxOrder())
	err = tx.ApplyChangeSet(storestate.ChangeSet{
		Id:        changeId,
		Order:     order,
		Changes:   params.Changes,
		Creator:   fx.sourceCreator,
		Timestamp: params.Time.Unix(),
	})
	if err != nil {
		return "", errors.Join(tx.Rollback(), fmt.Errorf("apply change set: %w", err))
	}
	err = tx.Commit()
	if err != nil {
		return "", err
	}
	fx.onUpdate()
	return changeId, nil
}

func givenSimpleMessage(text string) *Message {
	return &Message{
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

func givenComplexMessage() *Message {
	return &Message{
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

func assertMessagesEqual(t *testing.T, want, got *Message) {
	// Cleanup order id
	assert.NotEmpty(t, got.OrderId)
	got.OrderId = ""
	// Cleanup timestamp
	assert.NotZero(t, got.CreatedAt)
	got.CreatedAt = 0

	assert.NotZero(t, got.AddedAt)
	got.AddedAt = 0

	assert.Equal(t, want, got)
}
