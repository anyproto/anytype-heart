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

func (mw *Middleware) BlockTableRowCreate(req *pb.RpcBlockTableRowCreateRequest) *pb.RpcBlockTableRowCreateResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockTableRowCreateResponseErrorCode, id string, err error) *pb.RpcBlockTableRowCreateResponse {
		m := &pb.RpcBlockTableRowCreateResponse{Error: &pb.RpcBlockTableRowCreateResponseError{Code: code}}
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
		return response(pb.RpcBlockTableRowCreateResponseError_UNKNOWN_ERROR, "", err)
	}
	return response(pb.RpcBlockTableRowCreateResponseError_NULL, id, nil)
}

func (mw *Middleware) BlockTableColumnCreate(req *pb.RpcBlockTableColumnCreateRequest) *pb.RpcBlockTableColumnCreateResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockTableColumnCreateResponseErrorCode, id string, err error) *pb.RpcBlockTableColumnCreateResponse {
		m := &pb.RpcBlockTableColumnCreateResponse{Error: &pb.RpcBlockTableColumnCreateResponseError{Code: code}}
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
		return response(pb.RpcBlockTableColumnCreateResponseError_UNKNOWN_ERROR, "", err)
	}
	return response(pb.RpcBlockTableColumnCreateResponseError_NULL, id, nil)
}

func (mw *Middleware) BlockTableRowDelete(req *pb.RpcBlockTableRowDeleteRequest) *pb.RpcBlockTableRowDeleteResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockTableRowDeleteResponseErrorCode, id string, err error) *pb.RpcBlockTableRowDeleteResponse {
		m := &pb.RpcBlockTableRowDeleteResponse{Error: &pb.RpcBlockTableRowDeleteResponseError{Code: code}}
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
		return response(pb.RpcBlockTableRowDeleteResponseError_UNKNOWN_ERROR, "", err)
	}
	return response(pb.RpcBlockTableRowDeleteResponseError_NULL, id, nil)
}

func (mw *Middleware) BlockTableColumnDelete(req *pb.RpcBlockTableColumnDeleteRequest) *pb.RpcBlockTableColumnDeleteResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockTableColumnDeleteResponseErrorCode, id string, err error) *pb.RpcBlockTableColumnDeleteResponse {
		m := &pb.RpcBlockTableColumnDeleteResponse{Error: &pb.RpcBlockTableColumnDeleteResponseError{Code: code}}
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
		return response(pb.RpcBlockTableColumnDeleteResponseError_UNKNOWN_ERROR, "", err)
	}
	return response(pb.RpcBlockTableColumnDeleteResponseError_NULL, id, nil)
}

func (mw *Middleware) BlockTableRowMove(req *pb.RpcBlockTableRowMoveRequest) *pb.RpcBlockTableRowMoveResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockTableRowMoveResponseErrorCode, id string, err error) *pb.RpcBlockTableRowMoveResponse {
		m := &pb.RpcBlockTableRowMoveResponse{Error: &pb.RpcBlockTableRowMoveResponseError{Code: code}}
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
		return response(pb.RpcBlockTableRowMoveResponseError_UNKNOWN_ERROR, "", err)
	}
	return response(pb.RpcBlockTableRowMoveResponseError_NULL, id, nil)
}

func (mw *Middleware) BlockTableColumnMove(req *pb.RpcBlockTableColumnMoveRequest) *pb.RpcBlockTableColumnMoveResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockTableColumnMoveResponseErrorCode, id string, err error) *pb.RpcBlockTableColumnMoveResponse {
		m := &pb.RpcBlockTableColumnMoveResponse{Error: &pb.RpcBlockTableColumnMoveResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	var id string
	err := mw.doBlockService(func(bs block.Service) (err error) {
		err = bs.TableMoveColumn(ctx, *req)
		return
	})
	if err != nil {
		return response(pb.RpcBlockTableColumnMoveResponseError_UNKNOWN_ERROR, "", err)
	}
	return response(pb.RpcBlockTableColumnMoveResponseError_NULL, id, nil)
}

func (mw *Middleware) BlockTableCellSetVerticalAlign(req *pb.RpcBlockTableCellSetVerticalAlignRequest) *pb.RpcBlockTableCellSetVerticalAlignResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockTableCellSetVerticalAlignResponseErrorCode, id string, err error) *pb.RpcBlockTableCellSetVerticalAlignResponse {
		m := &pb.RpcBlockTableCellSetVerticalAlignResponse{Error: &pb.RpcBlockTableCellSetVerticalAlignResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	var id string
	err := mw.doBlockService(func(bs block.Service) (err error) {
		err = bs.TableCellSetVerticalAlign(ctx, *req)
		return
	})
	if err != nil {
		return response(pb.RpcBlockTableCellSetVerticalAlignResponseError_UNKNOWN_ERROR, "", err)
	}
	return response(pb.RpcBlockTableCellSetVerticalAlignResponseError_NULL, id, nil)
}
