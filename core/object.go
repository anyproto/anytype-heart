package core

import (
	"context"
	"errors"
	"fmt"

	"github.com/gogo/protobuf/types"
	"github.com/hashicorp/go-multierror"
	"github.com/samber/lo"

	"github.com/anyproto/anytype-heart/core/block"
	importer "github.com/anyproto/anytype-heart/core/block/import"
	"github.com/anyproto/anytype-heart/core/block/import/common"
	"github.com/anyproto/anytype-heart/core/block/object/objectgraph"
	"github.com/anyproto/anytype-heart/core/date"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/domain/objectorigin"
	"github.com/anyproto/anytype-heart/core/indexer"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/core/subscription/crossspacesub"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space"
	"github.com/anyproto/anytype-heart/util/builtinobjects"
	"github.com/anyproto/anytype-heart/util/slice"
)

func (mw *Middleware) ObjectDuplicate(cctx context.Context, req *pb.RpcObjectDuplicateRequest) *pb.RpcObjectDuplicateResponse {
	response := func(templateId string, err error) *pb.RpcObjectDuplicateResponse {
		m := &pb.RpcObjectDuplicateResponse{
			Error: &pb.RpcObjectDuplicateResponseError{Code: pb.RpcObjectDuplicateResponseError_NULL},
			Id:    templateId,
		}
		if err != nil {
			m.Error.Code = pb.RpcObjectDuplicateResponseError_UNKNOWN_ERROR
			m.Error.Description = getErrorDescription(err)
		}
		return m
	}
	var objectIds []string
	err := mw.doBlockService(func(bs *block.Service) (err error) {
		objectIds, err = bs.ObjectsDuplicate(cctx, []string{req.ContextId})
		return
	})
	if len(objectIds) == 0 {
		return response("", err)
	}
	return response(objectIds[0], err)
}

func (mw *Middleware) ObjectListDuplicate(cctx context.Context, req *pb.RpcObjectListDuplicateRequest) *pb.RpcObjectListDuplicateResponse {
	response := func(objectIds []string, err error) *pb.RpcObjectListDuplicateResponse {
		m := &pb.RpcObjectListDuplicateResponse{
			Error: &pb.RpcObjectListDuplicateResponseError{Code: pb.RpcObjectListDuplicateResponseError_NULL},
			Ids:   objectIds,
		}
		if err != nil {
			m.Error.Code = pb.RpcObjectListDuplicateResponseError_UNKNOWN_ERROR
			m.Error.Description = getErrorDescription(err)
		}
		return m
	}
	var objectIds []string
	err := mw.doBlockService(func(bs *block.Service) (err error) {
		objectIds, err = bs.ObjectsDuplicate(cctx, req.ObjectIds)
		return
	})
	return response(objectIds, err)
}

func (mw *Middleware) ObjectSearch(cctx context.Context, req *pb.RpcObjectSearchRequest) *pb.RpcObjectSearchResponse {
	response := func(code pb.RpcObjectSearchResponseErrorCode, records []*types.Struct, err error) *pb.RpcObjectSearchResponse {
		m := &pb.RpcObjectSearchResponse{Error: &pb.RpcObjectSearchResponseError{Code: code}, Records: records}
		if err != nil {
			m.Error.Description = getErrorDescription(err)
		}

		return m
	}

	if mw.applicationService.GetApp() == nil {
		return response(pb.RpcObjectSearchResponseError_BAD_INPUT, nil, fmt.Errorf("account must be started"))
	}

	if req.FullText != "" {
		mw.applicationService.GetApp().MustComponent(indexer.CName).(indexer.Indexer).ForceFTIndex()
	}

	ds := mw.applicationService.GetApp().MustComponent(objectstore.CName).(objectstore.ObjectStore)
	records, err := ds.SpaceIndex(req.SpaceId).Query(database.Query{
		Filters:         database.FiltersFromProto(req.Filters),
		SpaceId:         req.SpaceId,
		Sorts:           database.SortsFromProto(req.Sorts),
		Offset:          int(req.Offset),
		Limit:           int(req.Limit),
		TextQuery:       req.FullText,
		PrefixNameQuery: true,
	})
	if err != nil {
		return response(pb.RpcObjectSearchResponseError_UNKNOWN_ERROR, nil, err)
	}

	// Add dates only to the first page of search results
	if req.Offset == 0 {
		records, err = date.EnrichRecordsWithDateSuggestions(cctx, req.SpaceId, req.FullText, records, req.Filters, ds, mustService[space.Service](mw))
		if err != nil {
			return response(pb.RpcObjectSearchResponseError_UNKNOWN_ERROR, nil, err)
		}
	}

	var records2 = make([]*domain.Details, 0, len(records))
	for _, rec := range records {
		if len(req.Keys) == 0 {
			records2 = append(records2, rec.Details)
		} else {
			records2 = append(records2, rec.Details.CopyOnlyKeys(slice.StringsInto[domain.RelationKey](req.Keys)...))
		}
	}

	protoRecords := lo.Map(records2, func(item *domain.Details, _ int) *types.Struct {
		return item.ToProto()
	})
	return response(pb.RpcObjectSearchResponseError_NULL, protoRecords, nil)
}

