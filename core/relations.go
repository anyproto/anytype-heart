package core

import (
	"context"
	"errors"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/detailservice"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/order"
	"github.com/anyproto/anytype-heart/pb"
)

func (mw *Middleware) ObjectTypePropertyAdd(cctx context.Context, req *pb.RpcObjectTypePropertyAddRequest) *pb.RpcObjectTypePropertyAddResponse {
	detailsService := mustService[detailservice.Service](mw)
	res, err := detailsService.ObjectTypePropertyAdd(cctx, detailservice.ObjectTypePropertyAddRequest{
		ObjectTypeId:  req.ObjectTypeId,
		Key:           domain.RelationKey(req.Key),
		Name:          req.Name,
		Format:        req.Format,
		Section:       detailservice.TypePropertySection(req.Section),
		EnableInViews: req.EnableInViews,
	})
	code := mapErrorCode(err,
		errToCode(detailservice.ErrBundledTypeIsReadonly, pb.RpcObjectTypePropertyAddResponseError_READONLY_OBJECT_TYPE),
		errToCode(detailservice.ErrTypePropertyBadInput, pb.RpcObjectTypePropertyAddResponseError_BAD_INPUT),
	)
	return &pb.RpcObjectTypePropertyAddResponse{
		Error: &pb.RpcObjectTypePropertyAddResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
		Key:        res.Key.String(),
		PropertyId: res.PropertyId,
		ViewIds:    res.ViewIds,
	}
}

func (mw *Middleware) ObjectTypePropertyRemove(cctx context.Context, req *pb.RpcObjectTypePropertyRemoveRequest) *pb.RpcObjectTypePropertyRemoveResponse {
	detailsService := mustService[detailservice.Service](mw)
	res, err := detailsService.ObjectTypePropertyRemove(cctx, req.ObjectTypeId, domain.RelationKey(req.Key))
	code := mapErrorCode(err,
		errToCode(detailservice.ErrBundledTypeIsReadonly, pb.RpcObjectTypePropertyRemoveResponseError_READONLY_OBJECT_TYPE),
		errToCode(detailservice.ErrTypePropertyBadInput, pb.RpcObjectTypePropertyRemoveResponseError_BAD_INPUT),
	)
	return &pb.RpcObjectTypePropertyRemoveResponse{
		Error: &pb.RpcObjectTypePropertyRemoveResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
		InUseViewIds: res.InUseViewIds,
	}
}

