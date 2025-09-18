package core

import (
	"context"
	"errors"
	"fmt"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/object/objectcreator"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

func (mw *Middleware) ObjectCreate(cctx context.Context, req *pb.RpcObjectCreateRequest) *pb.RpcObjectCreateResponse {
	ctx := mw.newContext(cctx)
	response := func(code pb.RpcObjectCreateResponseErrorCode, id string, newDetails *types.Struct, err error) *pb.RpcObjectCreateResponse {
		m := &pb.RpcObjectCreateResponse{Error: &pb.RpcObjectCreateResponseError{Code: code}, Details: newDetails, ObjectId: id}
		if err != nil {
			m.Error.Description = getErrorDescription(err)
		} else {
			m.Event = mw.getResponseEvent(ctx)
			m.Details = newDetails
		}
		return m
	}

	creator := mustService[objectcreator.Service](mw)
	createReq := objectcreator.CreateObjectRequest{
		Details:       domain.NewDetailsFromProto(req.Details),
		InternalFlags: req.InternalFlags,
		TemplateId:    req.TemplateId,
	}
	id, newDetails, err := creator.CreateObjectUsingObjectUniqueTypeKey(cctx, req.SpaceId, req.ObjectTypeUniqueKey, createReq)
	if err != nil {
		return response(pb.RpcObjectCreateResponseError_UNKNOWN_ERROR, "", nil, err)
	}
	if req.WithChat {
		return response(pb.RpcObjectCreateResponseError_UNKNOWN_ERROR, "", nil, fmt.Errorf("WithChat is not implemented"))
	}
	return response(pb.RpcObjectCreateResponseError_NULL, id, newDetails.ToProto(), nil)
}

func (mw *Middleware) ObjectChatAdd(cctx context.Context, req *pb.RpcObjectChatAddRequest) *pb.RpcObjectChatAddResponse {
	return &pb.RpcObjectChatAddResponse{
		Error: &pb.RpcObjectChatAddResponseError{
			Code:        pb.RpcObjectChatAddResponseError_UNKNOWN_ERROR,
			Description: "not implemented",
		},
	}
}

func (mw *Middleware) ObjectCreateSet(cctx context.Context, req *pb.RpcObjectCreateSetRequest) *pb.RpcObjectCreateSetResponse {
	ctx := mw.newContext(cctx)
	response := func(code pb.RpcObjectCreateSetResponseErrorCode, id string, newDetails *types.Struct, err error) *pb.RpcObjectCreateSetResponse {
		m := &pb.RpcObjectCreateSetResponse{Error: &pb.RpcObjectCreateSetResponseError{Code: code}, ObjectId: id}
		if err != nil {
			m.Error.Description = getErrorDescription(err)
		} else {
			m.Event = mw.getResponseEvent(ctx)
			m.Details = newDetails
		}
		return m
	}

	if req.Details == nil {
		req.Details = &types.Struct{}
	}
	if req.Details.Fields == nil {
		req.Details.Fields = map[string]*types.Value{}
	}
	details := domain.NewDetailsFromProto(req.Details)
	details.SetStringList(bundle.RelationKeySetOf, req.Source)

	creator := mustService[objectcreator.Service](mw)
	createReq := objectcreator.CreateObjectRequest{
		ObjectTypeKey: bundle.TypeKeySet,
		InternalFlags: req.InternalFlags,
		Details:       details,
	}
	id, newDetails, err := creator.CreateObject(cctx, req.SpaceId, createReq)
	if err != nil {
		if errors.Is(err, block.ErrUnknownObjectType) {
			return response(pb.RpcObjectCreateSetResponseError_UNKNOWN_OBJECT_TYPE_URL, "", nil, err)
		}
		return response(pb.RpcObjectCreateSetResponseError_UNKNOWN_ERROR, "", nil, err)
	}
	if req.WithChat {
		return response(pb.RpcObjectCreateSetResponseError_UNKNOWN_ERROR, "", nil, fmt.Errorf("WithChat is not implemented"))
	}
	return response(pb.RpcObjectCreateSetResponseError_NULL, id, newDetails.ToProto(), nil)
}

func (mw *Middleware) ObjectCreateBookmark(cctx context.Context, req *pb.RpcObjectCreateBookmarkRequest) *pb.RpcObjectCreateBookmarkResponse {
	response := func(code pb.RpcObjectCreateBookmarkResponseErrorCode, id string, details *types.Struct, err error) *pb.RpcObjectCreateBookmarkResponse {
		m := &pb.RpcObjectCreateBookmarkResponse{Error: &pb.RpcObjectCreateBookmarkResponseError{Code: code}, ObjectId: id, Details: details}
		if err != nil {
			m.Error.Description = getErrorDescription(err)
		}
		return m
	}

	creator := mustService[objectcreator.Service](mw)
	createReq := objectcreator.CreateObjectRequest{
		ObjectTypeKey: bundle.TypeKeyBookmark,
		Details:       domain.NewDetailsFromProto(req.Details),
		TemplateId:    req.TemplateId,
	}
	id, newDetails, err := creator.CreateObject(cctx, req.SpaceId, createReq)
	if err != nil {
		return response(pb.RpcObjectCreateBookmarkResponseError_UNKNOWN_ERROR, "", nil, err)
	}
	if req.WithChat {
		return response(pb.RpcObjectCreateBookmarkResponseError_UNKNOWN_ERROR, "", nil, fmt.Errorf("WithChat is not implemented"))
	}
	return response(pb.RpcObjectCreateBookmarkResponseError_NULL, id, newDetails.ToProto(), nil)
}

func (mw *Middleware) ObjectCreateObjectType(cctx context.Context, req *pb.RpcObjectCreateObjectTypeRequest) *pb.RpcObjectCreateObjectTypeResponse {
	response := func(code pb.RpcObjectCreateObjectTypeResponseErrorCode, id string, details *types.Struct, err error) *pb.RpcObjectCreateObjectTypeResponse {
		m := &pb.RpcObjectCreateObjectTypeResponse{ObjectId: id, Details: details, Error: &pb.RpcObjectCreateObjectTypeResponseError{Code: code}}
		if err != nil {
			m.Error.Description = getErrorDescription(err)
		}
		return m
	}

	creator := mustService[objectcreator.Service](mw)
	createReq := objectcreator.CreateObjectRequest{
		ObjectTypeKey: bundle.TypeKeyObjectType,
		InternalFlags: req.InternalFlags,
		Details:       domain.NewDetailsFromProto(req.Details),
	}
	id, newDetails, err := creator.CreateObject(cctx, req.SpaceId, createReq)
	if err != nil {
		return response(pb.RpcObjectCreateObjectTypeResponseError_UNKNOWN_ERROR, "", nil, err)
	}

	return response(pb.RpcObjectCreateObjectTypeResponseError_NULL, id, newDetails.ToProto(), nil)
}

func (mw *Middleware) ObjectCreateRelation(cctx context.Context, req *pb.RpcObjectCreateRelationRequest) *pb.RpcObjectCreateRelationResponse {
	response := func(id string, object *domain.Details, err error) *pb.RpcObjectCreateRelationResponse {
		if err != nil {
			return &pb.RpcObjectCreateRelationResponse{
				Error: &pb.RpcObjectCreateRelationResponseError{
					Code:        pb.RpcObjectCreateRelationResponseError_UNKNOWN_ERROR,
					Description: getErrorDescription(err),
				},
			}
		}
		key := object.GetString(bundle.RelationKeyRelationKey)
		return &pb.RpcObjectCreateRelationResponse{
			Error: &pb.RpcObjectCreateRelationResponseError{
				Code: pb.RpcObjectCreateRelationResponseError_NULL,
			},
			ObjectId: id,
			Key:      key,
			Details:  object.ToProto(),
		}
	}
	creator := mustService[objectcreator.Service](mw)
	createReq := objectcreator.CreateObjectRequest{
		ObjectTypeKey: bundle.TypeKeyRelation,
		Details:       domain.NewDetailsFromProto(req.Details),
	}
	id, newDetails, err := creator.CreateObject(cctx, req.SpaceId, createReq)
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
					Description: getErrorDescription(err),
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

	creator := mustService[objectcreator.Service](mw)
	createReq := objectcreator.CreateObjectRequest{
		ObjectTypeKey: bundle.TypeKeyRelationOption,
		Details:       domain.NewDetailsFromProto(req.Details),
	}
	id, newDetails, err := creator.CreateObject(cctx, req.SpaceId, createReq)
	return response(id, newDetails.ToProto(), err)
}

func (mw *Middleware) ObjectCreateFromUrl(cctx context.Context, req *pb.RpcObjectCreateFromUrlRequest) *pb.RpcObjectCreateFromUrlResponse {
	response := func(code pb.RpcObjectCreateFromUrlResponseErrorCode, id string, err error, newDetails *types.Struct) *pb.RpcObjectCreateFromUrlResponse {
		m := &pb.RpcObjectCreateFromUrlResponse{Details: newDetails, Error: &pb.RpcObjectCreateFromUrlResponseError{Code: code}, ObjectId: id}
		if err != nil {
			m.Error.Description = getErrorDescription(err)
		}
		return m
	}
	bs := mustService[*block.Service](mw)

	id, newDetails, err := bs.CreateObjectFromUrl(cctx, req)
	if err != nil {
		return response(pb.RpcObjectCreateFromUrlResponseError_UNKNOWN_ERROR, "", err, nil)
	}

	if req.WithChat {
		return response(pb.RpcObjectCreateFromUrlResponseError_UNKNOWN_ERROR, "", fmt.Errorf("WithChat is not implemented"), nil)
	}
	return response(pb.RpcObjectCreateFromUrlResponseError_NULL, id, nil, newDetails.ToProto())
}