func (mw *Middleware) ObjectSearchWithMeta(cctx context.Context, req *pb.RpcObjectSearchWithMetaRequest) *pb.RpcObjectSearchWithMetaResponse {
	response := func(code pb.RpcObjectSearchWithMetaResponseErrorCode, results []*model.SearchResult, err error) *pb.RpcObjectSearchWithMetaResponse {
		m := &pb.RpcObjectSearchWithMetaResponse{Error: &pb.RpcObjectSearchWithMetaResponseError{Code: code}, Results: results}
		if err != nil {
			m.Error.Description = getErrorDescription(err)
		}

		return m
	}

	if mw.applicationService.GetApp() == nil {
		return response(pb.RpcObjectSearchWithMetaResponseError_BAD_INPUT, nil, fmt.Errorf("account must be started"))
	}

	if req.FullText != "" {
		mw.applicationService.GetApp().MustComponent(indexer.CName).(indexer.Indexer).ForceFTIndex()
	}

	ds := mw.applicationService.GetApp().MustComponent(objectstore.CName).(objectstore.ObjectStore)
	results, err := ds.SpaceIndex(req.SpaceId).Query(database.Query{
		Filters:   database.FiltersFromProto(req.Filters),
		Sorts:     database.SortsFromProto(req.Sorts),
		Offset:    int(req.Offset),
		Limit:     int(req.Limit),
		TextQuery: req.FullText,
		SpaceId:   req.SpaceId,
	})

	// Add dates only to the first page of search results
	if req.Offset == 0 {
		results, err = date.EnrichRecordsWithDateSuggestions(cctx, req.SpaceId, req.FullText, results, req.Filters, ds, mustService[space.Service](mw))
		if err != nil {
			return response(pb.RpcObjectSearchWithMetaResponseError_UNKNOWN_ERROR, nil, err)
		}
	}

	var resultsModels = make([]*model.SearchResult, 0, len(results))
	for i, rec := range results {
		if len(req.Keys) > 0 {
			rec.Details = rec.Details.CopyOnlyKeys(slice.StringsInto[domain.RelationKey](req.Keys)...)
		}
		resultsModels = append(resultsModels, &model.SearchResult{

			ObjectId: rec.Details.GetString(bundle.RelationKeyId),
			Details:  rec.Details.ToProto(),
			Meta:     []*model.SearchMeta{&(results[i].Meta)},
		})
	}
	if err != nil {
		return response(pb.RpcObjectSearchWithMetaResponseError_UNKNOWN_ERROR, nil, err)
	}

	return response(pb.RpcObjectSearchWithMetaResponseError_NULL, resultsModels, nil)
}

