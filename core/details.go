package core

import (
	"context"
	"fmt"

	"github.com/anyproto/anytype-heart/core/block/detailservice"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/internalflag"
)

func (mw *Middleware) ObjectSetDetails(cctx context.Context, req *pb.RpcObjectSetDetailsRequest) *pb.RpcObjectSetDetailsResponse {
	ctx := mw.newContext(cctx)
	response := func(code pb.RpcObjectSetDetailsResponseErrorCode, err error) *pb.RpcObjectSetDetailsResponse {
		m := &pb.RpcObjectSetDetailsResponse{Error: &pb.RpcObjectSetDetailsResponseError{Code: code}}
		if err != nil {
			m.Error.Description = getErrorDescription(err)
		} else {
			m.Event = mw.getResponseEvent(ctx)
		}
		return m
	}

	err := mustService[detailservice.Service](mw).SetDetails(ctx, req.ContextId, requestDetailsListToDomain(req.GetDetails()))
	if err != nil {
		return response(pb.RpcObjectSetDetailsResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcObjectSetDetailsResponseError_NULL, nil)
}

func (mw *Middleware) ObjectListSetDetails(cctx context.Context, req *pb.RpcObjectListSetDetailsRequest) *pb.RpcObjectListSetDetailsResponse {
	ctx := mw.newContext(cctx)
	response := func(code pb.RpcObjectListSetDetailsResponseErrorCode, err error) *pb.RpcObjectListSetDetailsResponse {
		m := &pb.RpcObjectListSetDetailsResponse{Error: &pb.RpcObjectListSetDetailsResponseError{Code: code}}
		if err != nil {
			m.Error.Description = getErrorDescription(err)
		} else {
			m.Event = mw.getResponseEvent(ctx)
		}
		return m
	}

	err := mustService[detailservice.Service](mw).SetDetailsList(ctx, req.ObjectIds, requestDetailsListToDomain(req.Details))
	if err != nil {
		return response(pb.RpcObjectListSetDetailsResponseError_UNKNOWN_ERROR, err)
	}

	return response(pb.RpcObjectListSetDetailsResponseError_NULL, nil)
}

func (mw *Middleware) ObjectSetInternalFlags(cctx context.Context, req *pb.RpcObjectSetInternalFlagsRequest) *pb.RpcObjectSetInternalFlagsResponse {
	ctx := mw.newContext(cctx)
	response := func(code pb.RpcObjectSetInternalFlagsResponseErrorCode, err error) *pb.RpcObjectSetInternalFlagsResponse {
		m := &pb.RpcObjectSetInternalFlagsResponse{Error: &pb.RpcObjectSetInternalFlagsResponseError{Code: code}}
		if err != nil {
			m.Error.Description = getErrorDescription(err)
		} else {
			m.Event = mw.getResponseEvent(ctx)
		}
		return m
	}
	ds := mustService[detailservice.Service](mw)
	err := ds.ModifyDetails(ctx, req.ContextId, func(current *domain.Details) (*domain.Details, error) {
		d := current.Copy()
		return internalflag.PutToDetails(d, req.InternalFlags), nil
	})
	if err != nil {
		return response(pb.RpcObjectSetInternalFlagsResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcObjectSetInternalFlagsResponseError_NULL, nil)
}

func (mw *Middleware) ObjectListModifyDetailValues(_ context.Context, req *pb.RpcObjectListModifyDetailValuesRequest) *pb.RpcObjectListModifyDetailValuesResponse {
	response := func(code pb.RpcObjectListModifyDetailValuesResponseErrorCode, err error) *pb.RpcObjectListModifyDetailValuesResponse {
		m := &pb.RpcObjectListModifyDetailValuesResponse{Error: &pb.RpcObjectListModifyDetailValuesResponseError{Code: code}}
		if err != nil {
			m.Error.Description = getErrorDescription(err)
		}
		return m
	}
	err := mustService[detailservice.Service](mw).ModifyDetailsList(req)
	if err != nil {
		return response(pb.RpcObjectListModifyDetailValuesResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcObjectListModifyDetailValuesResponseError_NULL, nil)
}

func (mw *Middleware) ObjectWorkspaceSetDashboard(cctx context.Context, req *pb.RpcObjectWorkspaceSetDashboardRequest) *pb.RpcObjectWorkspaceSetDashboardResponse {
	ctx := mw.newContext(cctx)
	response := func(setId string, err error) *pb.RpcObjectWorkspaceSetDashboardResponse {
		resp := &pb.RpcObjectWorkspaceSetDashboardResponse{
			ObjectId: setId,
			Error: &pb.RpcObjectWorkspaceSetDashboardResponseError{
				Code: pb.RpcObjectWorkspaceSetDashboardResponseError_NULL,
			},
		}
		if err != nil {
			resp.Error.Code = pb.RpcObjectWorkspaceSetDashboardResponseError_UNKNOWN_ERROR
			resp.Error.Description = getErrorDescription(err)
		} else {
			resp.Event = mw.getResponseEvent(ctx)
		}
		return resp
	}
	setId, err := mustService[detailservice.Service](mw).SetWorkspaceDashboardId(ctx, req.ContextId, req.ObjectId)
	return response(setId, err)
}

func (mw *Middleware) ObjectSetIsFavorite(_ context.Context, req *pb.RpcObjectSetIsFavoriteRequest) *pb.RpcObjectSetIsFavoriteResponse {
	response := func(code pb.RpcObjectSetIsFavoriteResponseErrorCode, err error) *pb.RpcObjectSetIsFavoriteResponse {
		m := &pb.RpcObjectSetIsFavoriteResponse{Error: &pb.RpcObjectSetIsFavoriteResponseError{Code: code}}
		if err != nil {
			m.Error.Description = getErrorDescription(err)
		}
		return m
	}
	err := mustService[detailservice.Service](mw).SetIsFavorite(req.ContextId, req.IsFavorite)
	if err != nil {
		return response(pb.RpcObjectSetIsFavoriteResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcObjectSetIsFavoriteResponseError_NULL, nil)
}

func (mw *Middleware) ObjectSetIsArchived(cctx context.Context, req *pb.RpcObjectSetIsArchivedRequest) *pb.RpcObjectSetIsArchivedResponse {
	response := func(code pb.RpcObjectSetIsArchivedResponseErrorCode, err error) *pb.RpcObjectSetIsArchivedResponse {
		m := &pb.RpcObjectSetIsArchivedResponse{Error: &pb.RpcObjectSetIsArchivedResponseError{Code: code}}
		if err != nil {
			m.Error.Description = getErrorDescription(err)
		}
		return m
	}
	err := mustService[detailservice.Service](mw).SetIsArchived(cctx, req.ContextId, req.IsArchived)
	if err != nil {
		return response(pb.RpcObjectSetIsArchivedResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcObjectSetIsArchivedResponseError_NULL, nil)
}

func (mw *Middleware) ObjectListSetIsArchived(cctx context.Context, req *pb.RpcObjectListSetIsArchivedRequest) *pb.RpcObjectListSetIsArchivedResponse {
	response := func(code pb.RpcObjectListSetIsArchivedResponseErrorCode, err error) *pb.RpcObjectListSetIsArchivedResponse {
		m := &pb.RpcObjectListSetIsArchivedResponse{Error: &pb.RpcObjectListSetIsArchivedResponseError{Code: code}}
		if err != nil {
			m.Error.Description = getErrorDescription(err)
		}
		return m
	}
	err := mustService[detailservice.Service](mw).SetListIsArchived(cctx, req.ObjectIds, req.IsArchived)
	if err != nil {
		return response(pb.RpcObjectListSetIsArchivedResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcObjectListSetIsArchivedResponseError_NULL, nil)
}

func (mw *Middleware) ObjectListSetIsFavorite(_ context.Context, req *pb.RpcObjectListSetIsFavoriteRequest) *pb.RpcObjectListSetIsFavoriteResponse {
	response := func(code pb.RpcObjectListSetIsFavoriteResponseErrorCode, err error) *pb.RpcObjectListSetIsFavoriteResponse {
		m := &pb.RpcObjectListSetIsFavoriteResponse{Error: &pb.RpcObjectListSetIsFavoriteResponseError{Code: code}}
		if err != nil {
			m.Error.Description = getErrorDescription(err)
		}
		return m
	}
	err := mustService[detailservice.Service](mw).SetListIsFavorite(req.ObjectIds, req.IsFavorite)
	if err != nil {
		return response(pb.RpcObjectListSetIsFavoriteResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcObjectListSetIsFavoriteResponseError_NULL, nil)
}

func (mw *Middleware) ObjectRelationAdd(cctx context.Context, req *pb.RpcObjectRelationAddRequest) *pb.RpcObjectRelationAddResponse {
	ctx := mw.newContext(cctx)
	if len(req.RelationKeys) == 0 {
		return &pb.RpcObjectRelationAddResponse{Error: &pb.RpcObjectRelationAddResponseError{
			Code:        pb.RpcObjectRelationAddResponseError_BAD_INPUT,
			Description: fmt.Errorf("relation keys list is empty").Error(),
		}}
	}

	detailsService := mustService[detailservice.Service](mw)
	objectStore := mustService[objectstore.ObjectStore](mw)
	err := detailsService.ModifyDetails(ctx, req.ContextId, func(current *domain.Details) (*domain.Details, error) {
		for _, key := range req.RelationKeys {
			if current.Has(domain.RelationKey(key)) {
				continue
			}
			format, err := mw.extractRelationFormat(current, objectStore, key)
			if err != nil {
				log.Errorf("failed to fetch relation from store to get format %s, falling back to basic", err)
			}
			switch format {
			case model.RelationFormat_checkbox:
				current.Set(domain.RelationKey(key), domain.Bool(false))
			default:
				current.Set(domain.RelationKey(key), domain.Null())
			}
		}
		return current, nil
	})
	if err != nil {
		return &pb.RpcObjectRelationAddResponse{Error: &pb.RpcObjectRelationAddResponseError{
			Code:        pb.RpcObjectRelationAddResponseError_BAD_INPUT,
			Description: getErrorDescription(err),
		}}
	}

	return &pb.RpcObjectRelationAddResponse{
		Error: &pb.RpcObjectRelationAddResponseError{},
		Event: mw.getResponseEvent(ctx),
	}
}

func (mw *Middleware) extractRelationFormat(current *domain.Details, objectStore objectstore.ObjectStore, key string) (model.RelationFormat, error) {
	spaceId := current.GetString(bundle.RelationKeySpaceId)
	relation, err := objectStore.SpaceIndex(spaceId).FetchRelationByKeys(domain.RelationKey(key))
	if err != nil {
		return model.RelationFormat_longtext, err
	}
	var format model.RelationFormat
	if len(relation) != 0 {
		format = relation[0].Format
	}
	return format, nil
}
