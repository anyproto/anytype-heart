package core

import (
	"github.com/anytypeio/go-anytype-middleware/pb"
)

func (mw *Middleware) BlockHistoryMove(req *pb.RpcBlockHistoryMoveRequest) *pb.RpcBlockHistoryMoveResponse {
	response := func(code pb.RpcBlockHistoryMoveResponseErrorCode, err error) *pb.RpcBlockHistoryMoveResponse {
		m := &pb.RpcBlockHistoryMoveResponse{Error: &pb.RpcBlockHistoryMoveResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	// TODO

	return response(pb.RpcBlockHistoryMoveResponseError_NULL, nil)
}
