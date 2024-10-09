package core

import (
	"context"
	"fmt"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/block/object/idresolver"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/spacecore/typeprovider"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func (mw *Middleware) NavigationListObjects(cctx context.Context, req *pb.RpcNavigationListObjectsRequest) *pb.RpcNavigationListObjectsResponse {
	response := func(code pb.RpcNavigationListObjectsResponseErrorCode, Objects []*model.ObjectInfo, err error) *pb.RpcNavigationListObjectsResponse {
		m := &pb.RpcNavigationListObjectsResponse{Error: &pb.RpcNavigationListObjectsResponseError{Code: code}, Objects: Objects}
		if err != nil {
			m.Error.Description = getErrorDescription(err)
		}
		return m
	}
	return response(pb.RpcNavigationListObjectsResponseError_UNKNOWN_ERROR, nil, fmt.Errorf("not implemented"))
}

func (mw *Middleware) NavigationGetObjectInfoWithLinks(_ context.Context, req *pb.RpcNavigationGetObjectInfoWithLinksRequest) *pb.RpcNavigationGetObjectInfoWithLinksResponse {
	response := func(code pb.RpcNavigationGetObjectInfoWithLinksResponseErrorCode, object *model.ObjectInfoWithLinks, err error) *pb.RpcNavigationGetObjectInfoWithLinksResponse {
		m := &pb.RpcNavigationGetObjectInfoWithLinksResponse{Error: &pb.RpcNavigationGetObjectInfoWithLinksResponseError{Code: code}, Object: object}
		if err != nil {
			m.Error.Description = getErrorDescription(err)
		}

		return m
	}

	if mw.applicationService.GetApp() == nil {
		return response(pb.RpcNavigationGetObjectInfoWithLinksResponseError_BAD_INPUT, nil, fmt.Errorf("account must be started"))
	}

	resolver := getService[idresolver.Resolver](mw)
	store := app.MustComponent[spaceindex.Store](mw.applicationService.GetApp())
	spaceId, err := resolver.ResolveSpaceID(req.ObjectId)
	if err != nil {
		return response(pb.RpcNavigationGetObjectInfoWithLinksResponseError_UNKNOWN_ERROR, nil, fmt.Errorf("resolve spaceId: %w", err))
	}
	page, err := store.GetWithLinksInfoById(req.ObjectId)
	if err != nil {
		return response(pb.RpcNavigationGetObjectInfoWithLinksResponseError_UNKNOWN_ERROR, nil, err)
	}

	sbTypeProvider := getService[typeprovider.SmartBlockTypeProvider](mw)
	filter := func(objects []*model.ObjectInfo) []*model.ObjectInfo {
		var filtered []*model.ObjectInfo
		for _, obj := range objects {
			sbType, err := sbTypeProvider.Type(spaceId, obj.Id)
			if err != nil {
				log.Error("get smartblock type: %v", err)
			}
			if sbType == smartblock.SmartBlockTypeArchive || sbType == smartblock.SmartBlockTypeFileObject || sbType == smartblock.SmartBlockTypeWidget {
				continue
			}

			if pbtypes.GetString(obj.Details, bundle.RelationKeySpaceId.String()) != spaceId {
				continue
			}

			filtered = append(filtered, obj)
		}
		return filtered
	}

	if req.Context != pb.RpcNavigation_Navigation && page.Links != nil {
		page.Links.Inbound = filter(page.Links.Inbound)
		page.Links.Outbound = filter(page.Links.Outbound)
	}

	return response(pb.RpcNavigationGetObjectInfoWithLinksResponseError_NULL, page, nil)
}
