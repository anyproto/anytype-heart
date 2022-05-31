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

func (mw *Middleware) BlockTableCreateRow(req *pb.RpcBlockTableCreateRowRequest) *pb.RpcBlockTableCreateRowResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockTableCreateRowResponseErrorCode, id string, err error) *pb.RpcBlockTableCreateRowResponse {
		m := &pb.RpcBlockTableCreateRowResponse{Error: &pb.RpcBlockTableCreateRowResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	var id string
	err := mw.doBlockService(func(bs block.Service) (err error) {
		err = bs.TableCreateRow(ctx, *req)
		return
	})
	if err != nil {
		return response(pb.RpcBlockTableCreateRowResponseError_UNKNOWN_ERROR, "", err)
	}
	return response(pb.RpcBlockTableCreateRowResponseError_NULL, id, nil)
}

func (mw *Middleware) BlockTableCreateColumn(req *pb.RpcBlockTableCreateColumnRequest) *pb.RpcBlockTableCreateColumnResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockTableCreateColumnResponseErrorCode, id string, err error) *pb.RpcBlockTableCreateColumnResponse {
		m := &pb.RpcBlockTableCreateColumnResponse{Error: &pb.RpcBlockTableCreateColumnResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	var id string
	err := mw.doBlockService(func(bs block.Service) (err error) {
		err = bs.TableCreateColumn(ctx, *req)
		return
	})
	if err != nil {
		return response(pb.RpcBlockTableCreateColumnResponseError_UNKNOWN_ERROR, "", err)
	}
	return response(pb.RpcBlockTableCreateColumnResponseError_NULL, id, nil)
}

func (mw *Middleware) BlockTableDeleteRow(req *pb.RpcBlockTableDeleteRowRequest) *pb.RpcBlockTableDeleteRowResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockTableDeleteRowResponseErrorCode, id string, err error) *pb.RpcBlockTableDeleteRowResponse {
		m := &pb.RpcBlockTableDeleteRowResponse{Error: &pb.RpcBlockTableDeleteRowResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	var id string
	err := mw.doBlockService(func(bs block.Service) (err error) {
		err = bs.TableDeleteRow(ctx, *req)
		return
	})
	if err != nil {
		return response(pb.RpcBlockTableDeleteRowResponseError_UNKNOWN_ERROR, "", err)
	}
	return response(pb.RpcBlockTableDeleteRowResponseError_NULL, id, nil)
}

func (mw *Middleware) BlockTableDeleteColumn(req *pb.RpcBlockTableDeleteColumnRequest) *pb.RpcBlockTableDeleteColumnResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockTableDeleteColumnResponseErrorCode, id string, err error) *pb.RpcBlockTableDeleteColumnResponse {
		m := &pb.RpcBlockTableDeleteColumnResponse{Error: &pb.RpcBlockTableDeleteColumnResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	var id string
	err := mw.doBlockService(func(bs block.Service) (err error) {
		err = bs.TableDeleteColumn(ctx, *req)
		return
	})
	if err != nil {
		return response(pb.RpcBlockTableDeleteColumnResponseError_UNKNOWN_ERROR, "", err)
	}
	return response(pb.RpcBlockTableDeleteColumnResponseError_NULL, id, nil)
}

func (mw *Middleware) BlockTableMoveRow(req *pb.RpcBlockTableMoveRowRequest) *pb.RpcBlockTableMoveRowResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockTableMoveRowResponseErrorCode, id string, err error) *pb.RpcBlockTableMoveRowResponse {
		m := &pb.RpcBlockTableMoveRowResponse{Error: &pb.RpcBlockTableMoveRowResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	var id string
	err := mw.doBlockService(func(bs block.Service) (err error) {
		err = bs.TableMoveRow(ctx, *req)
		return
	})
	if err != nil {
		return response(pb.RpcBlockTableMoveRowResponseError_UNKNOWN_ERROR, "", err)
	}
	return response(pb.RpcBlockTableMoveRowResponseError_NULL, id, nil)
}
