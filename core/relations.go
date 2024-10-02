package core

import (
	"context"
	"errors"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/detailservice"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
)

func (mw *Middleware) ObjectTypeRelationAdd(cctx context.Context, req *pb.RpcObjectTypeRelationAddRequest) *pb.RpcObjectTypeRelationAddResponse {
	detailsService := getService[detailservice.Service](mw)
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
	detailsService := getService[detailservice.Service](mw)
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

func (mw *Middleware) RelationListWithValue(_ context.Context, req *pb.RpcRelationListWithValueRequest) *pb.RpcRelationListWithValueResponse {
	response := func(keys []string, counters []int64, err error) *pb.RpcRelationListWithValueResponse {
		m := &pb.RpcRelationListWithValueResponse{Error: &pb.RpcRelationListWithValueResponseError{Code: pb.RpcRelationListWithValueResponseError_NULL}}
		if err != nil {
			m.Error.Description = getErrorDescription(err)
		} else {
			m.RelationKeys = keys
			m.Counters = counters
		}
		return m
	}

	keys, counters, err := getService[detailservice.Service](mw).ListRelationsWithValue(req.SpaceId, req.Value)
	return response(keys, counters, err)
}
