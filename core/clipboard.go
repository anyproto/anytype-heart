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

	mw.blockService.PasteAnySlot(*req)
	// 	mw.blockService.PasteAnySlot(req.ContextId, req.FocusedBlockId, req.SelectedTextRange, req.SelectedBlocks, req.AnySlot)

	/*
		ContextId         string
		FocusedBlockId    string
		SelectedTextRange *model.Range
		SelectedBlocks    []string
		TextSlot          string
		HtmlSlot          string
		AnySlot           []string
	*/

	// IGNORE HtmlSlot
	// IGNORE TextSlot

	// if len(AnySlot) == 0 {
	// 	// NOTHING TO DO
	// 	return;
	// }

	// if len(FocusedBlockId) > 0 &&

	return response(pb.RpcBlockPasteResponseError_NULL, nil)
}
