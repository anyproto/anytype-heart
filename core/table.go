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
		err = bs.TableRowCreate(ctx, *req)
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
		err = bs.TableColumnCreate(ctx, *req)
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
		err = bs.TableRowDelete(ctx, *req)
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
		err = bs.TableColumnDelete(ctx, *req)
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
		err = bs.TableRowMove(ctx, *req)
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
		err = bs.TableColumnMove(ctx, *req)
		return
	})
	if err != nil {
		return response(pb.RpcBlockTableColumnMoveResponseError_UNKNOWN_ERROR, "", err)
	}
	return response(pb.RpcBlockTableColumnMoveResponseError_NULL, id, nil)
}

func (mw *Middleware) BlockTableRowDuplicate(req *pb.RpcBlockTableRowDuplicateRequest) *pb.RpcBlockTableRowDuplicateResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockTableRowDuplicateResponseErrorCode, id string, err error) *pb.RpcBlockTableRowDuplicateResponse {
		m := &pb.RpcBlockTableRowDuplicateResponse{Error: &pb.RpcBlockTableRowDuplicateResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	var id string
	err := mw.doBlockService(func(bs block.Service) (err error) {
		err = bs.TableRowDuplicate(ctx, *req)
		return
	})
	if err != nil {
		return response(pb.RpcBlockTableRowDuplicateResponseError_UNKNOWN_ERROR, "", err)
	}
	return response(pb.RpcBlockTableRowDuplicateResponseError_NULL, id, nil)
}

func (mw *Middleware) BlockTableColumnDuplicate(req *pb.RpcBlockTableColumnDuplicateRequest) *pb.RpcBlockTableColumnDuplicateResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockTableColumnDuplicateResponseErrorCode, id string, err error) *pb.RpcBlockTableColumnDuplicateResponse {
		m := &pb.RpcBlockTableColumnDuplicateResponse{Error: &pb.RpcBlockTableColumnDuplicateResponseError{Code: code, BlockId: id}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	var id string
	err := mw.doBlockService(func(bs block.Service) (err error) {
		id, err = bs.TableColumnDuplicate(ctx, *req)
		return
	})
	if err != nil {
		return response(pb.RpcBlockTableColumnDuplicateResponseError_UNKNOWN_ERROR, "", err)
	}
	return response(pb.RpcBlockTableColumnDuplicateResponseError_NULL, id, nil)
}