func (mw *Middleware) ObjectTypeRecommendedRelationsSet(_ context.Context, req *pb.RpcObjectTypeRecommendedRelationsSetRequest) *pb.RpcObjectTypeRecommendedRelationsSetResponse {
	detailsService := mustService[detailservice.Service](mw)
	err := detailsService.ObjectTypeSetRelations(req.TypeObjectId, req.RelationObjectIds)
	code := mapErrorCode(err,
		errToCode(detailservice.ErrBundledTypeIsReadonly, pb.RpcObjectTypeRecommendedRelationsSetResponseError_READONLY_OBJECT_TYPE),
	)
	return &pb.RpcObjectTypeRecommendedRelationsSetResponse{
		Error: &pb.RpcObjectTypeRecommendedRelationsSetResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) ObjectTypeRecommendedFeaturedRelationsSet(_ context.Context, req *pb.RpcObjectTypeRecommendedFeaturedRelationsSetRequest) *pb.RpcObjectTypeRecommendedFeaturedRelationsSetResponse {
	detailsService := mustService[detailservice.Service](mw)
	err := detailsService.ObjectTypeSetFeaturedRelations(req.TypeObjectId, req.RelationObjectIds)
	code := mapErrorCode(err,
		errToCode(detailservice.ErrBundledTypeIsReadonly, pb.RpcObjectTypeRecommendedFeaturedRelationsSetResponseError_READONLY_OBJECT_TYPE),
	)
	return &pb.RpcObjectTypeRecommendedFeaturedRelationsSetResponse{
		Error: &pb.RpcObjectTypeRecommendedFeaturedRelationsSetResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) RelationListRemoveOption(cctx context.Context, request *pb.RpcRelationListRemoveOptionRequest) *pb.RpcRelationListRemoveOptionResponse {
	response := func(code pb.RpcRelationListRemoveOptionResponseErrorCode, err error) *pb.RpcRelationListRemoveOptionResponse {
		if err != nil {
			return &pb.RpcRelationListRemoveOptionResponse{
				Error: &pb.RpcRelationListRemoveOptionResponseError{
					Code:        code,
					Description: getErrorDescription(err),
				},
			}
		}

		return &pb.RpcRelationListRemoveOptionResponse{
			Error: &pb.RpcRelationListRemoveOptionResponseError{
				Code: code,
			},
		}
	}

	err := mw.doBlockService(func(bs *block.Service) error {
		var err error
		err = bs.RemoveListOption(request.OptionIds, request.CheckInObjects)
		return err
	})
	if err != nil {
		if errors.Is(err, block.ErrOptionUsedByOtherObjects) {
			return response(pb.RpcRelationListRemoveOptionResponseError_OPTION_USED_BY_OBJECTS, err)
		}
		return response(pb.RpcRelationListRemoveOptionResponseError_UNKNOWN_ERROR, err)
	}

	return response(pb.RpcRelationListRemoveOptionResponseError_NULL, nil)
}

func (mw *Middleware) RelationOptions(_ context.Context, _ *pb.RpcRelationOptionsRequest) *pb.RpcRelationOptionsResponse {
	// TODO implement me
	panic("implement me")
}

func (mw *Middleware) RelationOptionSetOrder(_ context.Context, req *pb.RpcRelationOptionSetOrderRequest) *pb.RpcRelationOptionSetOrderResponse {
	orderIds, err := mustService[order.OrderSetter](mw).SetOptionsOrder(req.SpaceId, domain.RelationKey(req.RelationKey), req.RelationOptionOrder)
	return &pb.RpcRelationOptionSetOrderResponse{
		RelationOptionOrder: orderIds,
		Error: &pb.RpcRelationOptionSetOrderResponseError{
			Code:        mapErrorCode[pb.RpcRelationOptionSetOrderResponseErrorCode](err),
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) RelationListWithValue(_ context.Context, req *pb.RpcRelationListWithValueRequest) *pb.RpcRelationListWithValueResponse {
	response := func(list []*pb.RpcRelationListWithValueResponseResponseItem, err error) *pb.RpcRelationListWithValueResponse {
		m := &pb.RpcRelationListWithValueResponse{Error: &pb.RpcRelationListWithValueResponseError{Code: pb.RpcRelationListWithValueResponseError_NULL}}
		if err != nil {
			m.Error.Description = getErrorDescription(err)
		} else {
			m.List = list
		}
		return m
	}

	list, err := mustService[detailservice.Service](mw).ListRelationsWithValue(req.SpaceId, domain.ValueFromProto(req.Value))
	return response(list, err)
}

func (mw *Middleware) ObjectTypeListConflictingRelations(_ context.Context, req *pb.RpcObjectTypeListConflictingRelationsRequest) *pb.RpcObjectTypeListConflictingRelationsResponse {
	detailsService := mustService[detailservice.Service](mw)
	conflictingRelations, err := detailsService.ObjectTypeListConflictingRelations(req.SpaceId, req.TypeObjectId)
	code := mapErrorCode(err,
		errToCode(detailservice.ErrBundledTypeIsReadonly, pb.RpcObjectTypeListConflictingRelationsResponseError_READONLY_OBJECT_TYPE),
	)
	return &pb.RpcObjectTypeListConflictingRelationsResponse{
		Error: &pb.RpcObjectTypeListConflictingRelationsResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
		RelationIds: conflictingRelations,
	}
}

func (mw *Middleware) ObjectTypeResolveLayoutConflicts(_ context.Context, req *pb.RpcObjectTypeResolveLayoutConflictsRequest) *pb.RpcObjectTypeResolveLayoutConflictsResponse {
	code := pb.RpcObjectTypeResolveLayoutConflictsResponseError_NULL
	err := mw.doBlockService(func(bs *block.Service) error {
		return bs.SyncObjectsWithType(req.TypeObjectId)
	})
	if err != nil {
		code = pb.RpcObjectTypeResolveLayoutConflictsResponseError_UNKNOWN_ERROR
	}
	return &pb.RpcObjectTypeResolveLayoutConflictsResponse{
		Error: &pb.RpcObjectTypeResolveLayoutConflictsResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) ObjectTypeSetOrder(cctx context.Context, req *pb.RpcObjectTypeSetOrderRequest) *pb.RpcObjectTypeSetOrderResponse {
	orderService := mustService[order.OrderSetter](mw)
	orderIds, err := orderService.SetObjectTypesOrder(req.SpaceId, req.TypeIds)

	code := mapErrorCode[pb.RpcObjectTypeSetOrderResponseErrorCode](err)

	return &pb.RpcObjectTypeSetOrderResponse{
		OrderIds: orderIds,
		Error: &pb.RpcObjectTypeSetOrderResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}