func (mw *Middleware) ObjectSearchSubscribe(cctx context.Context, req *pb.RpcObjectSearchSubscribeRequest) *pb.RpcObjectSearchSubscribeResponse {
	errResponse := func(err error) *pb.RpcObjectSearchSubscribeResponse {
		r := &pb.RpcObjectSearchSubscribeResponse{
			Error: &pb.RpcObjectSearchSubscribeResponseError{
				Code: pb.RpcObjectSearchSubscribeResponseError_UNKNOWN_ERROR,
			},
		}
		if err != nil {
			r.Error.Description = getErrorDescription(err)
		}
		return r
	}

	if mw.applicationService.GetApp() == nil {
		return errResponse(fmt.Errorf("account must be started"))
	}

	subService := mw.applicationService.GetApp().MustComponent(subscription.CName).(subscription.Service)

	resp, err := subService.Search(subscription.SubscribeRequest{
		SpaceId:           req.SpaceId,
		SubId:             req.SubId,
		Filters:           database.FiltersFromProto(req.Filters),
		Sorts:             database.SortsFromProto(req.Sorts),
		Limit:             req.Limit,
		Offset:            req.Offset,
		Keys:              req.Keys,
		AfterId:           req.AfterId,
		BeforeId:          req.BeforeId,
		Source:            req.Source,
		NoDepSubscription: req.NoDepSubscription,
		CollectionId:      req.CollectionId,
	})
	if err != nil {
		return errResponse(err)
	}

	return &pb.RpcObjectSearchSubscribeResponse{
		SubId:        resp.SubId,
		Records:      domain.DetailsListToProtos(resp.Records),
		Dependencies: domain.DetailsListToProtos(resp.Dependencies),
		Counters:     resp.Counters,
	}
}

func (mw *Middleware) ObjectCrossSpaceSearchSubscribe(cctx context.Context, req *pb.RpcObjectCrossSpaceSearchSubscribeRequest) *pb.RpcObjectCrossSpaceSearchSubscribeResponse {
	subService := mustService[crossspacesub.Service](mw)
	resp, err := subService.Subscribe(subscription.SubscribeRequest{
		SubId:             req.SubId,
		Filters:           database.FiltersFromProto(req.Filters),
		Sorts:             database.SortsFromProto(req.Sorts),
		Keys:              req.Keys,
		Source:            req.Source,
		NoDepSubscription: req.NoDepSubscription,
		CollectionId:      req.CollectionId,
	}, crossspacesub.NoOpPredicate())
	if err != nil {
		return &pb.RpcObjectCrossSpaceSearchSubscribeResponse{
			Error: &pb.RpcObjectCrossSpaceSearchSubscribeResponseError{
				Code:        pb.RpcObjectCrossSpaceSearchSubscribeResponseError_UNKNOWN_ERROR,
				Description: getErrorDescription(err),
			},
		}
	}

	return &pb.RpcObjectCrossSpaceSearchSubscribeResponse{
		SubId:        resp.SubId,
		Records:      domain.DetailsListToProtos(resp.Records),
		Dependencies: domain.DetailsListToProtos(resp.Dependencies),
		Counters:     resp.Counters,
	}
}

func (mw *Middleware) ObjectCrossSpaceSearchUnsubscribe(cctx context.Context, req *pb.RpcObjectCrossSpaceSearchUnsubscribeRequest) *pb.RpcObjectCrossSpaceSearchUnsubscribeResponse {
	subService := mustService[crossspacesub.Service](mw)
	err := subService.Unsubscribe(req.SubId)
	if err != nil {
		return &pb.RpcObjectCrossSpaceSearchUnsubscribeResponse{
			Error: &pb.RpcObjectCrossSpaceSearchUnsubscribeResponseError{
				Code:        pb.RpcObjectCrossSpaceSearchUnsubscribeResponseError_UNKNOWN_ERROR,
				Description: getErrorDescription(err),
			},
		}
	}
	return &pb.RpcObjectCrossSpaceSearchUnsubscribeResponse{}
}

