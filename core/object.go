package core

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/anyproto/go-naturaldate/v2"
	"github.com/araddon/dateparse"
	"github.com/gogo/protobuf/types"
	"github.com/hashicorp/go-multierror"

	"github.com/anyproto/anytype-heart/core/block"
	importer "github.com/anyproto/anytype-heart/core/block/import"
	"github.com/anyproto/anytype-heart/core/block/import/converter"
	"github.com/anyproto/anytype-heart/core/block/object/objectgraph"
	"github.com/anyproto/anytype-heart/core/indexer"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/core"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/database/filter"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/builtinobjects"
	"github.com/anyproto/anytype-heart/util/internalflag"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func (mw *Middleware) ObjectSetDetails(cctx context.Context, req *pb.RpcObjectSetDetailsRequest) *pb.RpcObjectSetDetailsResponse {
	ctx := mw.newContext(cctx)
	response := func(code pb.RpcObjectSetDetailsResponseErrorCode, err error) *pb.RpcObjectSetDetailsResponse {
		m := &pb.RpcObjectSetDetailsResponse{Error: &pb.RpcObjectSetDetailsResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = mw.getResponseEvent(ctx)
		}
		return m
	}
	err := mw.doBlockService(func(bs *block.Service) (err error) {
		return bs.SetDetails(ctx, *req)
	})
	if err != nil {
		return response(pb.RpcObjectSetDetailsResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcObjectSetDetailsResponseError_NULL, nil)
}

func (mw *Middleware) ObjectDuplicate(cctx context.Context, req *pb.RpcObjectDuplicateRequest) *pb.RpcObjectDuplicateResponse {
	response := func(templateId string, err error) *pb.RpcObjectDuplicateResponse {
		m := &pb.RpcObjectDuplicateResponse{
			Error: &pb.RpcObjectDuplicateResponseError{Code: pb.RpcObjectDuplicateResponseError_NULL},
			Id:    templateId,
		}
		if err != nil {
			m.Error.Code = pb.RpcObjectDuplicateResponseError_UNKNOWN_ERROR
			m.Error.Description = err.Error()
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
			m.Error.Description = err.Error()
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
			m.Error.Description = err.Error()
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
	records, _, err := ds.Query(database.Query{
		Filters:  req.Filters,
		Sorts:    req.Sorts,
		Offset:   int(req.Offset),
		Limit:    int(req.Limit),
		FullText: req.FullText,
	})
	if err != nil {
		return response(pb.RpcObjectSearchResponseError_UNKNOWN_ERROR, nil, err)
	}

	// Add dates only to the first page of search results
	if req.Offset == 0 {
		records, err = mw.enrichWithDateSuggestion(records, req, ds)
		if err != nil {
			return response(pb.RpcObjectSearchResponseError_UNKNOWN_ERROR, nil, err)
		}
	}

	var records2 = make([]*types.Struct, 0, len(records))
	for _, rec := range records {
		records2 = append(records2, pbtypes.Map(rec.Details, req.Keys...))
	}

	return response(pb.RpcObjectSearchResponseError_NULL, records2, nil)
}

func (mw *Middleware) enrichWithDateSuggestion(records []database.Record, req *pb.RpcObjectSearchRequest, store objectstore.ObjectStore) ([]database.Record, error) {
	dt := suggestDateForSearch(time.Now(), req.FullText)
	if dt.IsZero() {
		return records, nil
	}

	id := deriveDateId(dt)

	// Don't duplicate search suggestions
	var found bool
	for _, r := range records {
		if r.Details == nil || r.Details.Fields == nil {
			continue
		}
		if v, ok := r.Details.Fields[bundle.RelationKeyId.String()]; ok {
			if v.GetStringValue() == id {
				found = true
				break
			}
		}

	}
	if found {
		return records, nil
	}

	var rec database.Record
	var spaceID string
	for _, f := range req.Filters {
		if f.RelationKey == bundle.RelationKeySpaceId.String() && f.Condition == model.BlockContentDataviewFilter_Equal {
			spaceID = f.Value.GetStringValue()
			break
		}
	}
	rec = mw.makeSuggestedDateRecord(spaceID, dt)
	f, _ := filter.MakeAndFilter(req.Filters, store) //nolint:errcheck
	if vg := pbtypes.ValueGetter(rec.Details); f.FilterObject(vg) {
		return append([]database.Record{rec}, records...), nil
	}
	return records, nil
}

func suggestDateForSearch(now time.Time, raw string) time.Time {
	suggesters := []func() time.Time{
		func() time.Time {
			var exprType naturaldate.ExprType
			t, exprType, err := naturaldate.Parse(raw, now)
			if err != nil {
				return time.Time{}
			}
			if exprType == naturaldate.ExprTypeInvalid {
				return time.Time{}
			}

			// naturaldate parses numbers without qualifiers (m,s) as hours in 24 hours clock format. It leads to weird behavior
			// when inputs like "123" represented as "current time + 123 hours"
			if (exprType & naturaldate.ExprTypeClock24Hour) != 0 {
				t = time.Time{}
			}
			return t
		},
		func() time.Time {
			// Don't use plain numbers, because they will be represented as years
			if _, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil {
				return time.Time{}
			}
			// todo: use system locale to get preferred date format
			t, err := dateparse.ParseIn(raw, now.Location(), dateparse.PreferMonthFirst(false))
			if err != nil {
				return time.Time{}
			}
			return t
		},
	}

	var t time.Time
	for _, s := range suggesters {
		if t = s(); !t.IsZero() {
			break
		}
	}
	if t.IsZero() {
		return t
	}

	// Sanitize date

	// Date without year
	if t.Year() == 0 {
		_, month, day := t.Date()
		h, m, s := t.Clock()
		t = time.Date(now.Year(), month, day, h, m, s, 0, t.Location())
	}

	return t
}

func deriveDateId(t time.Time) string {
	return "_date_" + t.Format("2006-01-02")
}

func (mw *Middleware) makeSuggestedDateRecord(spaceID string, t time.Time) database.Record {
	id := deriveDateId(t)

	typeID := getService[core.Service](mw).GetSystemTypeID(spaceID, bundle.TypeKeyDate)
	d := &types.Struct{Fields: map[string]*types.Value{
		bundle.RelationKeyId.String():        pbtypes.String(id),
		bundle.RelationKeyName.String():      pbtypes.String(t.Format("Mon Jan  2 2006")),
		bundle.RelationKeyLayout.String():    pbtypes.Int64(int64(model.ObjectType_date)),
		bundle.RelationKeyType.String():      pbtypes.String(typeID),
		bundle.RelationKeyIconEmoji.String(): pbtypes.String("📅"),
		bundle.RelationKeySpaceId.String():   pbtypes.String(spaceID),
	}}

	return database.Record{
		Details: d,
	}
}

func (mw *Middleware) ObjectSearchSubscribe(cctx context.Context, req *pb.RpcObjectSearchSubscribeRequest) *pb.RpcObjectSearchSubscribeResponse {
	ctx := mw.newContext(cctx)
	errResponse := func(err error) *pb.RpcObjectSearchSubscribeResponse {
		r := &pb.RpcObjectSearchSubscribeResponse{
			Error: &pb.RpcObjectSearchSubscribeResponseError{
				Code: pb.RpcObjectSearchSubscribeResponseError_UNKNOWN_ERROR,
			},
		}
		if err != nil {
			r.Error.Description = err.Error()
		}
		return r
	}

	if mw.applicationService.GetApp() == nil {
		return errResponse(fmt.Errorf("account must be started"))
	}

	subService := mw.applicationService.GetApp().MustComponent(subscription.CName).(subscription.Service)

	resp, err := subService.Search(ctx, *req)
	if err != nil {
		return errResponse(err)
	}

	return resp
}

func (mw *Middleware) ObjectGroupsSubscribe(cctx context.Context, req *pb.RpcObjectGroupsSubscribeRequest) *pb.RpcObjectGroupsSubscribeResponse {
	ctx := mw.newContext(cctx)
	errResponse := func(err error) *pb.RpcObjectGroupsSubscribeResponse {
		r := &pb.RpcObjectGroupsSubscribeResponse{
			Error: &pb.RpcObjectGroupsSubscribeResponseError{
				Code: pb.RpcObjectGroupsSubscribeResponseError_UNKNOWN_ERROR,
			},
		}
		if err != nil {
			r.Error.Description = err.Error()
		}
		return r
	}

	if mw.applicationService.GetApp() == nil {
		return errResponse(errors.New("app must be started"))
	}

	subService := mw.applicationService.GetApp().MustComponent(subscription.CName).(subscription.Service)

	resp, err := subService.SubscribeGroups(ctx, *req)
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
			r.Error.Description = err.Error()
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
			r.Error.Description = err.Error()
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

	nodes, edges, err := getService[objectgraph.Service](mw).ObjectGraph(req)
	if err != nil {
		return unknownError(err)
	}
	return objectResponse(pb.RpcObjectGraphResponseError_NULL, nodes, edges, nil)
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

func (mw *Middleware) ObjectRelationAdd(cctx context.Context, req *pb.RpcObjectRelationAddRequest) *pb.RpcObjectRelationAddResponse {
	ctx := mw.newContext(cctx)
	response := func(code pb.RpcObjectRelationAddResponseErrorCode, err error) *pb.RpcObjectRelationAddResponse {
		m := &pb.RpcObjectRelationAddResponse{Error: &pb.RpcObjectRelationAddResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = mw.getResponseEvent(ctx)
		}
		return m
	}
	if len(req.RelationKeys) == 0 {
		return response(pb.RpcObjectRelationAddResponseError_BAD_INPUT, fmt.Errorf("relation is nil"))
	}

	err := mw.doBlockService(func(bs *block.Service) (err error) {
		return bs.AddExtraRelations(ctx, req.ContextId, req.RelationKeys)
	})
	if err != nil {
		return response(pb.RpcObjectRelationAddResponseError_BAD_INPUT, err)
	}

	return response(pb.RpcObjectRelationAddResponseError_NULL, nil)
}

func (mw *Middleware) ObjectRelationDelete(cctx context.Context, req *pb.RpcObjectRelationDeleteRequest) *pb.RpcObjectRelationDeleteResponse {
	ctx := mw.newContext(cctx)
	response := func(code pb.RpcObjectRelationDeleteResponseErrorCode, err error) *pb.RpcObjectRelationDeleteResponse {
		m := &pb.RpcObjectRelationDeleteResponse{Error: &pb.RpcObjectRelationDeleteResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = mw.getResponseEvent(ctx)
		}
		return m
	}
	err := mw.doBlockService(func(bs *block.Service) (err error) {
		return bs.RemoveExtraRelations(ctx, req.ContextId, req.RelationKeys)
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
			m.Error.Description = err.Error()
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
			m.Error.Description = err.Error()
		} else {
			m.Event = mw.getResponseEvent(ctx)
		}
		return m
	}

	if err := mw.doBlockService(func(bs *block.Service) (err error) {
		return bs.SetObjectTypes(ctx, req.ContextId, []string{req.ObjectTypeUrl})
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
			m.Error.Description = err.Error()
		}
		return m
	}

	if err := mw.doBlockService(func(bs *block.Service) (err error) {
		var (
			mErr       multierror.Error
			anySucceed bool
		)
		for _, objID := range req.ObjectIds {
			if err = bs.SetObjectTypes(ctx, objID, []string{req.ObjectTypeId}); err != nil {
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
			m.Error.Description = err.Error()
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

func (mw *Middleware) ObjectSetIsArchived(cctx context.Context, req *pb.RpcObjectSetIsArchivedRequest) *pb.RpcObjectSetIsArchivedResponse {
	response := func(code pb.RpcObjectSetIsArchivedResponseErrorCode, err error) *pb.RpcObjectSetIsArchivedResponse {
		m := &pb.RpcObjectSetIsArchivedResponse{Error: &pb.RpcObjectSetIsArchivedResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}
	err := mw.doBlockService(func(bs *block.Service) (err error) {
		return bs.SetPageIsArchived(*req)
	})
	if err != nil {
		return response(pb.RpcObjectSetIsArchivedResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcObjectSetIsArchivedResponseError_NULL, nil)
}

func (mw *Middleware) ObjectSetSource(cctx context.Context,
	req *pb.RpcObjectSetSourceRequest) *pb.RpcObjectSetSourceResponse {
	ctx := mw.newContext(cctx)
	response := func(code pb.RpcObjectSetSourceResponseErrorCode, err error) *pb.RpcObjectSetSourceResponse {
		m := &pb.RpcObjectSetSourceResponse{Error: &pb.RpcObjectSetSourceResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = mw.getResponseEvent(ctx)
		}
		return m
	}
	err := mw.doBlockService(func(bs *block.Service) (err error) {
		return bs.SetSource(ctx, *req)
	})
	if err != nil {
		return response(pb.RpcObjectSetSourceResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcObjectSetSourceResponseError_NULL, nil)
}

func (mw *Middleware) ObjectWorkspaceSetDashboard(cctx context.Context, req *pb.RpcObjectWorkspaceSetDashboardRequest) *pb.RpcObjectWorkspaceSetDashboardResponse {
	ctx := mw.newContext(cctx)
	response := func(setId string, err error) *pb.RpcObjectWorkspaceSetDashboardResponse {
		resp := &pb.RpcObjectWorkspaceSetDashboardResponse{
			ObjectId: setId,
			Error: &pb.RpcObjectWorkspaceSetDashboardResponseError{
				Code: pb.RpcObjectWorkspaceSetDashboardResponseError_NULL,
			},
		}
		if err != nil {
			resp.Error.Code = pb.RpcObjectWorkspaceSetDashboardResponseError_UNKNOWN_ERROR
			resp.Error.Description = err.Error()
		} else {
			resp.Event = mw.getResponseEvent(ctx)
		}
		return resp
	}
	var (
		setId string
		err   error
	)
	err = mw.doBlockService(func(bs *block.Service) error {
		if setId, err = bs.SetWorkspaceDashboardId(ctx, req.ContextId, req.ObjectId); err != nil {
			return err
		}
		return nil
	})
	return response(setId, err)
}

func (mw *Middleware) ObjectSetIsFavorite(cctx context.Context, req *pb.RpcObjectSetIsFavoriteRequest) *pb.RpcObjectSetIsFavoriteResponse {
	ctx := mw.newContext(cctx)
	response := func(code pb.RpcObjectSetIsFavoriteResponseErrorCode, err error) *pb.RpcObjectSetIsFavoriteResponse {
		m := &pb.RpcObjectSetIsFavoriteResponse{Error: &pb.RpcObjectSetIsFavoriteResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = mw.getResponseEvent(ctx)
		}
		return m
	}
	err := mw.doBlockService(func(bs *block.Service) (err error) {
		return bs.SetPageIsFavorite(*req)
	})
	if err != nil {
		return response(pb.RpcObjectSetIsFavoriteResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcObjectSetIsFavoriteResponseError_NULL, nil)
}

func (mw *Middleware) ObjectRelationAddFeatured(cctx context.Context, req *pb.RpcObjectRelationAddFeaturedRequest) *pb.RpcObjectRelationAddFeaturedResponse {
	ctx := mw.newContext(cctx)
	response := func(code pb.RpcObjectRelationAddFeaturedResponseErrorCode, err error) *pb.RpcObjectRelationAddFeaturedResponse {
		m := &pb.RpcObjectRelationAddFeaturedResponse{Error: &pb.RpcObjectRelationAddFeaturedResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
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
			m.Error.Description = err.Error()
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
	ctx := mw.newContext(cctx)
	response := func(err error) *pb.RpcObjectToSetResponse {
		resp := &pb.RpcObjectToSetResponse{
			Error: &pb.RpcObjectToSetResponseError{
				Code: pb.RpcObjectToSetResponseError_NULL,
			},
		}
		if err != nil {
			resp.Error.Code = pb.RpcObjectToSetResponseError_UNKNOWN_ERROR
			resp.Error.Description = err.Error()
		}
		return resp
	}
	var (
		err error
	)
	err = mw.doBlockService(func(bs *block.Service) error {
		if err = bs.ObjectToSet(ctx, req.ContextId, req.Source); err != nil {
			return err
		}
		return nil
	})
	return response(err)
}

func (mw *Middleware) ObjectCreateBookmark(cctx context.Context, req *pb.RpcObjectCreateBookmarkRequest) *pb.RpcObjectCreateBookmarkResponse {
	response := func(code pb.RpcObjectCreateBookmarkResponseErrorCode, id string, details *types.Struct, err error) *pb.RpcObjectCreateBookmarkResponse {
		m := &pb.RpcObjectCreateBookmarkResponse{Error: &pb.RpcObjectCreateBookmarkResponseError{Code: code}, ObjectId: id, Details: details}
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
		id, newDetails, err = bs.CreateObject(cctx, req.SpaceId, req, bundle.TypeKeyBookmark)
		return err
	})
	if err != nil {
		return response(pb.RpcObjectCreateBookmarkResponseError_UNKNOWN_ERROR, "", newDetails, err)
	}
	return response(pb.RpcObjectCreateBookmarkResponseError_NULL, id, newDetails, nil)
}

func (mw *Middleware) ObjectBookmarkFetch(cctx context.Context, req *pb.RpcObjectBookmarkFetchRequest) *pb.RpcObjectBookmarkFetchResponse {
	response := func(code pb.RpcObjectBookmarkFetchResponseErrorCode, err error) *pb.RpcObjectBookmarkFetchResponse {
		m := &pb.RpcObjectBookmarkFetchResponse{Error: &pb.RpcObjectBookmarkFetchResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
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

func (mw *Middleware) ObjectToBookmark(cctx context.Context, req *pb.RpcObjectToBookmarkRequest) *pb.RpcObjectToBookmarkResponse {
	response := func(code pb.RpcObjectToBookmarkResponseErrorCode, id string, err error) *pb.RpcObjectToBookmarkResponse {
		m := &pb.RpcObjectToBookmarkResponse{Error: &pb.RpcObjectToBookmarkResponseError{Code: code}, ObjectId: id}
		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}

	var id string
	err := mw.doBlockService(func(bs *block.Service) error {
		var err error
		id, err = bs.ObjectToBookmark(cctx, req.ContextId, req.Url)
		return err
	})

	if err != nil {
		return response(pb.RpcObjectToBookmarkResponseError_UNKNOWN_ERROR, "", err)
	}
	return response(pb.RpcObjectToBookmarkResponseError_NULL, id, nil)
}

func (mw *Middleware) ObjectSetInternalFlags(cctx context.Context, req *pb.RpcObjectSetInternalFlagsRequest) *pb.RpcObjectSetInternalFlagsResponse {
	ctx := mw.newContext(cctx)
	response := func(code pb.RpcObjectSetInternalFlagsResponseErrorCode, err error) *pb.RpcObjectSetInternalFlagsResponse {
		m := &pb.RpcObjectSetInternalFlagsResponse{Error: &pb.RpcObjectSetInternalFlagsResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = mw.getResponseEvent(ctx)
		}
		return m
	}
	err := mw.doBlockService(func(bs *block.Service) (err error) {
		return bs.ModifyDetails(ctx, req.ContextId, func(current *types.Struct) (*types.Struct, error) {
			d := pbtypes.CopyStruct(current)
			return internalflag.PutToDetails(d, req.InternalFlags), nil
		})
	})
	if err != nil {
		return response(pb.RpcObjectSetInternalFlagsResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcObjectSetInternalFlagsResponseError_NULL, nil)
}

func (mw *Middleware) ObjectImport(cctx context.Context, req *pb.RpcObjectImportRequest) *pb.RpcObjectImportResponse {
	response := func(code pb.RpcObjectImportResponseErrorCode, err error) *pb.RpcObjectImportResponse {
		m := &pb.RpcObjectImportResponse{Error: &pb.RpcObjectImportResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}

	if mw.applicationService.GetApp() == nil {
		return response(pb.RpcObjectImportResponseError_ACCOUNT_IS_NOT_RUNNING, fmt.Errorf("user didn't log in"))
	}

	importer := mw.applicationService.GetApp().MustComponent(importer.CName).(importer.Importer)
	err := importer.Import(cctx, req)

	if err == nil {
		return response(pb.RpcObjectImportResponseError_NULL, nil)
	}

	switch {
	case errors.Is(err, converter.ErrNoObjectsToImport):
		return response(pb.RpcObjectImportResponseError_NO_OBJECTS_TO_IMPORT, err)
	case errors.Is(err, converter.ErrCancel):
		return response(pb.RpcObjectImportResponseError_IMPORT_IS_CANCELED, err)
	case errors.Is(err, converter.ErrLimitExceeded):
		return response(pb.RpcObjectImportResponseError_LIMIT_OF_ROWS_OR_RELATIONS_EXCEEDED, err)
	default:
		return response(pb.RpcObjectImportResponseError_INTERNAL_ERROR, err)
	}
}

func (mw *Middleware) ObjectImportList(cctx context.Context, req *pb.RpcObjectImportListRequest) *pb.RpcObjectImportListResponse {
	response := func(res []*pb.RpcObjectImportListImportResponse, code pb.RpcObjectImportListResponseErrorCode, err error) *pb.RpcObjectImportListResponse {
		m := &pb.RpcObjectImportListResponse{Response: res, Error: &pb.RpcObjectImportListResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
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
	response := func(code pb.RpcObjectImportUseCaseResponseErrorCode, err error) *pb.RpcObjectImportUseCaseResponse {
		resp := &pb.RpcObjectImportUseCaseResponse{
			Error: &pb.RpcObjectImportUseCaseResponseError{
				Code: code,
			},
		}
		if err != nil {
			resp.Error.Description = err.Error()
		}
		return resp
	}

	objCreator := getService[builtinobjects.BuiltinObjects](mw)
	return response(objCreator.CreateObjectsForUseCase(cctx, req.SpaceId, req.UseCase))
}
