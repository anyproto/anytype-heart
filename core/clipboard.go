package core

import (
	"github.com/anytypeio/go-anytype-middleware/pb"
)

/*
	message Copy {
	    message Request {
	        string contextId = 1;
	        string focusedBlockId = 2;
	        anytype.model.Range selectedTextRange = 3;
	        repeated string selectedBlocks = 4;
	    }

	    message Response {
	        Error error = 1;
	        string clipboardText = 2;
	        string clipboardHtml = 3;
	        string clipboardAny = 4; // TODO: type – is string ok?

	message Paste {
	    message Request {
	        string contextId = 1;
	        string focusedBlockId = 2;
	        anytype.model.Range selectedTextRange = 3;
	        repeated string selectedBlocks = 4;

	        string clipboardText = 5;
	        string clipboardHtml = 6;
	        string clipboardAny = 7;
	    }

	    message Response {
	        Error error = 1;
*/

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
