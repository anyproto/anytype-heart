package core

import (
	"context"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/undo"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func (mw *Middleware) ObjectUndo(cctx context.Context, req *pb.RpcObjectUndoRequest) *pb.RpcObjectUndoResponse {
	ctx := mw.newContext(cctx)
	var (
		counters     pb.RpcObjectUndoRedoCounter
		carriageInfo undo.CarriageInfo
		err          error
	)
	response := func(code pb.RpcObjectUndoResponseErrorCode, err error) *pb.RpcObjectUndoResponse {
		m := &pb.RpcObjectUndoResponse{Error: &pb.RpcObjectUndoResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
			m.Counters = &counters
		}
		m.Range = &model.Range{
			From: carriageInfo.RangeFrom,
			To:   carriageInfo.RangeTo,
		}
		m.BlockId = carriageInfo.CarriageBlockID
		return m
	}
	err = mw.doBlockService(func(bs *block.Service) error {
		counters, carriageInfo, err = bs.Undo(ctx, *req)
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

func (mw *Middleware) ObjectRedo(cctx context.Context, req *pb.RpcObjectRedoRequest) *pb.RpcObjectRedoResponse {
	ctx := mw.newContext(cctx)
	var (
		counters     pb.RpcObjectUndoRedoCounter
		carriageInfo undo.CarriageInfo
		err          error
	)
	response := func(code pb.RpcObjectRedoResponseErrorCode, err error) *pb.RpcObjectRedoResponse {
		m := &pb.RpcObjectRedoResponse{Error: &pb.RpcObjectRedoResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
			m.Counters = &counters
			m.Range = &model.Range{
				From: carriageInfo.RangeFrom,
				To:   carriageInfo.RangeTo,
			}
			m.BlockId = carriageInfo.CarriageBlockID
		}
		return m
	}

	err = mw.doBlockService(func(bs *block.Service) error {
		counters, carriageInfo, err = bs.Redo(ctx, *req)
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
