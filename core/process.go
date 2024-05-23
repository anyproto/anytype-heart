package core

import (
	"context"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/pb"
)

func (mw *Middleware) ProcessCancel(cctx context.Context, req *pb.RpcProcessCancelRequest) *pb.RpcProcessCancelResponse {
	response := func(code pb.RpcProcessCancelResponseErrorCode, err error) *pb.RpcProcessCancelResponse {
		m := &pb.RpcProcessCancelResponse{Error: &pb.RpcProcessCancelResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}
	err := mw.doBlockService(func(bs *block.Service) error {
		return bs.ProcessCancel(req.Id)
	})
	if err != nil {
		return response(pb.RpcProcessCancelResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcProcessCancelResponseError_NULL, nil)
}

func (mw *Middleware) ProcessSubscribe(cctx context.Context, req *pb.RpcProcessSubscribeRequest) *pb.RpcProcessSubscribeResponse {
	response := func(code pb.RpcProcessSubscribeResponseErrorCode, err error) *pb.RpcProcessSubscribeResponse {
		m := &pb.RpcProcessSubscribeResponse{Error: &pb.RpcProcessSubscribeResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}
	return response(pb.RpcProcessSubscribeResponseError_NULL, nil)
}

func (mw *Middleware) ProcessUnsubscribe(cctx context.Context, req *pb.RpcProcessUnsubscribeRequest) *pb.RpcProcessUnsubscribeResponse {
	response := func(code pb.RpcProcessUnsubscribeResponseErrorCode, err error) *pb.RpcProcessUnsubscribeResponse {
		m := &pb.RpcProcessUnsubscribeResponse{Error: &pb.RpcProcessUnsubscribeResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}
	return response(pb.RpcProcessUnsubscribeResponseError_NULL, nil)
}
