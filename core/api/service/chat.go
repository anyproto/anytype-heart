package service

import (
	"context"
	"errors"
	"strings"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var (
	ErrChatNotFound           = errors.New("chat not found")
	ErrMessageNotFound        = errors.New("message not found")
	ErrFailedRetrieveMessages = errors.New("failed to retrieve messages")
	ErrFailedCreateMessage    = errors.New("failed to create message")
	ErrFailedUpdateMessage    = errors.New("failed to update message")
	ErrFailedDeleteMessage    = errors.New("failed to delete message")
	ErrFailedToggleReaction   = errors.New("failed to toggle reaction")
	ErrFailedSearchMessages   = errors.New("failed to search messages")
)

// GetMessages retrieves a paginated list of chat messages.
func (s *Service) GetMessages(ctx context.Context, chatId string, beforeOrderId string, afterOrderId string, limit int) ([]apimodel.ChatMessage, *apimodel.ChatState, error) {
	resp := s.mw.ChatGetMessages(ctx, &pb.RpcChatGetMessagesRequest{
		ChatObjectId:  chatId,
		BeforeOrderId: beforeOrderId,
		AfterOrderId:  afterOrderId,
		Limit:         int32(limit), //nolint:gosec
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcChatGetMessagesResponseError_NULL {
		if resp.Error.Code == pb.RpcChatGetMessagesResponseError_BAD_INPUT {
			return nil, nil, ErrChatNotFound
		}
		return nil, nil, ErrFailedRetrieveMessages
	}

	messages := make([]apimodel.ChatMessage, 0, len(resp.Messages))
	for _, msg := range resp.Messages {
		messages = append(messages, s.protoMessageToApiMessage(msg))
	}

	var chatState *apimodel.ChatState
	if resp.ChatState != nil {
		chatState = s.protoChatStateToApiChatState(resp.ChatState)
	}

	return messages, chatState, nil
}

// GetMessageById retrieves a single message by its ID.
func (s *Service) GetMessageById(ctx context.Context, chatId string, messageId string) (*apimodel.ChatMessage, error) {
	resp := s.mw.ChatGetMessagesByIds(ctx, &pb.RpcChatGetMessagesByIdsRequest{
		ChatObjectId: chatId,
		MessageIds:   []string{messageId},
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcChatGetMessagesByIdsResponseError_NULL {
		return nil, ErrFailedRetrieveMessages
	}

	if len(resp.Messages) == 0 {
		return nil, ErrMessageNotFound
	}

	message := s.protoMessageToApiMessage(resp.Messages[0])
	return &message, nil
}

// CreateMessage creates a new chat message.
func (s *Service) CreateMessage(ctx context.Context, chatId string, req apimodel.CreateMessageRequest) (*apimodel.ChatMessage, error) {
	protoMsg := &model.ChatMessage{
		ReplyToMessageId: req.ReplyToMessageId,
		Message: &model.ChatMessageMessageContent{
			Text:  req.Text,
			Style: s.apiStyleToProtoStyle(req.Style),
		},
		Attachments: s.apiAttachmentsToProtoAttachments(req.Attachments),
	}

	resp := s.mw.ChatAddMessage(ctx, &pb.RpcChatAddMessageRequest{
		ChatObjectId: chatId,
		Message:      protoMsg,
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcChatAddMessageResponseError_NULL {
		if resp.Error.Code == pb.RpcChatAddMessageResponseError_BAD_INPUT {
			return nil, ErrChatNotFound
		}
		return nil, ErrFailedCreateMessage
	}

	// Retrieve the created message
	return s.GetMessageById(ctx, chatId, resp.MessageId)
}

// UpdateMessage updates an existing chat message.
func (s *Service) UpdateMessage(ctx context.Context, chatId string, messageId string, req apimodel.UpdateMessageRequest) error {
	editedMsg := &model.ChatMessage{
		Id: messageId,
		Message: &model.ChatMessageMessageContent{
			Text:  req.Text,
			Style: s.apiStyleToProtoStyle(req.Style),
		},
	}

	resp := s.mw.ChatEditMessageContent(ctx, &pb.RpcChatEditMessageContentRequest{
		ChatObjectId:  chatId,
		MessageId:     messageId,
		EditedMessage: editedMsg,
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcChatEditMessageContentResponseError_NULL {
		if resp.Error.Code == pb.RpcChatEditMessageContentResponseError_BAD_INPUT {
			return ErrMessageNotFound
		}
		return ErrFailedUpdateMessage
	}

	return nil
}

// DeleteMessage deletes a chat message.
func (s *Service) DeleteMessage(ctx context.Context, chatId string, messageId string) error {
	resp := s.mw.ChatDeleteMessage(ctx, &pb.RpcChatDeleteMessageRequest{
		ChatObjectId: chatId,
		MessageId:    messageId,
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcChatDeleteMessageResponseError_NULL {
		if resp.Error.Code == pb.RpcChatDeleteMessageResponseError_BAD_INPUT {
			return ErrMessageNotFound
		}
		return ErrFailedDeleteMessage
	}

	return nil
}

// ToggleReaction toggles a reaction on a message.
func (s *Service) ToggleReaction(ctx context.Context, chatId string, messageId string, emoji string) (bool, error) {
	resp := s.mw.ChatToggleMessageReaction(ctx, &pb.RpcChatToggleMessageReactionRequest{
		ChatObjectId: chatId,
		MessageId:    messageId,
		Emoji:        emoji,
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcChatToggleMessageReactionResponseError_NULL {
		if resp.Error.Code == pb.RpcChatToggleMessageReactionResponseError_BAD_INPUT {
			return false, ErrMessageNotFound
		}
		return false, ErrFailedToggleReaction
	}

	return resp.Added, nil
}

// SearchMessages searches for messages in a chat.
func (s *Service) SearchMessages(ctx context.Context, spaceId string, chatId string, query string, offset int, limit int) ([]apimodel.SearchMessageResult, error) {
	resp := s.mw.ChatSearch(ctx, &pb.RpcChatSearchRequest{
		SpaceId:  spaceId,
		ChatId:   chatId,
		FullText: query,
		Offset:   int32(offset), //nolint:gosec
		Limit:    int32(limit),  //nolint:gosec
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcChatSearchResponseError_NULL {
		return nil, ErrFailedSearchMessages
	}

	results := make([]apimodel.SearchMessageResult, 0, len(resp.Results))
	for _, r := range resp.Results {
		result := apimodel.SearchMessageResult{
			ChatId:    r.ChatId,
			MessageId: r.MessageId,
			Score:     r.Score,
			Highlight: r.Highlight,
		}

		if r.HighlightRanges != nil {
			result.HighlightRanges = make([]apimodel.Range, 0, len(r.HighlightRanges))
			for _, hr := range r.HighlightRanges {
				result.HighlightRanges = append(result.HighlightRanges, apimodel.Range{
					From: hr.From,
					To:   hr.To,
				})
			}
		}

		if r.Message != nil {
			result.Message = s.protoMessageToApiMessage(r.Message)
		}

		results = append(results, result)
	}

	return results, nil
}

// protoMessageToApiMessage converts a protobuf ChatMessage to an API ChatMessage.
func (s *Service) protoMessageToApiMessage(msg *model.ChatMessage) apimodel.ChatMessage {
	apiMsg := apimodel.ChatMessage{
		Object:           "chat_message",
		Id:               msg.Id,
		OrderId:          msg.OrderId,
		Creator:          msg.Creator,
		CreatedAt:        msg.CreatedAt,
		ModifiedAt:       msg.ModifiedAt,
		ReplyToMessageId: msg.ReplyToMessageId,
		Read:             msg.Read,
		Attachments:      make([]apimodel.MessageAttachment, 0),
		Reactions:        make(map[string][]string),
	}

	if msg.Message != nil {
		apiMsg.Content = apimodel.MessageContent{
			Text:  msg.Message.Text,
			Style: s.protoStyleToApiStyle(msg.Message.Style),
		}

		if msg.Message.Marks != nil {
			apiMsg.Content.Marks = make([]apimodel.TextMark, 0, len(msg.Message.Marks))
			for _, mark := range msg.Message.Marks {
				apiMsg.Content.Marks = append(apiMsg.Content.Marks, apimodel.TextMark{
					Type:  s.protoMarkTypeToApiMarkType(mark.Type),
					From:  mark.Range.From,
					To:    mark.Range.To,
					Param: mark.Param,
				})
			}
		}
	}

	for _, att := range msg.Attachments {
		apiMsg.Attachments = append(apiMsg.Attachments, apimodel.MessageAttachment{
			Target: att.Target,
			Type:   s.protoAttachmentTypeToApiType(att.Type),
		})
	}

	if msg.Reactions != nil && msg.Reactions.Reactions != nil {
		for emoji, identityList := range msg.Reactions.Reactions {
			apiMsg.Reactions[emoji] = identityList.Ids
		}
	}

	return apiMsg
}

// protoChatStateToApiChatState converts a protobuf ChatState to an API ChatState.
func (s *Service) protoChatStateToApiChatState(state *model.ChatState) *apimodel.ChatState {
	apiState := &apimodel.ChatState{
		LastStateId: state.LastStateId,
	}

	if state.Messages != nil {
		apiState.Messages = &apimodel.UnreadState{
			Counter:       state.Messages.Counter,
			OldestOrderId: state.Messages.OldestOrderId,
		}
	}

	if state.Mentions != nil {
		apiState.Mentions = &apimodel.UnreadState{
			Counter:       state.Mentions.Counter,
			OldestOrderId: state.Mentions.OldestOrderId,
		}
	}

	return apiState
}

// protoStyleToApiStyle converts a protobuf BlockContentTextStyle to an API MessageStyle.
func (s *Service) protoStyleToApiStyle(style model.BlockContentTextStyle) apimodel.MessageStyle {
	switch style {
	case model.BlockContentText_Paragraph:
		return apimodel.MessageStyleParagraph
	case model.BlockContentText_Header1:
		return apimodel.MessageStyleHeader1
	case model.BlockContentText_Header2:
		return apimodel.MessageStyleHeader2
	case model.BlockContentText_Header3:
		return apimodel.MessageStyleHeader3
	case model.BlockContentText_Header4:
		return apimodel.MessageStyleHeader4
	case model.BlockContentText_Quote:
		return apimodel.MessageStyleQuote
	case model.BlockContentText_Code:
		return apimodel.MessageStyleCode
	case model.BlockContentText_Title:
		return apimodel.MessageStyleTitle
	case model.BlockContentText_Checkbox:
		return apimodel.MessageStyleCheckbox
	case model.BlockContentText_Marked:
		return apimodel.MessageStyleMarked
	case model.BlockContentText_Numbered:
		return apimodel.MessageStyleNumbered
	case model.BlockContentText_Toggle:
		return apimodel.MessageStyleToggle
	case model.BlockContentText_Description:
		return apimodel.MessageStyleDescription
	case model.BlockContentText_Callout:
		return apimodel.MessageStyleCallout
	default:
		return apimodel.MessageStyleParagraph
	}
}

// apiStyleToProtoStyle converts an API MessageStyle to a protobuf BlockContentTextStyle.
func (s *Service) apiStyleToProtoStyle(style apimodel.MessageStyle) model.BlockContentTextStyle {
	switch style {
	case apimodel.MessageStyleParagraph:
		return model.BlockContentText_Paragraph
	case apimodel.MessageStyleHeader1:
		return model.BlockContentText_Header1
	case apimodel.MessageStyleHeader2:
		return model.BlockContentText_Header2
	case apimodel.MessageStyleHeader3:
		return model.BlockContentText_Header3
	case apimodel.MessageStyleHeader4:
		return model.BlockContentText_Header4
	case apimodel.MessageStyleQuote:
		return model.BlockContentText_Quote
	case apimodel.MessageStyleCode:
		return model.BlockContentText_Code
	case apimodel.MessageStyleTitle:
		return model.BlockContentText_Title
	case apimodel.MessageStyleCheckbox:
		return model.BlockContentText_Checkbox
	case apimodel.MessageStyleMarked:
		return model.BlockContentText_Marked
	case apimodel.MessageStyleNumbered:
		return model.BlockContentText_Numbered
	case apimodel.MessageStyleToggle:
		return model.BlockContentText_Toggle
	case apimodel.MessageStyleDescription:
		return model.BlockContentText_Description
	case apimodel.MessageStyleCallout:
		return model.BlockContentText_Callout
	default:
		return model.BlockContentText_Paragraph
	}
}

// protoAttachmentTypeToApiType converts a protobuf attachment type to an API attachment type.
func (s *Service) protoAttachmentTypeToApiType(t model.ChatMessageAttachmentAttachmentType) apimodel.AttachmentType {
	switch t {
	case model.ChatMessageAttachment_FILE:
		return apimodel.AttachmentTypeFile
	case model.ChatMessageAttachment_IMAGE:
		return apimodel.AttachmentTypeImage
	case model.ChatMessageAttachment_LINK:
		return apimodel.AttachmentTypeLink
	default:
		return apimodel.AttachmentTypeFile
	}
}

// apiAttachmentTypeToProtoType converts an API attachment type to a protobuf attachment type.
func (s *Service) apiAttachmentTypeToProtoType(t apimodel.AttachmentType) model.ChatMessageAttachmentAttachmentType {
	switch t {
	case apimodel.AttachmentTypeFile:
		return model.ChatMessageAttachment_FILE
	case apimodel.AttachmentTypeImage:
		return model.ChatMessageAttachment_IMAGE
	case apimodel.AttachmentTypeLink:
		return model.ChatMessageAttachment_LINK
	default:
		return model.ChatMessageAttachment_FILE
	}
}

// apiAttachmentsToProtoAttachments converts API attachments to protobuf attachments.
func (s *Service) apiAttachmentsToProtoAttachments(attachments []apimodel.MessageAttachment) []*model.ChatMessageAttachment {
	if attachments == nil {
		return nil
	}

	protoAttachments := make([]*model.ChatMessageAttachment, 0, len(attachments))
	for _, att := range attachments {
		protoAttachments = append(protoAttachments, &model.ChatMessageAttachment{
			Target: att.Target,
			Type:   s.apiAttachmentTypeToProtoType(att.Type),
		})
	}
	return protoAttachments
}

// protoMarkTypeToApiMarkType converts a protobuf mark type to an API string.
func (s *Service) protoMarkTypeToApiMarkType(t model.BlockContentTextMarkType) string {
	return strings.ToLower(model.BlockContentTextMarkType_name[int32(t)])
}
