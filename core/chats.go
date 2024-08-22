package core

import (
	"context"

	"github.com/anyproto/anytype-heart/core/block/chats"
	"github.com/anyproto/anytype-heart/pb"
)

func (mw *Middleware) ChatAddMessage(cctx context.Context, req *pb.RpcChatAddMessageRequest) *pb.RpcChatAddMessageResponse {
	chatService := getService[chats.Service](mw)

	messageId, err := chatService.AddMessage(req.ChatObjectId, req.Message)
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

	err := chatService.EditMessage(req.ChatObjectId, req.MessageId, req.EditedMessage)
	code := mapErrorCode[pb.RpcChatEditMessageResponseErrorCode](err)
	return &pb.RpcChatEditMessageResponse{
		Error: &pb.RpcChatEditMessageResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) ChatGetMessages(cctx context.Context, req *pb.RpcChatGetMessagesRequest) *pb.RpcChatGetMessagesResponse {
	chatService := getService[chats.Service](mw)

	messages, err := chatService.GetMessages(req.ChatObjectId)
	code := mapErrorCode[pb.RpcChatGetMessagesResponseErrorCode](err)
	return &pb.RpcChatGetMessagesResponse{
		Messages: messages,
		Error: &pb.RpcChatGetMessagesResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}
