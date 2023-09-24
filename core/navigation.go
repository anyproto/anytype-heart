package core

import (
	"context"
	"fmt"

	"github.com/anyproto/any-sync/app"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block"
	"github.com/anyproto/anytype-heart/core/block/object/objectcache"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func (mw *Middleware) NavigationListObjects(cctx context.Context, req *pb.RpcNavigationListObjectsRequest) *pb.RpcNavigationListObjectsResponse {
	response := func(code pb.RpcNavigationListObjectsResponseErrorCode, Objects []*model.ObjectInfo, err error) *pb.RpcNavigationListObjectsResponse {
		m := &pb.RpcNavigationListObjectsResponse{Error: &pb.RpcNavigationListObjectsResponseError{Code: code}, Objects: Objects}
		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}
	return response(pb.RpcNavigationListObjectsResponseError_UNKNOWN_ERROR, nil, fmt.Errorf("not implemented"))
}

func (mw *Middleware) NavigationGetObjectInfoWithLinks(cctx context.Context, req *pb.RpcNavigationGetObjectInfoWithLinksRequest) *pb.RpcNavigationGetObjectInfoWithLinksResponse {
	response := func(code pb.RpcNavigationGetObjectInfoWithLinksResponseErrorCode, object *model.ObjectInfoWithLinks, err error) *pb.RpcNavigationGetObjectInfoWithLinksResponse {
		m := &pb.RpcNavigationGetObjectInfoWithLinksResponse{Error: &pb.RpcNavigationGetObjectInfoWithLinksResponseError{Code: code}, Object: object}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	if mw.applicationService.GetApp() == nil {
		return response(pb.RpcNavigationGetObjectInfoWithLinksResponseError_BAD_INPUT, nil, fmt.Errorf("account must be started"))
	}

	filter := func(Objects []*model.ObjectInfo) []*model.ObjectInfo {
		var filtered []*model.ObjectInfo
		for _, page := range Objects {
			if page.ObjectType == model.SmartBlockType_Archive || page.ObjectType == model.SmartBlockType_File {
				continue
			}

			filtered = append(filtered, page)
		}
		return filtered
	}

	cache := getService[objectcache.Cache](mw)
	store := app.MustComponent[objectstore.ObjectStore](mw.applicationService.GetApp())
	spaceID, err := cache.ResolveSpaceID(req.ObjectId)
	if err != nil {
		return response(pb.RpcNavigationGetObjectInfoWithLinksResponseError_UNKNOWN_ERROR, nil, fmt.Errorf("resolve spaceID: %w", err))
	}
	page, err := store.GetWithLinksInfoByID(spaceID, req.ObjectId)
	if err != nil {
		return response(pb.RpcNavigationGetObjectInfoWithLinksResponseError_UNKNOWN_ERROR, nil, err)
	}

	if req.Context != pb.RpcNavigation_Navigation && page.Links != nil {
		page.Links.Inbound = filter(page.Links.Inbound)
		page.Links.Outbound = filter(page.Links.Outbound)
	}

	return response(pb.RpcNavigationGetObjectInfoWithLinksResponseError_NULL, page, nil)
}

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

	var (
		id         string
		newDetails *types.Struct
	)
	err := mw.doBlockService(func(bs *block.Service) error {
		var err error
		id, newDetails, err = bs.CreateObjectUsingObjectUniqueTypeKey(cctx, req.SpaceId, req, req.ObjectTypeUniqueKey)
		return err
	})
	if err != nil {
		return response(pb.RpcObjectCreateResponseError_UNKNOWN_ERROR, "", nil, err)
	}
	return response(pb.RpcObjectCreateResponseError_NULL, id, newDetails, nil)
}
