package core

import (
	"github.com/anytypeio/go-anytype-middleware/pb"
)

func (mw *Middleware) BlockUndo(req *pb.RpcBlockUndoRequest) *pb.RpcBlockUndoResponse {
	response := func(code pb.RpcBlockUndoResponseErrorCode, err error) *pb.RpcBlockUndoResponse {
		m := &pb.RpcBlockUndoResponse{Error: &pb.RpcBlockUndoResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	// TODO

	return response(pb.RpcBlockUndoResponseError_NULL, nil)
}

func (mw *Middleware) BlockRedo(req *pb.RpcBlockRedoRequest) *pb.RpcBlockRedoResponse {
	response := func(code pb.RpcBlockRedoResponseErrorCode, err error) *pb.RpcBlockRedoResponse {
		m := &pb.RpcBlockRedoResponse{Error: &pb.RpcBlockRedoResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	// TODO

	return response(pb.RpcBlockRedoResponseError_NULL, nil)
}
