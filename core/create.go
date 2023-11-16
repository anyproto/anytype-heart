package core

import (
	"context"
	"errors"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/object/objectcreator"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func (mw *Middleware) ObjectCreate(cctx context.Context, req *pb.RpcObjectCreateRequest) *pb.RpcObjectCreateResponse {
	ctx := mw.newContext(cctx)
	response := func(code pb.RpcObjectCreateResponseErrorCode, id string, newDetails *types.Struct, err error) *pb.RpcObjectCreateResponse {
		m := &pb.RpcObjectCreateResponse{Error: &pb.RpcObjectCreateResponseError{Code: code}, Details: newDetails, ObjectId: id}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = mw.getResponseEvent(ctx)
			m.Details = newDetails
		}
		return m
	}

	creator := getService[objectcreator.Service](mw)
	createReq := objectcreator.CreateObjectRequest{
		Details:       req.Details,
		InternalFlags: req.InternalFlags,
		TemplateId:    req.TemplateId,
	}
	id, newDetails, err := creator.CreateObjectUsingObjectUniqueTypeKey(cctx, req.SpaceId, req.ObjectTypeUniqueKey, createReq)

	if err != nil {
		return response(pb.RpcObjectCreateResponseError_UNKNOWN_ERROR, "", nil, err)
	}
	return response(pb.RpcObjectCreateResponseError_NULL, id, newDetails, nil)
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

	if req.Details == nil {
		req.Details = &types.Struct{}
	}
	if req.Details.Fields == nil {
		req.Details.Fields = map[string]*types.Value{}
	}
	req.Details.Fields[bundle.RelationKeySetOf.String()] = pbtypes.StringList(req.Source)

	creator := getService[objectcreator.Service](mw)
	createReq := objectcreator.CreateObjectRequest{
		ObjectTypeKey: bundle.TypeKeySet,
		InternalFlags: req.InternalFlags,
		Details:       req.Details,
	}
	id, newDetails, err := creator.CreateObject(cctx, req.SpaceId, createReq)
	if err != nil {
		if errors.Is(err, block.ErrUnknownObjectType) {
			return response(pb.RpcObjectCreateSetResponseError_UNKNOWN_OBJECT_TYPE_URL, "", nil, err)
		}
		return response(pb.RpcObjectCreateSetResponseError_UNKNOWN_ERROR, "", nil, err)
	}

	return response(pb.RpcObjectCreateSetResponseError_NULL, id, newDetails, nil)
}

func (mw *Middleware) ObjectCreateBookmark(cctx context.Context, req *pb.RpcObjectCreateBookmarkRequest) *pb.RpcObjectCreateBookmarkResponse {
	response := func(code pb.RpcObjectCreateBookmarkResponseErrorCode, id string, details *types.Struct, err error) *pb.RpcObjectCreateBookmarkResponse {
		m := &pb.RpcObjectCreateBookmarkResponse{Error: &pb.RpcObjectCreateBookmarkResponseError{Code: code}, ObjectId: id, Details: details}
		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}

	creator := getService[objectcreator.Service](mw)
	createReq := objectcreator.CreateObjectRequest{
		ObjectTypeKey: bundle.TypeKeyBookmark,
		Details:       req.Details,
	}
	id, newDetails, err := creator.CreateObject(cctx, req.SpaceId, createReq)
	if err != nil {
		return response(pb.RpcObjectCreateBookmarkResponseError_UNKNOWN_ERROR, "", newDetails, err)
	}
	return response(pb.RpcObjectCreateBookmarkResponseError_NULL, id, newDetails, nil)
}
