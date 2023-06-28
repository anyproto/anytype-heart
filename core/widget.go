package core

import (
	"context"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/pb"
)

func (mw *Middleware) BlockCreateWidget(cctx context.Context, req *pb.RpcBlockCreateWidgetRequest) *pb.RpcBlockCreateWidgetResponse {
	ctx := mw.newContext(cctx)
	response := func(code pb.RpcBlockCreateWidgetResponseErrorCode, id string, err error) *pb.RpcBlockCreateWidgetResponse {
		m := &pb.RpcBlockCreateWidgetResponse{Error: &pb.RpcBlockCreateWidgetResponseError{Code: code}, BlockId: id}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = mw.getResponseEvent(ctx)
		}
		return m
	}
	var id string
	err := mw.doBlockService(func(bs *block.Service) (err error) {
		id, err = bs.CreateWidgetBlock(ctx, req)
		return err
	})
	if err != nil {
		return response(pb.RpcBlockCreateWidgetResponseError_UNKNOWN_ERROR, "", err)
	}
	return response(pb.RpcBlockCreateWidgetResponseError_NULL, id, nil)
}

func (mw *Middleware) BlockWidgetSetTargetId(cctx context.Context, req *pb.RpcBlockWidgetSetTargetIdRequest) *pb.RpcBlockWidgetSetTargetIdResponse {
	ctx := mw.newContext(cctx)
	response := func(code pb.RpcBlockWidgetSetTargetIdResponseErrorCode, id string, err error) *pb.RpcBlockWidgetSetTargetIdResponse {
		m := &pb.RpcBlockWidgetSetTargetIdResponse{Error: &pb.RpcBlockWidgetSetTargetIdResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = mw.getResponseEvent(ctx)
		}
		return m
	}
	var id string
	err := mw.doBlockService(func(bs *block.Service) (err error) {
		return bs.SetWidgetBlockTargetId(ctx, req)
	})
	if err != nil {
		return response(pb.RpcBlockWidgetSetTargetIdResponseError_UNKNOWN_ERROR, "", err)
	}
	return response(pb.RpcBlockWidgetSetTargetIdResponseError_NULL, id, nil)
}

func (mw *Middleware) BlockWidgetSetLayout(cctx context.Context, req *pb.RpcBlockWidgetSetLayoutRequest) *pb.RpcBlockWidgetSetLayoutResponse {
	ctx := mw.newContext(cctx)
	response := func(code pb.RpcBlockWidgetSetLayoutResponseErrorCode, id string, err error) *pb.RpcBlockWidgetSetLayoutResponse {
		m := &pb.RpcBlockWidgetSetLayoutResponse{Error: &pb.RpcBlockWidgetSetLayoutResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = mw.getResponseEvent(ctx)
		}
		return m
	}
	var id string
	err := mw.doBlockService(func(bs *block.Service) (err error) {
		return bs.SetWidgetBlockLayout(ctx, req)
	})
	if err != nil {
		return response(pb.RpcBlockWidgetSetLayoutResponseError_UNKNOWN_ERROR, "", err)
	}
	return response(pb.RpcBlockWidgetSetLayoutResponseError_NULL, id, nil)
}

func (mw *Middleware) BlockWidgetSetLimit(cctx context.Context, req *pb.RpcBlockWidgetSetLimitRequest) *pb.RpcBlockWidgetSetLimitResponse {
	ctx := mw.newContext(cctx)
	response := func(code pb.RpcBlockWidgetSetLimitResponseErrorCode, id string, err error) *pb.RpcBlockWidgetSetLimitResponse {
		m := &pb.RpcBlockWidgetSetLimitResponse{Error: &pb.RpcBlockWidgetSetLimitResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = mw.getResponseEvent(ctx)
		}
		return m
	}
	var id string
	err := mw.doBlockService(func(bs *block.Service) (err error) {
		return bs.SetWidgetBlockLimit(ctx, req)
	})
	if err != nil {
		return response(pb.RpcBlockWidgetSetLimitResponseError_UNKNOWN_ERROR, "", err)
	}
	return response(pb.RpcBlockWidgetSetLimitResponseError_NULL, id, nil)
}

func (mw *Middleware) BlockWidgetSetViewId(cctx context.Context, req *pb.RpcBlockWidgetSetViewIdRequest) *pb.RpcBlockWidgetSetViewIdResponse {
	ctx := mw.newContext(cctx)
	response := func(code pb.RpcBlockWidgetSetViewIdResponseErrorCode, id string, err error) *pb.RpcBlockWidgetSetViewIdResponse {
		m := &pb.RpcBlockWidgetSetViewIdResponse{Error: &pb.RpcBlockWidgetSetViewIdResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = mw.getResponseEvent(ctx)
		}
		return m
	}
	var id string
	err := mw.doBlockService(func(bs *block.Service) (err error) {
		return bs.SetWidgetBlockViewId(ctx, req)
	})
	if err != nil {
		return response(pb.RpcBlockWidgetSetViewIdResponseError_UNKNOWN_ERROR, "", err)
	}
	return response(pb.RpcBlockWidgetSetViewIdResponseError_NULL, id, nil)
}
