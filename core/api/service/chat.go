package service

import (
	"context"
	"errors"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var (
	ErrFailedGetMessages    = errors.New("failed to get chat messages")
	ErrFailedAddMessage     = errors.New("failed to add chat message")
	ErrFailedEditMessage    = errors.New("failed to edit chat message")
	ErrFailedDeleteMessage  = errors.New("failed to delete chat message")
	ErrFailedToggleReaction = errors.New("failed to toggle reaction")
)

func (s *Service) GetChatMessages(ctx context.Context, chatId string, beforeOrderId, afterOrderId string, limit int) ([]apimodel.ChatMessage, error) {
	resp := s.mw.ChatGetMessages(ctx, &pb.RpcChatGetMessagesRequest{
		ChatObjectId:  chatId,
		BeforeOrderId: beforeOrderId,
		AfterOrderId:  afterOrderId,
		Limit:         int32(limit),
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcChatGetMessagesResponseError_NULL {
		return nil, ErrFailedGetMessages
	}

	return convertMessages(resp.Messages), nil
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
