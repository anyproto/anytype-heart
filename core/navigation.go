package core

import (
	"fmt"

	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/pb"
	coresb "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

func (mw *Middleware) NavigationListObjects(req *pb.RpcNavigationListObjectsRequest) *pb.RpcNavigationListObjectsResponse {
	response := func(code pb.RpcNavigationListObjectsResponseErrorCode, Objects []*model.ObjectInfo, err error) *pb.RpcNavigationListObjectsResponse {
		m := &pb.RpcNavigationListObjectsResponse{Error: &pb.RpcNavigationListObjectsResponseError{Code: code}, Objects: Objects}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	if mw.Anytype == nil {
		return response(pb.RpcNavigationListObjectsResponseError_BAD_INPUT, nil, fmt.Errorf("account must be started"))
	}
	objectTypes := []coresb.SmartBlockType{
		coresb.SmartBlockTypeArchive,
		coresb.SmartBlockTypeFile,
	}
	if req.Context != pb.RpcNavigation_Navigation {
		objectTypes = []coresb.SmartBlockType{
			coresb.SmartBlockTypeSet,
			coresb.SmartBlockTypeArchive,
			coresb.SmartBlockTypeHome,
			coresb.SmartBlockTypeFile,
		}
	}
	records, _, err := mw.Anytype.ObjectStore().QueryObjectInfo(database.Query{
		FullText: req.FullText,
		Limit:    int(req.Limit),
		Offset:   int(req.Offset),
	}, objectTypes)
	if err != nil {
		return response(pb.RpcNavigationListObjectsResponseError_UNKNOWN_ERROR, nil, err)
	}

	return response(pb.RpcNavigationListObjectsResponseError_NULL, records, nil)
}

func (mw *Middleware) NavigationGetObjectInfoWithLinks(req *pb.RpcNavigationGetObjectInfoWithLinksRequest) *pb.RpcNavigationGetObjectInfoWithLinksResponse {
	response := func(code pb.RpcNavigationGetObjectInfoWithLinksResponseErrorCode, object *model.ObjectInfoWithLinks, err error) *pb.RpcNavigationGetObjectInfoWithLinksResponse {
		m := &pb.RpcNavigationGetObjectInfoWithLinksResponse{Error: &pb.RpcNavigationGetObjectInfoWithLinksResponseError{Code: code}, Object: object}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	if mw.Anytype == nil {
		return response(pb.RpcNavigationGetObjectInfoWithLinksResponseError_BAD_INPUT, nil, fmt.Errorf("account must be started"))
	}

	filter := func(Objects []*model.ObjectInfo) []*model.ObjectInfo {
		var filtered []*model.ObjectInfo
		for _, page := range Objects {
			if page.ObjectType == model.ObjectInfo_Set || page.ObjectType == model.ObjectInfo_Archive {
				continue
			}

			filtered = append(filtered, page)
		}
		return filtered
	}

	page, err := mw.Anytype.ObjectInfoWithLinks(req.ObjectId)
	if err != nil {
		return response(pb.RpcNavigationGetObjectInfoWithLinksResponseError_UNKNOWN_ERROR, nil, err)
	}

	if req.Context != pb.RpcNavigation_Navigation && page.Links != nil {
		page.Links.Inbound = filter(page.Links.Inbound)
		page.Links.Outbound = filter(page.Links.Outbound)
	}

	return response(pb.RpcNavigationGetObjectInfoWithLinksResponseError_NULL, page, nil)
}

func (mw *Middleware) PageCreate(req *pb.RpcPageCreateRequest) *pb.RpcPageCreateResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcPageCreateResponseErrorCode, id string, err error) *pb.RpcPageCreateResponse {
		m := &pb.RpcPageCreateResponse{Error: &pb.RpcPageCreateResponseError{Code: code}, PageId: id}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}

	var id string
	err := mw.doBlockService(func(bs block.Service) (err error) {
		id, _, err = bs.CreateSmartBlock(coresb.SmartBlockTypePage, req.Details, nil)
		return
	})

	if err != nil {
		return response(pb.RpcPageCreateResponseError_UNKNOWN_ERROR, "", err)
	}
	return response(pb.RpcPageCreateResponseError_NULL, id, nil)
}
