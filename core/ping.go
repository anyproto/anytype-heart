package core

import (
	"context"
	"fmt"
	"time"

	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"
)

func nsToMs(n int64) int64 {
	n = n / 1000
	return (n - int64(n/1000000)*1000000)
}

func (mw *Middleware) DebugPing(cctx context.Context, req *pb.RpcDebugPingRequest) *pb.RpcDebugPingResponse {
	n := time.Now()
	fmt.Printf("%d.%d go got ping req\n", n.Unix(), nsToMs(n.UnixNano()))

	response := func(index int32, code pb.RpcDebugPingResponseErrorCode, err error) *pb.RpcDebugPingResponse {
		m := &pb.RpcDebugPingResponse{Index: index, Error: &pb.RpcDebugPingResponseError{Code: code}}
		if err != nil {
			m.Error.Description = getErrorDescription(err)
		}

		n = time.Now()
		fmt.Printf("%d.%d go send ping resp\n", n.Unix(), nsToMs(n.UnixNano()))

		return m
	}

	for i := 0; i < int(req.NumberOfEventsToSend); i++ {
		n = time.Now()
		fmt.Printf("%d.%d go send ping event %d\n", n.Unix(), nsToMs(n.UnixNano()), i)

		mw.applicationService.GetEventSender().Broadcast(event.NewEventSingleMessage("", &pb.EventMessageValueOfPing{
			Ping: &pb.EventPing{Index: int32(i)},
		}))
	}

	return response(req.Index, pb.RpcDebugPingResponseError_NULL, nil)
}
