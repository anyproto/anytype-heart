package service

import (
	"context"
	"errors"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/api/pagination"
	"github.com/anyproto/anytype-heart/core/api/util"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var (
	ErrFailedGetMessages    = errors.New("failed to get chat messages")
	ErrFailedAddMessage     = errors.New("failed to add chat message")
	ErrFailedEditMessage    = errors.New("failed to edit chat message")
	ErrFailedDeleteMessage  = errors.New("failed to delete chat message")
	ErrFailedToggleReaction = errors.New("failed to toggle reaction")
	ErrFailedListChats      = errors.New("failed to list chats")
)

// ListChats retrieves a paginated list of chat objects in a specific space.
func (s *Service) ListChats(ctx context.Context, spaceId string, additionalFilters []*model.BlockContentDataviewFilter, offset int, limit int) (chats []apimodel.Object, total int, hasMore bool, err error) {
	filters := append([]*model.BlockContentDataviewFilter{
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
	}, additionalFilters...)

	resp := s.mw.ObjectSearch(ctx, &pb.RpcObjectSearchRequest{
		SpaceId: spaceId,
		Filters: filters,
		Sorts: []*model.BlockContentDataviewSort{{
			RelationKey: bundle.RelationKeyLastModifiedDate.String(),
			Type:        model.BlockContentDataviewSort_Desc,
			IncludeTime: true,
		}},
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		return nil, 0, false, ErrFailedListChats
	}

	total = len(resp.Records)
	paginatedRecords, hasMore := pagination.Paginate(resp.Records, offset, limit)
	chats = make([]apimodel.Object, 0, len(paginatedRecords))

	for _, record := range paginatedRecords {
		chats = append(chats, s.getObjectFromStruct(record))
	}
	return chats, total, hasMore, nil
}

func (s *Service) GetChatMessages(ctx context.Context, spaceId, chatId string, beforeOrderId, afterOrderId string, limit int) ([]apimodel.ChatMessage, error) {
	resp := s.mw.ChatGetMessages(ctx, &pb.RpcChatGetMessagesRequest{
		ChatObjectId:  chatId,
		BeforeOrderId: beforeOrderId,
		AfterOrderId:  afterOrderId,
		Limit:         int32(limit),
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcChatGetMessagesResponseError_NULL {
		return nil, ErrFailedGetMessages
	}

	messages := convertMessages(resp.Messages)
	s.EnrichChatMessageCreators(ctx, spaceId, messages)
	return messages, nil
}

// EnrichChatMessageCreators rewrites each message's Creator field from the raw
// identity emitted by the middleware to the deterministic participant id, and
// fills CreatorName by batch-querying participant objects in the space.
// Messages with no creator are left untouched. Identities that do not resolve
// to a participant object still receive a participant id (it is computable
// without an RPC) but an empty CreatorName.
func (s *Service) EnrichChatMessageCreators(ctx context.Context, spaceId string, msgs []apimodel.ChatMessage) {
	if len(msgs) == 0 || spaceId == "" {
		return
	}

	identitySet := make(map[string]struct{}, len(msgs))
	for _, m := range msgs {
		if m.Creator != "" {
			identitySet[m.Creator] = struct{}{}
		}
	}
	if len(identitySet) == 0 {
		return
	}

	identities := make([]string, 0, len(identitySet))
	for id := range identitySet {
		identities = append(identities, id)
	}

	nameByIdentity := make(map[string]string, len(identities))
	resp := s.mw.ObjectSearch(ctx, &pb.RpcObjectSearchRequest{
		SpaceId: spaceId,
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyResolvedLayout.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Int64(int64(model.ObjectType_participant)),
			},
			{
				RelationKey: bundle.RelationKeyIdentity.String(),
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       pbtypes.StringList(identities),
			},
		},
		Keys: []string{
			bundle.RelationKeyIdentity.String(),
			bundle.RelationKeyName.String(),
			bundle.RelationKeyGlobalName.String(),
		},
	})
	if resp.Error == nil || resp.Error.Code == pb.RpcObjectSearchResponseError_NULL {
		for _, rec := range resp.Records {
			identity := rec.Fields[bundle.RelationKeyIdentity.String()].GetStringValue()
			if identity == "" {
				continue
			}
			name := rec.Fields[bundle.RelationKeyName.String()].GetStringValue()
			if name == "" {
				name = rec.Fields[bundle.RelationKeyGlobalName.String()].GetStringValue()
			}
			nameByIdentity[identity] = name
		}
	}

	for i := range msgs {
		identity := msgs[i].Creator
		if identity == "" {
			continue
		}
		msgs[i].Creator = domain.NewParticipantId(spaceId, identity)
		msgs[i].CreatorName = nameByIdentity[identity]
	}
}

func (s *Service) AddChatMessage(ctx context.Context, chatId string, req apimodel.AddChatMessageRequest) (string, error) {
	msg := apimodel.MessageContentToProto(req)

	resp := s.mw.ChatAddMessage(ctx, &pb.RpcChatAddMessageRequest{
		ChatObjectId: chatId,
		Message:      msg,
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcChatAddMessageResponseError_NULL {
		return "", ErrFailedAddMessage
	}

	return resp.MessageId, nil
}

func (s *Service) EditChatMessage(ctx context.Context, chatId, messageId string, req apimodel.EditChatMessageRequest) error {
	msg := apimodel.EditContentToProto(req)

	resp := s.mw.ChatEditMessageContent(ctx, &pb.RpcChatEditMessageContentRequest{
		ChatObjectId:  chatId,
		MessageId:     messageId,
		EditedMessage: msg,
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcChatEditMessageContentResponseError_NULL {
		return ErrFailedEditMessage
	}

	return nil
}

func (s *Service) DeleteChatMessage(ctx context.Context, chatId, messageId string) error {
	resp := s.mw.ChatDeleteMessage(ctx, &pb.RpcChatDeleteMessageRequest{
		ChatObjectId: chatId,
		MessageId:    messageId,
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcChatDeleteMessageResponseError_NULL {
		return ErrFailedDeleteMessage
	}

	return nil
}

func (s *Service) ToggleChatReaction(ctx context.Context, chatId, messageId, emoji string) (bool, error) {
	resp := s.mw.ChatToggleMessageReaction(ctx, &pb.RpcChatToggleMessageReactionRequest{
		ChatObjectId: chatId,
		MessageId:    messageId,
		Emoji:        emoji,
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcChatToggleMessageReactionResponseError_NULL {
		return false, ErrFailedToggleReaction
	}

	return resp.Added, nil
}

func convertMessages(msgs []*model.ChatMessage) []apimodel.ChatMessage {
	result := make([]apimodel.ChatMessage, 0, len(msgs))
	for _, msg := range msgs {
		result = append(result, apimodel.ChatMessageFromProto(msg))
	}
	return result
}
