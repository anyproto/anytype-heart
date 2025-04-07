package chatobject

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/editor/storestate"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

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
	messagesResp := fx.assertReadStatus(t, ctx, "", "", false, false)

	err := fx.MarkReadMessages(ctx, "", messagesResp.Messages[2].OrderId, messagesResp.ChatState.LastStateId, CounterTypeMessage)
	require.NoError(t, err)

	fx.assertReadStatus(t, ctx, "", messagesResp.Messages[2].OrderId, true, false)
	fx.assertReadStatus(t, ctx, messagesResp.Messages[3].OrderId, "", false, false)
}

func TestReadMessagesLoadedInBackground(t *testing.T) {
	ctx := context.Background()
	fx := newFixture(t)
	fx.chatHandler.forceNotRead = true

	firstMessageId, err := fx.AddMessage(ctx, nil, givenSimpleMessage(fmt.Sprintf("first message")))
	require.NoError(t, err)

	firstMessage, err := fx.GetMessageById(ctx, firstMessageId)
	require.NoError(t, err)

	fx.generateOrderIdFunc = func(tx *storestate.StoreStateTx) string {
		prev, err := storestate.LexId.NextBefore("", firstMessage.OrderId)
		require.NoError(t, err)
		return prev
	}

	// The second messages is before the first one
	secondMessageId, err := fx.AddMessage(ctx, nil, givenSimpleMessage(fmt.Sprintf("second message")))
	require.NoError(t, err)

	secondMessage, err := fx.GetMessageById(ctx, secondMessageId)
	require.NoError(t, err)

	err = fx.MarkReadMessages(ctx, "", firstMessage.OrderId, firstMessage.StateId, CounterTypeMessage)
	require.NoError(t, err)

	gotResponse, err := fx.GetMessages(ctx, GetMessagesRequest{})
	require.NoError(t, err)

	firstMessage.Read = true
	wantMessages := []*Message{
		secondMessage,
		firstMessage,
	}

	wantResponse := &GetMessagesResponse{
		Messages: wantMessages,
		ChatState: &model.ChatState{
			Messages: &model.ChatStateUnreadState{
				Counter:       1,
				OldestOrderId: secondMessage.OrderId,
			},
			Mentions:    &model.ChatStateUnreadState{},
			LastStateId: secondMessage.StateId,
		},
	}
	assert.Equal(t, wantResponse, gotResponse)
}

func TestReadMentions(t *testing.T) {
	t.Run("mentioned directly in marks", func(t *testing.T) {
		ctx := context.Background()
		fx := newFixture(t)
		fx.chatHandler.forceNotRead = true
		const n = 10
		for i := 0; i < n; i++ {
			_, err := fx.AddMessage(ctx, nil, givenMessageWithMention(fmt.Sprintf("message %d", i+1)))
			require.NoError(t, err)
		}
		// All messages forced as not read
		messagesResp := fx.assertReadStatus(t, ctx, "", "", false, false)

		err := fx.MarkReadMessages(ctx, "", messagesResp.Messages[2].OrderId, messagesResp.ChatState.LastStateId, CounterTypeMention)
		require.NoError(t, err)

		fx.assertReadStatus(t, ctx, "", messagesResp.Messages[2].OrderId, false, true)
		fx.assertReadStatus(t, ctx, messagesResp.Messages[3].OrderId, "", false, false)
	})

	t.Run("author of replied message", func(t *testing.T) {
		ctx := context.Background()
		fx := newFixture(t)
		fx.chatHandler.forceNotRead = true

		firstMessageId, err := fx.AddMessage(ctx, nil, givenSimpleMessage("message to reply to"))
		require.NoError(t, err)

		secondMessageInput := givenSimpleMessage("a reply")
		secondMessageInput.ReplyToMessageId = firstMessageId

		secondMessageId, err := fx.AddMessage(ctx, nil, secondMessageInput)
		require.NoError(t, err)

		secondMessage, err := fx.GetMessageById(ctx, secondMessageId)
		require.NoError(t, err)

		// All messages forced as not read
		messagesResp := fx.assertReadStatus(t, ctx, "", "", false, false)

		err = fx.MarkReadMessages(ctx, "", secondMessage.OrderId, messagesResp.ChatState.LastStateId, CounterTypeMention)
		require.NoError(t, err)

		fx.assertReadStatus(t, ctx, secondMessage.OrderId, secondMessage.OrderId, false, true)
	})
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
	fx.assertReadStatus(t, ctx, "", "", true, true)

	err := fx.MarkMessagesAsUnread(ctx, "", CounterTypeMessage)
	require.NoError(t, err)

	fx.assertReadStatus(t, ctx, "", "", false, true)
}

func TestMarkMentionsAsNotRead(t *testing.T) {
	ctx := context.Background()
	fx := newFixture(t)

	const n = 10
	for i := 0; i < n; i++ {
		_, err := fx.AddMessage(ctx, nil, givenMessageWithMention(fmt.Sprintf("message %d", i+1)))
		require.NoError(t, err)
	}
	// All messages added by myself are read
	fx.assertReadStatus(t, ctx, "", "", true, true)

	err := fx.MarkMessagesAsUnread(ctx, "", CounterTypeMention)
	require.NoError(t, err)

	fx.assertReadStatus(t, ctx, "", "", true, false)
}
