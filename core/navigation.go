package core

import (
	"context"
	"fmt"

	"github.com/anytypeio/any-sync/app"
	"github.com/gogo/protobuf/types"

	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	coresb "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/objectstore"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

func (mw *Middleware) NavigationListObjects(cctx context.Context, req *pb.RpcNavigationListObjectsRequest) *pb.RpcNavigationListObjectsResponse {
	response := func(code pb.RpcNavigationListObjectsResponseErrorCode, Objects []*model.ObjectInfo, err error) *pb.RpcNavigationListObjectsResponse {
		m := &pb.RpcNavigationListObjectsResponse{Error: &pb.RpcNavigationListObjectsResponseError{Code: code}, Objects: Objects}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}
	mw.m.RLock()
	defer mw.m.RUnlock()

	if mw.app == nil {
		return response(pb.RpcNavigationListObjectsResponseError_BAD_INPUT, nil, fmt.Errorf("account must be started"))
	}

	objectTypes := []coresb.SmartBlockType{
		coresb.SmartBlockTypePage,
		coresb.SmartBlockTypeProfilePage,
		coresb.SmartBlockTypeHome,
		coresb.SmartBlockTypeSet,
		coresb.SmartBlockTypeObjectType,
	}
	if req.Context != pb.RpcNavigation_Navigation {
		objectTypes = []coresb.SmartBlockType{
			coresb.SmartBlockTypePage,
			coresb.SmartBlockTypeProfilePage,
			coresb.SmartBlockTypeObjectType,
		}
	}

	store := app.MustComponent[objectstore.ObjectStore](mw.app)
	records, _, err := store.QueryObjectInfo(database.Query{
		FullText: req.FullText,
		Limit:    int(req.Limit),
		Offset:   int(req.Offset),
	}, objectTypes)
	if err != nil {
		return response(pb.RpcNavigationListObjectsResponseError_UNKNOWN_ERROR, nil, err)
	}

	return response(pb.RpcNavigationListObjectsResponseError_NULL, records, nil)
}

func (mw *Middleware) NavigationGetObjectInfoWithLinks(cctx context.Context, req *pb.RpcNavigationGetObjectInfoWithLinksRequest) *pb.RpcNavigationGetObjectInfoWithLinksResponse {
	response := func(code pb.RpcNavigationGetObjectInfoWithLinksResponseErrorCode, object *model.ObjectInfoWithLinks, err error) *pb.RpcNavigationGetObjectInfoWithLinksResponse {
		m := &pb.RpcNavigationGetObjectInfoWithLinksResponse{Error: &pb.RpcNavigationGetObjectInfoWithLinksResponseError{Code: code}, Object: object}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}
	mw.m.RLock()
	defer mw.m.RUnlock()

	if mw.app == nil {
		return response(pb.RpcNavigationGetObjectInfoWithLinksResponseError_BAD_INPUT, nil, fmt.Errorf("account must be started"))
	}

	at := mw.app.MustComponent(core.CName).(core.Service)

	filter := func(Objects []*model.ObjectInfo) []*model.ObjectInfo {
		var filtered []*model.ObjectInfo
		for _, page := range Objects {
			if page.ObjectType == model.SmartBlockType_Set || page.ObjectType == model.SmartBlockType_Archive || page.ObjectType == model.SmartBlockType_File {
				continue
			}

			filtered = append(filtered, page)
		}
		return filtered
	}

	page, err := at.ObjectInfoWithLinks(req.ObjectId)
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
			m.Event = ctx.GetResponseEvent()
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
		id, newDetails, err = bs.CreateObject(req, "")
		return err
	})
	if err != nil {
		return response(pb.RpcObjectCreateResponseError_UNKNOWN_ERROR, "", nil, err)
	}
	return response(pb.RpcObjectCreateResponseError_NULL, id, newDetails, nil)
}
