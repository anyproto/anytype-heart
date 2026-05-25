package core

import (
	"context"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/pb"
)

func (mw *Middleware) BlockCanvasNodeCreate(cctx context.Context, req *pb.RpcBlockCanvasNodeCreateRequest) *pb.RpcBlockCanvasNodeCreateResponse {
	ctx := mw.newContext(cctx)
	response := func(code pb.RpcBlockCanvasNodeCreateResponseErrorCode, id string, err error) *pb.RpcBlockCanvasNodeCreateResponse {
		m := &pb.RpcBlockCanvasNodeCreateResponse{Error: &pb.RpcBlockCanvasNodeCreateResponseError{Code: code}, BlockId: id}
		if err != nil {
			m.Error.Description = getErrorDescription(err)
		} else {
			m.Event = mw.getResponseEvent(ctx)
		}
		return m
	}
	var id string
	err := mw.doBlockService(func(bs *block.Service) (err error) {
		id, err = bs.CanvasNodeCreate(ctx, *req)
		return
	})
	if err != nil {
		return response(pb.RpcBlockCanvasNodeCreateResponseError_UNKNOWN_ERROR, "", err)
	}
	return response(pb.RpcBlockCanvasNodeCreateResponseError_NULL, id, nil)
}

func (mw *Middleware) BlockCanvasNodeUpdate(cctx context.Context, req *pb.RpcBlockCanvasNodeUpdateRequest) *pb.RpcBlockCanvasNodeUpdateResponse {
	ctx := mw.newContext(cctx)
	response := func(code pb.RpcBlockCanvasNodeUpdateResponseErrorCode, err error) *pb.RpcBlockCanvasNodeUpdateResponse {
		m := &pb.RpcBlockCanvasNodeUpdateResponse{Error: &pb.RpcBlockCanvasNodeUpdateResponseError{Code: code}}
		if err != nil {
			m.Error.Description = getErrorDescription(err)
		} else {
			m.Event = mw.getResponseEvent(ctx)
		}
		return m
	}
	err := mw.doBlockService(func(bs *block.Service) error {
		return bs.CanvasNodeUpdate(ctx, *req)
	})
	if err != nil {
		return response(pb.RpcBlockCanvasNodeUpdateResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcBlockCanvasNodeUpdateResponseError_NULL, nil)
}

func (mw *Middleware) BlockCanvasNodeDelete(cctx context.Context, req *pb.RpcBlockCanvasNodeDeleteRequest) *pb.RpcBlockCanvasNodeDeleteResponse {
	ctx := mw.newContext(cctx)
	response := func(code pb.RpcBlockCanvasNodeDeleteResponseErrorCode, err error) *pb.RpcBlockCanvasNodeDeleteResponse {
		m := &pb.RpcBlockCanvasNodeDeleteResponse{Error: &pb.RpcBlockCanvasNodeDeleteResponseError{Code: code}}
		if err != nil {
			m.Error.Description = getErrorDescription(err)
		} else {
			m.Event = mw.getResponseEvent(ctx)
		}
		return m
	}
	err := mw.doBlockService(func(bs *block.Service) error {
		return bs.CanvasNodeDelete(ctx, *req)
	})
	if err != nil {
		return response(pb.RpcBlockCanvasNodeDeleteResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcBlockCanvasNodeDeleteResponseError_NULL, nil)
}

func (mw *Middleware) BlockCanvasEdgeCreate(cctx context.Context, req *pb.RpcBlockCanvasEdgeCreateRequest) *pb.RpcBlockCanvasEdgeCreateResponse {
	ctx := mw.newContext(cctx)
	response := func(code pb.RpcBlockCanvasEdgeCreateResponseErrorCode, id string, err error) *pb.RpcBlockCanvasEdgeCreateResponse {
		m := &pb.RpcBlockCanvasEdgeCreateResponse{Error: &pb.RpcBlockCanvasEdgeCreateResponseError{Code: code}, BlockId: id}
		if err != nil {
			m.Error.Description = getErrorDescription(err)
		} else {
			m.Event = mw.getResponseEvent(ctx)
		}
		return m
	}
	var id string
	err := mw.doBlockService(func(bs *block.Service) (err error) {
		id, err = bs.CanvasEdgeCreate(ctx, *req)
		return
	})
	if err != nil {
		return response(pb.RpcBlockCanvasEdgeCreateResponseError_UNKNOWN_ERROR, "", err)
	}
	return response(pb.RpcBlockCanvasEdgeCreateResponseError_NULL, id, nil)
}

func (mw *Middleware) BlockCanvasEdgeUpdate(cctx context.Context, req *pb.RpcBlockCanvasEdgeUpdateRequest) *pb.RpcBlockCanvasEdgeUpdateResponse {
	ctx := mw.newContext(cctx)
	response := func(code pb.RpcBlockCanvasEdgeUpdateResponseErrorCode, err error) *pb.RpcBlockCanvasEdgeUpdateResponse {
		m := &pb.RpcBlockCanvasEdgeUpdateResponse{Error: &pb.RpcBlockCanvasEdgeUpdateResponseError{Code: code}}
		if err != nil {
			m.Error.Description = getErrorDescription(err)
		} else {
			m.Event = mw.getResponseEvent(ctx)
		}
		return m
	}
	err := mw.doBlockService(func(bs *block.Service) error {
		return bs.CanvasEdgeUpdate(ctx, *req)
	})
	if err != nil {
		return response(pb.RpcBlockCanvasEdgeUpdateResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcBlockCanvasEdgeUpdateResponseError_NULL, nil)
}

func (mw *Middleware) BlockCanvasEdgeDelete(cctx context.Context, req *pb.RpcBlockCanvasEdgeDeleteRequest) *pb.RpcBlockCanvasEdgeDeleteResponse {
	ctx := mw.newContext(cctx)
	response := func(code pb.RpcBlockCanvasEdgeDeleteResponseErrorCode, err error) *pb.RpcBlockCanvasEdgeDeleteResponse {
		m := &pb.RpcBlockCanvasEdgeDeleteResponse{Error: &pb.RpcBlockCanvasEdgeDeleteResponseError{Code: code}}
		if err != nil {
			m.Error.Description = getErrorDescription(err)
		} else {
			m.Event = mw.getResponseEvent(ctx)
		}
		return m
	}
	err := mw.doBlockService(func(bs *block.Service) error {
		return bs.CanvasEdgeDelete(ctx, *req)
	})
	if err != nil {
		return response(pb.RpcBlockCanvasEdgeDeleteResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcBlockCanvasEdgeDeleteResponseError_NULL, nil)
}
