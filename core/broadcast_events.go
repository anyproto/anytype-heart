package core

import (
	"context"

	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"
)

func (mw *Middleware) BroadcastPayloadEvent(cctx context.Context, req *pb.RpcBroadcastPayloadEventRequest) *pb.RpcBroadcastPayloadEventResponse {
	messages := []*pb.EventMessage{
		event.NewMessage("", &pb.EventMessageValueOfPayloadBroadcast{
			PayloadBroadcast: &pb.EventPayloadBroadcast{Payload: req.Payload},
		}),
	}
	mustService[event.Sender](mw).Broadcast(&pb.Event{
		Messages: messages,
	})
	return &pb.RpcBroadcastPayloadEventResponse{
		Event: &pb.ResponseEvent{
			Messages: messages,
		},
		Error: &pb.RpcBroadcastPayloadEventResponseError{
			Code: pb.RpcBroadcastPayloadEventResponseError_NULL,
		},
	}
}
