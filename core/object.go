package core

import (
	"fmt"
	"github.com/gogo/protobuf/types"

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

	var includeArchived bool
	for _, filter := range req.Filters {
		// include archived objects if we have explicit filter about it
		if filter.RelationKey == bundle.RelationKeyIsArchived.String() {
			includeArchived = true
		}
	}
	records, _, err := at.ObjectStore().Query(nil, database.Query{
		Filters:                req.Filters,
		Sorts:                  req.Sorts,
		Offset:                 int(req.Offset),
		Limit:                  int(req.Limit),
		FullText:               req.FullText,
		ObjectTypeFilter:       req.ObjectTypeFilter,
		IncludeArchivedObjects: includeArchived,
	})
	if err != nil {
		return response(pb.RpcObjectSearchResponseError_UNKNOWN_ERROR, nil, err)
	}

	var records2 = make([]*types.Struct, 0, len(records))
	for _, rec := range records {
		records2 = append(records2, pbtypes.Map(rec.Details, req.Keys...))
	}

	return response(pb.RpcObjectSearchResponseError_NULL, records2, nil)
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

	var includeArchived bool
	for _, filter := range req.Filters {
		// include archived objects if we have explicit filter about it
		if filter.RelationKey == bundle.RelationKeyIsArchived.String() {
			includeArchived = true
		}
	}
	records, _, err := at.ObjectStore().Query(nil, database.Query{
		Filters:                req.Filters,
		Limit:                  int(req.Limit),
		ObjectTypeFilter:       req.ObjectTypeFilter,
		IncludeArchivedObjects: includeArchived,
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

	for _, rec := range records {
		id := pbtypes.GetString(rec.Details, bundle.RelationKeyId.String())
		nodes = append(nodes, &pb.RpcObjectGraphNode{
			Id:          id,
			Type:        pbtypes.GetString(rec.Details, bundle.RelationKeyType.String()),
			Name:        pbtypes.GetString(rec.Details, bundle.RelationKeyName.String()),
			Layout:      int32(pbtypes.GetInt64(rec.Details, bundle.RelationKeyLayout.String())),
			Description: pbtypes.GetString(rec.Details, bundle.RelationKeyDescription.String()),
			IconImage:   pbtypes.GetString(rec.Details, bundle.RelationKeyIconImage.String()),
			IconEmoji:   pbtypes.GetString(rec.Details, bundle.RelationKeyIconEmoji.String()),
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

				if rel.Key == bundle.RelationKeyId.String() || rel.Key == bundle.RelationKeyType.String() {
					continue
				}

				for _, l := range list {
					if _, exists := nodeExists[l]; !exists {
						continue
					}
					edges = append(edges, &pb.RpcObjectGraphEdge{
						Source:      id,
						Target:      l,
						Name:        rel.Name,
						Type:        pb.RpcObjectGraphEdge_Relation,
						Description: rel.Description,
					})
					outgoingRelationLink[l] = struct{}{}
				}
			}
		}
		links, _ := at.ObjectStore().GetOutboundLinksById(id)
		for _, link := range links {
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
	response := func(relation *model.Relation, code pb.RpcObjectRelationAddResponseErrorCode, err error) *pb.RpcObjectRelationAddResponse {
		var relKey string
		if relation != nil {
			relKey = relation.Key
		}
		m := &pb.RpcObjectRelationAddResponse{RelationKey: relKey, Relation: relation, Error: &pb.RpcObjectRelationAddResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	if req.Relation == nil {
		return response(nil, pb.RpcObjectRelationAddResponseError_BAD_INPUT, fmt.Errorf("relation is nil"))
	}

	var relations []*model.Relation
	err := mw.doBlockService(func(bs block.Service) (err error) {
		relations, err = bs.AddExtraRelations(ctx, req.ContextId, []*model.Relation{req.Relation})
		return err
	})
	if err != nil {
		return response(nil, pb.RpcObjectRelationAddResponseError_BAD_INPUT, err)
	}

	return response(relations[0], pb.RpcObjectRelationAddResponseError_NULL, nil)
}

func (mw *Middleware) ObjectRelationUpdate(req *pb.RpcObjectRelationUpdateRequest) *pb.RpcObjectRelationUpdateResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcObjectRelationUpdateResponseErrorCode, err error) *pb.RpcObjectRelationUpdateResponse {
		m := &pb.RpcObjectRelationUpdateResponse{Error: &pb.RpcObjectRelationUpdateResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	err := mw.doBlockService(func(bs block.Service) (err error) {
		return bs.UpdateExtraRelations(nil, req.ContextId, []*model.Relation{req.Relation}, false)
	})
	if err != nil {
		return response(pb.RpcObjectRelationUpdateResponseError_BAD_INPUT, err)
	}

	return response(pb.RpcObjectRelationUpdateResponseError_NULL, nil)
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
		return bs.RemoveExtraRelations(ctx, req.ContextId, []string{req.RelationKey})
	})
	if err != nil {
		return response(pb.RpcObjectRelationDeleteResponseError_BAD_INPUT, err)
	}
	return response(pb.RpcObjectRelationDeleteResponseError_NULL, nil)
}

func (mw *Middleware) ObjectRelationOptionAdd(req *pb.RpcObjectRelationOptionAddRequest) *pb.RpcObjectRelationOptionAddResponse {
	ctx := state.NewContext(nil)
	response := func(opt *model.RelationOption, code pb.RpcObjectRelationOptionAddResponseErrorCode, err error) *pb.RpcObjectRelationOptionAddResponse {
		m := &pb.RpcObjectRelationOptionAddResponse{Option: opt, Error: &pb.RpcObjectRelationOptionAddResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	var opt *model.RelationOption
	err := mw.doBlockService(func(bs block.Service) (err error) {
		var err2 error
		opt, err2 = bs.AddExtraRelationOption(ctx, *req)
		return err2
	})
	if err != nil {
		return response(nil, pb.RpcObjectRelationOptionAddResponseError_BAD_INPUT, err)
	}

	return response(opt, pb.RpcObjectRelationOptionAddResponseError_NULL, nil)
}

func (mw *Middleware) ObjectRelationOptionUpdate(req *pb.RpcObjectRelationOptionUpdateRequest) *pb.RpcObjectRelationOptionUpdateResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcObjectRelationOptionUpdateResponseErrorCode, err error) *pb.RpcObjectRelationOptionUpdateResponse {
		m := &pb.RpcObjectRelationOptionUpdateResponse{Error: &pb.RpcObjectRelationOptionUpdateResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	err := mw.doBlockService(func(bs block.Service) (err error) {
		return bs.UpdateExtraRelationOption(ctx, *req)
	})
	if err != nil {
		return response(pb.RpcObjectRelationOptionUpdateResponseError_BAD_INPUT, err)
	}

	return response(pb.RpcObjectRelationOptionUpdateResponseError_NULL, nil)
}

func (mw *Middleware) ObjectRelationOptionDelete(req *pb.RpcObjectRelationOptionDeleteRequest) *pb.RpcObjectRelationOptionDeleteResponse {
	ctx := state.NewContext(nil)
	response := func(code pb.RpcObjectRelationOptionDeleteResponseErrorCode, err error) *pb.RpcObjectRelationOptionDeleteResponse {
		m := &pb.RpcObjectRelationOptionDeleteResponse{Error: &pb.RpcObjectRelationOptionDeleteResponseError{Code: code}}
		if err != nil {
			m.Error.Description = err.Error()
		} else {
			m.Event = ctx.GetResponseEvent()
		}
		return m
	}
	err := mw.doBlockService(func(bs block.Service) (err error) {
		return bs.DeleteExtraRelationOption(ctx, *req)
	})
	if err != nil {
		return response(pb.RpcObjectRelationOptionDeleteResponseError_BAD_INPUT, err)
	}

	return response(pb.RpcObjectRelationOptionDeleteResponseError_NULL, nil)
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
