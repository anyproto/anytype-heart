package core

import (
	"context"
	"errors"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
)

func (mw *Middleware) ObjectTypeRelationAdd(cctx context.Context, req *pb.RpcObjectTypeRelationAddRequest) *pb.RpcObjectTypeRelationAddResponse {
	blockService := getService[*block.Service](mw)
	keys := make([]domain.RelationKey, 0, len(req.RelationKeys))
	for _, relKey := range req.RelationKeys {
		keys = append(keys, domain.RelationKey(relKey))
	}

	err := blockService.ObjectTypeRelationAdd(cctx, req.ObjectTypeUrl, keys)
	code := mapErrorCode(err,
		errToCode(block.ErrBundledTypeIsReadonly, pb.RpcObjectTypeRelationAddResponseError_READONLY_OBJECT_TYPE),
	)
	return &pb.RpcObjectTypeRelationAddResponse{
		Error: &pb.RpcObjectTypeRelationAddResponseError{
			Code:        code,
			Description: getErrorDescription(err),
		},
	}
}

func (mw *Middleware) ObjectTypeRelationRemove(cctx context.Context, req *pb.RpcObjectTypeRelationRemoveRequest) *pb.RpcObjectTypeRelationRemoveResponse {
	blockService := getService[*block.Service](mw)
	keys := make([]domain.RelationKey, 0, len(req.RelationKeys))
	for _, relKey := range req.RelationKeys {
		keys = append(keys, domain.RelationKey(relKey))
	}

	err := blockService.ObjectTypeRemoveRelations(cctx, req.ObjectTypeUrl, keys)
	code := mapErrorCode(err,
		errToCode(block.ErrBundledTypeIsReadonly, pb.RpcObjectTypeRelationRemoveResponseError_READONLY_OBJECT_TYPE),
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
					Description: err.Error(),
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

func (mw *Middleware) RelationMoveOption(cctx context.Context, request *pb.RpcRelationMoveOptionRequest) *pb.RpcRelationMoveOptionResponse {
	response := func(code pb.RpcRelationMoveOptionResponseErrorCode, err error) *pb.RpcRelationMoveOptionResponse {
		if err != nil {
			return &pb.RpcRelationMoveOptionResponse{
				Error: &pb.RpcRelationMoveOptionResponseError{
					Code:        code,
					Description: err.Error(),
				},
			}
		}

		return &pb.RpcRelationMoveOptionResponse{
			Error: &pb.RpcRelationMoveOptionResponseError{
				Code: code,
			},
		}
	}

	err := mw.doBlockService(func(bs *block.Service) error {
		var err error
		err = bs.MoveOption(request.OptionId, request.AfterId, request.BeforeId)
		return err
	})
	if err != nil {
		return response(pb.RpcRelationMoveOptionResponseError_UNKNOWN_ERROR, err)
	}

	return response(pb.RpcRelationMoveOptionResponseError_NULL, nil)
}

func (mw *Middleware) RelationOptions(cctx context.Context, request *pb.RpcRelationOptionsRequest) *pb.RpcRelationOptionsResponse {
	// TODO implement me
	panic("implement me")
}
