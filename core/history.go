package core

import (
	"github.com/anytypeio/go-anytype-middleware/pb"
)

func (mw *Middleware) BlockHistoryMove(req *pb.BlockHistoryMoveRequest) *pb.BlockHistoryMoveResponse {
	response := func(code pb.BlockHistoryMoveResponse_Error_Code, err error) *pb.BlockHistoryMoveResponse {
		m := &pb.BlockHistoryMoveResponse{Error: &pb.BlockHistoryMoveResponse_Error{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	// TODO

	return response(pb.BlockHistoryMoveResponse_Error_NULL, nil)
}
