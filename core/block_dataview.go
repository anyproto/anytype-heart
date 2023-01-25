package core

import (
	"context"

	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

func (mw *Middleware) BlockDataviewRelationListAvailable(cctx context.Context, req *pb.RpcBlockDataviewRelationListAvailableRequest) *pb.RpcBlockDataviewRelationListAvailableResponse {
	response := func(code pb.RpcBlockDataviewRelationListAvailableResponseErrorCode, relations []*model.Relation, err error) *pb.RpcBlockDataviewRelationListAvailableResponse {
		m := &pb.RpcBlockDataviewRelationListAvailableResponse{Relations: relations, Error: &pb.RpcBlockDataviewRelationListAvailableResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}
	var (
		err       error
		relations []*model.Relation
	)

	err = mw.doBlockService(func(bs *block.Service) (err error) {
		relations, err = bs.GetAggregatedRelations(*req)
		return
	})
	if err != nil {
		return response(pb.RpcBlockDataviewRelationListAvailableResponseError_BAD_INPUT, relations, err)
	}

	return response(pb.RpcBlockDataviewRelationListAvailableResponseError_NULL, relations, nil)
}

func (mw *Middleware) BlockDataviewGroupOrderUpdate(cctx context.Context, req *pb.RpcBlockDataviewGroupOrderUpdateRequest) *pb.RpcBlockDataviewGroupOrderUpdateResponse {
	ctx := mw.newContext(cctx)
	response := func(code pb.RpcBlockDataviewGroupOrderUpdateResponseErrorCode, err error) *pb.RpcBlockDataviewGroupOrderUpdateResponse {
		m := &pb.RpcBlockDataviewGroupOrderUpdateResponse{Error: &pb.RpcBlockDataviewGroupOrderUpdateResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	err := mw.doBlockService(func(bs *block.Service) (err error) {
		return bs.UpdateDataviewGroupOrder(ctx, *req)
	})
	if err != nil {
		return response(pb.RpcBlockDataviewGroupOrderUpdateResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcBlockDataviewGroupOrderUpdateResponseError_NULL, nil)
}

func (mw *Middleware) BlockDataviewObjectOrderUpdate(cctx context.Context, req *pb.RpcBlockDataviewObjectOrderUpdateRequest) *pb.RpcBlockDataviewObjectOrderUpdateResponse {
	ctx := mw.newContext(cctx)
	response := func(code pb.RpcBlockDataviewObjectOrderUpdateResponseErrorCode, err error) *pb.RpcBlockDataviewObjectOrderUpdateResponse {
		m := &pb.RpcBlockDataviewObjectOrderUpdateResponse{Error: &pb.RpcBlockDataviewObjectOrderUpdateResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	err := mw.doBlockService(func(bs *block.Service) (err error) {
		return bs.UpdateDataviewObjectOrder(ctx, *req)
	})
	if err != nil {
		return response(pb.RpcBlockDataviewObjectOrderUpdateResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcBlockDataviewObjectOrderUpdateResponseError_NULL, nil)
}

func (mw *Middleware) BlockDataviewCreateFromExistingObject(cctx context.Context,
	req *pb.RpcBlockDataviewCreateFromExistingObjectRequest) *pb.RpcBlockDataviewCreateFromExistingObjectResponse {
	ctx := mw.newContext(cctx)
	response := func(code pb.RpcBlockDataviewCreateFromExistingObjectResponseErrorCode,
		blockId string,
		targetObjectId string,
		views []*model.BlockContentDataviewView,
		err error) *pb.RpcBlockDataviewCreateFromExistingObjectResponse {
		m := &pb.RpcBlockDataviewCreateFromExistingObjectResponse{
			BlockId:        blockId,
			TargetObjectId: targetObjectId,
			View:           views,
			Error:          &pb.RpcBlockDataviewCreateFromExistingObjectResponseError{Code: code},
		}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}

		return m
	}

	var views []*model.BlockContentDataviewView

	err := mw.doBlockService(func(bs *block.Service) error {
		var err error
		views, err = bs.CopyDataviewToBlock(ctx, req)
		return err
	})

	if err != nil {
		return response(pb.RpcBlockDataviewCreateFromExistingObjectResponseError_UNKNOWN_ERROR,
			req.BlockId, req.TargetObjectId, views, err)
	}

	return response(pb.RpcBlockDataviewCreateFromExistingObjectResponseError_NULL,
		req.BlockId, req.TargetObjectId, views, err)
}

func (mw *Middleware) BlockDataviewViewUpdate(cctx context.Context, req *pb.RpcBlockDataviewViewUpdateRequest) *pb.RpcBlockDataviewViewUpdateResponse {
	ctx := mw.newContext(cctx)
	response := func(code pb.RpcBlockDataviewViewUpdateResponseErrorCode, err error) *pb.RpcBlockDataviewViewUpdateResponse {
		m := &pb.RpcBlockDataviewViewUpdateResponse{Error: &pb.RpcBlockDataviewViewUpdateResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	err := mw.doBlockService(func(bs *block.Service) (err error) {
		return bs.UpdateDataviewView(ctx, *req)
	})
	if err != nil {
		return response(pb.RpcBlockDataviewViewUpdateResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcBlockDataviewViewUpdateResponseError_NULL, nil)
}

func (mw *Middleware) BlockDataviewViewCreate(cctx context.Context, req *pb.RpcBlockDataviewViewCreateRequest) *pb.RpcBlockDataviewViewCreateResponse {
	ctx := mw.newContext(cctx)
	response := func(viewId string, code pb.RpcBlockDataviewViewCreateResponseErrorCode, err error) *pb.RpcBlockDataviewViewCreateResponse {
		m := &pb.RpcBlockDataviewViewCreateResponse{ViewId: viewId, Error: &pb.RpcBlockDataviewViewCreateResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	var viewId string
	err := mw.doBlockService(func(bs *block.Service) (err error) {
		viewId, err = bs.CreateDataviewView(ctx, *req)
		return err
	})
	if err != nil {
		return response("", pb.RpcBlockDataviewViewCreateResponseError_UNKNOWN_ERROR, err)
	}
	return response(viewId, pb.RpcBlockDataviewViewCreateResponseError_NULL, nil)
}

func (mw *Middleware) BlockDataviewViewDelete(cctx context.Context, req *pb.RpcBlockDataviewViewDeleteRequest) *pb.RpcBlockDataviewViewDeleteResponse {
	ctx := mw.newContext(cctx)
	response := func(code pb.RpcBlockDataviewViewDeleteResponseErrorCode, err error) *pb.RpcBlockDataviewViewDeleteResponse {
		m := &pb.RpcBlockDataviewViewDeleteResponse{Error: &pb.RpcBlockDataviewViewDeleteResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	err := mw.doBlockService(func(bs *block.Service) (err error) {
		return bs.DeleteDataviewView(ctx, *req)
	})
	if err != nil {
		return response(pb.RpcBlockDataviewViewDeleteResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcBlockDataviewViewDeleteResponseError_NULL, nil)
}

func (mw *Middleware) BlockDataviewViewSetActive(cctx context.Context, req *pb.RpcBlockDataviewViewSetActiveRequest) *pb.RpcBlockDataviewViewSetActiveResponse {
	ctx := mw.newContext(cctx)
	response := func(code pb.RpcBlockDataviewViewSetActiveResponseErrorCode, err error) *pb.RpcBlockDataviewViewSetActiveResponse {
		m := &pb.RpcBlockDataviewViewSetActiveResponse{Error: &pb.RpcBlockDataviewViewSetActiveResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	err := mw.doBlockService(func(bs *block.Service) (err error) {
		return bs.SetDataviewActiveView(ctx, *req)
	})
	if err != nil {
		return response(pb.RpcBlockDataviewViewSetActiveResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcBlockDataviewViewSetActiveResponseError_NULL, nil)
}

func (mw *Middleware) BlockDataviewViewSetPosition(cctx context.Context, req *pb.RpcBlockDataviewViewSetPositionRequest) *pb.RpcBlockDataviewViewSetPositionResponse {
	ctx := mw.newContext(cctx)
	response := func(code pb.RpcBlockDataviewViewSetPositionResponseErrorCode, err error) *pb.RpcBlockDataviewViewSetPositionResponse {
		m := &pb.RpcBlockDataviewViewSetPositionResponse{Error: &pb.RpcBlockDataviewViewSetPositionResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	err := mw.doBlockService(func(bs *block.Service) (err error) {
		return bs.SetDataviewViewPosition(ctx, *req)
	})
	if err != nil {
		return response(pb.RpcBlockDataviewViewSetPositionResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcBlockDataviewViewSetPositionResponseError_NULL, nil)
}

func (mw *Middleware) BlockDataviewRelationAdd(cctx context.Context, req *pb.RpcBlockDataviewRelationAddRequest) *pb.RpcBlockDataviewRelationAddResponse {
	ctx := mw.newContext(cctx)
	response := func(code pb.RpcBlockDataviewRelationAddResponseErrorCode, err error) *pb.RpcBlockDataviewRelationAddResponse {

		m := &pb.RpcBlockDataviewRelationAddResponse{Error: &pb.RpcBlockDataviewRelationAddResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	err := mw.doBlockService(func(bs *block.Service) (err error) {
		return bs.AddDataviewRelation(ctx, *req)
	})
	if err != nil {
		return response(pb.RpcBlockDataviewRelationAddResponseError_BAD_INPUT, err)
	}

	return response(pb.RpcBlockDataviewRelationAddResponseError_NULL, nil)
}

func (mw *Middleware) BlockDataviewRelationDelete(cctx context.Context, req *pb.RpcBlockDataviewRelationDeleteRequest) *pb.RpcBlockDataviewRelationDeleteResponse {
	ctx := mw.newContext(cctx)
	response := func(code pb.RpcBlockDataviewRelationDeleteResponseErrorCode, err error) *pb.RpcBlockDataviewRelationDeleteResponse {
		m := &pb.RpcBlockDataviewRelationDeleteResponse{Error: &pb.RpcBlockDataviewRelationDeleteResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	err := mw.doBlockService(func(bs *block.Service) (err error) {
		return bs.DeleteDataviewRelation(ctx, *req)
	})
	if err != nil {
		return response(pb.RpcBlockDataviewRelationDeleteResponseError_BAD_INPUT, err)
	}
	return response(pb.RpcBlockDataviewRelationDeleteResponseError_NULL, nil)
}

func (mw *Middleware) BlockDataviewSetSource(cctx context.Context, req *pb.RpcBlockDataviewSetSourceRequest) *pb.RpcBlockDataviewSetSourceResponse {
	ctx := mw.newContext(cctx)
	resp := func(err error) *pb.RpcBlockDataviewSetSourceResponse {
		r := &pb.RpcBlockDataviewSetSourceResponse{
			Error: &pb.RpcBlockDataviewSetSourceResponseError{
				Code: pb.RpcBlockDataviewSetSourceResponseError_NULL,
			},
		}
		if err != nil {
			r.Error.Code = pb.RpcBlockDataviewSetSourceResponseError_UNKNOWN_ERROR
			r.Error.Description = err.Error()
		} else {
			r.Event = ctx.GetResponseEvent()
		}
		return r
	}

	err := mw.doBlockService(func(bs *block.Service) error {
		return bs.SetDataviewSource(ctx, req.ContextId, req.BlockId, req.Source)
	})

	return resp(err)
}

func (mw *Middleware) BlockDataviewFilterAdd(cctx context.Context, req *pb.RpcBlockDataviewFilterAddRequest) *pb.RpcBlockDataviewFilterAddResponse {
	ctx := mw.newContext(cctx)
	resp := func(err error) *pb.RpcBlockDataviewFilterAddResponse {
		r := &pb.RpcBlockDataviewFilterAddResponse{
			Error: &pb.RpcBlockDataviewFilterAddResponseError{
				Code: pb.RpcBlockDataviewFilterAddResponseError_NULL,
			},
		}
		if err != nil {
			r.Error.Code = pb.RpcBlockDataviewFilterAddResponseError_UNKNOWN_ERROR
			r.Error.Description = err.Error()
		} else {
			r.Event = ctx.GetResponseEvent()
		}
		return r
	}

	err := mw.doBlockService(func(bs *block.Service) error {
		return bs.AddDataviewFilter(ctx, req.ContextId, req.BlockId, req.ViewId, req.Filter)
	})

	return resp(err)
}

func (mw *Middleware) BlockDataviewFilterRemove(cctx context.Context, req *pb.RpcBlockDataviewFilterRemoveRequest) *pb.RpcBlockDataviewFilterRemoveResponse {
	ctx := mw.newContext(cctx)
	resp := func(err error) *pb.RpcBlockDataviewFilterRemoveResponse {
		r := &pb.RpcBlockDataviewFilterRemoveResponse{
			Error: &pb.RpcBlockDataviewFilterRemoveResponseError{
				Code: pb.RpcBlockDataviewFilterRemoveResponseError_NULL,
			},
		}
		if err != nil {
			r.Error.Code = pb.RpcBlockDataviewFilterRemoveResponseError_UNKNOWN_ERROR
			r.Error.Description = err.Error()
		} else {
			r.Event = ctx.GetResponseEvent()
		}
		return r
	}

	err := mw.doBlockService(func(bs *block.Service) error {
		return bs.RemoveDataviewFilters(ctx, req.ContextId, req.BlockId, req.ViewId, req.FilterIds)
	})

	return resp(err)
}

func (mw *Middleware) BlockDataviewFilterReplace(cctx context.Context, req *pb.RpcBlockDataviewFilterReplaceRequest) *pb.RpcBlockDataviewFilterReplaceResponse {
	ctx := mw.newContext(cctx)
	resp := func(err error) *pb.RpcBlockDataviewFilterReplaceResponse {
		r := &pb.RpcBlockDataviewFilterReplaceResponse{
			Error: &pb.RpcBlockDataviewFilterReplaceResponseError{
				Code: pb.RpcBlockDataviewFilterReplaceResponseError_NULL,
			},
		}
		if err != nil {
			r.Error.Code = pb.RpcBlockDataviewFilterReplaceResponseError_UNKNOWN_ERROR
			r.Error.Description = err.Error()
		} else {
			r.Event = ctx.GetResponseEvent()
		}
		return r
	}

	err := mw.doBlockService(func(bs *block.Service) error {
		return bs.ReplaceDataviewFilter(ctx, req.ContextId, req.BlockId, req.ViewId, req.FilterId, req.Filter)
	})

	return resp(err)
}

func (mw *Middleware) BlockDataviewFilterSort(cctx context.Context, req *pb.RpcBlockDataviewFilterSortRequest) *pb.RpcBlockDataviewFilterSortResponse {
	ctx := mw.newContext(cctx)
	resp := func(err error) *pb.RpcBlockDataviewFilterSortResponse {
		r := &pb.RpcBlockDataviewFilterSortResponse{
			Error: &pb.RpcBlockDataviewFilterSortResponseError{
				Code: pb.RpcBlockDataviewFilterSortResponseError_NULL,
			},
		}
		if err != nil {
			r.Error.Code = pb.RpcBlockDataviewFilterSortResponseError_UNKNOWN_ERROR
			r.Error.Description = err.Error()
		} else {
			r.Event = ctx.GetResponseEvent()
		}
		return r
	}

	err := mw.doBlockService(func(bs *block.Service) error {
		return bs.ReorderDataviewFilters(ctx, req.ContextId, req.BlockId, req.ViewId, req.FilterIds)
	})

	return resp(err)
}

func (mw *Middleware) BlockDataviewSortAdd(cctx context.Context, req *pb.RpcBlockDataviewSortAddRequest) *pb.RpcBlockDataviewSortAddResponse {
	ctx := mw.newContext(cctx)
	resp := func(err error) *pb.RpcBlockDataviewSortAddResponse {
		r := &pb.RpcBlockDataviewSortAddResponse{
			Error: &pb.RpcBlockDataviewSortAddResponseError{
				Code: pb.RpcBlockDataviewSortAddResponseError_NULL,
			},
		}
		if err != nil {
			r.Error.Code = pb.RpcBlockDataviewSortAddResponseError_UNKNOWN_ERROR
			r.Error.Description = err.Error()
		} else {
			r.Event = ctx.GetResponseEvent()
		}
		return r
	}

	err := mw.doBlockService(func(bs *block.Service) error {
		return bs.AddDataviewSort(ctx, req.ContextId, req.BlockId, req.ViewId, req.Sort)
	})

	return resp(err)
}

func (mw *Middleware) BlockDataviewSortRemove(cctx context.Context, req *pb.RpcBlockDataviewSortRemoveRequest) *pb.RpcBlockDataviewSortRemoveResponse {
	ctx := mw.newContext(cctx)
	resp := func(err error) *pb.RpcBlockDataviewSortRemoveResponse {
		r := &pb.RpcBlockDataviewSortRemoveResponse{
			Error: &pb.RpcBlockDataviewSortRemoveResponseError{
				Code: pb.RpcBlockDataviewSortRemoveResponseError_NULL,
			},
		}
		if err != nil {
			r.Error.Code = pb.RpcBlockDataviewSortRemoveResponseError_UNKNOWN_ERROR
			r.Error.Description = err.Error()
		} else {
			r.Event = ctx.GetResponseEvent()
		}
		return r
	}

	err := mw.doBlockService(func(bs *block.Service) error {
		return bs.RemoveDataviewSorts(ctx, req.ContextId, req.BlockId, req.ViewId, req.Ids)
	})

	return resp(err)
}

func (mw *Middleware) BlockDataviewSortReplace(cctx context.Context, req *pb.RpcBlockDataviewSortReplaceRequest) *pb.RpcBlockDataviewSortReplaceResponse {
	ctx := mw.newContext(cctx)
	resp := func(err error) *pb.RpcBlockDataviewSortReplaceResponse {
		r := &pb.RpcBlockDataviewSortReplaceResponse{
			Error: &pb.RpcBlockDataviewSortReplaceResponseError{
				Code: pb.RpcBlockDataviewSortReplaceResponseError_NULL,
			},
		}
		if err != nil {
			r.Error.Code = pb.RpcBlockDataviewSortReplaceResponseError_UNKNOWN_ERROR
			r.Error.Description = err.Error()
		} else {
			r.Event = ctx.GetResponseEvent()
		}
		return r
	}

	err := mw.doBlockService(func(bs *block.Service) error {
		return bs.ReplaceDataviewSort(ctx, req.ContextId, req.BlockId, req.ViewId, req.Id, req.Sort)
	})

	return resp(err)
}

func (mw *Middleware) BlockDataviewSortSort(cctx context.Context, req *pb.RpcBlockDataviewSortSortRequest) *pb.RpcBlockDataviewSortSortResponse {
	ctx := mw.newContext(cctx)
	resp := func(err error) *pb.RpcBlockDataviewSortSortResponse {
		r := &pb.RpcBlockDataviewSortSortResponse{
			Error: &pb.RpcBlockDataviewSortSortResponseError{
				Code: pb.RpcBlockDataviewSortSortResponseError_NULL,
			},
		}
		if err != nil {
			r.Error.Code = pb.RpcBlockDataviewSortSortResponseError_UNKNOWN_ERROR
			r.Error.Description = err.Error()
		} else {
			r.Event = ctx.GetResponseEvent()
		}
		return r
	}

	err := mw.doBlockService(func(bs *block.Service) error {
		return bs.ReorderDataviewSorts(ctx, req.ContextId, req.BlockId, req.ViewId, req.Ids)
	})

	return resp(err)
}

func (mw *Middleware) BlockDataviewViewRelationAdd(cctx context.Context, req *pb.RpcBlockDataviewViewRelationAddRequest) *pb.RpcBlockDataviewViewRelationAddResponse {
	ctx := mw.newContext(cctx)
	resp := func(err error) *pb.RpcBlockDataviewViewRelationAddResponse {
		r := &pb.RpcBlockDataviewViewRelationAddResponse{
			Error: &pb.RpcBlockDataviewViewRelationAddResponseError{
				Code: pb.RpcBlockDataviewViewRelationAddResponseError_NULL,
			},
		}
		if err != nil {
			r.Error.Code = pb.RpcBlockDataviewViewRelationAddResponseError_UNKNOWN_ERROR
			r.Error.Description = err.Error()
		} else {
			r.Event = ctx.GetResponseEvent()
		}
		return r
	}

	err := mw.doBlockService(func(bs *block.Service) error {
		return bs.AddDataviewViewRelation(ctx, req.ContextId, req.BlockId, req.ViewId, req.Relation)
	})

	return resp(err)
}

func (mw *Middleware) BlockDataviewViewRelationRemove(cctx context.Context, req *pb.RpcBlockDataviewViewRelationRemoveRequest) *pb.RpcBlockDataviewViewRelationRemoveResponse {
	ctx := mw.newContext(cctx)
	resp := func(err error) *pb.RpcBlockDataviewViewRelationRemoveResponse {
		r := &pb.RpcBlockDataviewViewRelationRemoveResponse{
			Error: &pb.RpcBlockDataviewViewRelationRemoveResponseError{
				Code: pb.RpcBlockDataviewViewRelationRemoveResponseError_NULL,
			},
		}
		if err != nil {
			r.Error.Code = pb.RpcBlockDataviewViewRelationRemoveResponseError_UNKNOWN_ERROR
			r.Error.Description = err.Error()
		} else {
			r.Event = ctx.GetResponseEvent()
		}
		return r
	}

	err := mw.doBlockService(func(bs *block.Service) error {
		return bs.RemoveDataviewViewRelations(ctx, req.ContextId, req.BlockId, req.ViewId, req.RelationKeys)
	})

	return resp(err)
}

func (mw *Middleware) BlockDataviewViewRelationReplace(cctx context.Context, req *pb.RpcBlockDataviewViewRelationReplaceRequest) *pb.RpcBlockDataviewViewRelationReplaceResponse {
	ctx := mw.newContext(cctx)
	resp := func(err error) *pb.RpcBlockDataviewViewRelationReplaceResponse {
		r := &pb.RpcBlockDataviewViewRelationReplaceResponse{
			Error: &pb.RpcBlockDataviewViewRelationReplaceResponseError{
				Code: pb.RpcBlockDataviewViewRelationReplaceResponseError_NULL,
			},
		}
		if err != nil {
			r.Error.Code = pb.RpcBlockDataviewViewRelationReplaceResponseError_UNKNOWN_ERROR
			r.Error.Description = err.Error()
		} else {
			r.Event = ctx.GetResponseEvent()
		}
		return r
	}

	err := mw.doBlockService(func(bs *block.Service) error {
		return bs.ReplaceDataviewViewRelation(ctx, req.ContextId, req.BlockId, req.ViewId, req.RelationKey, req.Relation)
	})

	return resp(err)
}

func (mw *Middleware) BlockDataviewViewRelationSort(cctx context.Context, req *pb.RpcBlockDataviewViewRelationSortRequest) *pb.RpcBlockDataviewViewRelationSortResponse {
	ctx := mw.newContext(cctx)
	resp := func(err error) *pb.RpcBlockDataviewViewRelationSortResponse {
		r := &pb.RpcBlockDataviewViewRelationSortResponse{
			Error: &pb.RpcBlockDataviewViewRelationSortResponseError{
				Code: pb.RpcBlockDataviewViewRelationSortResponseError_NULL,
			},
		}
		if err != nil {
			r.Error.Code = pb.RpcBlockDataviewViewRelationSortResponseError_UNKNOWN_ERROR
			r.Error.Description = err.Error()
		} else {
			r.Event = ctx.GetResponseEvent()
		}
		return r
	}

	err := mw.doBlockService(func(bs *block.Service) error {
		return bs.ReorderDataviewViewRelations(ctx, req.ContextId, req.BlockId, req.ViewId, req.RelationKeys)
	})

	return resp(err)
}
