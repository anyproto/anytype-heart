package core

import (
	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/history"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

func (mw *Middleware) BlockUndo(req *pb.RpcBlockUndoRequest) *pb.RpcBlockUndoResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockUndoResponseErrorCode, err error) *pb.RpcBlockUndoResponse {
		m := &pb.RpcBlockUndoResponse{Error: &pb.RpcBlockUndoResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}

	err := mw.doBlockService(func(bs block.Service) error {
		return bs.Undo(ctx, *req)
	})
	if err != nil {
		if err == history.ErrNoHistory {
			return response(pb.RpcBlockUndoResponseError_CAN_NOT_MOVE, err)
		}
		return response(pb.RpcBlockUndoResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcBlockUndoResponseError_NULL, nil)
}

func (mw *Middleware) BlockRedo(req *pb.RpcBlockRedoRequest) *pb.RpcBlockRedoResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockRedoResponseErrorCode, err error) *pb.RpcBlockRedoResponse {
		m := &pb.RpcBlockRedoResponse{Error: &pb.RpcBlockRedoResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}

	err := mw.doBlockService(func(bs block.Service) error {
		return bs.Redo(ctx, *req)
	})
	if err != nil {
		if err == history.ErrNoHistory {
			return response(pb.RpcBlockRedoResponseError_CAN_NOT_MOVE, err)
		}
		return response(pb.RpcBlockRedoResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcBlockRedoResponseError_NULL, nil)
}
