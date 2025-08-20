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

func (mw *Middleware) ObjectTypeRelationAdd(cctx context.Context, req *pb.RpcObjectTypeRelationAddRequest) *pb.RpcObjectTypeRelationAddResponse {
	detailsService := mustService[detailservice.Service](mw)
	keys := make([]domain.RelationKey, 0, len(req.RelationKeys))
	for _, relKey := range req.RelationKeys {
		keys = append(keys, domain.RelationKey(relKey))
	}

	err := detailsService.ObjectTypeAddRelations(cctx, req.ObjectTypeUrl, keys)
	code := mapErrorCode(err,
		errToCode(detailservice.ErrBundledTypeIsReadonly, pb.RpcObjectTypeRelationAddResponseError_READONLY_OBJECT_TYPE),
	)
	return &pb.RpcObjectTypeRelationAddResponse{
		Error: &pb.RpcObjectTypeRelationAddResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) ObjectTypeRelationRemove(cctx context.Context, req *pb.RpcObjectTypeRelationRemoveRequest) *pb.RpcObjectTypeRelationRemoveResponse {
	detailsService := mustService[detailservice.Service](mw)
	keys := make([]domain.RelationKey, 0, len(req.RelationKeys))
	for _, relKey := range req.RelationKeys {
		keys = append(keys, domain.RelationKey(relKey))
	}

	err := detailsService.ObjectTypeRemoveRelations(cctx, req.ObjectTypeUrl, keys)
	code := mapErrorCode(err,
		errToCode(detailservice.ErrBundledTypeIsReadonly, pb.RpcObjectTypeRelationRemoveResponseError_READONLY_OBJECT_TYPE),
	)
	return &pb.RpcObjectTypeRelationRemoveResponse{
		Error: &pb.RpcObjectTypeRelationRemoveResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
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

func (mw *Middleware) RelationOptionUnsetOrder(_ context.Context, req *pb.RpcRelationOptionUnsetOrderRequest) *pb.RpcRelationOptionUnsetOrderResponse {
	err := mustService[order.OrderSetter](mw).UnsetOrder(req.RelationOptionId)
	return &pb.RpcRelationOptionUnsetOrderResponse{
		Error: &pb.RpcRelationOptionUnsetOrderResponseError{
			Code:        mapErrorCode[pb.RpcRelationOptionUnsetOrderResponseErrorCode](err),
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
