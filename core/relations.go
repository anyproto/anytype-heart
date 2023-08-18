package core

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/anyproto/any-sync/app"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
)

func (mw *Middleware) ObjectTypeRelationList(cctx context.Context, req *pb.RpcObjectTypeRelationListRequest) *pb.RpcObjectTypeRelationListResponse {
	response := func(code pb.RpcObjectTypeRelationListResponseErrorCode, relations []*model.RelationLink, err error) *pb.RpcObjectTypeRelationListResponse {
		m := &pb.RpcObjectTypeRelationListResponse{Relations: relations, Error: &pb.RpcObjectTypeRelationListResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}
	at := mw.GetAnytype()
	if at == nil {
		return response(pb.RpcObjectTypeRelationListResponseError_BAD_INPUT, nil, fmt.Errorf("account must be started"))
	}

	store := app.MustComponent[objectstore.ObjectStore](mw.applicationService.GetApp())
	objType, err := store.GetObjectType(req.ObjectTypeUrl)
	if err != nil {
		if err == block.ErrUnknownObjectType {
			return response(pb.RpcObjectTypeRelationListResponseError_UNKNOWN_OBJECT_TYPE_URL, nil, err)
		}
		return response(pb.RpcObjectTypeRelationListResponseError_UNKNOWN_ERROR, nil, err)
	}

	// todo: AppendRelationsFromOtherTypes case
	return response(pb.RpcObjectTypeRelationListResponseError_NULL, objType.RelationLinks, nil)
}

func (mw *Middleware) ObjectTypeRelationAdd(cctx context.Context, req *pb.RpcObjectTypeRelationAddRequest) *pb.RpcObjectTypeRelationAddResponse {
	ctx := mw.newContext(cctx)
	response := func(code pb.RpcObjectTypeRelationAddResponseErrorCode, err error) *pb.RpcObjectTypeRelationAddResponse {
		m := &pb.RpcObjectTypeRelationAddResponse{Error: &pb.RpcObjectTypeRelationAddResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}

	rs, err := mw.getRelationService()
	if err != nil {
		return response(pb.RpcObjectTypeRelationAddResponseError_UNKNOWN_ERROR, err)
	}
	at := mw.GetAnytype()
	if at == nil {
		return response(pb.RpcObjectTypeRelationAddResponseError_BAD_INPUT, fmt.Errorf("account must be started"))
	}

	if strings.HasPrefix(req.ObjectTypeUrl, bundle.TypePrefix) {
		return response(pb.RpcObjectTypeRelationAddResponseError_READONLY_OBJECT_TYPE, fmt.Errorf("can't modify bundled object type"))
	}

	err = mw.doBlockService(func(bs *block.Service) (err error) {
		spaceId, err := bs.ResolveSpaceID(req.ObjectTypeUrl)
		if err != nil {
			return err
		}

		err = bs.ModifyDetails(ctx, req.ObjectTypeUrl, func(current *types.Struct) (*types.Struct, error) {
			list := pbtypes.GetStringList(current, bundle.RelationKeyRecommendedRelations.String())

			for _, relKey := range req.RelationKeys {
				relId, err := rs.GetRelationIdByKey(cctx, spaceId, bundle.RelationKey(relKey))
				if err != nil {
					return nil, err
				}

				if slice.FindPos(list, relId) == -1 {
					list = append(list, relId)
				}
			}
			detCopy := pbtypes.CopyStruct(current)
			detCopy.Fields[bundle.RelationKeyRecommendedRelations.String()] = pbtypes.StringList(list)
			return detCopy, nil
		})
		if err != nil {
			return err
		}

		return nil
	})

	if err != nil {
		return response(pb.RpcObjectTypeRelationAddResponseError_UNKNOWN_ERROR, err)
	}

	return response(pb.RpcObjectTypeRelationAddResponseError_NULL, nil)
}

func (mw *Middleware) ObjectTypeRelationRemove(cctx context.Context, req *pb.RpcObjectTypeRelationRemoveRequest) *pb.RpcObjectTypeRelationRemoveResponse {
	ctx := mw.newContext(cctx)
	response := func(code pb.RpcObjectTypeRelationRemoveResponseErrorCode, err error) *pb.RpcObjectTypeRelationRemoveResponse {
		m := &pb.RpcObjectTypeRelationRemoveResponse{Error: &pb.RpcObjectTypeRelationRemoveResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}

	at := mw.GetAnytype()
	if at == nil {
		return response(pb.RpcObjectTypeRelationRemoveResponseError_BAD_INPUT, fmt.Errorf("account must be started"))
	}

	if strings.HasPrefix(req.ObjectTypeUrl, bundle.TypePrefix) {
		return response(pb.RpcObjectTypeRelationRemoveResponseError_READONLY_OBJECT_TYPE, fmt.Errorf("can't modify bundled object type"))
	}

	err := mw.doBlockService(func(bs *block.Service) (err error) {
		err = bs.ModifyDetails(ctx, req.ObjectTypeUrl, func(current *types.Struct) (*types.Struct, error) {
			list := pbtypes.GetStringList(current, bundle.RelationKeyRecommendedRelations.String())
			for _, relKey := range req.RelationKeys {
				relId := addr.RelationKeyToIdPrefix + relKey
				if pos := slice.FindPos(list, relId); pos != -1 {
					list = append(list[:pos], list[pos+1:]...)
				}
			}

			detCopy := pbtypes.CopyStruct(current)
			detCopy.Fields[bundle.RelationKeyRecommendedRelations.String()] = pbtypes.StringList(list)
			return detCopy, nil
		})
		if err != nil {
			return err
		}
		return
	})

	if err != nil {
		return response(pb.RpcObjectTypeRelationRemoveResponseError_UNKNOWN_ERROR, err)
	}

	return response(pb.RpcObjectTypeRelationRemoveResponseError_NULL, nil)
}

func (mw *Middleware) ObjectCreateObjectType(cctx context.Context, req *pb.RpcObjectCreateObjectTypeRequest) *pb.RpcObjectCreateObjectTypeResponse {
	response := func(code pb.RpcObjectCreateObjectTypeResponseErrorCode, id string, details *types.Struct, err error) *pb.RpcObjectCreateObjectTypeResponse {
		m := &pb.RpcObjectCreateObjectTypeResponse{ObjectId: id, Details: details, Error: &pb.RpcObjectCreateObjectTypeResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}

	var (
		id         string
		newDetails *types.Struct
	)
	err := mw.doBlockService(func(bs *block.Service) error {
		var err error
		id, newDetails, err = bs.CreateObject(cctx, req.SpaceId, req, bundle.TypeKeyObjectType)
		return err
	})
	if err != nil {
		return response(pb.RpcObjectCreateObjectTypeResponseError_UNKNOWN_ERROR, "", nil, err)
	}

	return response(pb.RpcObjectCreateObjectTypeResponseError_NULL, id, newDetails, nil)
}

func (mw *Middleware) ObjectCreateSet(cctx context.Context, req *pb.RpcObjectCreateSetRequest) *pb.RpcObjectCreateSetResponse {
	ctx := mw.newContext(cctx)
	response := func(code pb.RpcObjectCreateSetResponseErrorCode, id string, newDetails *types.Struct, err error) *pb.RpcObjectCreateSetResponse {
		m := &pb.RpcObjectCreateSetResponse{Error: &pb.RpcObjectCreateSetResponseError{Code: code}, ObjectId: id}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = mw.getResponseEvent(ctx)
			m.Details = newDetails
		}
		return m
	}

	var (
		id         string
		newDetails *types.Struct
	)
	err := mw.doBlockService(func(bs *block.Service) error {
		var err error
		if req.Details == nil {
			req.Details = &types.Struct{}
		}
		if req.Details.Fields == nil {
			req.Details.Fields = map[string]*types.Value{}
		}
		req.Details.Fields[bundle.RelationKeySetOf.String()] = pbtypes.StringList(req.Source)
		id, newDetails, err = bs.CreateObject(cctx, req.SpaceId, req, bundle.TypeKeySet)
		return err
	})
	if err != nil {
		if err == block.ErrUnknownObjectType {
			return response(pb.RpcObjectCreateSetResponseError_UNKNOWN_OBJECT_TYPE_URL, "", nil, err)
		}
		return response(pb.RpcObjectCreateSetResponseError_UNKNOWN_ERROR, "", nil, err)
	}

	return response(pb.RpcObjectCreateSetResponseError_NULL, id, newDetails, nil)
}

func (mw *Middleware) ObjectCreateRelation(cctx context.Context, req *pb.RpcObjectCreateRelationRequest) *pb.RpcObjectCreateRelationResponse {
	response := func(id string, object *types.Struct, err error) *pb.RpcObjectCreateRelationResponse {
		if err != nil {
			return &pb.RpcObjectCreateRelationResponse{
				Error: &pb.RpcObjectCreateRelationResponseError{
					Code:        pb.RpcObjectCreateRelationResponseError_UNKNOWN_ERROR,
					Description: err.Error(),
				},
			}
		}
		key := pbtypes.GetString(object, bundle.RelationKeyRelationKey.String())
		return &pb.RpcObjectCreateRelationResponse{
			Error: &pb.RpcObjectCreateRelationResponseError{
				Code: pb.RpcObjectCreateRelationResponseError_NULL,
			},
			ObjectId: id,
			Key:      key,
			Details:  object,
		}
	}
	var (
		id         string
		newDetails *types.Struct
	)
	err := mw.doBlockService(func(bs *block.Service) error {
		var err error
		id, newDetails, err = bs.CreateObject(cctx, req.SpaceId, req, bundle.TypeKeyRelation)
		return err
	})
	if err != nil {
		return response("", nil, err)
	}
	return response(id, newDetails, err)
}

func (mw *Middleware) ObjectCreateRelationOption(cctx context.Context, req *pb.RpcObjectCreateRelationOptionRequest) *pb.RpcObjectCreateRelationOptionResponse {
	response := func(id string, newDetails *types.Struct, err error) *pb.RpcObjectCreateRelationOptionResponse {
		if err != nil {
			return &pb.RpcObjectCreateRelationOptionResponse{
				Error: &pb.RpcObjectCreateRelationOptionResponseError{
					Code:        pb.RpcObjectCreateRelationOptionResponseError_UNKNOWN_ERROR,
					Description: err.Error(),
				},
			}
		}
		return &pb.RpcObjectCreateRelationOptionResponse{
			Error: &pb.RpcObjectCreateRelationOptionResponseError{
				Code: pb.RpcObjectCreateRelationOptionResponseError_NULL,
			},
			ObjectId: id,
			Details:  newDetails,
		}
	}

	var (
		id         string
		newDetails *types.Struct
	)
	err := mw.doBlockService(func(bs *block.Service) error {
		var err error
		id, newDetails, err = bs.CreateObject(cctx, req.SpaceId, req, bundle.TypeKeyRelationOption)
		return err
	})
	return response(id, newDetails, err)
}

func (mw *Middleware) RelationListRemoveOption(cctx context.Context, request *pb.RpcRelationListRemoveOptionRequest) *pb.RpcRelationListRemoveOptionResponse {
	ctx := mw.newContext(cctx)
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
		err = bs.RemoveListOption(ctx, request.OptionIds, request.CheckInObjects)
		return err
	})
	if err != nil {
		if errors.Is(err, block.ErrOptionUsedByOtherObjects) {
			return response(pb.RpcRelationListRemoveOptionResponseError_OPTION_USED_BY_OBJECTS, nil)
		}
		return response(pb.RpcRelationListRemoveOptionResponseError_UNKNOWN_ERROR, nil)
	}

	return response(pb.RpcRelationListRemoveOptionResponseError_NULL, nil)
}

func (mw *Middleware) RelationOptions(cctx context.Context, request *pb.RpcRelationOptionsRequest) *pb.RpcRelationOptionsResponse {
	// TODO implement me
	panic("implement me")
}
