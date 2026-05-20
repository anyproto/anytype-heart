package service

import (
	"context"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/api/util"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const (
	mockedChatId    = "chat-object-id"
	mockedMessageId = "msg-123"
)

func TestChatService_ListChats(t *testing.T) {
	expectedBaseFilters := []*model.BlockContentDataviewFilter{
		{
			RelationKey: bundle.RelationKeyResolvedLayout.String(),
			Condition:   model.BlockContentDataviewFilter_In,
			Value:       pbtypes.IntList(util.LayoutsToIntArgs(util.ChatLayouts)...),
		},
		{
			RelationKey: bundle.RelationKeyIsHidden.String(),
			Condition:   model.BlockContentDataviewFilter_NotEqual,
			Value:       pbtypes.Bool(true),
		},
	}
	expectedSorts := []*model.BlockContentDataviewSort{{
		RelationKey: bundle.RelationKeyLastModifiedDate.String(),
		Type:        model.BlockContentDataviewSort_Desc,
		IncludeTime: true,
	}}

	t.Run("returns chats with chat layout", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)
		fx.populateCache(mockedSpaceId)

		fx.mwMock.On("ObjectSearch", mock.Anything, &pb.RpcObjectSearchRequest{
			SpaceId: mockedSpaceId,
			Filters: expectedBaseFilters,
			Sorts:   expectedSorts,
		}).Return(&pb.RpcObjectSearchResponse{
			Records: []*types.Struct{
				{
					Fields: map[string]*types.Value{
						bundle.RelationKeyId.String():             pbtypes.String(mockedChatId),
						bundle.RelationKeyName.String():           pbtypes.String("Team Discussions"),
						bundle.RelationKeyType.String():           pbtypes.String(mockedTypeId),
						bundle.RelationKeyResolvedLayout.String(): pbtypes.Float64(float64(model.ObjectType_chatDerived)),
						bundle.RelationKeySpaceId.String():        pbtypes.String(mockedSpaceId),
					},
				},
			},
			Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
		}).Once()

		// when
		chats, total, hasMore, err := fx.service.ListChats(ctx, mockedSpaceId, nil, offset, limit)

		// then
		require.NoError(t, err)
		require.Len(t, chats, 1)
		assert.Equal(t, mockedChatId, chats[0].Id)
		assert.Equal(t, "Team Discussions", chats[0].Name)
		assert.Equal(t, apimodel.ObjectLayoutChat, chats[0].Layout)
		assert.Equal(t, 1, total)
		assert.False(t, hasMore)
	})

	t.Run("no chats found", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)
		fx.populateCache(mockedSpaceId)

		fx.mwMock.On("ObjectSearch", mock.Anything, &pb.RpcObjectSearchRequest{
			SpaceId: mockedSpaceId,
			Filters: expectedBaseFilters,
			Sorts:   expectedSorts,
		}).Return(&pb.RpcObjectSearchResponse{
			Records: []*types.Struct{},
			Error:   &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
		}).Once()

		// when
		chats, total, hasMore, err := fx.service.ListChats(ctx, mockedSpaceId, nil, offset, limit)

		// then
		require.NoError(t, err)
		require.Len(t, chats, 0)
		assert.Equal(t, 0, total)
		assert.False(t, hasMore)
	})

	t.Run("error from middleware", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)
		fx.populateCache(mockedSpaceId)

		fx.mwMock.On("ObjectSearch", mock.Anything, mock.Anything).Return(&pb.RpcObjectSearchResponse{
			Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_UNKNOWN_ERROR},
		}).Once()

		// when
		_, _, _, err := fx.service.ListChats(ctx, mockedSpaceId, nil, offset, limit)

		// then
		require.ErrorIs(t, err, ErrFailedListChats)
	})

	t.Run("additional filters are forwarded", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)
		fx.populateCache(mockedSpaceId)

		extra := &model.BlockContentDataviewFilter{
			RelationKey: bundle.RelationKeyName.String(),
			Condition:   model.BlockContentDataviewFilter_Like,
			Value:       pbtypes.String("team"),
		}

		fx.mwMock.On("ObjectSearch", mock.Anything, mock.MatchedBy(func(req *pb.RpcObjectSearchRequest) bool {
			if req.SpaceId != mockedSpaceId {
				return false
			}
			if len(req.Filters) != len(expectedBaseFilters)+1 {
				return false
			}
			return req.Filters[len(req.Filters)-1] == extra
		})).Return(&pb.RpcObjectSearchResponse{
			Records: []*types.Struct{},
			Error:   &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
		}).Once()

		// when
		_, _, _, err := fx.service.ListChats(ctx, mockedSpaceId, []*model.BlockContentDataviewFilter{extra}, offset, limit)

		// then
		require.NoError(t, err)
	})
}

