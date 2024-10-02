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
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type accountServiceStub struct {
	accountId string
}

func (a *accountServiceStub) AccountID() string {
	return a.accountId
}

type fixture struct {
	*storeObject
	source             *mock_source.MockStore
	accountServiceStub *accountServiceStub
	sourceCreator      string
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

	object := New(sb, accountService, eventSender, db)

	fx := &fixture{
		storeObject:        object.(*storeObject),
		accountServiceStub: accountService,
		sourceCreator:      testCreator,
	}
	eventSender.EXPECT().Broadcast(mock.Anything).Run(func(event *pb.Event) {
		for _, msg := range event.Messages {
			fx.events = append(fx.events, msg)
		}
	}).Return().Maybe()

	source := mock_source.NewMockStore(t)
	source.EXPECT().ReadStoreDoc(ctx, mock.Anything, mock.Anything).Return(nil)
	source.EXPECT().PushStoreChange(mock.Anything, mock.Anything).RunAndReturn(fx.applyToStore).Maybe()
	fx.source = source

	err = object.Init(&smartblock.InitContext{
		Ctx:    ctx,
		Source: source,
	})
	require.NoError(t, err)

	return fx
}

func TestAddMessage(t *testing.T) {
	ctx := context.Background()
	sessionCtx := session.NewContext()

	fx := newFixture(t)

	inputMessage := givenMessage()
	messageId, err := fx.AddMessage(ctx, sessionCtx, inputMessage)
	require.NoError(t, err)
	assert.NotEmpty(t, messageId)
	assert.NotEmpty(t, sessionCtx.GetMessages())

	messages, err := fx.GetMessages(ctx, "", 0)
	require.NoError(t, err)

	require.Len(t, messages, 1)

	want := givenMessage()
	want.Id = messageId
	want.Creator = testCreator

	got := messages[0]
	assertMessagesEqual(t, want, got)
}

func TestGetMessages(t *testing.T) {
	ctx := context.Background()
	fx := newFixture(t)

	for i := 0; i < 10; i++ {
		inputMessage := givenMessage()
		inputMessage.Message.Text = fmt.Sprintf("text %d", i+1)
		messageId, err := fx.AddMessage(ctx, nil, inputMessage)
		require.NoError(t, err)
		assert.NotEmpty(t, messageId)
	}

	messages, err := fx.GetMessages(ctx, "", 5)
	require.NoError(t, err)
	wantTexts := []string{"text 6", "text 7", "text 8", "text 9", "text 10"}
	for i, msg := range messages {
		assert.Equal(t, wantTexts[i], msg.Message.Text)
	}

	lastOrderId := messages[0].OrderId
	messages, err = fx.GetMessages(ctx, lastOrderId, 10)
	require.NoError(t, err)
	wantTexts = []string{"text 1", "text 2", "text 3", "text 4", "text 5"}
	for i, msg := range messages {
		assert.Equal(t, wantTexts[i], msg.Message.Text)
	}
}

func TestGetMessagesByIds(t *testing.T) {
	ctx := context.Background()
	sessionCtx := session.NewContext()

	fx := newFixture(t)

	inputMessage := givenMessage()
	messageId, err := fx.AddMessage(ctx, sessionCtx, inputMessage)
	require.NoError(t, err)

	messages, err := fx.GetMessagesByIds(ctx, []string{messageId, "wrongId"})
	require.NoError(t, err)
	require.Len(t, messages, 1)

	want := givenMessage()
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
		inputMessage := givenMessage()

		messageId, err := fx.AddMessage(ctx, nil, inputMessage)
		require.NoError(t, err)
		assert.NotEmpty(t, messageId)

		// Edit
		editedMessage := givenMessage()
		editedMessage.Message.Text = "edited text!"

		err = fx.EditMessage(ctx, messageId, editedMessage)
		require.NoError(t, err)

		messages, err := fx.GetMessages(ctx, "", 0)
		require.NoError(t, err)
		require.Len(t, messages, 1)

		want := editedMessage
		want.Id = messageId
		want.Creator = testCreator

		got := messages[0]
		assert.True(t, got.ModifiedAt > 0)
		got.ModifiedAt = 0
		assertMessagesEqual(t, want, got)
	})

	t.Run("edit other's message", func(t *testing.T) {
		ctx := context.Background()
		fx := newFixture(t)

		// Add
		inputMessage := givenMessage()

		messageId, err := fx.AddMessage(ctx, nil, inputMessage)
		require.NoError(t, err)
		assert.NotEmpty(t, messageId)

		// Edit
		editedMessage := givenMessage()
		editedMessage.Message.Text = "edited text!"

		fx.sourceCreator = "maliciousPerson"

		err = fx.EditMessage(ctx, messageId, editedMessage)
		require.Error(t, err)

		// Check that nothing is changed
		messages, err := fx.GetMessages(ctx, "", 0)
		require.NoError(t, err)
		require.Len(t, messages, 1)

		want := inputMessage
		want.Id = messageId
		want.Creator = testCreator

		got := messages[0]
		assertMessagesEqual(t, want, got)
	})

}

func TestToggleReaction(t *testing.T) {
	ctx := context.Background()
	fx := newFixture(t)

	// Add
	inputMessage := givenMessage()
	inputMessage.Reactions = nil

	messageId, err := fx.AddMessage(ctx, nil, inputMessage)
	require.NoError(t, err)
	assert.NotEmpty(t, messageId)

	// Edit
	editedMessage := givenMessage()
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

	messages, err := fx.GetMessages(ctx, "", 0)
	require.NoError(t, err)
	require.Len(t, messages, 1)

	got := messages[0].Reactions

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

func givenMessage() *model.ChatMessage {
	return &model.ChatMessage{
		Id:               "",
		OrderId:          "",
		Creator:          "",
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
	}
}

func assertMessagesEqual(t *testing.T, want, got *model.ChatMessage) {
	// Cleanup order id
	assert.NotEmpty(t, got.OrderId)
	got.OrderId = ""
	// Cleanup timestamp
	assert.NotZero(t, got.CreatedAt)
	got.CreatedAt = 0
	assert.Equal(t, want, got)
}