func (mw *Middleware) ObjectGroupsSubscribe(_ context.Context, req *pb.RpcObjectGroupsSubscribeRequest) *pb.RpcObjectGroupsSubscribeResponse {
	errResponse := func(err error) *pb.RpcObjectGroupsSubscribeResponse {
		r := &pb.RpcObjectGroupsSubscribeResponse{
			Error: &pb.RpcObjectGroupsSubscribeResponseError{
				Code: pb.RpcObjectGroupsSubscribeResponseError_UNKNOWN_ERROR,
			},
		}
		if err != nil {
			r.Error.Description = getErrorDescription(err)
		}
		return r
	}

	if mw.applicationService.GetApp() == nil {
		return errResponse(errors.New("app must be started"))
	}

	subService := mw.applicationService.GetApp().MustComponent(subscription.CName).(subscription.Service)

	resp, err := subService.SubscribeGroups(subscription.SubscribeGroupsRequest{
		SpaceId:      req.SpaceId,
		SubId:        req.SubId,
		RelationKey:  req.RelationKey,
		Filters:      database.FiltersFromProto(req.Filters),
		Source:       req.Source,
		CollectionId: req.CollectionId,
	})
	if err != nil {
		return errResponse(err)
	}

	return resp
}

func (mw *Middleware) ObjectSubscribeIds(_ context.Context, req *pb.RpcObjectSubscribeIdsRequest) *pb.RpcObjectSubscribeIdsResponse {
	errResponse := func(err error) *pb.RpcObjectSubscribeIdsResponse {
		r := &pb.RpcObjectSubscribeIdsResponse{
			Error: &pb.RpcObjectSubscribeIdsResponseError{
				Code: pb.RpcObjectSubscribeIdsResponseError_UNKNOWN_ERROR,
			},
		}
		if err != nil {
			r.Error.Description = getErrorDescription(err)
		}
		return r
	}

	if mw.applicationService.GetApp() == nil {
		return errResponse(fmt.Errorf("account must be started"))
	}

	subService := mw.applicationService.GetApp().MustComponent(subscription.CName).(subscription.Service)

	resp, err := subService.SubscribeIdsReq(*req)
	if err != nil {
		return errResponse(err)
	}

	return resp
}

func (mw *Middleware) ObjectSearchUnsubscribe(cctx context.Context, req *pb.RpcObjectSearchUnsubscribeRequest) *pb.RpcObjectSearchUnsubscribeResponse {
	response := func(err error) *pb.RpcObjectSearchUnsubscribeResponse {
		r := &pb.RpcObjectSearchUnsubscribeResponse{
			Error: &pb.RpcObjectSearchUnsubscribeResponseError{
				Code: pb.RpcObjectSearchUnsubscribeResponseError_NULL,
			},
		}
		if err != nil {
			r.Error.Code = pb.RpcObjectSearchUnsubscribeResponseError_UNKNOWN_ERROR
			r.Error.Description = getErrorDescription(err)
		}
		return r
	}

	if mw.applicationService.GetApp() == nil {
		return response(fmt.Errorf("account must be started"))
	}

	subService := mw.applicationService.GetApp().MustComponent(subscription.CName).(subscription.Service)

	err := subService.Unsubscribe(req.SubIds...)
	if err != nil {
		return response(err)
	}
	return response(nil)
}

func (mw *Middleware) ObjectGraph(cctx context.Context, req *pb.RpcObjectGraphRequest) *pb.RpcObjectGraphResponse {
	if mw.applicationService.GetApp() == nil {
		return objectResponse(
			pb.RpcObjectGraphResponseError_BAD_INPUT,
			nil,
			nil,
			fmt.Errorf("account must be started"),
		)
	}

	nodes, edges, err := mustService[objectgraph.Service](mw).ObjectGraph(objectgraph.ObjectGraphRequest{
		Filters:          database.FiltersFromProto(req.Filters),
		Limit:            req.Limit,
		ObjectTypeFilter: req.ObjectTypeFilter,
		Keys:             req.Keys,
		SpaceId:          req.SpaceId,
		CollectionId:     req.CollectionId,
		SetSource:        req.SetSource,
		IncludeTypeEdges: req.IncludeTypeEdges,
	})
	if err != nil {
		return unknownError(err)
	}
	return objectResponse(pb.RpcObjectGraphResponseError_NULL, domain.DetailsListToProtos(nodes), edges, nil)
}

