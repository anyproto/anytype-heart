package core

import (
	"context"

	"github.com/anyproto/anytype-heart/core/block/chats"
	"github.com/anyproto/anytype-heart/pb"
)

func (mw *Middleware) ChatAddMessage(cctx context.Context, req *pb.RpcChatAddMessageRequest) *pb.RpcChatAddMessageResponse {
	chatService := getService[chats.Service](mw)

	messageId, err := chatService.AddMessage(cctx, req.ChatObjectId, req.Message)
	code := mapErrorCode[pb.RpcChatAddMessageResponseErrorCode](err)
	return &pb.RpcChatAddMessageResponse{
		MessageId: messageId,
		Error: &pb.RpcChatAddMessageResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) ChatEditMessage(cctx context.Context, req *pb.RpcChatEditMessageRequest) *pb.RpcChatEditMessageResponse {
	chatService := getService[chats.Service](mw)

	err := chatService.EditMessage(cctx, req.ChatObjectId, req.MessageId, req.EditedMessage)
	code := mapErrorCode[pb.RpcChatEditMessageResponseErrorCode](err)
	return &pb.RpcChatEditMessageResponse{
		Error: &pb.RpcChatEditMessageResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) ChatDeleteMessage(cctx context.Context, req *pb.RpcChatDeleteMessageRequest) *pb.RpcChatDeleteMessageResponse {
	chatService := getService[chats.Service](mw)

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
	chatService := getService[chats.Service](mw)

	messages, err := chatService.GetMessages(cctx, req.ChatObjectId, req.BeforeOrderId, int(req.Limit))
	code := mapErrorCode[pb.RpcChatGetMessagesResponseErrorCode](err)
	return &pb.RpcChatGetMessagesResponse{
		Messages: messages,
		Error: &pb.RpcChatGetMessagesResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) ChatSubscribeLastMessages(cctx context.Context, req *pb.RpcChatSubscribeLastMessagesRequest) *pb.RpcChatSubscribeLastMessagesResponse {
	chatService := getService[chats.Service](mw)

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
	chatService := getService[chats.Service](mw)

	err := chatService.Unsubscribe(req.ChatObjectId)
	code := mapErrorCode[pb.RpcChatUnsubscribeResponseErrorCode](err)
	return &pb.RpcChatUnsubscribeResponse{
		Error: &pb.RpcChatUnsubscribeResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}
