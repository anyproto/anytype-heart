package chatobject

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	anystore "github.com/anyproto/any-store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock/smarttest"
	"github.com/anyproto/anytype-heart/core/block/editor/storestate"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/block/source/mock_source"
	"github.com/anyproto/anytype-heart/core/event/mock_event"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type dbProviderStub struct {
	db anystore.DB
}

func (d *dbProviderStub) GetStoreDb() anystore.DB {
	return d.db
}

type accountServiceStub struct {
	accountId string
}

func (a *accountServiceStub) AccountID() string {
	return a.accountId
}

type fixture struct {
	*storeObject
	source *mock_source.MockStore
}

const testCreator = "accountId1"

func newFixture(t *testing.T) *fixture {
	ctx := context.Background()
	db, err := anystore.Open(ctx, filepath.Join(t.TempDir(), "db"), nil)
	require.NoError(t, err)
	dbProvider := &dbProviderStub{db: db}

	accountService := &accountServiceStub{accountId: testCreator}

	eventSender := mock_event.NewMockSender(t)
	eventSender.EXPECT().Broadcast(mock.Anything).Return().Maybe()

	sb := smarttest.New("chatId1")

	object := New(sb, accountService, dbProvider, eventSender)

	source := mock_source.NewMockStore(t)
	source.EXPECT().ReadStoreDoc(ctx, mock.Anything, mock.Anything).Return(nil)

	err = object.Init(&smartblock.InitContext{
		Ctx:    ctx,
		Source: source,
	})
	require.NoError(t, err)

	return &fixture{
		storeObject: object.(*storeObject),
		source:      source,
	}
}

// TODO Test ChatHandler: validate in BeforeCreate that creator from change equals to creator from message

func TestAddMessage(t *testing.T) {
	ctx := context.Background()
	fx := newFixture(t)
	changeId := "messageId1"
	fx.source.EXPECT().PushStoreChange(mock.Anything, mock.Anything).RunAndReturn(applyToStore(changeId))

	inputMessage := givenMessage()
	messageId, err := fx.AddMessage(ctx, inputMessage)
	require.NoError(t, err)
	assert.Equal(t, changeId, messageId)

	messages, err := fx.GetMessages(ctx, "", 0)
	require.NoError(t, err)

	require.Len(t, messages, 1)

	want := givenMessage()
	want.Id = messageId
	want.Creator = testCreator

	got := messages[0]
	// Cleanup order id
	assert.NotEmpty(t, got.OrderId)
	got.OrderId = ""
	// Cleanup timestamp
	assert.NotZero(t, got.CreatedAt)
	got.CreatedAt = 0
	assert.Equal(t, want, got)
}

func TestEditMessage(t *testing.T) {
	// TODO Test attempt to edit other's message

	ctx := context.Background()
	fx := newFixture(t)

	// Add
	inputMessage := givenMessage()
	changeId := "messageId1"
	fx.source.EXPECT().PushStoreChange(mock.Anything, mock.Anything).RunAndReturn(applyToStore(changeId))
	messageId, err := fx.AddMessage(ctx, inputMessage)
	require.NoError(t, err)

	// Edit
	editedMessage := givenMessage()
	editedMessage.Message.Text = "edited text!"
	editedMessage.Reactions.Reactions["🥰"].Ids = []string{"identity1"}

	changeId = "messageId2"
	fx.source.EXPECT().PushStoreChange(mock.Anything, mock.Anything).RunAndReturn(applyToStore(changeId))
	err = fx.EditMessage(ctx, messageId, editedMessage)
	require.NoError(t, err)

	messages, err := fx.GetMessages(ctx, "", 0)
	require.NoError(t, err)

	require.Len(t, messages, 1)

	want := editedMessage
	want.Id = messageId
	want.Creator = testCreator

	got := messages[0]
	// Cleanup order id
	assert.NotEmpty(t, got.OrderId)
	got.OrderId = ""
	// Cleanup timestamp
	assert.NotZero(t, got.CreatedAt)
	got.CreatedAt = 0
	assert.Equal(t, want, got)
}

func applyToStore(changeId string) func(ctx context.Context, params source.PushStoreChangeParams) (string, error) {
	return func(ctx context.Context, params source.PushStoreChangeParams) (string, error) {
		tx, err := params.State.NewTx(ctx)
		if err != nil {
			return "", fmt.Errorf("new tx: %w", err)
		}
		order := tx.NextOrder(tx.GetMaxOrder())
		err = tx.ApplyChangeSet(storestate.ChangeSet{
			Id:        changeId,
			Order:     order,
			Changes:   params.Changes,
			Creator:   testCreator,
			Timestamp: params.Time.Unix(),
		})
		if err != nil {
			return "", fmt.Errorf("apply change set: %w", err)
		}
		return changeId, tx.Commit()
	}
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
