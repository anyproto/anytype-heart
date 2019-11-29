package core

import (
	"fmt"
	"time"

	"github.com/anytypeio/go-anytype-middleware/pb"
)

func nsToMs(n int64) int64 {
	n = n / 1000
	return (n - int64(n/1000000)*1000000)
}

func (mw *Middleware) Ping(req *pb.RpcPingRequest) *pb.RpcPingResponse {
	n := time.Now()
	fmt.Printf("%d.%d go got ping req\n", n.Unix(), nsToMs(n.UnixNano()))

	response := func(index int32, code pb.RpcPingResponseErrorCode, err error) *pb.RpcPingResponse {
		m := &pb.RpcPingResponse{Index: index, Error: &pb.RpcPingResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		n = time.Now()
		fmt.Printf("%d.%d go send ping resp\n", n.Unix(), nsToMs(n.UnixNano()))

		return m
	}

	for i := 0; i < int(req.NumberOfEventsToSend); i++ {
		n = time.Now()
		fmt.Printf("%d.%d go send ping event %d\n", n.Unix(), nsToMs(n.UnixNano()), i)

		mw.SendEvent(&pb.Event{
			Messages: []*pb.EventMessage{
				&pb.EventMessage{
					Value: &pb.EventMessageValueOfPing{
						Ping: &pb.EventPing{Index: int32(i)},
					},
				}},
		})
	}

	return response(req.Index, pb.RpcPingResponseError_NULL, nil)
}
