package core

import (
	"github.com/anytypeio/go-anytype-middleware/pb"
)

func (mw *Middleware) LogSend(req *pb.RpcLogSendRequest) *pb.RpcLogSendResponse {
	response := func(code pb.RpcLogSendResponseErrorCode, err error) *pb.RpcLogSendResponse {
		m := &pb.RpcLogSendResponse{Error: &pb.RpcLogSendResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	switch req.Level {
	case pb.RpcLogSendRequest_FATAL:
		log.Fatal(req.Message)
	case pb.RpcLogSendRequest_PANIC:
		log.Panic(req.Message)
	case pb.RpcLogSendRequest_DEBUG:
		log.Debug(req.Message)
	case pb.RpcLogSendRequest_INFO:
		log.Info(req.Message)
	case pb.RpcLogSendRequest_WARNING:
		log.Warn(req.Message)
	case pb.RpcLogSendRequest_ERROR:
		log.Error(req.Message)
	}

	return response(pb.RpcLogSendResponseError_NULL, nil)
}
