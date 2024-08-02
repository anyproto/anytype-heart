package core

import (
	"context"

	"github.com/anyproto/anytype-heart/core/block/chats"
	"github.com/anyproto/anytype-heart/pb"
)

func (mw *Middleware) ChatAddMessage(cctx context.Context, req *pb.RpcChatAddMessageRequest) *pb.RpcChatAddMessageResponse {
	chatService := getService[chats.Service](mw)

	err := chatService.AddMessage(req.ChatObjectId, req.Message)
	code := mapErrorCode[pb.RpcChatAddMessageResponseErrorCode](err)
	return &pb.RpcChatAddMessageResponse{
		Error: &pb.RpcChatAddMessageResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}
