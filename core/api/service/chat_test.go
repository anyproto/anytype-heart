package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const (
	mockedChatId    = "chat-object-id"
	mockedMessageId = "msg-123"
)

func TestChatService_GetChatMessages(t *testing.T) {
	t.Run("successful get messages", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)

		fx.mwMock.EXPECT().ChatGetMessages(ctx, &pb.RpcChatGetMessagesRequest{
			ChatObjectId: mockedChatId,
			Limit:        50,
		}).Return(&pb.RpcChatGetMessagesResponse{
			Messages: []*model.ChatMessage{
				{
					Id:      mockedMessageId,
					Creator: "user1",
					Message: &model.ChatMessageMessageContent{Text: "hello"},
				},
			},
			Error: &pb.RpcChatGetMessagesResponseError{Code: pb.RpcChatGetMessagesResponseError_NULL},
		})

		// when
		messages, err := fx.service.GetChatMessages(ctx, mockedChatId, "", "", 50)

		// then
		require.NoError(t, err)
		require.Len(t, messages, 1)
		assert.Equal(t, mockedMessageId, messages[0].Id)
		assert.Equal(t, "hello", messages[0].Content.Text)
	})

	t.Run("error from middleware", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)

		fx.mwMock.EXPECT().ChatGetMessages(ctx, &pb.RpcChatGetMessagesRequest{
			ChatObjectId: mockedChatId,
			Limit:        50,
		}).Return(&pb.RpcChatGetMessagesResponse{
			Error: &pb.RpcChatGetMessagesResponseError{Code: pb.RpcChatGetMessagesResponseError_UNKNOWN_ERROR},
		})

		// when
		_, err := fx.service.GetChatMessages(ctx, mockedChatId, "", "", 50)

		// then
		require.ErrorIs(t, err, ErrFailedGetMessages)
	})
}

func TestChatService_AddChatMessage(t *testing.T) {
	t.Run("successful add message", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)

		fx.mwMock.EXPECT().ChatAddMessage(mock.Anything, mock.Anything).Return(&pb.RpcChatAddMessageResponse{
			MessageId: mockedMessageId,
			Error:     &pb.RpcChatAddMessageResponseError{Code: pb.RpcChatAddMessageResponseError_NULL},
		})

		// when
		messageId, err := fx.service.AddChatMessage(ctx, mockedChatId, apimodel.AddChatMessageRequest{
			Text: "new message",
		})

		// then
		require.NoError(t, err)
		assert.Equal(t, mockedMessageId, messageId)
	})

	t.Run("error from middleware", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)

		fx.mwMock.EXPECT().ChatAddMessage(mock.Anything, mock.Anything).Return(&pb.RpcChatAddMessageResponse{
			Error: &pb.RpcChatAddMessageResponseError{Code: pb.RpcChatAddMessageResponseError_UNKNOWN_ERROR},
		})

		// when
		_, err := fx.service.AddChatMessage(ctx, mockedChatId, apimodel.AddChatMessageRequest{
			Text: "fail",
		})

		// then
		require.ErrorIs(t, err, ErrFailedAddMessage)
	})
}

func TestChatService_EditChatMessage(t *testing.T) {
	t.Run("successful edit", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)

		fx.mwMock.EXPECT().ChatEditMessageContent(mock.Anything, mock.Anything).Return(&pb.RpcChatEditMessageContentResponse{
			Error: &pb.RpcChatEditMessageContentResponseError{Code: pb.RpcChatEditMessageContentResponseError_NULL},
		})

		// when
		err := fx.service.EditChatMessage(ctx, mockedChatId, mockedMessageId, apimodel.EditChatMessageRequest{
			Text: "edited text",
		})

		// then
		require.NoError(t, err)
	})
}

func TestChatService_DeleteChatMessage(t *testing.T) {
	t.Run("successful delete", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)

		fx.mwMock.EXPECT().ChatDeleteMessage(ctx, &pb.RpcChatDeleteMessageRequest{
			ChatObjectId: mockedChatId,
			MessageId:    mockedMessageId,
		}).Return(&pb.RpcChatDeleteMessageResponse{
			Error: &pb.RpcChatDeleteMessageResponseError{Code: pb.RpcChatDeleteMessageResponseError_NULL},
		})

		// when
		err := fx.service.DeleteChatMessage(ctx, mockedChatId, mockedMessageId)

		// then
		require.NoError(t, err)
	})
}

func TestChatService_ToggleChatReaction(t *testing.T) {
	t.Run("successful toggle reaction", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)

		fx.mwMock.EXPECT().ChatToggleMessageReaction(ctx, &pb.RpcChatToggleMessageReactionRequest{
			ChatObjectId: mockedChatId,
			MessageId:    mockedMessageId,
			Emoji:        "👍",
		}).Return(&pb.RpcChatToggleMessageReactionResponse{
			Added: true,
			Error: &pb.RpcChatToggleMessageReactionResponseError{Code: pb.RpcChatToggleMessageReactionResponseError_NULL},
		})

		// when
		added, err := fx.service.ToggleChatReaction(ctx, mockedChatId, mockedMessageId, "👍")

		// then
		require.NoError(t, err)
		assert.True(t, added)
	})
}

func TestChatMessageConversion(t *testing.T) {
	t.Run("proto to api model", func(t *testing.T) {
		// given
		protoMsg := &model.ChatMessage{
			Id:      "msg1",
			OrderId: "order1",
			Creator: "user1",
			Message: &model.ChatMessageMessageContent{
				Text:  "hello **world**",
				Style: model.BlockContentText_Paragraph,
				Marks: []*model.BlockContentTextMark{
					{
						Range: &model.Range{From: 6, To: 15},
						Type:  model.BlockContentTextMark_Bold,
					},
				},
			},
			Attachments: []*model.ChatMessageAttachment{
				{Target: "file1", Type: model.ChatMessageAttachment_IMAGE},
			},
			Reactions: &model.ChatMessageReactions{
				Reactions: map[string]*model.ChatMessageReactionsIdentityList{
					"👍": {Ids: []string{"user1", "user2"}},
				},
			},
			Pinned: true,
		}

		// when
		apiMsg := apimodel.ChatMessageFromProto(protoMsg)

		// then
		assert.Equal(t, "msg1", apiMsg.Id)
		assert.Equal(t, "hello **world**", apiMsg.Content.Text)
		assert.Equal(t, "paragraph", apiMsg.Content.Style)
		require.Len(t, apiMsg.Content.Marks, 1)
		assert.Equal(t, "bold", apiMsg.Content.Marks[0].Type)
		assert.Equal(t, int32(6), apiMsg.Content.Marks[0].From)
		assert.Equal(t, int32(15), apiMsg.Content.Marks[0].To)
		require.Len(t, apiMsg.Attachments, 1)
		assert.Equal(t, "image", apiMsg.Attachments[0].Type)
		assert.Equal(t, []string{"user1", "user2"}, apiMsg.Reactions["👍"])
		assert.True(t, apiMsg.Pinned)
	})

	t.Run("nil message returns empty struct with initialized collections", func(t *testing.T) {
		// when
		apiMsg := apimodel.ChatMessageFromProto(nil)

		// then
		assert.Empty(t, apiMsg.Id)
		assert.Equal(t, []apimodel.ChatAttachment{}, apiMsg.Attachments)
		assert.Equal(t, map[string][]string{}, apiMsg.Reactions)
	})
}
