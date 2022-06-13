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
		m := &pb.RpcBlockTableColumnDuplicateResponse{BlockId: id, Error: &pb.RpcBlockTableColumnDuplicateResponseError{Code: code}}
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

func (mw *Middleware) BlockTableExpand(req *pb.RpcBlockTableExpandRequest) *pb.RpcBlockTableExpandResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockTableExpandResponseErrorCode, id string, err error) *pb.RpcBlockTableExpandResponse {
		m := &pb.RpcBlockTableExpandResponse{Error: &pb.RpcBlockTableExpandResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	var id string
	err := mw.doBlockService(func(bs block.Service) (err error) {
		err = bs.TableExpand(ctx, *req)
		return
	})
	if err != nil {
		return response(pb.RpcBlockTableExpandResponseError_UNKNOWN_ERROR, "", err)
	}
	return response(pb.RpcBlockTableExpandResponseError_NULL, id, nil)
}

func (mw *Middleware) BlockTableRowListFill(req *pb.RpcBlockTableRowListFillRequest) *pb.RpcBlockTableRowListFillResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockTableRowListFillResponseErrorCode, id string, err error) *pb.RpcBlockTableRowListFillResponse {
		m := &pb.RpcBlockTableRowListFillResponse{Error: &pb.RpcBlockTableRowListFillResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	var id string
	err := mw.doBlockService(func(bs block.Service) (err error) {
		err = bs.TableRowListFill(ctx, *req)
		return
	})
	if err != nil {
		return response(pb.RpcBlockTableRowListFillResponseError_UNKNOWN_ERROR, "", err)
	}
	return response(pb.RpcBlockTableRowListFillResponseError_NULL, id, nil)
}

func (mw *Middleware) BlockTableRowListClean(req *pb.RpcBlockTableRowListCleanRequest) *pb.RpcBlockTableRowListCleanResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockTableRowListCleanResponseErrorCode, id string, err error) *pb.RpcBlockTableRowListCleanResponse {
		m := &pb.RpcBlockTableRowListCleanResponse{Error: &pb.RpcBlockTableRowListCleanResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	var id string
	err := mw.doBlockService(func(bs block.Service) (err error) {
		err = bs.TableRowListClean(ctx, *req)
		return
	})
	if err != nil {
		return response(pb.RpcBlockTableRowListCleanResponseError_UNKNOWN_ERROR, "", err)
	}
	return response(pb.RpcBlockTableRowListCleanResponseError_NULL, id, nil)
}