func unknownError(err error) *pb.RpcObjectGraphResponse {
	return objectResponse(pb.RpcObjectGraphResponseError_UNKNOWN_ERROR, nil, nil, err)
}

func objectResponse(
	code pb.RpcObjectGraphResponseErrorCode,
	nodes []*types.Struct,
	edges []*pb.RpcObjectGraphEdge,
	err error,
) *pb.RpcObjectGraphResponse {
	response := &pb.RpcObjectGraphResponse{
		Error: &pb.RpcObjectGraphResponseError{
			Code: code,
		},
		Nodes: nodes,
		Edges: edges,
	}

	if err != nil {
		response.Error.Description = err.Error()
	}

	return response
}

func (mw *Middleware) ObjectRelationDelete(cctx context.Context, req *pb.RpcObjectRelationDeleteRequest) *pb.RpcObjectRelationDeleteResponse {
	ctx := mw.newContext(cctx)
	response := func(code pb.RpcObjectRelationDeleteResponseErrorCode, err error) *pb.RpcObjectRelationDeleteResponse {
		m := &pb.RpcObjectRelationDeleteResponse{Error: &pb.RpcObjectRelationDeleteResponseError{Code: code}}
		if err != nil {
			m.Error.Description = getErrorDescription(err)
		} else {
			m.Event = mw.getResponseEvent(ctx)
		}
		return m
	}
	err := mw.doBlockService(func(bs *block.Service) (err error) {
		return bs.RemoveRelations(ctx, req.ContextId, req.RelationKeys)
	})
	if err != nil {
		return response(pb.RpcObjectRelationDeleteResponseError_BAD_INPUT, err)
	}
	return response(pb.RpcObjectRelationDeleteResponseError_NULL, nil)
}

func (mw *Middleware) ObjectRelationListAvailable(cctx context.Context, req *pb.RpcObjectRelationListAvailableRequest) *pb.RpcObjectRelationListAvailableResponse {
	ctx := mw.newContext(cctx)
	response := func(code pb.RpcObjectRelationListAvailableResponseErrorCode, relations []*model.Relation, err error) *pb.RpcObjectRelationListAvailableResponse {
		m := &pb.RpcObjectRelationListAvailableResponse{Relations: relations, Error: &pb.RpcObjectRelationListAvailableResponseError{Code: code}}
		if err != nil {
			m.Error.Description = getErrorDescription(err)
		}
		return m
	}
	var rels []*model.Relation
	err := mw.doBlockService(func(bs *block.Service) (err error) {
		rels, err = bs.ListAvailableRelations(ctx, req.ContextId)
		return
	})

	if err != nil {
		return response(pb.RpcObjectRelationListAvailableResponseError_UNKNOWN_ERROR, nil, err)
	}

	return response(pb.RpcObjectRelationListAvailableResponseError_NULL, rels, nil)
}

func (mw *Middleware) ObjectSetObjectType(cctx context.Context, req *pb.RpcObjectSetObjectTypeRequest) *pb.RpcObjectSetObjectTypeResponse {
	ctx := mw.newContext(cctx)
	response := func(code pb.RpcObjectSetObjectTypeResponseErrorCode, err error) *pb.RpcObjectSetObjectTypeResponse {
		m := &pb.RpcObjectSetObjectTypeResponse{Error: &pb.RpcObjectSetObjectTypeResponseError{Code: code}}
		if err != nil {
			m.Error.Description = getErrorDescription(err)
		} else {
			m.Event = mw.getResponseEvent(ctx)
		}
		return m
	}

	if err := mw.doBlockService(func(bs *block.Service) (err error) {
		return bs.SetObjectTypes(ctx, req.ContextId, []string{req.ObjectTypeUniqueKey})
	}); err != nil {
		return response(pb.RpcObjectSetObjectTypeResponseError_UNKNOWN_ERROR, err)
	}

	return response(pb.RpcObjectSetObjectTypeResponseError_NULL, nil)
}

