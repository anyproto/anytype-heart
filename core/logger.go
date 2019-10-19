package core

import (
	"github.com/anytypeio/go-anytype-middleware/pb"
)

func (mw *Middleware) Log(req *pb.LogRequest) *pb.LogResponse {
	response := func(code pb.LogResponse_Error_Code, err error) *pb.LogResponse {
		m := &pb.LogResponse{Error: &pb.LogResponse_Error{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	switch req.Level {
	case pb.LogRequest_FATAL:
		log.Fatal(req.Message)
	case pb.LogRequest_PANIC:
		log.Panic(req.Message)
	case pb.LogRequest_DEBUG:
		log.Debug(req.Message)
	case pb.LogRequest_INFO:
		log.Info(req.Message)
	case pb.LogRequest_WARNING:
		log.Warning(req.Message)
	case pb.LogRequest_ERROR:
		log.Error(req.Message)
	}

	return response(pb.LogResponse_Error_NULL, nil)
}
