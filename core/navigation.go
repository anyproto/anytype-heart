package core

import (
	"context"
	"fmt"
	"strings"

	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	coresb "github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/internalflag"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
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

	at := mw.app.MustComponent(core.CName).(core.Service)

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
	records, _, err := at.ObjectStore().QueryObjectInfo(database.Query{
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
	response := func(code pb.RpcObjectCreateResponseErrorCode, id string, err error) *pb.RpcObjectCreateResponse {
		m := &pb.RpcObjectCreateResponse{Error: &pb.RpcObjectCreateResponseError{Code: code}, ObjectId: id}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}

	var id string
	var err error

	req.Details = internalflag.AddToDetails(req.Details, req.InternalFlags)

	ot := strings.TrimPrefix(pbtypes.GetString(req.Details, bundle.RelationKeyType.String()), bundle.TypePrefix)
	switch bundle.TypeKey(ot) {
	case bundle.TypeKeyBookmark:
		id, err = mw.objectCreateBookmark(&pb.RpcObjectCreateBookmarkRequest{
			Details: req.Details,
		})
	case bundle.TypeKeySet:
		id, err = mw.objectCreateSet(&pb.RpcObjectCreateSetRequest{
			Details:       req.Details,
			InternalFlags: req.InternalFlags,
			Source:        pbtypes.GetStringList(req.Details, bundle.RelationKeySetOf.String()),
		})
	case bundle.TypeKeyObjectType:
		id, _, err = mw.objectTypeCreate(&pb.RpcObjectCreateObjectTypeRequest{
			Details:       req.Details,
			InternalFlags: req.InternalFlags,
		})
	case bundle.TypeKeyRelation:
		rl, err2 := mw.relationCreate(&pb.RpcObjectCreateRelationRequest{
			Details: req.Details,
		})
		id = rl.Id
		err = err2
	case bundle.TypeKeyRelationOption:
		id, err = mw.objectCreateRelationOption(&pb.RpcObjectCreateRelationOptionRequest{
			Details: req.Details,
		})

	default:
		err = mw.doBlockService(func(bs block.Service) (err error) {
			id, _, err = bs.CreateSmartBlockFromTemplate(context.TODO(), coresb.SmartBlockTypePage, req.Details, nil, req.TemplateId)
			return
		})
	}

	if err != nil {
		return response(pb.RpcObjectCreateResponseError_UNKNOWN_ERROR, "", err)
	}
	return response(pb.RpcObjectCreateResponseError_NULL, id, nil)
}
