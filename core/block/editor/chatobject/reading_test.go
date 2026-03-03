package chatobject

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/block/chats/chatmodel"
	"github.com/anyproto/anytype-heart/core/block/chats/chatrepository"
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

	_, err := fx.MarkReadMessages(ctx, ReadMessagesRequest{
		AfterOrderId:  "",
		BeforeOrderId: messagesResp.Messages[2].OrderId,
		LastStateId:   messagesResp.ChatState.LastStateId,
		CounterType:   chatmodel.CounterTypeMessage,
	})
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

	_, err = fx.MarkReadMessages(ctx, ReadMessagesRequest{
		AfterOrderId:  "",
		BeforeOrderId: firstMessage.OrderId,
		LastStateId:   firstMessage.StateId,
		CounterType:   chatmodel.CounterTypeMessage,
	})

	gotResponse, err := fx.GetMessages(ctx, chatrepository.GetMessagesRequest{})
	require.NoError(t, err)

	firstMessage.Read = true
	wantMessages := []*chatmodel.Message{
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
			Order:       4,
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

		_, err := fx.MarkReadMessages(ctx, ReadMessagesRequest{
			AfterOrderId:  "",
			BeforeOrderId: messagesResp.Messages[2].OrderId,
			LastStateId:   messagesResp.ChatState.LastStateId,
			CounterType:   chatmodel.CounterTypeMention,
		})
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

		_, err = fx.MarkReadMessages(ctx, ReadMessagesRequest{
			AfterOrderId:  "",
			BeforeOrderId: secondMessage.OrderId,
			LastStateId:   messagesResp.ChatState.LastStateId,
			CounterType:   chatmodel.CounterTypeMention,
		})
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

	err := fx.MarkMessagesAsUnread(ctx, "", chatmodel.CounterTypeMessage)
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

	err := fx.MarkMessagesAsUnread(ctx, "", chatmodel.CounterTypeMention)
	require.NoError(t, err)

	fx.assertReadStatus(t, ctx, "", "", true, false)
}

func TestMarkReadReactions(t *testing.T) {
	const (
		anotherPerson = "anotherPerson"
		thirdPerson   = "thirdPerson"
	)

	t.Run("marks all unread reactions as read", func(t *testing.T) {
		ctx := context.Background()
		fx := newFixture(t)

		// Create message as testCreator (current identity)
		messageId, err := fx.AddMessage(ctx, nil, givenSimpleMessage("my message"))
		require.NoError(t, err)

		// Another person adds a reaction
		fx.sourceCreator = anotherPerson
		fx.accountServiceStub.accountId = anotherPerson
		_, err = fx.ToggleMessageReaction(ctx, messageId, "👍")
		require.NoError(t, err)

		// Third person adds a reaction
		fx.sourceCreator = thirdPerson
		fx.accountServiceStub.accountId = thirdPerson
		_, err = fx.ToggleMessageReaction(ctx, messageId, "❤️")
		require.NoError(t, err)

		// Verify unread
		fx.sourceCreator = testCreator
		fx.accountServiceStub.accountId = testCreator
		msg, err := fx.GetMessageById(ctx, messageId)
		require.NoError(t, err)
		assert.True(t, msg.UnreadReaction)

		changeIds, err := fx.repository.GetAllUnreadReactionChangeIds(ctx)
		require.NoError(t, err)
		require.Len(t, changeIds, 2)

		// Mark all reactions as read
		err = fx.MarkReadReactions(ctx)
		require.NoError(t, err)

		// Verify read
		msg, err = fx.GetMessageById(ctx, messageId)
		require.NoError(t, err)
		assert.False(t, msg.UnreadReaction)

		changeIds, err = fx.repository.GetAllUnreadReactionChangeIds(ctx)
		require.NoError(t, err)
		assert.Empty(t, changeIds)
	})

	t.Run("no-op when no unread reactions", func(t *testing.T) {
		ctx := context.Background()
		fx := newFixture(t)

		messageId, err := fx.AddMessage(ctx, nil, givenSimpleMessage("my message"))
		require.NoError(t, err)

		err = fx.MarkReadReactions(ctx)
		require.NoError(t, err)

		msg, err := fx.GetMessageById(ctx, messageId)
		require.NoError(t, err)
		assert.False(t, msg.UnreadReaction)
	})
}

