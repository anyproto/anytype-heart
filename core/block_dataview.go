package core

import (
	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/pb"
	pbrelation "github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/relation"
	"github.com/gogo/protobuf/types"
)

func (mw *Middleware) BlockGetDataviewAvailableRelations(req *pb.RpcBlockGetDataviewAvailableRelationsRequest) *pb.RpcBlockGetDataviewAvailableRelationsResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockGetDataviewAvailableRelationsResponseErrorCode, relations []*pbrelation.Relation, err error) *pb.RpcBlockGetDataviewAvailableRelationsResponse {
		m := &pb.RpcBlockGetDataviewAvailableRelationsResponse{Relations: relations, Error: &pb.RpcBlockGetDataviewAvailableRelationsResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}
	var (
		err       error
		relations []*pbrelation.Relation
	)

	err = mw.doBlockService(func(bs block.Service) (err error) {
		relations, err = bs.GetAggregatedRelations(ctx, *req)
		return
	})
	if err != nil {
		return response(pb.RpcBlockGetDataviewAvailableRelationsResponseError_BAD_INPUT, relations, err)
	}

	return response(pb.RpcBlockGetDataviewAvailableRelationsResponseError_NULL, relations, nil)
}

func (mw *Middleware) BlockSetDataviewView(req *pb.RpcBlockSetDataviewViewRequest) *pb.RpcBlockSetDataviewViewResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockSetDataviewViewResponseErrorCode, err error) *pb.RpcBlockSetDataviewViewResponse {
		m := &pb.RpcBlockSetDataviewViewResponse{Error: &pb.RpcBlockSetDataviewViewResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	err := mw.doBlockService(func(bs block.Service) (err error) {
		return bs.SetDataviewView(ctx, *req)
	})
	if err != nil {
		return response(pb.RpcBlockSetDataviewViewResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcBlockSetDataviewViewResponseError_NULL, nil)
}

func (mw *Middleware) BlockCreateDataviewView(req *pb.RpcBlockCreateDataviewViewRequest) *pb.RpcBlockCreateDataviewViewResponse {
	ctx := state.NewContext(nil)
	response := func(viewId string, code pb.RpcBlockCreateDataviewViewResponseErrorCode, err error) *pb.RpcBlockCreateDataviewViewResponse {
		m := &pb.RpcBlockCreateDataviewViewResponse{ViewId: viewId, Error: &pb.RpcBlockCreateDataviewViewResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	var viewId string
	err := mw.doBlockService(func(bs block.Service) (err error) {
		viewId, err = bs.CreateDataviewView(ctx, *req)
		return err
	})
	if err != nil {
		return response("", pb.RpcBlockCreateDataviewViewResponseError_UNKNOWN_ERROR, err)
	}
	return response(viewId, pb.RpcBlockCreateDataviewViewResponseError_NULL, nil)
}

func (mw *Middleware) BlockDeleteDataviewView(req *pb.RpcBlockDeleteDataviewViewRequest) *pb.RpcBlockDeleteDataviewViewResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockDeleteDataviewViewResponseErrorCode, err error) *pb.RpcBlockDeleteDataviewViewResponse {
		m := &pb.RpcBlockDeleteDataviewViewResponse{Error: &pb.RpcBlockDeleteDataviewViewResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	err := mw.doBlockService(func(bs block.Service) (err error) {
		return bs.DeleteDataviewView(ctx, *req)
	})
	if err != nil {
		return response(pb.RpcBlockDeleteDataviewViewResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcBlockDeleteDataviewViewResponseError_NULL, nil)
}

func (mw *Middleware) BlockSetDataviewActiveView(req *pb.RpcBlockSetDataviewActiveViewRequest) *pb.RpcBlockSetDataviewActiveViewResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockSetDataviewActiveViewResponseErrorCode, err error) *pb.RpcBlockSetDataviewActiveViewResponse {
		m := &pb.RpcBlockSetDataviewActiveViewResponse{Error: &pb.RpcBlockSetDataviewActiveViewResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	err := mw.doBlockService(func(bs block.Service) (err error) {
		return bs.SetDataviewActiveView(ctx, *req)
	})
	if err != nil {
		return response(pb.RpcBlockSetDataviewActiveViewResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcBlockSetDataviewActiveViewResponseError_NULL, nil)
}

func (mw *Middleware) BlockCreateDataviewRecord(req *pb.RpcBlockCreateDataviewRecordRequest) *pb.RpcBlockCreateDataviewRecordResponse {
	ctx := state.NewContext(nil)
	response := func(details *types.Struct, code pb.RpcBlockCreateDataviewRecordResponseErrorCode, err error) *pb.RpcBlockCreateDataviewRecordResponse {
		m := &pb.RpcBlockCreateDataviewRecordResponse{Record: details, Error: &pb.RpcBlockCreateDataviewRecordResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}
		// no events generated
		return m
	}

	var details *types.Struct
	if err := mw.doBlockService(func(bs block.Service) (err error) {
		details, err = bs.CreateDataviewRecord(ctx, *req)
		return err
	}); err != nil {
		return response(nil, pb.RpcBlockCreateDataviewRecordResponseError_UNKNOWN_ERROR, err)
	}

	return response(details, pb.RpcBlockCreateDataviewRecordResponseError_NULL, nil)
}

func (mw *Middleware) BlockUpdateDataviewRecord(req *pb.RpcBlockUpdateDataviewRecordRequest) *pb.RpcBlockUpdateDataviewRecordResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockUpdateDataviewRecordResponseErrorCode, err error) *pb.RpcBlockUpdateDataviewRecordResponse {
		m := &pb.RpcBlockUpdateDataviewRecordResponse{Error: &pb.RpcBlockUpdateDataviewRecordResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}
		// no events generated
		return m
	}

	if err := mw.doBlockService(func(bs block.Service) (err error) {
		return bs.UpdateDataviewRecord(ctx, *req)
	}); err != nil {
		return response(pb.RpcBlockUpdateDataviewRecordResponseError_UNKNOWN_ERROR, err)
	}

	return response(pb.RpcBlockUpdateDataviewRecordResponseError_NULL, nil)
}

func (mw *Middleware) BlockDeleteDataviewRecord(req *pb.RpcBlockDeleteDataviewRecordRequest) *pb.RpcBlockDeleteDataviewRecordResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockDeleteDataviewRecordResponseErrorCode, err error) *pb.RpcBlockDeleteDataviewRecordResponse {
		m := &pb.RpcBlockDeleteDataviewRecordResponse{Error: &pb.RpcBlockDeleteDataviewRecordResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}

	if err := mw.doBlockService(func(bs block.Service) (err error) {
		return bs.DeleteDataviewRecord(ctx, *req)
	}); err != nil {
		return response(pb.RpcBlockDeleteDataviewRecordResponseError_UNKNOWN_ERROR, err)
	}

	return response(pb.RpcBlockDeleteDataviewRecordResponseError_NULL, nil)
}

func (mw *Middleware) BlockDataviewRelationAdd(req *pb.RpcBlockDataviewRelationAddRequest) *pb.RpcBlockDataviewRelationAddResponse {
	ctx := state.NewContext(nil)
	response := func(relationKey string, code pb.RpcBlockDataviewRelationAddResponseErrorCode, err error) *pb.RpcBlockDataviewRelationAddResponse {
		m := &pb.RpcBlockDataviewRelationAddResponse{RelationKey: relationKey, Error: &pb.RpcBlockDataviewRelationAddResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	var relationKey string
	err := mw.doBlockService(func(bs block.Service) (err error) {
		relationKey, err = bs.AddDataviewRelation(ctx, *req)
		return err
	})
	if err != nil {
		return response("", pb.RpcBlockDataviewRelationAddResponseError_BAD_INPUT, err)
	}

	return response(relationKey, pb.RpcBlockDataviewRelationAddResponseError_NULL, nil)
}

func (mw *Middleware) BlockDataviewRelationDelete(req *pb.RpcBlockDataviewRelationDeleteRequest) *pb.RpcBlockDataviewRelationDeleteResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockDataviewRelationDeleteResponseErrorCode, err error) *pb.RpcBlockDataviewRelationDeleteResponse {
		m := &pb.RpcBlockDataviewRelationDeleteResponse{Error: &pb.RpcBlockDataviewRelationDeleteResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	err := mw.doBlockService(func(bs block.Service) (err error) {
		return bs.DeleteDataviewRelation(ctx, *req)
	})
	if err != nil {
		return response(pb.RpcBlockDataviewRelationDeleteResponseError_BAD_INPUT, err)
	}
	return response(pb.RpcBlockDataviewRelationDeleteResponseError_NULL, nil)
}
