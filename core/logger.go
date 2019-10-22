package core

import (
	"github.com/anytypeio/go-anytype-middleware/pb"
)

func (mw *Middleware) LogSend(req *pb.LogSendRequest) *pb.LogSendResponse {
	response := func(code pb.LogSendResponse_Error_Code, err error) *pb.LogSendResponse {
		m := &pb.LogSendResponse{Error: &pb.LogSendResponse_Error{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	switch req.Level {
	case pb.LogSendRequest_FATAL:
		log.Fatal(req.Message)
	case pb.LogSendRequest_PANIC:
		log.Panic(req.Message)
	case pb.LogSendRequest_DEBUG:
		log.Debug(req.Message)
	case pb.LogSendRequest_INFO:
		log.Info(req.Message)
	case pb.LogSendRequest_WARNING:
		log.Warning(req.Message)
	case pb.LogSendRequest_ERROR:
		log.Error(req.Message)
	}

	return response(pb.LogSendResponse_Error_NULL, nil)
}
