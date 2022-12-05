package core

import (
	"context"
	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
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

	err = mw.doBlockService(func(bs block.Service) (err error) {
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
	err := mw.doBlockService(func(bs block.Service) (err error) {
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
	err := mw.doBlockService(func(bs block.Service) (err error) {
		return bs.UpdateDataviewObjectOrder(ctx, *req)
	})
	if err != nil {
		return response(pb.RpcBlockDataviewObjectOrderUpdateResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcBlockDataviewObjectOrderUpdateResponseError_NULL, nil)
}

func (mw *Middleware) BlockDataviewCreateWithObject(cctx context.Context, req *pb.RpcBlockDataviewCreateWithObjectRequest) *pb.RpcBlockDataviewCreateWithObjectResponse {
	ctx := mw.newContext(cctx)
	response := func(code pb.RpcBlockDataviewCreateWithObjectResponseErrorCode, id, targetObjectId string, err error) *pb.RpcBlockDataviewCreateWithObjectResponse {
		m := &pb.RpcBlockDataviewCreateWithObjectResponse{Error: &pb.RpcBlockDataviewCreateWithObjectResponseError{Code: code}, BlockId: id}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}

	setId, _, err := mw.objectCreateSet(&pb.RpcObjectCreateSetRequest{
		Details:       req.Details,
		InternalFlags: req.InternalFlags,
		Source:        pbtypes.GetStringList(req.Details, bundle.RelationKeySetOf.String()),
	})

	if req.Block != nil && req.Block.Content != nil {
		if dvContent, ok := req.Block.Content.(*model.BlockContentOfDataview); ok {
			dvContent.Dataview.TargetObjectId = setId
		}
	}

	var blockId string
	err = mw.doBlockService(func(bs block.Service) (err error) {
		blockId, err = bs.CreateBlock(ctx, pb.RpcBlockCreateRequest{
			ContextId: req.ContextId,
			TargetId:  req.TargetId,
			Block:     req.Block,
			Position:  req.Position,
		})
		return
	})

	if err != nil {
		return response(pb.RpcBlockDataviewCreateWithObjectResponseError_UNKNOWN_ERROR, "", "", err)
	}
	return response(pb.RpcBlockDataviewCreateWithObjectResponseError_NULL, blockId, setId, nil)
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
	err := mw.doBlockService(func(bs block.Service) (err error) {
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
	err := mw.doBlockService(func(bs block.Service) (err error) {
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
	err := mw.doBlockService(func(bs block.Service) (err error) {
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
	err := mw.doBlockService(func(bs block.Service) (err error) {
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
	err := mw.doBlockService(func(bs block.Service) (err error) {
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
	err := mw.doBlockService(func(bs block.Service) (err error) {
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
	err := mw.doBlockService(func(bs block.Service) (err error) {
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

	err := mw.doBlockService(func(bs block.Service) error {
		return bs.SetDataviewSource(ctx, req.ContextId, req.BlockId, req.Source)
	})

	return resp(err)
}
