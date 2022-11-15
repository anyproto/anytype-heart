package core

import (
	"context"

	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/widget"
	"github.com/anytypeio/go-anytype-middleware/pb"
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
	err := mw.doBlockService(func(bs block.Service) (err error) {
		return bs.DoWithContext(cctx, req.ContextId, func(sb smartblock.SmartBlock) error {
			s := sb.NewStateCtx(ctx)
			var err error
			id, err = widget.CreateBlock(s, req)
			if err != nil {
				return err
			}
			return sb.Apply(s)
		})
	})
	if err != nil {
		return response(pb.RpcBlockCreateWidgetResponseError_UNKNOWN_ERROR, "", err)
	}
	return response(pb.RpcBlockCreateWidgetResponseError_NULL, id, nil)
}
