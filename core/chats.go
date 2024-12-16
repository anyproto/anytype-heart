package core

import (
	"context"

	"github.com/anyproto/anytype-heart/core/block/chats"
	"github.com/anyproto/anytype-heart/core/block/editor/chatobject"
	"github.com/anyproto/anytype-heart/pb"
)

func (mw *Middleware) ChatAddMessage(cctx context.Context, req *pb.RpcChatAddMessageRequest) *pb.RpcChatAddMessageResponse {
	ctx := mw.newContext(cctx)
	chatService := mustService[chats.Service](mw)

	messageId, err := chatService.AddMessage(cctx, ctx, req.ChatObjectId, req.Message)
	code := mapErrorCode[pb.RpcChatAddMessageResponseErrorCode](err)
	return &pb.RpcChatAddMessageResponse{
		MessageId: messageId,
		Event:     ctx.GetResponseEvent(),
		Error: &pb.RpcChatAddMessageResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) ChatEditMessageContent(cctx context.Context, req *pb.RpcChatEditMessageContentRequest) *pb.RpcChatEditMessageContentResponse {
	chatService := mustService[chats.Service](mw)

	err := chatService.EditMessage(cctx, req.ChatObjectId, req.MessageId, req.EditedMessage)
	code := mapErrorCode[pb.RpcChatEditMessageContentResponseErrorCode](err)
	return &pb.RpcChatEditMessageContentResponse{
		Error: &pb.RpcChatEditMessageContentResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) ChatToggleMessageReaction(cctx context.Context, req *pb.RpcChatToggleMessageReactionRequest) *pb.RpcChatToggleMessageReactionResponse {
	chatService := mustService[chats.Service](mw)

	err := chatService.ToggleMessageReaction(cctx, req.ChatObjectId, req.MessageId, req.Emoji)
	code := mapErrorCode[pb.RpcChatToggleMessageReactionResponseErrorCode](err)
	return &pb.RpcChatToggleMessageReactionResponse{
		Error: &pb.RpcChatToggleMessageReactionResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) ChatDeleteMessage(cctx context.Context, req *pb.RpcChatDeleteMessageRequest) *pb.RpcChatDeleteMessageResponse {
	chatService := mustService[chats.Service](mw)

	err := chatService.DeleteMessage(cctx, req.ChatObjectId, req.MessageId)
	code := mapErrorCode[pb.RpcChatDeleteMessageResponseErrorCode](err)
	return &pb.RpcChatDeleteMessageResponse{
		Error: &pb.RpcChatDeleteMessageResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) ChatGetMessages(cctx context.Context, req *pb.RpcChatGetMessagesRequest) *pb.RpcChatGetMessagesResponse {
	chatService := mustService[chats.Service](mw)

	messages, err := chatService.GetMessages(cctx, req.ChatObjectId, chatobject.GetMessagesRequest{
		AfterOrderId:  req.AfterOrderId,
		BeforeOrderId: req.BeforeOrderId,
		Limit:         int(req.Limit),
	})
	code := mapErrorCode[pb.RpcChatGetMessagesResponseErrorCode](err)
	return &pb.RpcChatGetMessagesResponse{
		Messages: messages,
		Error: &pb.RpcChatGetMessagesResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) ChatGetMessagesByIds(cctx context.Context, req *pb.RpcChatGetMessagesByIdsRequest) *pb.RpcChatGetMessagesByIdsResponse {
	chatService := mustService[chats.Service](mw)

	messages, err := chatService.GetMessagesByIds(cctx, req.ChatObjectId, req.MessageIds)
	code := mapErrorCode[pb.RpcChatGetMessagesByIdsResponseErrorCode](err)
	return &pb.RpcChatGetMessagesByIdsResponse{
		Messages: messages,
		Error: &pb.RpcChatGetMessagesByIdsResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) ChatSubscribeLastMessages(cctx context.Context, req *pb.RpcChatSubscribeLastMessagesRequest) *pb.RpcChatSubscribeLastMessagesResponse {
	chatService := mustService[chats.Service](mw)

	messages, numBefore, err := chatService.SubscribeLastMessages(cctx, req.ChatObjectId, int(req.Limit))
	code := mapErrorCode[pb.RpcChatSubscribeLastMessagesResponseErrorCode](err)
	return &pb.RpcChatSubscribeLastMessagesResponse{
		Messages:          messages,
		NumMessagesBefore: int32(numBefore),
		Error: &pb.RpcChatSubscribeLastMessagesResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) ChatUnsubscribe(cctx context.Context, req *pb.RpcChatUnsubscribeRequest) *pb.RpcChatUnsubscribeResponse {
	chatService := mustService[chats.Service](mw)

	err := chatService.Unsubscribe(req.ChatObjectId)
	code := mapErrorCode[pb.RpcChatUnsubscribeResponseErrorCode](err)
	return &pb.RpcChatUnsubscribeResponse{
		Error: &pb.RpcChatUnsubscribeResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}
