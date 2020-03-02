package core

import (
	"github.com/anytypeio/go-anytype-middleware/pb"
)

func (mw *Middleware) ProcessCancel(req *pb.RpcProcessCancelRequest) *pb.RpcProcessCancelResponse {
	response := func(code pb.RpcProcessCancelResponseErrorCode, err error) *pb.RpcProcessCancelResponse {
		m := &pb.RpcProcessCancelResponse{Error: &pb.RpcProcessCancelResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}
	if err := mw.blockService.ProcessCancel(req.Id); err != nil {
		return response(pb.RpcProcessCancelResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcProcessCancelResponseError_NULL, nil)
}