func (mw *Middleware) ObjectListSetObjectType(cctx context.Context, req *pb.RpcObjectListSetObjectTypeRequest) *pb.RpcObjectListSetObjectTypeResponse {
	ctx := mw.newContext(cctx)
	response := func(code pb.RpcObjectListSetObjectTypeResponseErrorCode, err error) *pb.RpcObjectListSetObjectTypeResponse {
		m := &pb.RpcObjectListSetObjectTypeResponse{Error: &pb.RpcObjectListSetObjectTypeResponseError{Code: code}}
		if err != nil {
			m.Error.Description = getErrorDescription(err)
		} else {
			m.Event = mw.getResponseEvent(ctx)
		}
		return m
	}

	if err := mw.doBlockService(func(bs *block.Service) (err error) {
		var (
			mErr       multierror.Error
			anySucceed bool
		)
		for _, objID := range req.ObjectIds {
			if err = bs.SetObjectTypes(ctx, objID, []string{req.ObjectTypeUniqueKey}); err != nil {
				log.With("objectID", objID).Errorf("failed to set object type to object '%s': %v", objID, err)
				mErr.Errors = append(mErr.Errors, err)
			} else {
				anySucceed = true
			}
		}
		if anySucceed {
			return nil
		}
		return mErr.ErrorOrNil()
	}); err != nil {
		return response(pb.RpcObjectListSetObjectTypeResponseError_UNKNOWN_ERROR, err)
	}

	return response(pb.RpcObjectListSetObjectTypeResponseError_NULL, nil)
}

