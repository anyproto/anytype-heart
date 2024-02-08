package core

import (
	"context"

	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"
)

func (mw *Middleware) ExtensionBroadcast(cctx context.Context, req *pb.RpcExtensionBroadcastRequest) *pb.RpcExtensionBroadcastResponse {
	messages := []*pb.EventMessage{
		{
			Value: &pb.EventMessageValueOfExtensionBroadcast{
				ExtensionBroadcast: &pb.EventExtensionBroadcast{Payload: req.Payload},
			},
		},
	}
	getService[event.Sender](mw).Broadcast(&pb.Event{
		Messages: messages,
	})
	return &pb.RpcExtensionBroadcastResponse{
		Event: &pb.ResponseEvent{
			Messages: messages,
		},
		Error: &pb.RpcExtensionBroadcastResponseError{
			Code: pb.RpcExtensionBroadcastResponseError_NULL,
		},
	}
}
