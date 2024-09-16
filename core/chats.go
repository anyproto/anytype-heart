package core

import (
	"context"

	"github.com/anyproto/anytype-heart/pb"
)

// TODO: chats are temporary done as dummy API for clients to merge the API

func (mw *Middleware) ObjectChatAdd(ctx context.Context, request *pb.RpcObjectChatAddRequest) *pb.RpcObjectChatAddResponse {
	// TODO implement me
	return &pb.RpcObjectChatAddResponse{
		Error: &pb.RpcObjectChatAddResponseError{
			Code:        pb.RpcObjectChatAddResponseError_UNKNOWN_ERROR,
			Description: "not implemented",
		},
	}
}

func (mw *Middleware) ChatAddMessage(ctx context.Context, request *pb.RpcChatAddMessageRequest) *pb.RpcChatAddMessageResponse {
	// TODO implement me
	return &pb.RpcChatAddMessageResponse{
		Error: &pb.RpcChatAddMessageResponseError{
			Code:        pb.RpcChatAddMessageResponseError_UNKNOWN_ERROR,
			Description: "not implemented",
		},
	}
}

func (mw *Middleware) ChatEditMessageContent(ctx context.Context, request *pb.RpcChatEditMessageContentRequest) *pb.RpcChatEditMessageContentResponse {
	// TODO implement me
	return &pb.RpcChatEditMessageContentResponse{
		Error: &pb.RpcChatEditMessageContentResponseError{
			Code:        pb.RpcChatEditMessageContentResponseError_UNKNOWN_ERROR,
			Description: "not implemented",
		},
	}
}

func (mw *Middleware) ChatToggleMessageReaction(ctx context.Context, request *pb.RpcChatToggleMessageReactionRequest) *pb.RpcChatToggleMessageReactionResponse {
	// TODO implement me
	return &pb.RpcChatToggleMessageReactionResponse{
		Error: &pb.RpcChatToggleMessageReactionResponseError{
			Code:        pb.RpcChatToggleMessageReactionResponseError_UNKNOWN_ERROR,
			Description: "not implemented",
		},
	}
}

func (mw *Middleware) ChatDeleteMessage(ctx context.Context, request *pb.RpcChatDeleteMessageRequest) *pb.RpcChatDeleteMessageResponse {
	// TODO implement me
	return &pb.RpcChatDeleteMessageResponse{
		Error: &pb.RpcChatDeleteMessageResponseError{
			Code:        pb.RpcChatDeleteMessageResponseError_UNKNOWN_ERROR,
			Description: "not implemented",
		},
	}
}

func (mw *Middleware) ChatGetMessages(ctx context.Context, request *pb.RpcChatGetMessagesRequest) *pb.RpcChatGetMessagesResponse {
	// TODO implement me
	return &pb.RpcChatGetMessagesResponse{
		Error: &pb.RpcChatGetMessagesResponseError{
			Code:        pb.RpcChatGetMessagesResponseError_UNKNOWN_ERROR,
			Description: "not implemented",
		},
	}
}

func (mw *Middleware) ChatGetMessagesByIds(ctx context.Context, request *pb.RpcChatGetMessagesByIdsRequest) *pb.RpcChatGetMessagesByIdsResponse {
	// TODO implement me
	return &pb.RpcChatGetMessagesByIdsResponse{
		Error: &pb.RpcChatGetMessagesByIdsResponseError{
			Code:        pb.RpcChatGetMessagesByIdsResponseError_UNKNOWN_ERROR,
			Description: "not implemented",
		},
	}
}

func (mw *Middleware) ChatSubscribeLastMessages(ctx context.Context, request *pb.RpcChatSubscribeLastMessagesRequest) *pb.RpcChatSubscribeLastMessagesResponse {
	// TODO implement me
	return &pb.RpcChatSubscribeLastMessagesResponse{
		Error: &pb.RpcChatSubscribeLastMessagesResponseError{
			Code:        pb.RpcChatSubscribeLastMessagesResponseError_UNKNOWN_ERROR,
			Description: "not implemented",
		},
	}
}

func (mw *Middleware) ChatUnsubscribe(ctx context.Context, request *pb.RpcChatUnsubscribeRequest) *pb.RpcChatUnsubscribeResponse {
	// TODO implement me
	return &pb.RpcChatUnsubscribeResponse{
		Error: &pb.RpcChatUnsubscribeResponseError{
			Code:        pb.RpcChatUnsubscribeResponseError_UNKNOWN_ERROR,
			Description: "not implemented",
		},
	}
}