func TestChatService_GetChatMessages(t *testing.T) {
	t.Run("successful get messages enriches creator", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)

		const creatorIdentity = "AAjEbEzQx9FNvf5LQFEJEGRojZt3L1MRmBFzP2Q"
		fx.cacheParticipant(mockedSpaceId, creatorIdentity, "Alice")

		fx.mwMock.EXPECT().ChatGetMessages(ctx, &pb.RpcChatGetMessagesRequest{
			ChatObjectId: mockedChatId,
			Limit:        50,
		}).Return(&pb.RpcChatGetMessagesResponse{
			Messages: []*model.ChatMessage{
				{
					Id:      mockedMessageId,
					Creator: creatorIdentity,
					Message: &model.ChatMessageMessageContent{Text: "hello"},
				},
			},
			Error: &pb.RpcChatGetMessagesResponseError{Code: pb.RpcChatGetMessagesResponseError_NULL},
		})

		// when
		messages, err := fx.service.GetChatMessages(ctx, mockedSpaceId, mockedChatId, "", "", 50)

		// then
		require.NoError(t, err)
		require.Len(t, messages, 1)
		assert.Equal(t, mockedMessageId, messages[0].Id)
		assert.Equal(t, "hello", messages[0].Content.Text)
		assert.Equal(t, domain.NewParticipantId(mockedSpaceId, creatorIdentity), messages[0].Creator)
		assert.Equal(t, "Alice", messages[0].CreatorName)
	})

	t.Run("falls back to global name when participant has no name", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)

		const creatorIdentity = "AAjEbEzQx9FNvf5LQFEJEGRojZt3L1MRmBFzP2Q"
		// The participant cache stores Name pre-resolved from name | global_name.
		fx.cacheParticipant(mockedSpaceId, creatorIdentity, "alice.any")

		fx.mwMock.EXPECT().ChatGetMessages(ctx, &pb.RpcChatGetMessagesRequest{
			ChatObjectId: mockedChatId,
			Limit:        50,
		}).Return(&pb.RpcChatGetMessagesResponse{
			Messages: []*model.ChatMessage{
				{Id: mockedMessageId, Creator: creatorIdentity, Message: &model.ChatMessageMessageContent{Text: "hi"}},
			},
			Error: &pb.RpcChatGetMessagesResponseError{Code: pb.RpcChatGetMessagesResponseError_NULL},
		})

		// when
		messages, err := fx.service.GetChatMessages(ctx, mockedSpaceId, mockedChatId, "", "", 50)

		// then
		require.NoError(t, err)
		require.Len(t, messages, 1)
		assert.Equal(t, "alice.any", messages[0].CreatorName)
	})

	t.Run("unknown identity still gets participant id", func(t *testing.T) {
		// given
		ctx := context.Background()
		fx := newFixture(t)

		const creatorIdentity = "AAjUnknown"
		// Note: do not populate the participant cache; lookup returns nil.

		fx.mwMock.EXPECT().ChatGetMessages(ctx, &pb.RpcChatGetMessagesRequest{
			ChatObjectId: mockedChatId,
			Limit:        50,
		}).Return(&pb.RpcChatGetMessagesResponse{
			Messages: []*model.ChatMessage{
				{Id: mockedMessageId, Creator: creatorIdentity, Message: &model.ChatMessageMessageContent{Text: "?"}},
			},
			Error: &pb.RpcChatGetMessagesResponseError{Code: pb.RpcChatGetMessagesResponseError_NULL},
		})

		// when
		messages, err := fx.service.GetChatMessages(ctx, mockedSpaceId, mockedChatId, "", "", 50)

		// then
		require.NoError(t, err)
		require.Len(t, messages, 1)
		assert.Equal(t, domain.NewParticipantId(mockedSpaceId, creatorIdentity), messages[0].Creator)
		assert.Empty(t, messages[0].CreatorName)
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
		_, err := fx.service.GetChatMessages(ctx, mockedSpaceId, mockedChatId, "", "", 50)

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
