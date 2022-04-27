package core

import (
	"fmt"
	"github.com/anytypeio/go-anytype-middleware/core/indexer"
	"strings"
	"time"

	"github.com/anytypeio/go-anytype-middleware/core/subscription"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core/smartblock"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database/filter"
	"github.com/araddon/dateparse"
	"github.com/gogo/protobuf/types"
	"github.com/tj/go-naturaldate"

	"github.com/anytypeio/go-anytype-middleware/core/block"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/core"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/database"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

// To be renamed to ObjectSetDetails
func (mw *Middleware) BlockSetDetails(req *pb.RpcBlockSetDetailsRequest) *pb.RpcBlockSetDetailsResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcBlockSetDetailsResponseErrorCode, err error) *pb.RpcBlockSetDetailsResponse {
		m := &pb.RpcBlockSetDetailsResponse{Error: &pb.RpcBlockSetDetailsResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	err := mw.doBlockService(func(bs block.Service) (err error) {
		return bs.SetDetails(ctx, *req)
	})
	if err != nil {
		return response(pb.RpcBlockSetDetailsResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcBlockSetDetailsResponseError_NULL, nil)
}

func (mw *Middleware) ObjectDuplicate(req *pb.RpcObjectDuplicateRequest) *pb.RpcObjectDuplicateResponse {
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
	var objectId string
	err := mw.doBlockService(func(bs block.Service) (err error) {
		objectId, err = bs.ObjectDuplicate(req.ContextId)
		return
	})
	return response(objectId, err)
}

func handleDateSearch(req *pb.RpcObjectSearchRequest, records []database.Record) []database.Record {
	n := time.Now()
	f, _ := filter.MakeAndFilter(req.Filters)
	t, err := naturaldate.Parse(req.FullText, n)
	if err == nil {
		if t.Equal(n) && !strings.EqualFold(req.FullText, "now") {
			// naturaldate pkg returns NOW by default, but we don't need it
			t = time.Time{}
		}
	} else {
		// todo: use system locale to get preferred date format
		t, err = dateparse.ParseAny(req.FullText, dateparse.PreferMonthFirst(false))
	}

	if !t.IsZero() {
		d := &types.Struct{Fields: map[string]*types.Value{
			"id":        pbtypes.String("_date_" + t.Format("2006-01-02")),
			"name":      pbtypes.String(t.Format("Mon Jan  2 2006")),
			"type":      pbtypes.String(bundle.TypeKeyDate.URL()),
			"iconEmoji": pbtypes.String("📅"),
		}}
		if vg := pbtypes.ValueGetter(d); f.FilterObject(vg) {
			records = append([]database.Record{{Details: d}}, records...)
		}
	}

	return records
}

func (mw *Middleware) ObjectSearch(req *pb.RpcObjectSearchRequest) *pb.RpcObjectSearchResponse {
	response := func(code pb.RpcObjectSearchResponseErrorCode, records []*types.Struct, err error) *pb.RpcObjectSearchResponse {
		m := &pb.RpcObjectSearchResponse{Error: &pb.RpcObjectSearchResponseError{Code: code}, Records: records}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	mw.m.RLock()
	defer mw.m.RUnlock()

	if mw.app == nil {
		return response(pb.RpcObjectSearchResponseError_BAD_INPUT, nil, fmt.Errorf("account must be started"))
	}

	at := mw.app.MustComponent(core.CName).(core.Service)
	if req.FullText != "" {
		mw.app.MustComponent(indexer.CName).(indexer.Indexer).ForceFTIndex()
	}
	records, _, err := at.ObjectStore().Query(nil, database.Query{
		Filters:          req.Filters,
		Sorts:            req.Sorts,
		Offset:           int(req.Offset),
		Limit:            int(req.Limit),
		FullText:         req.FullText,
		ObjectTypeFilter: req.ObjectTypeFilter,
	})
	if err != nil {
		return response(pb.RpcObjectSearchResponseError_UNKNOWN_ERROR, nil, err)
	}

	records = handleDateSearch(req, records)
	var records2 = make([]*types.Struct, 0, len(records))
	for _, rec := range records {
		records2 = append(records2, pbtypes.Map(rec.Details, req.Keys...))
	}

	return response(pb.RpcObjectSearchResponseError_NULL, records2, nil)
}

func (mw *Middleware) ObjectSearchSubscribe(req *pb.RpcObjectSearchSubscribeRequest) *pb.RpcObjectSearchSubscribeResponse {
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

	mw.m.RLock()
	defer mw.m.RUnlock()

	if mw.app == nil {
		return errResponse(fmt.Errorf("account must be started"))
	}

	subService := mw.app.MustComponent(subscription.CName).(subscription.Service)

	resp, err := subService.Search(*req)
	if err != nil {
		return errResponse(err)
	}

	return resp
}

func (mw *Middleware) ObjectIdsSubscribe(req *pb.RpcObjectIdsSubscribeRequest) *pb.RpcObjectIdsSubscribeResponse {
	errResponse := func(err error) *pb.RpcObjectIdsSubscribeResponse {
		r := &pb.RpcObjectIdsSubscribeResponse{
			Error: &pb.RpcObjectIdsSubscribeResponseError{
				Code: pb.RpcObjectIdsSubscribeResponseError_UNKNOWN_ERROR,
			},
		}
		if err != nil {
			r.Error.Description = err.Error()
		}
		return r
	}

	mw.m.RLock()
	defer mw.m.RUnlock()

	if mw.app == nil {
		return errResponse(fmt.Errorf("account must be started"))
	}

	subService := mw.app.MustComponent(subscription.CName).(subscription.Service)

	resp, err := subService.SubscribeIdsReq(*req)
	if err != nil {
		return errResponse(err)
	}

	return resp
}

func (mw *Middleware) ObjectSearchUnsubscribe(req *pb.RpcObjectSearchUnsubscribeRequest) *pb.RpcObjectSearchUnsubscribeResponse {
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

	mw.m.RLock()
	defer mw.m.RUnlock()

	if mw.app == nil {
		return response(fmt.Errorf("account must be started"))
	}

	subService := mw.app.MustComponent(subscription.CName).(subscription.Service)

	err := subService.Unsubscribe(req.SubIds...)
	if err != nil {
		return response(err)
	}
	return response(nil)
}

func (mw *Middleware) ObjectGraph(req *pb.RpcObjectGraphRequest) *pb.RpcObjectGraphResponse {
	response := func(code pb.RpcObjectGraphResponseErrorCode, nodes []*pb.RpcObjectGraphNode, edges []*pb.RpcObjectGraphEdge, err error) *pb.RpcObjectGraphResponse {
		m := &pb.RpcObjectGraphResponse{Error: &pb.RpcObjectGraphResponseError{Code: code}, Nodes: nodes, Edges: edges}
		if err != nil {
			m.Error.Description = err.Error()
		}

		return m
	}

	mw.m.RLock()
	defer mw.m.RUnlock()

	if mw.app == nil {
		return response(pb.RpcObjectGraphResponseError_BAD_INPUT, nil, nil, fmt.Errorf("account must be started"))
	}

	at := mw.app.MustComponent(core.CName).(core.Service)

	records, _, err := at.ObjectStore().Query(nil, database.Query{
		Filters:          req.Filters,
		Limit:            int(req.Limit),
		ObjectTypeFilter: req.ObjectTypeFilter,
	})
	if err != nil {
		return response(pb.RpcObjectGraphResponseError_UNKNOWN_ERROR, nil, nil, err)
	}

	var nodes = make([]*pb.RpcObjectGraphNode, 0, len(records))
	var edges = make([]*pb.RpcObjectGraphEdge, 0, len(records)*2)
	var nodeExists = make(map[string]struct{}, len(records))

	for _, rec := range records {
		id := pbtypes.GetString(rec.Details, bundle.RelationKeyId.String())
		nodeExists[id] = struct{}{}
	}

	homeId := at.PredefinedBlocks().Home
	if _, exists := nodeExists[homeId]; !exists {
		records = append(records, database.Record{&types.Struct{
			Fields: map[string]*types.Value{
				"id":        pbtypes.String(homeId),
				"name":      pbtypes.String("Home"),
				"iconEmoji": pbtypes.String("🏠"),
			},
		}})
	}

	for _, rec := range records {
		id := pbtypes.GetString(rec.Details, bundle.RelationKeyId.String())
		nodes = append(nodes, &pb.RpcObjectGraphNode{
			Id:             id,
			Type:           pbtypes.GetString(rec.Details, bundle.RelationKeyType.String()),
			Name:           pbtypes.GetString(rec.Details, bundle.RelationKeyName.String()),
			Layout:         int32(pbtypes.GetInt64(rec.Details, bundle.RelationKeyLayout.String())),
			Description:    pbtypes.GetString(rec.Details, bundle.RelationKeyDescription.String()),
			IconImage:      pbtypes.GetString(rec.Details, bundle.RelationKeyIconImage.String()),
			IconEmoji:      pbtypes.GetString(rec.Details, bundle.RelationKeyIconEmoji.String()),
			Done:           pbtypes.GetBool(rec.Details, bundle.RelationKeyDone.String()),
			RelationFormat: int32(pbtypes.GetInt64(rec.Details, bundle.RelationKeyRelationFormat.String())),
			Snippet:        pbtypes.GetString(rec.Details, bundle.RelationKeySnippet.String()),
		})

		var outgoingRelationLink = make(map[string]struct{}, 10)
		for k, v := range rec.Details.GetFields() {
			if list := pbtypes.GetStringListValue(v); len(list) == 0 {
				continue
			} else {

				rel, err := at.ObjectStore().GetRelation(k)
				if err != nil {
					log.Errorf("ObjectGraph failed to get relation %s: %s", k, err.Error())
					continue
				}

				if rel.Format != model.RelationFormat_object && rel.Format != model.RelationFormat_file {
					continue
				}

				for _, l := range list {
					if _, exists := nodeExists[l]; !exists {
						continue
					}

					if rel.Hidden ||
						rel.Key == bundle.RelationKeyId.String() ||
						rel.Key == bundle.RelationKeyCreator.String() ||
						rel.Key == bundle.RelationKeyLastModifiedBy.String() {
						continue
					}

					edges = append(edges, &pb.RpcObjectGraphEdge{
						Source:      id,
						Target:      l,
						Name:        rel.Name,
						Type:        pb.RpcObjectGraphEdge_Relation,
						Description: rel.Description,
						Hidden:      rel.Hidden,
					})
					outgoingRelationLink[l] = struct{}{}
				}
			}
		}
		links := pbtypes.GetStringList(rec.Details, bundle.RelationKeyLinks.String())
		for _, link := range links {
			sbType, _ := smartblock.SmartBlockTypeFromID(link)
			// ignore files because we index all file blocks as outgoing links
			if sbType == smartblock.SmartBlockTypeFile {
				continue
			}
			if _, exists := outgoingRelationLink[link]; !exists {
				if _, exists := nodeExists[link]; !exists {
					continue
				}
				edges = append(edges, &pb.RpcObjectGraphEdge{
					Source: id,
					Target: link,
					Name:   "",
					Type:   pb.RpcObjectGraphEdge_Link,
				})
			}
		}
	}

	return response(pb.RpcObjectGraphResponseError_NULL, nodes, edges, nil)
}

func (mw *Middleware) ObjectRelationAdd(req *pb.RpcObjectRelationAddRequest) *pb.RpcObjectRelationAddResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcObjectRelationAddResponseErrorCode, err error) *pb.RpcObjectRelationAddResponse {
		m := &pb.RpcObjectRelationAddResponse{Error: &pb.RpcObjectRelationAddResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	if len(req.RelationIds) == 0 {
		return response(pb.RpcObjectRelationAddResponseError_BAD_INPUT, fmt.Errorf("relation is nil"))
	}

	err := mw.doBlockService(func(bs block.Service) (err error) {
		return bs.AddExtraRelations(ctx, req.ContextId, req.RelationIds)
	})
	if err != nil {
		return response(pb.RpcObjectRelationAddResponseError_BAD_INPUT, err)
	}

	return response(pb.RpcObjectRelationAddResponseError_NULL, nil)
}

func (mw *Middleware) ObjectRelationDelete(req *pb.RpcObjectRelationDeleteRequest) *pb.RpcObjectRelationDeleteResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcObjectRelationDeleteResponseErrorCode, err error) *pb.RpcObjectRelationDeleteResponse {
		m := &pb.RpcObjectRelationDeleteResponse{Error: &pb.RpcObjectRelationDeleteResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	err := mw.doBlockService(func(bs block.Service) (err error) {
		return bs.RemoveExtraRelations(ctx, req.ContextId, []string{req.RelationId})
	})
	if err != nil {
		return response(pb.RpcObjectRelationDeleteResponseError_BAD_INPUT, err)
	}
	return response(pb.RpcObjectRelationDeleteResponseError_NULL, nil)
}

func (mw *Middleware) ObjectRelationListAvailable(req *pb.RpcObjectRelationListAvailableRequest) *pb.RpcObjectRelationListAvailableResponse {
	response := func(code pb.RpcObjectRelationListAvailableResponseErrorCode, relations []*model.Relation, err error) *pb.RpcObjectRelationListAvailableResponse {
		m := &pb.RpcObjectRelationListAvailableResponse{Relations: relations, Error: &pb.RpcObjectRelationListAvailableResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		}
		return m
	}
	var rels []*model.Relation
	err := mw.doBlockService(func(bs block.Service) (err error) {
		rels, err = bs.ListAvailableRelations(req.ContextId)
		return
	})

	if err != nil {
		return response(pb.RpcObjectRelationListAvailableResponseError_UNKNOWN_ERROR, nil, err)
	}

	return response(pb.RpcObjectRelationListAvailableResponseError_NULL, rels, nil)
}

func (mw *Middleware) ObjectSetLayout(req *pb.RpcObjectSetLayoutRequest) *pb.RpcObjectSetLayoutResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcObjectSetLayoutResponseErrorCode, err error) *pb.RpcObjectSetLayoutResponse {
		m := &pb.RpcObjectSetLayoutResponse{Error: &pb.RpcObjectSetLayoutResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	err := mw.doBlockService(func(bs block.Service) (err error) {
		return bs.SetLayout(ctx, req.ContextId, req.Layout)
	})
	if err != nil {
		return response(pb.RpcObjectSetLayoutResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcObjectSetLayoutResponseError_NULL, nil)
}

func (mw *Middleware) ObjectSetIsArchived(req *pb.RpcObjectSetIsArchivedRequest) *pb.RpcObjectSetIsArchivedResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcObjectSetIsArchivedResponseErrorCode, err error) *pb.RpcObjectSetIsArchivedResponse {
		m := &pb.RpcObjectSetIsArchivedResponse{Error: &pb.RpcObjectSetIsArchivedResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	err := mw.doBlockService(func(bs block.Service) (err error) {
		return bs.SetPageIsArchived(*req)
	})
	if err != nil {
		return response(pb.RpcObjectSetIsArchivedResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcObjectSetIsArchivedResponseError_NULL, nil)
}

func (mw *Middleware) ObjectSetIsFavorite(req *pb.RpcObjectSetIsFavoriteRequest) *pb.RpcObjectSetIsFavoriteResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcObjectSetIsFavoriteResponseErrorCode, err error) *pb.RpcObjectSetIsFavoriteResponse {
		m := &pb.RpcObjectSetIsFavoriteResponse{Error: &pb.RpcObjectSetIsFavoriteResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	err := mw.doBlockService(func(bs block.Service) (err error) {
		return bs.SetPageIsFavorite(*req)
	})
	if err != nil {
		return response(pb.RpcObjectSetIsFavoriteResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcObjectSetIsFavoriteResponseError_NULL, nil)
}

func (mw *Middleware) ObjectFeaturedRelationAdd(req *pb.RpcObjectFeaturedRelationAddRequest) *pb.RpcObjectFeaturedRelationAddResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcObjectFeaturedRelationAddResponseErrorCode, err error) *pb.RpcObjectFeaturedRelationAddResponse {
		m := &pb.RpcObjectFeaturedRelationAddResponse{Error: &pb.RpcObjectFeaturedRelationAddResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	err := mw.doBlockService(func(bs block.Service) (err error) {
		return bs.FeaturedRelationAdd(ctx, req.ContextId, req.Relations...)
	})
	if err != nil {
		return response(pb.RpcObjectFeaturedRelationAddResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcObjectFeaturedRelationAddResponseError_NULL, nil)
}

func (mw *Middleware) ObjectFeaturedRelationRemove(req *pb.RpcObjectFeaturedRelationRemoveRequest) *pb.RpcObjectFeaturedRelationRemoveResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcObjectFeaturedRelationRemoveResponseErrorCode, err error) *pb.RpcObjectFeaturedRelationRemoveResponse {
		m := &pb.RpcObjectFeaturedRelationRemoveResponse{Error: &pb.RpcObjectFeaturedRelationRemoveResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	err := mw.doBlockService(func(bs block.Service) (err error) {
		return bs.FeaturedRelationRemove(ctx, req.ContextId, req.Relations...)
	})
	if err != nil {
		return response(pb.RpcObjectFeaturedRelationRemoveResponseError_UNKNOWN_ERROR, err)
	}
	return response(pb.RpcObjectFeaturedRelationRemoveResponseError_NULL, nil)
}

func (mw *Middleware) ObjectToSet(req *pb.RpcObjectToSetRequest) *pb.RpcObjectToSetResponse {
	response := func(setId string, err error) *pb.RpcObjectToSetResponse {
		resp := &pb.RpcObjectToSetResponse{
			SetId: setId,
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
		setId string
		err   error
	)
	err = mw.doBlockService(func(bs block.Service) error {
		if setId, err = bs.ObjectToSet(req.ContextId, req.Source); err != nil {
			return err
		}
		return nil
	})
	return response(setId, err)
}
