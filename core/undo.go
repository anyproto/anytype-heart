package core

import (
	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/undo"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

func (mw *Middleware) ObjectUndo(req *pb.RpcObjectUndoRequest) *pb.RpcObjectUndoResponse {
	ctx := state.NewContext(nil)
	var (
		counters pb.RpcObjectUndoRedoCounter
		err      error
	)
	response := func(code pb.RpcObjectUndoResponseErrorCode, err error) *pb.RpcObjectUndoResponse {
		m := &pb.RpcObjectUndoResponse{Error: &pb.RpcObjectUndoResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
			m.Counters = &counters
		}
		return m
	}
	err = mw.doBlockService(func(bs block.Service) error {
		counters, err = bs.Undo(ctx, *req)
		return err
	})
	if err != nil {
		if err == undo.ErrNoHistory {
			return response(pb.RpcObjectUndoResponseError_CAN_NOT_MOVE, err)
		}
		return response(pb.RpcObjectUndoResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcObjectUndoResponseError_NULL, nil)
}

func (mw *Middleware) ObjectRedo(req *pb.RpcObjectRedoRequest) *pb.RpcObjectRedoResponse {
	ctx := state.NewContext(nil)
	var (
		counters pb.RpcObjectUndoRedoCounter
		err      error
	)
	response := func(code pb.RpcObjectRedoResponseErrorCode, err error) *pb.RpcObjectRedoResponse {
		m := &pb.RpcObjectRedoResponse{Error: &pb.RpcObjectRedoResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
			m.Counters = &counters
		}
		return m
	}

	err = mw.doBlockService(func(bs block.Service) error {
		counters, err = bs.Redo(ctx, *req)
		return err
	})
	if err != nil {
		if err == undo.ErrNoHistory {
			return response(pb.RpcObjectRedoResponseError_CAN_NOT_MOVE, err)
		}
		return response(pb.RpcObjectRedoResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcObjectRedoResponseError_NULL, nil)
}
