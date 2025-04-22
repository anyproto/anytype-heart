package core

import (
	"context"

	anystore "github.com/anyproto/any-store"

	"github.com/anyproto/anytype-heart/core/block/chats"
	"github.com/anyproto/anytype-heart/core/block/editor/chatobject"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func (mw *Middleware) ChatAddMessage(cctx context.Context, req *pb.RpcChatAddMessageRequest) *pb.RpcChatAddMessageResponse {
	ctx := mw.newContext(cctx)
	chatService := mustService[chats.Service](mw)

	messageId, err := chatService.AddMessage(cctx, ctx, req.ChatObjectId, &chatobject.Message{ChatMessage: req.Message})
	if err != nil {
		code := mapErrorCode[pb.RpcChatAddMessageResponseErrorCode](err)
		return &pb.RpcChatAddMessageResponse{
			Error: &pb.RpcChatAddMessageResponseError{
				Code:        code,
				Description: getErrorDescription(err),
			},
		}
	}
	return &pb.RpcChatAddMessageResponse{
		MessageId: messageId,
		Event:     ctx.GetResponseEvent(),
	}
}

func (mw *Middleware) ChatEditMessageContent(cctx context.Context, req *pb.RpcChatEditMessageContentRequest) *pb.RpcChatEditMessageContentResponse {
	chatService := mustService[chats.Service](mw)

	err := chatService.EditMessage(cctx, req.ChatObjectId, req.MessageId, &chatobject.Message{ChatMessage: req.EditedMessage})
	if err != nil {
		code := mapErrorCode[pb.RpcChatEditMessageContentResponseErrorCode](err)
		return &pb.RpcChatEditMessageContentResponse{
			Error: &pb.RpcChatEditMessageContentResponseError{
				Code:        code,
				Description: getErrorDescription(err),
			},
		}
	}
	return &pb.RpcChatEditMessageContentResponse{}
}

func (mw *Middleware) ChatToggleMessageReaction(cctx context.Context, req *pb.RpcChatToggleMessageReactionRequest) *pb.RpcChatToggleMessageReactionResponse {
	chatService := mustService[chats.Service](mw)

	err := chatService.ToggleMessageReaction(cctx, req.ChatObjectId, req.MessageId, req.Emoji)
	if err != nil {
		code := mapErrorCode[pb.RpcChatToggleMessageReactionResponseErrorCode](err)
		return &pb.RpcChatToggleMessageReactionResponse{
			Error: &pb.RpcChatToggleMessageReactionResponseError{
				Code:        code,
				Description: getErrorDescription(err),
			},
		}
	}
	return &pb.RpcChatToggleMessageReactionResponse{}
}

func (mw *Middleware) ChatDeleteMessage(cctx context.Context, req *pb.RpcChatDeleteMessageRequest) *pb.RpcChatDeleteMessageResponse {
	chatService := mustService[chats.Service](mw)

	err := chatService.DeleteMessage(cctx, req.ChatObjectId, req.MessageId)
	if err != nil {
		code := mapErrorCode[pb.RpcChatDeleteMessageResponseErrorCode](err)
		return &pb.RpcChatDeleteMessageResponse{
			Error: &pb.RpcChatDeleteMessageResponseError{
				Code:        code,
				Description: getErrorDescription(err),
			},
		}
	}
	return &pb.RpcChatDeleteMessageResponse{}
}

func (mw *Middleware) ChatGetMessages(cctx context.Context, req *pb.RpcChatGetMessagesRequest) *pb.RpcChatGetMessagesResponse {
	chatService := mustService[chats.Service](mw)

	resp, err := chatService.GetMessages(cctx, req.ChatObjectId, chatobject.GetMessagesRequest{
		AfterOrderId:    req.AfterOrderId,
		BeforeOrderId:   req.BeforeOrderId,
		Limit:           int(req.Limit),
		IncludeBoundary: req.IncludeBoundary,
	})
	if err != nil {
		code := mapErrorCode[pb.RpcChatGetMessagesResponseErrorCode](err)
		return &pb.RpcChatGetMessagesResponse{
			Error: &pb.RpcChatGetMessagesResponseError{
				Code:        code,
				Description: getErrorDescription(err),
			},
		}
	}

	return &pb.RpcChatGetMessagesResponse{
		Messages:  messagesToProto(resp.Messages),
		ChatState: resp.ChatState,
	}
}

func (mw *Middleware) ChatGetMessagesByIds(cctx context.Context, req *pb.RpcChatGetMessagesByIdsRequest) *pb.RpcChatGetMessagesByIdsResponse {
	chatService := mustService[chats.Service](mw)

	messages, err := chatService.GetMessagesByIds(cctx, req.ChatObjectId, req.MessageIds)
	if err != nil {
		code := mapErrorCode[pb.RpcChatGetMessagesByIdsResponseErrorCode](err)
		return &pb.RpcChatGetMessagesByIdsResponse{
			Error: &pb.RpcChatGetMessagesByIdsResponseError{
				Code:        code,
				Description: getErrorDescription(err),
			},
		}
	}
	return &pb.RpcChatGetMessagesByIdsResponse{
		Messages: messagesToProto(messages),
	}
}

func (mw *Middleware) ChatSubscribeLastMessages(cctx context.Context, req *pb.RpcChatSubscribeLastMessagesRequest) *pb.RpcChatSubscribeLastMessagesResponse {
	chatService := mustService[chats.Service](mw)

	resp, err := chatService.SubscribeLastMessages(cctx, req.ChatObjectId, int(req.Limit), req.SubId)
	if err != nil {
		code := mapErrorCode[pb.RpcChatSubscribeLastMessagesResponseErrorCode](err)
		return &pb.RpcChatSubscribeLastMessagesResponse{
			Error: &pb.RpcChatSubscribeLastMessagesResponseError{
				Code:        code,
				Description: getErrorDescription(err),
			},
		}
	}
	return &pb.RpcChatSubscribeLastMessagesResponse{
		Messages:          messagesToProto(resp.Messages),
		NumMessagesBefore: 0,
		ChatState:         resp.ChatState,
	}
}

func (mw *Middleware) ChatUnsubscribe(cctx context.Context, req *pb.RpcChatUnsubscribeRequest) *pb.RpcChatUnsubscribeResponse {
	chatService := mustService[chats.Service](mw)

	err := chatService.Unsubscribe(req.ChatObjectId, req.SubId)
	if err != nil {
		code := mapErrorCode[pb.RpcChatUnsubscribeResponseErrorCode](err)
		return &pb.RpcChatUnsubscribeResponse{
			Error: &pb.RpcChatUnsubscribeResponseError{
				Code:        code,
				Description: getErrorDescription(err),
			},
		}
	}
	return &pb.RpcChatUnsubscribeResponse{}
}

func (mw *Middleware) ChatSubscribeToMessagePreviews(cctx context.Context, req *pb.RpcChatSubscribeToMessagePreviewsRequest) *pb.RpcChatSubscribeToMessagePreviewsResponse {
	chatService := mustService[chats.Service](mw)

	subId, err := chatService.SubscribeToMessagePreviews(cctx)
	if err != nil {
		code := mapErrorCode[pb.RpcChatSubscribeToMessagePreviewsResponseErrorCode](err)
		return &pb.RpcChatSubscribeToMessagePreviewsResponse{
			Error: &pb.RpcChatSubscribeToMessagePreviewsResponseError{
				Code:        code,
				Description: getErrorDescription(err),
			},
		}
	}
	return &pb.RpcChatSubscribeToMessagePreviewsResponse{
		SubId: subId,
	}
}

func (mw *Middleware) ChatUnsubscribeFromMessagePreviews(cctx context.Context, req *pb.RpcChatUnsubscribeFromMessagePreviewsRequest) *pb.RpcChatUnsubscribeFromMessagePreviewsResponse {
	chatService := mustService[chats.Service](mw)

	err := chatService.UnsubscribeFromMessagePreviews()
	if err != nil {
		code := mapErrorCode[pb.RpcChatUnsubscribeFromMessagePreviewsResponseErrorCode](err)
		return &pb.RpcChatUnsubscribeFromMessagePreviewsResponse{
			Error: &pb.RpcChatUnsubscribeFromMessagePreviewsResponseError{
				Code:        code,
				Description: getErrorDescription(err),
			},
		}
	}
	return &pb.RpcChatUnsubscribeFromMessagePreviewsResponse{}
}

func (mw *Middleware) ChatReadMessages(cctx context.Context, request *pb.RpcChatReadMessagesRequest) *pb.RpcChatReadMessagesResponse {
	chatService := mustService[chats.Service](mw)
	err := chatService.ReadMessages(cctx, chats.ReadMessagesRequest{
		ChatObjectId:  request.ChatObjectId,
		AfterOrderId:  request.AfterOrderId,
		BeforeOrderId: request.BeforeOrderId,
		LastStateId:   request.LastStateId,
		CounterType:   chatobject.CounterType(request.Type),
	})
	if err != nil {
		code := mapErrorCode(err,
			errToCode(anystore.ErrDocNotFound, pb.RpcChatReadMessagesResponseError_MESSAGES_NOT_FOUND),
		)
		return &pb.RpcChatReadMessagesResponse{
			Error: &pb.RpcChatReadMessagesResponseError{
				Code:        code,
				Description: getErrorDescription(err),
			},
		}
	}
	return &pb.RpcChatReadMessagesResponse{}
}

func (mw *Middleware) ChatUnreadMessages(cctx context.Context, request *pb.RpcChatUnreadRequest) *pb.RpcChatUnreadResponse {
	chatService := mustService[chats.Service](mw)
	err := chatService.UnreadMessages(cctx, request.ChatObjectId, request.AfterOrderId, chatobject.CounterType(request.Type))
	if err != nil {
		code := mapErrorCode[pb.RpcChatUnreadResponseErrorCode](err)
		return &pb.RpcChatUnreadResponse{
			Error: &pb.RpcChatUnreadResponseError{
				Code:        code,
				Description: getErrorDescription(err),
			},
		}
	}
	return &pb.RpcChatUnreadResponse{}
}

func messagesToProto(msgs []*chatobject.Message) []*model.ChatMessage {
	res := make([]*model.ChatMessage, 0, len(msgs))
	for _, msg := range msgs {
		res = append(res, msg.ChatMessage)
	}
	return res
}
