package core

import (
	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

func (mw *Middleware) BlockTableCreate(req *pb.RpcBlockTableCreateRequest) *pb.RpcBlockTableCreateResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockTableCreateResponseErrorCode, id string, err error) *pb.RpcBlockTableCreateResponse {
		m := &pb.RpcBlockTableCreateResponse{Error: &pb.RpcBlockTableCreateResponseError{Code: code}, BlockId: id}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	var id string
	err := mw.doBlockService(func(bs block.Service) (err error) {
		id, err = bs.CreateTableBlock(ctx, *req)
		return
	})
	if err != nil {
		return response(pb.RpcBlockTableCreateResponseError_UNKNOWN_ERROR, "", err)
	}
	return response(pb.RpcBlockTableCreateResponseError_NULL, id, nil)
}
