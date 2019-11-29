package core

import (
	"github.com/anytypeio/go-anytype-middleware/pb"
)

func (mw *Middleware) BlockCopy(req *pb.RpcBlockCopyRequest) *pb.RpcBlockCopyResponse {
	response := func(code pb.RpcBlockCopyResponseErrorCode, err error) *pb.RpcBlockCopyResponse {
		m := &pb.RpcBlockCopyResponse{Error: &pb.RpcBlockCopyResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}
	// TODO
	return response(pb.RpcBlockCopyResponseError_NULL, nil)
}

func (mw *Middleware) BlockPaste(req *pb.RpcBlockPasteRequest) *pb.RpcBlockPasteResponse {
	response := func(code pb.RpcBlockPasteResponseErrorCode, err error) *pb.RpcBlockPasteResponse {
		m := &pb.RpcBlockPasteResponse{Error: &pb.RpcBlockPasteResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}
	// TODO
	return response(pb.RpcBlockPasteResponseError_NULL, nil)
}
