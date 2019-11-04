package core

import (
	"github.com/anytypeio/go-anytype-middleware/pb"
)

func (mw *Middleware) BlockHistoryMove(req *pb.Rpc_Block_History_Move_Request) *pb.Rpc_Block_History_Move_Response {
	response := func(code pb.Rpc_Block_History_Move_Response_Error_Code, err error) *pb.Rpc_Block_History_Move_Response {
		m := &pb.Rpc_Block_History_Move_Response{Error: &pb.Rpc_Block_History_Move_Response_Error{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	// TODO

	return response(pb.Rpc_Block_History_Move_Response_Error_NULL, nil)
}
