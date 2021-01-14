package core

import (
	"fmt"

	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	pbrelation "github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/relation"
	"github.com/gogo/protobuf/types"
)

// To be renamed to ObjectSetDetails
func (mw *Middleware) BlockSetDetails(req *pb.RpcBlockSetDetailsRequest) *pb.RpcBlockSetDetailsResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockSetDetailsResponseErrorCode, err error) *pb.RpcBlockSetDetailsResponse {
		m := &pb.RpcBlockSetDetailsResponse{Error: &pb.RpcBlockSetDetailsResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	err := mw.doBlockService(func(bs block.Service) (err error) {
		return bs.SetDetails(ctx, *req)
	})
	if err != nil {
		return response(pb.RpcBlockSetDetailsResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcBlockSetDetailsResponseError_NULL, nil)
}

func (mw *Middleware) ObjectSearch(req *pb.RpcObjectSearchRequest) *pb.RpcObjectSearchResponse {
	response := func(code pb.RpcObjectSearchResponseErrorCode, records []*types.Struct, err error) *pb.RpcObjectSearchResponse {
		m := &pb.RpcObjectSearchResponse{Error: &pb.RpcObjectSearchResponseError{Code: code}, Records: records}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	if mw.Anytype == nil {
		return response(pb.RpcObjectSearchResponseError_BAD_INPUT, nil, fmt.Errorf("account must be started"))
	}

	records, _, err := mw.Anytype.ObjectStore().Query(nil, database.Query{
		Filters:  req.Filters,
		Sorts:    req.Sorts,
		Offset:   int(req.Offset),
		Limit:    int(req.Limit),
		FullText: req.FullText,
	})
	if err != nil {
		return response(pb.RpcObjectSearchResponseError_UNKNOWN_ERROR, nil, err)
	}

	var records2 []*types.Struct
	for _, rec := range records {
		records2 = append(records2, rec.Details)
	}

	return response(pb.RpcObjectSearchResponseError_NULL, records2, nil)
}

func (mw *Middleware) ObjectRelationAdd(req *pb.RpcObjectRelationAddRequest) *pb.RpcObjectRelationAddResponse {
	ctx := state.NewContext(nil)
	response := func(relationKey string, code pb.RpcObjectRelationAddResponseErrorCode, err error) *pb.RpcObjectRelationAddResponse {
		m := &pb.RpcObjectRelationAddResponse{RelationKey: relationKey, Error: &pb.RpcObjectRelationAddResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	var relations []*pbrelation.Relation
	err := mw.doBlockService(func(bs block.Service) (err error) {
		relations, err = bs.AddExtraRelations(req.ContextId, []*pbrelation.Relation{req.Relation})
		return err
	})
	if err != nil {
		return response("", pb.RpcObjectRelationAddResponseError_BAD_INPUT, err)
	}

	return response(relations[0].Key, pb.RpcObjectRelationAddResponseError_NULL, nil)
}

func (mw *Middleware) ObjectRelationUpdate(req *pb.RpcObjectRelationUpdateRequest) *pb.RpcObjectRelationUpdateResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcObjectRelationUpdateResponseErrorCode, err error) *pb.RpcObjectRelationUpdateResponse {
		m := &pb.RpcObjectRelationUpdateResponse{Error: &pb.RpcObjectRelationUpdateResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	err := mw.doBlockService(func(bs block.Service) (err error) {
		return bs.UpdateExtraRelations(req.ContextId, []*pbrelation.Relation{req.Relation}, false)
	})
	if err != nil {
		return response(pb.RpcObjectRelationUpdateResponseError_BAD_INPUT, err)
	}

	return response(pb.RpcObjectRelationUpdateResponseError_NULL, nil)
}

func (mw *Middleware) ObjectRelationDelete(req *pb.RpcObjectRelationDeleteRequest) *pb.RpcObjectRelationDeleteResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcObjectRelationDeleteResponseErrorCode, err error) *pb.RpcObjectRelationDeleteResponse {
		m := &pb.RpcObjectRelationDeleteResponse{Error: &pb.RpcObjectRelationDeleteResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	err := mw.doBlockService(func(bs block.Service) (err error) {
		return bs.RemoveExtraRelations(req.ContextId, []string{req.RelationKey})
	})
	if err != nil {
		return response(pb.RpcObjectRelationDeleteResponseError_BAD_INPUT, err)
	}
	return response(pb.RpcObjectRelationDeleteResponseError_NULL, nil)
}

func (mw *Middleware) ObjectRelationSelectOptionAdd(req *pb.RpcObjectRelationSelectOptionAddRequest) *pb.RpcObjectRelationSelectOptionAddResponse {
	ctx := state.NewContext(nil)
	response := func(opt *pbrelation.RelationSelectOption, code pb.RpcObjectRelationSelectOptionAddResponseErrorCode, err error) *pb.RpcObjectRelationSelectOptionAddResponse {
		m := &pb.RpcObjectRelationSelectOptionAddResponse{Option: opt, Error: &pb.RpcObjectRelationSelectOptionAddResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	var opt *pbrelation.RelationSelectOption
	err := mw.doBlockService(func(bs block.Service) (err error) {
		//opt, err = bs.UpdateExtraRelations(ctx, *req)
		return fmt.Errorf("not implemented")
	})
	if err != nil {
		return response(nil, pb.RpcObjectRelationSelectOptionAddResponseError_BAD_INPUT, err)
	}

	return response(opt, pb.RpcObjectRelationSelectOptionAddResponseError_NULL, nil)
}

func (mw *Middleware) ObjectRelationSelectOptionUpdate(req *pb.RpcObjectRelationSelectOptionUpdateRequest) *pb.RpcObjectRelationSelectOptionUpdateResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcObjectRelationSelectOptionUpdateResponseErrorCode, err error) *pb.RpcObjectRelationSelectOptionUpdateResponse {
		m := &pb.RpcObjectRelationSelectOptionUpdateResponse{Error: &pb.RpcObjectRelationSelectOptionUpdateResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	err := mw.doBlockService(func(bs block.Service) (err error) {
		//err = bs.UpdateDataviewRelationSelectOption(ctx, *req)
		return fmt.Errorf("not implemented")
	})
	if err != nil {
		return response(pb.RpcObjectRelationSelectOptionUpdateResponseError_BAD_INPUT, err)
	}

	return response(pb.RpcObjectRelationSelectOptionUpdateResponseError_NULL, nil)
}

func (mw *Middleware) ObjectRelationSelectOptionDelete(req *pb.RpcObjectRelationSelectOptionDeleteRequest) *pb.RpcObjectRelationSelectOptionDeleteResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcObjectRelationSelectOptionDeleteResponseErrorCode, err error) *pb.RpcObjectRelationSelectOptionDeleteResponse {
		m := &pb.RpcObjectRelationSelectOptionDeleteResponse{Error: &pb.RpcObjectRelationSelectOptionDeleteResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	err := mw.doBlockService(func(bs block.Service) (err error) {
		//err = bs.DeleteDataviewRelationSelectOption(ctx, *req)
		return fmt.Errorf("not implemented")
	})
	if err != nil {
		return response(pb.RpcObjectRelationSelectOptionDeleteResponseError_BAD_INPUT, err)
	}

	return response(pb.RpcObjectRelationSelectOptionDeleteResponseError_NULL, nil)
}

func (mw *Middleware) ObjectRelationListAvailable(req *pb.RpcObjectRelationListAvailableRequest) *pb.RpcObjectRelationListAvailableResponse {
	response := func(code pb.RpcObjectRelationListAvailableResponseErrorCode, relations []*pbrelation.Relation, err error) *pb.RpcObjectRelationListAvailableResponse {
		m := &pb.RpcObjectRelationListAvailableResponse{Relations: relations, Error: &pb.RpcObjectRelationListAvailableResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}
	// todo: to be implemented
	return response(pb.RpcObjectRelationListAvailableResponseError_UNKNOWN_ERROR, nil, fmt.Errorf("not implemented"))
	/*ctx := state.NewContext(nil)
	response := func(code pb.RpcObjectRelationListAvailableResponseErrorCode, relations []*pbrelation.ByRelation, err error) *pb.RpcObjectRelationListAvailableResponse {
		m := &pb.RpcObjectRelationListAvailableResponse{Relations: relations, Error: &pb.RpcObjectRelationListAvailableResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}
	var (
		err       error
		relations []*pbrelation.ByRelation
	)

	err = mw.doBlockService(func(bs block.Service) (err error) {
		bs.GetRelations()
	})
	if err != nil {
		return response(pb.RpcObjectRelationListAvailableResponseError_BAD_INPUT, relations, err)
	}

	return response(pb.RpcObjectRelationListAvailableResponseError_NULL, relations, nil)
	*/
}