func (mw *Middleware) ObjectSetLayout(cctx context.Context, req *pb.RpcObjectSetLayoutRequest) *pb.RpcObjectSetLayoutResponse {
	ctx := mw.newContext(cctx)
	response := func(code pb.RpcObjectSetLayoutResponseErrorCode, err error) *pb.RpcObjectSetLayoutResponse {
		m := &pb.RpcObjectSetLayoutResponse{Error: &pb.RpcObjectSetLayoutResponseError{Code: code}}
		if err != nil {
			m.Error.Description = getErrorDescription(err)
		} else {
			m.Event = mw.getResponseEvent(ctx)
		}
		return m
	}
	err := mw.doBlockService(func(bs *block.Service) (err error) {
		return bs.SetLayout(ctx, req.ContextId, req.Layout)
	})
	if err != nil {
		return response(pb.RpcObjectSetLayoutResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcObjectSetLayoutResponseError_NULL, nil)
}

func (mw *Middleware) ObjectRelationAddFeatured(cctx context.Context, req *pb.RpcObjectRelationAddFeaturedRequest) *pb.RpcObjectRelationAddFeaturedResponse {
	ctx := mw.newContext(cctx)
	response := func(code pb.RpcObjectRelationAddFeaturedResponseErrorCode, err error) *pb.RpcObjectRelationAddFeaturedResponse {
		m := &pb.RpcObjectRelationAddFeaturedResponse{Error: &pb.RpcObjectRelationAddFeaturedResponseError{Code: code}}
		if err != nil {
			m.Error.Description = getErrorDescription(err)
		} else {
			m.Event = mw.getResponseEvent(ctx)
		}
		return m
	}
	err := mw.doBlockService(func(bs *block.Service) (err error) {
		return bs.FeaturedRelationAdd(ctx, req.ContextId, req.Relations...)
	})
	if err != nil {
		return response(pb.RpcObjectRelationAddFeaturedResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcObjectRelationAddFeaturedResponseError_NULL, nil)
}

func (mw *Middleware) ObjectRelationRemoveFeatured(cctx context.Context, req *pb.RpcObjectRelationRemoveFeaturedRequest) *pb.RpcObjectRelationRemoveFeaturedResponse {
	ctx := mw.newContext(cctx)
	response := func(code pb.RpcObjectRelationRemoveFeaturedResponseErrorCode, err error) *pb.RpcObjectRelationRemoveFeaturedResponse {
		m := &pb.RpcObjectRelationRemoveFeaturedResponse{Error: &pb.RpcObjectRelationRemoveFeaturedResponseError{Code: code}}
		if err != nil {
			m.Error.Description = getErrorDescription(err)
		} else {
			m.Event = mw.getResponseEvent(ctx)
		}
		return m
	}
	err := mw.doBlockService(func(bs *block.Service) (err error) {
		return bs.FeaturedRelationRemove(ctx, req.ContextId, req.Relations...)
	})
	if err != nil {
		return response(pb.RpcObjectRelationRemoveFeaturedResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcObjectRelationRemoveFeaturedResponseError_NULL, nil)
}

func (mw *Middleware) ObjectToSet(cctx context.Context, req *pb.RpcObjectToSetRequest) *pb.RpcObjectToSetResponse {
	response := func(err error) *pb.RpcObjectToSetResponse {
		resp := &pb.RpcObjectToSetResponse{
			Error: &pb.RpcObjectToSetResponseError{
				Code: pb.RpcObjectToSetResponseError_NULL,
			},
		}
		if err != nil {
			resp.Error.Code = pb.RpcObjectToSetResponseError_UNKNOWN_ERROR
			resp.Error.Description = getErrorDescription(err)
		}
		return resp
	}
	err := mw.doBlockService(func(bs *block.Service) error {
		return bs.ObjectToSet(req.ContextId, req.Source)
	})
	return response(err)
}

func (mw *Middleware) ObjectBookmarkFetch(cctx context.Context, req *pb.RpcObjectBookmarkFetchRequest) *pb.RpcObjectBookmarkFetchResponse {
	response := func(code pb.RpcObjectBookmarkFetchResponseErrorCode, err error) *pb.RpcObjectBookmarkFetchResponse {
		m := &pb.RpcObjectBookmarkFetchResponse{Error: &pb.RpcObjectBookmarkFetchResponseError{Code: code}}
		if err != nil {
			m.Error.Description = getErrorDescription(err)
		}
		return m
	}

	err := mw.doBlockService(func(bs *block.Service) error {
		return bs.ObjectBookmarkFetch(*req)
	})

	if err != nil {
		return response(pb.RpcObjectBookmarkFetchResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcObjectBookmarkFetchResponseError_NULL, nil)
}

func (mw *Middleware) ObjectImport(cctx context.Context, req *pb.RpcObjectImportRequest) *pb.RpcObjectImportResponse {
	importRequest := &importer.ImportRequest{
		RpcObjectImportRequest: req,
		Origin:                 objectorigin.Import(req.Type),
		Progress:               nil,
		SendNotification:       true,
		IsSync:                 false,
	}

	mustService[importer.Importer](mw).Import(cctx, importRequest)
	return &pb.RpcObjectImportResponse{}
}

func (mw *Middleware) ObjectImportList(cctx context.Context, req *pb.RpcObjectImportListRequest) *pb.RpcObjectImportListResponse {
	response := func(res []*pb.RpcObjectImportListImportResponse, code pb.RpcObjectImportListResponseErrorCode, err error) *pb.RpcObjectImportListResponse {
		m := &pb.RpcObjectImportListResponse{Response: res, Error: &pb.RpcObjectImportListResponseError{Code: code}}
		if err != nil {
			m.Error.Description = getErrorDescription(err)
		}
		return m
	}

	importer := mw.applicationService.GetApp().MustComponent(importer.CName).(importer.Importer)
	res, err := importer.ListImports(req)

	if err != nil {
		return response(res, pb.RpcObjectImportListResponseError_INTERNAL_ERROR, err)
	}
	return response(res, pb.RpcObjectImportListResponseError_NULL, nil)
}

func (mw *Middleware) ObjectImportNotionValidateToken(ctx context.Context,
	request *pb.RpcObjectImportNotionValidateTokenRequest) *pb.RpcObjectImportNotionValidateTokenResponse {
	// nolint: lll
	response := func(code pb.RpcObjectImportNotionValidateTokenResponseErrorCode, e error) *pb.RpcObjectImportNotionValidateTokenResponse {
		err := &pb.RpcObjectImportNotionValidateTokenResponseError{Code: code}
		switch code {
		case pb.RpcObjectImportNotionValidateTokenResponseError_UNAUTHORIZED:
			err.Description = "Sorry, token not found. Please check Notion integrations."
		case pb.RpcObjectImportNotionValidateTokenResponseError_FORBIDDEN:
			err.Description = "Unable to access user information, check capabilities in Notion (requires reading user information)."
		case pb.RpcObjectImportNotionValidateTokenResponseError_SERVICE_UNAVAILABLE:
			err.Description = "Notion is currently unavailable."
		case pb.RpcObjectImportNotionValidateTokenResponseError_NULL:
			err.Description = ""
		case pb.RpcObjectImportNotionValidateTokenResponseError_INTERNAL_ERROR:
			err.Description = e.Error()
		case pb.RpcObjectImportNotionValidateTokenResponseError_ACCOUNT_IS_NOT_RUNNING:
			err.Description = "User didn't log in"
		default:
			err.Description = "Unknown internal error"
		}
		return &pb.RpcObjectImportNotionValidateTokenResponse{Error: err}
	}

	if mw.applicationService.GetApp() == nil {
		return response(pb.RpcObjectImportNotionValidateTokenResponseError_ACCOUNT_IS_NOT_RUNNING, nil)
	}

	importer := mw.applicationService.GetApp().MustComponent(importer.CName).(importer.Importer)
	errCode, err := importer.ValidateNotionToken(ctx, request)
	return response(errCode, err)
}

func (mw *Middleware) ObjectImportUseCase(cctx context.Context, req *pb.RpcObjectImportUseCaseRequest) *pb.RpcObjectImportUseCaseResponse {
	ctx := mw.newContext(cctx)

	response := func(startingPageId string, code pb.RpcObjectImportUseCaseResponseErrorCode, err error) *pb.RpcObjectImportUseCaseResponse {
		resp := &pb.RpcObjectImportUseCaseResponse{
			StartingObjectId: startingPageId,
			Error: &pb.RpcObjectImportUseCaseResponseError{
				Code: code,
			},
		}
		if err != nil {
			resp.Error.Description = getErrorDescription(err)
		} else {
			resp.Event = ctx.GetResponseEvent()
		}
		return resp
	}

	objCreator := mustService[builtinobjects.BuiltinObjects](mw)
	return response(objCreator.CreateObjectsForUseCase(ctx, req.SpaceId, req.UseCase))
}

func (mw *Middleware) ObjectImportExperience(ctx context.Context, req *pb.RpcObjectImportExperienceRequest) *pb.RpcObjectImportExperienceResponse {
	response := func(code pb.RpcObjectImportExperienceResponseErrorCode, err error) *pb.RpcObjectImportExperienceResponse {
		resp := &pb.RpcObjectImportExperienceResponse{
			Error: &pb.RpcObjectImportExperienceResponseError{
				Code: code,
			},
		}
		if err != nil {
			resp.Error.Description = getErrorDescription(err)
		}
		return resp
	}

	objCreator := mustService[builtinobjects.BuiltinObjects](mw)
	err := objCreator.CreateObjectsForExperience(ctx, req.SpaceId, req.Url, req.Title, req.IsNewSpace)
	return response(common.GetGalleryResponseCode(err), err)
}

func (mw *Middleware) ObjectDateByTimestamp(ctx context.Context, req *pb.RpcObjectDateByTimestampRequest) *pb.RpcObjectDateByTimestampResponse {
	spaceService := mustService[space.Service](mw)
	details, err := date.BuildDetailsFromTimestamp(ctx, spaceService, req.SpaceId, req.Timestamp)

	if err != nil {
		return &pb.RpcObjectDateByTimestampResponse{
			Error: &pb.RpcObjectDateByTimestampResponseError{
				Code:        pb.RpcObjectDateByTimestampResponseError_UNKNOWN_ERROR,
				Description: getErrorDescription(err),
			},
		}
	}

	return &pb.RpcObjectDateByTimestampResponse{
		Details: details.ToProto(),
	}
}
