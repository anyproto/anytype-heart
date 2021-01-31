package core

import (
	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/pb"
	pbrelation "github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/relation"
	"github.com/gogo/protobuf/types"
)

func (mw *Middleware) BlockDataviewRelationListAvailable(req *pb.RpcBlockDataviewRelationListAvailableRequest) *pb.RpcBlockDataviewRelationListAvailableResponse {
	response := func(code pb.RpcBlockDataviewRelationListAvailableResponseErrorCode, relations []*pbrelation.Relation, err error) *pb.RpcBlockDataviewRelationListAvailableResponse {
		m := &pb.RpcBlockDataviewRelationListAvailableResponse{Relations: relations, Error: &pb.RpcBlockDataviewRelationListAvailableResponseError{Code: code}}
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
		relations, err = bs.GetAggregatedRelations(*req)
		return
	})
	if err != nil {
		return response(pb.RpcBlockDataviewRelationListAvailableResponseError_BAD_INPUT, relations, err)
	}

	return response(pb.RpcBlockDataviewRelationListAvailableResponseError_NULL, relations, nil)
}

func (mw *Middleware) BlockDataviewViewUpdate(req *pb.RpcBlockDataviewViewUpdateRequest) *pb.RpcBlockDataviewViewUpdateResponse {
	ctx := state.NewContext(nil)
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

func (mw *Middleware) BlockDataviewViewCreate(req *pb.RpcBlockDataviewViewCreateRequest) *pb.RpcBlockDataviewViewCreateResponse {
	ctx := state.NewContext(nil)
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

func (mw *Middleware) BlockDataviewViewDelete(req *pb.RpcBlockDataviewViewDeleteRequest) *pb.RpcBlockDataviewViewDeleteResponse {
	ctx := state.NewContext(nil)
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

func (mw *Middleware) BlockDataviewViewSetActive(req *pb.RpcBlockDataviewViewSetActiveRequest) *pb.RpcBlockDataviewViewSetActiveResponse {
	ctx := state.NewContext(nil)
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

func (mw *Middleware) BlockDataviewRecordCreate(req *pb.RpcBlockDataviewRecordCreateRequest) *pb.RpcBlockDataviewRecordCreateResponse {
	ctx := state.NewContext(nil)
	response := func(details *types.Struct, code pb.RpcBlockDataviewRecordCreateResponseErrorCode, err error) *pb.RpcBlockDataviewRecordCreateResponse {
		m := &pb.RpcBlockDataviewRecordCreateResponse{Record: details, Error: &pb.RpcBlockDataviewRecordCreateResponseError{Code: code}}
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
		return response(nil, pb.RpcBlockDataviewRecordCreateResponseError_UNKNOWN_ERROR, err)
	}

	return response(details, pb.RpcBlockDataviewRecordCreateResponseError_NULL, nil)
}

func (mw *Middleware) BlockDataviewRecordUpdate(req *pb.RpcBlockDataviewRecordUpdateRequest) *pb.RpcBlockDataviewRecordUpdateResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockDataviewRecordUpdateResponseErrorCode, err error) *pb.RpcBlockDataviewRecordUpdateResponse {
		m := &pb.RpcBlockDataviewRecordUpdateResponse{Error: &pb.RpcBlockDataviewRecordUpdateResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}
		// no events generated
		return m
	}

	if err := mw.doBlockService(func(bs block.Service) (err error) {
		return bs.UpdateDataviewRecord(ctx, *req)
	}); err != nil {
		return response(pb.RpcBlockDataviewRecordUpdateResponseError_UNKNOWN_ERROR, err)
	}

	return response(pb.RpcBlockDataviewRecordUpdateResponseError_NULL, nil)
}

func (mw *Middleware) BlockDataviewRecordDelete(req *pb.RpcBlockDataviewRecordDeleteRequest) *pb.RpcBlockDataviewRecordDeleteResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockDataviewRecordDeleteResponseErrorCode, err error) *pb.RpcBlockDataviewRecordDeleteResponse {
		m := &pb.RpcBlockDataviewRecordDeleteResponse{Error: &pb.RpcBlockDataviewRecordDeleteResponseError{Code: code}}
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
		return response(pb.RpcBlockDataviewRecordDeleteResponseError_UNKNOWN_ERROR, err)
	}

	return response(pb.RpcBlockDataviewRecordDeleteResponseError_NULL, nil)
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

func (mw *Middleware) BlockDataviewRelationUpdate(req *pb.RpcBlockDataviewRelationUpdateRequest) *pb.RpcBlockDataviewRelationUpdateResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockDataviewRelationUpdateResponseErrorCode, err error) *pb.RpcBlockDataviewRelationUpdateResponse {
		m := &pb.RpcBlockDataviewRelationUpdateResponse{Error: &pb.RpcBlockDataviewRelationUpdateResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	err := mw.doBlockService(func(bs block.Service) (err error) {
		return bs.UpdateDataviewRelation(ctx, *req)
	})
	if err != nil {
		return response(pb.RpcBlockDataviewRelationUpdateResponseError_BAD_INPUT, err)
	}

	return response(pb.RpcBlockDataviewRelationUpdateResponseError_NULL, nil)
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

func (mw *Middleware) BlockDataviewRecordRelationOptionAdd(req *pb.RpcBlockDataviewRecordRelationOptionAddRequest) *pb.RpcBlockDataviewRecordRelationOptionAddResponse {
	ctx := state.NewContext(nil)
	response := func(opt *pbrelation.RelationOption, code pb.RpcBlockDataviewRecordRelationOptionAddResponseErrorCode, err error) *pb.RpcBlockDataviewRecordRelationOptionAddResponse {
		m := &pb.RpcBlockDataviewRecordRelationOptionAddResponse{Option: opt, Error: &pb.RpcBlockDataviewRecordRelationOptionAddResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	var opt *pbrelation.RelationOption
	err := mw.doBlockService(func(bs block.Service) (err error) {
		opt, err = bs.AddDataviewRecordRelationOption(ctx, *req)
		return err
	})
	if err != nil {
		return response(nil, pb.RpcBlockDataviewRecordRelationOptionAddResponseError_BAD_INPUT, err)
	}

	return response(opt, pb.RpcBlockDataviewRecordRelationOptionAddResponseError_NULL, nil)
}

func (mw *Middleware) BlockDataviewRecordRelationOptionUpdate(req *pb.RpcBlockDataviewRecordRelationOptionUpdateRequest) *pb.RpcBlockDataviewRecordRelationOptionUpdateResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockDataviewRecordRelationOptionUpdateResponseErrorCode, err error) *pb.RpcBlockDataviewRecordRelationOptionUpdateResponse {
		m := &pb.RpcBlockDataviewRecordRelationOptionUpdateResponse{Error: &pb.RpcBlockDataviewRecordRelationOptionUpdateResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	err := mw.doBlockService(func(bs block.Service) (err error) {
		err = bs.UpdateDataviewRecordRelationOption(ctx, *req)
		return err
	})
	if err != nil {
		return response(pb.RpcBlockDataviewRecordRelationOptionUpdateResponseError_BAD_INPUT, err)
	}

	return response(pb.RpcBlockDataviewRecordRelationOptionUpdateResponseError_NULL, nil)
}

func (mw *Middleware) BlockDataviewRecordRelationOptionDelete(req *pb.RpcBlockDataviewRecordRelationOptionDeleteRequest) *pb.RpcBlockDataviewRecordRelationOptionDeleteResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockDataviewRecordRelationOptionDeleteResponseErrorCode, err error) *pb.RpcBlockDataviewRecordRelationOptionDeleteResponse {
		m := &pb.RpcBlockDataviewRecordRelationOptionDeleteResponse{Error: &pb.RpcBlockDataviewRecordRelationOptionDeleteResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	err := mw.doBlockService(func(bs block.Service) (err error) {
		err = bs.DeleteDataviewRecordRelationOption(ctx, *req)
		return err
	})
	if err != nil {
		return response(pb.RpcBlockDataviewRecordRelationOptionDeleteResponseError_BAD_INPUT, err)
	}

	return response(pb.RpcBlockDataviewRecordRelationOptionDeleteResponseError_NULL, nil)
}