func TestUnreadReactionTracking(t *testing.T) {
	const (
		anotherPerson = "anotherPerson"
		thirdPerson   = "thirdPerson"
	)

	t.Run("reaction from another user marks unread and tracks in rUnreadChIds", func(t *testing.T) {
		ctx := context.Background()
		fx := newFixture(t)

		// Create message as testCreator (current identity)
		messageId, err := fx.AddMessage(ctx, nil, givenSimpleMessage("my message"))
		require.NoError(t, err)

		// Another person adds a reaction
		fx.sourceCreator = anotherPerson
		fx.accountServiceStub.accountId = anotherPerson
		added, err := fx.ToggleMessageReaction(ctx, messageId, "👍")
		require.NoError(t, err)
		assert.True(t, added)

		msg, err := fx.GetMessageById(ctx, messageId)
		require.NoError(t, err)
		assert.True(t, msg.UnreadReaction)
	})

	t.Run("second reaction from different user also tracked", func(t *testing.T) {
		ctx := context.Background()
		fx := newFixture(t)

		messageId, err := fx.AddMessage(ctx, nil, givenSimpleMessage("my message"))
		require.NoError(t, err)

		// First reaction
		fx.sourceCreator = anotherPerson
		fx.accountServiceStub.accountId = anotherPerson
		_, err = fx.ToggleMessageReaction(ctx, messageId, "👍")
		require.NoError(t, err)

		// Second reaction (different emoji, different user)
		fx.sourceCreator = thirdPerson
		fx.accountServiceStub.accountId = thirdPerson
		_, err = fx.ToggleMessageReaction(ctx, messageId, "❤️")
		require.NoError(t, err)

		msg, err := fx.GetMessageById(ctx, messageId)
		require.NoError(t, err)
		assert.True(t, msg.UnreadReaction)
	})

	t.Run("remove first reaction keeps unread when second remains", func(t *testing.T) {
		ctx := context.Background()
		fx := newFixture(t)

		messageId, err := fx.AddMessage(ctx, nil, givenSimpleMessage("my message"))
		require.NoError(t, err)

		// Add two reactions from different users
		fx.sourceCreator = anotherPerson
		fx.accountServiceStub.accountId = anotherPerson
		_, err = fx.ToggleMessageReaction(ctx, messageId, "👍")
		require.NoError(t, err)

		fx.sourceCreator = thirdPerson
		fx.accountServiceStub.accountId = thirdPerson
		_, err = fx.ToggleMessageReaction(ctx, messageId, "❤️")
		require.NoError(t, err)

		// Remove first reaction (toggle off)
		fx.sourceCreator = anotherPerson
		fx.accountServiceStub.accountId = anotherPerson
		added, err := fx.ToggleMessageReaction(ctx, messageId, "👍")
		require.NoError(t, err)
		assert.False(t, added)

		msg, err := fx.GetMessageById(ctx, messageId)
		require.NoError(t, err)
		assert.True(t, msg.UnreadReaction, "should still be unread because second reaction remains")
	})

	t.Run("remove last reaction clears unread", func(t *testing.T) {
		ctx := context.Background()
		fx := newFixture(t)

		messageId, err := fx.AddMessage(ctx, nil, givenSimpleMessage("my message"))
		require.NoError(t, err)

		// Add one reaction
		fx.sourceCreator = anotherPerson
		fx.accountServiceStub.accountId = anotherPerson
		_, err = fx.ToggleMessageReaction(ctx, messageId, "👍")
		require.NoError(t, err)

		msg, err := fx.GetMessageById(ctx, messageId)
		require.NoError(t, err)
		assert.True(t, msg.UnreadReaction)

		// Remove the reaction
		_, err = fx.ToggleMessageReaction(ctx, messageId, "👍")
		require.NoError(t, err)

		msg, err = fx.GetMessageById(ctx, messageId)
		require.NoError(t, err)
		assert.False(t, msg.UnreadReaction, "should be read because all unread reactions removed")
	})

	t.Run("ClearUnreadReactions clears both fields", func(t *testing.T) {
		ctx := context.Background()
		fx := newFixture(t)

		messageId, err := fx.AddMessage(ctx, nil, givenSimpleMessage("my message"))
		require.NoError(t, err)

		// Add reactions
		fx.sourceCreator = anotherPerson
		fx.accountServiceStub.accountId = anotherPerson
		_, err = fx.ToggleMessageReaction(ctx, messageId, "👍")
		require.NoError(t, err)

		fx.sourceCreator = thirdPerson
		fx.accountServiceStub.accountId = thirdPerson
		_, err = fx.ToggleMessageReaction(ctx, messageId, "❤️")
		require.NoError(t, err)

		msg, err := fx.GetMessageById(ctx, messageId)
		require.NoError(t, err)
		assert.True(t, msg.UnreadReaction)

		// Collect all change IDs and clear them directly via repository
		fx.sourceCreator = testCreator
		fx.accountServiceStub.accountId = testCreator
		changeIds, err := fx.repository.GetAllUnreadReactionChangeIds(ctx)
		require.NoError(t, err)
		require.Len(t, changeIds, 2)

		// Full clear — empty maxOrderId clears all
		_, err = fx.repository.ClearUnreadReactions(ctx, "")
		require.NoError(t, err)

		msg, err = fx.GetMessageById(ctx, messageId)
		require.NoError(t, err)
		assert.False(t, msg.UnreadReaction, "should be read after clearing all reaction change IDs")
	})

	t.Run("reaction on non-author message does not track unread", func(t *testing.T) {
		ctx := context.Background()
		fx := newFixture(t)

		// anotherPerson creates a message (not testCreator's message)
		fx.sourceCreator = anotherPerson
		fx.accountServiceStub.accountId = anotherPerson
		messageId, err := fx.AddMessage(ctx, nil, givenSimpleMessage("not my message"))
		require.NoError(t, err)

		// thirdPerson adds a reaction to anotherPerson's message
		fx.sourceCreator = thirdPerson
		fx.accountServiceStub.accountId = thirdPerson
		added, err := fx.ToggleMessageReaction(ctx, messageId, "👍")
		require.NoError(t, err)
		assert.True(t, added)

		// From testCreator's perspective, should not be unread (not my message)
		fx.sourceCreator = testCreator
		fx.accountServiceStub.accountId = testCreator
		msg, err := fx.GetMessageById(ctx, messageId)
		require.NoError(t, err)
		assert.False(t, msg.UnreadReaction)
	})

	t.Run("self-reaction does not track unread", func(t *testing.T) {
		ctx := context.Background()
		fx := newFixture(t)

		// testCreator creates a message
		messageId, err := fx.AddMessage(ctx, nil, givenSimpleMessage("my message"))
		require.NoError(t, err)

		// testCreator reacts to own message
		added, err := fx.ToggleMessageReaction(ctx, messageId, "👍")
		require.NoError(t, err)
		assert.True(t, added)

		msg, err := fx.GetMessageById(ctx, messageId)
		require.NoError(t, err)
		assert.False(t, msg.UnreadReaction)
	})
}
