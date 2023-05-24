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
			m.Event = ctx.GetResponseEvent()
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
