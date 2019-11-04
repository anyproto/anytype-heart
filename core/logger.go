package core

import (
	"github.com/anytypeio/go-anytype-middleware/pb"
)

func (mw *Middleware) LogSend(req *pb.Rpc_Log_Send_Request) *pb.Rpc_Log_Send_Response {
	response := func(code pb.Rpc_Log_Send_Response_Error_Code, err error) *pb.Rpc_Log_Send_Response {
		m := &pb.Rpc_Log_Send_Response{Error: &pb.Rpc_Log_Send_Response_Error{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	switch req.Level {
	case pb.Rpc_Log_Send_Request_FATAL:
		log.Fatal(req.Message)
	case pb.Rpc_Log_Send_Request_PANIC:
		log.Panic(req.Message)
	case pb.Rpc_Log_Send_Request_DEBUG:
		log.Debug(req.Message)
	case pb.Rpc_Log_Send_Request_INFO:
		log.Info(req.Message)
	case pb.Rpc_Log_Send_Request_WARNING:
		log.Warning(req.Message)
	case pb.Rpc_Log_Send_Request_ERROR:
		log.Error(req.Message)
	}

	return response(pb.Rpc_Log_Send_Response_Error_NULL, nil)
}
