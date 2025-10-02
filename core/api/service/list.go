package service

import (
	"context"
	"errors"

	"github.com/gogo/protobuf/types"
	"github.com/iancoleman/strcase"

	"github.com/anyproto/anytype-heart/pb"

	"github.com/anyproto/anytype-heart/core/api/filter"
	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/api/pagination"
	"github.com/anyproto/anytype-heart/core/api/util"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const (
	subId = "json-api-internal"
)

var (
	ErrFailedGetList               = errors.New("failed to get list")
	ErrFailedGetListDataview       = errors.New("failed to get list dataview")
	ErrFailedGetListDataviewView   = errors.New("failed to get list dataview view")
	ErrUnsupportedListType         = errors.New("unsupported list type")
	ErrFailedGetObjectsInList      = errors.New("failed to get objects in list")
	ErrFailedAddObjectsToList      = errors.New("failed to add objects to list")
	ErrFailedRemoveObjectsFromList = errors.New("failed to remove objects from list")
)

// GetListViews retrieves views of a list
func (s *Service) GetListViews(ctx context.Context, spaceId string, listId string, offset, limit int) ([]apimodel.View, int, bool, error) {
	resp := s.mw.ObjectShow(ctx, &pb.RpcObjectShowRequest{
		SpaceId:  spaceId,
		ObjectId: listId,
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcObjectShowResponseError_NULL {
		return nil, 0, false, ErrFailedGetList
	}

	var dataviewBlock *model.Block
	for _, block := range resp.ObjectView.Blocks {
		if block.Id == "dataview" {
			dataviewBlock = block
			break
		}
	}

	if dataviewBlock == nil {
		return nil, 0, false, ErrFailedGetListDataview
	}

	var views []apimodel.View
	switch content := dataviewBlock.Content.(type) {
	case *model.BlockContentOfDataview:
		for _, view := range content.Dataview.Views {
			var filters []apimodel.Filter
			for _, f := range view.Filters {
				if f.Condition == model.BlockContentDataviewFilter_None {
					continue
				}
				apiCond, _ := filter.ToApiCondition(f.Condition)
				filters = append(filters, apimodel.Filter{
					Id:          f.Id,
					PropertyKey: f.RelationKey,
					Format:      RelationFormatToPropertyFormat[f.Format],
					Condition:   apiCond,
					Value:       f.Value.GetStringValue(),
				})
			}
			var sorts []apimodel.Sort
			for _, srt := range view.Sorts {
				sorts = append(sorts, apimodel.Sort{
					Id:          srt.Id,
					PropertyKey: srt.RelationKey,
					Format:      RelationFormatToPropertyFormat[srt.Format],
					SortType:    strcase.ToSnake(model.BlockContentDataviewSortType_name[int32(srt.Type)]),
				})
			}
			views = append(views, apimodel.View{
				Id:      view.Id,
				Name:    view.Name,
				Layout:  s.mapDataviewTypeName(view.Type),
				Filters: filters,
				Sorts:   sorts,
			})
		}
	default:
		return nil, 0, false, ErrFailedGetListDataview
	}

	total := len(views)
	paginatedViews, hasMore := pagination.Paginate(views, offset, limit)

	return paginatedViews, total, hasMore, nil
}

// GetObjectsInList retrieves objects in a list
func (s *Service) GetObjectsInList(ctx context.Context, spaceId string, listId string, viewId string, additionalFilters []*model.BlockContentDataviewFilter, offset, limit int) ([]apimodel.Object, int, bool, error) {
	resp := s.mw.ObjectShow(ctx, &pb.RpcObjectShowRequest{
		SpaceId:  spaceId,
		ObjectId: listId,
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcObjectShowResponseError_NULL {
		return nil, 0, false, ErrFailedGetList
	}

	var dataviewBlock *model.Block
	for _, block := range resp.ObjectView.Blocks {
		if block.Id == "dataview" {
			dataviewBlock = block
			break
		}
	}

	if dataviewBlock == nil {
		return nil, 0, false, ErrFailedGetListDataview
	}

	var sorts []*model.BlockContentDataviewSort
	var filters []*model.BlockContentDataviewFilter

	switch content := dataviewBlock.Content.(type) {
	case *model.BlockContentOfDataview:
		// if view not specified: return all objects without filtering and sorting
		if viewId != "" {
			var targetView *model.BlockContentDataviewView
			for _, view := range content.Dataview.Views {
				if view.Id == viewId {
					targetView = view
					break
				}
			}
			if targetView == nil {
				return nil, 0, false, ErrFailedGetListDataviewView
			}
			sorts = targetView.Sorts
			filters = targetView.Filters
		}
	default:
		return nil, 0, false, ErrFailedGetListDataview
	}

	filters = append(filters, additionalFilters...)

	var typeDetail *types.Struct
	for _, detail := range resp.ObjectView.Details {
		if detail.Id == resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeyType.String()].GetStringValue() {
			typeDetail = detail.GetDetails()
			break
		}
	}

	var collectionId string
	var source []string
	switch model.ObjectTypeLayout(typeDetail.Fields[bundle.RelationKeyRecommendedLayout.String()].GetNumberValue()) {
	case model.ObjectType_set, model.ObjectType_objectType:
		// for queries, we search within the space for objects of the setOf type
		setOfValues := resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeySetOf.String()].GetListValue().Values
		for _, value := range setOfValues {
			uk, _, err := util.ResolveIdtoUniqueKeyAndRelationKey(s.mw, spaceId, value.GetStringValue())
			if err != nil {
				return nil, 0, false, err
			}
			source = append(source, uk)
		}
	case model.ObjectType_collection:
		// for collections, we need to search within that collection
		collectionId = listId
	default:
		return nil, 0, false, ErrUnsupportedListType
	}

	allRelationKeys, err := util.GetAllRelationKeys(s.mw, spaceId)
	if err != nil {
		return nil, 0, false, err
	}

	searchResp := s.mw.ObjectSearchSubscribe(ctx, &pb.RpcObjectSearchSubscribeRequest{
		SpaceId:      spaceId,
		SubId:        subId,
		Limit:        int64(limit),  // nolint: gosec
		Offset:       int64(offset), // nolint: gosec
		Keys:         allRelationKeys,
		Sorts:        sorts,
		Filters:      filters,
		Source:       source,
		CollectionId: collectionId,
	})

	s.mw.ObjectSearchUnsubscribe(ctx, &pb.RpcObjectSearchUnsubscribeRequest{
		SubIds: []string{subId},
	})

	if searchResp.Error != nil && searchResp.Error.Code != pb.RpcObjectSearchSubscribeResponseError_NULL {
		return nil, 0, false, ErrFailedGetObjectsInList
	}

	total := int(searchResp.Counters.Total)
	hasMore := searchResp.Counters.Total > int64(offset+limit)

	objects := make([]apimodel.Object, 0, len(searchResp.Records))
	for _, record := range searchResp.Records {
		objects = append(objects, s.getObjectFromStruct(record))
	}

	return objects, total, hasMore, nil
}

// AddObjectsToList adds objects to a list
func (s *Service) AddObjectsToList(ctx context.Context, _ string, listId string, request apimodel.AddObjectsToListRequest) error {
	resp := s.mw.ObjectCollectionAdd(ctx, &pb.RpcObjectCollectionAddRequest{
		ContextId: listId,
		ObjectIds: request.Objects,
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcObjectCollectionAddResponseError_NULL {
		return ErrFailedAddObjectsToList
	}

	return nil
}

// RemoveObjectsFromList removes objects from a list
func (s *Service) RemoveObjectsFromList(ctx context.Context, spaceId string, listId string, objectIds []string) error {
	resp := s.mw.ObjectCollectionRemove(ctx, &pb.RpcObjectCollectionRemoveRequest{
		ContextId: listId,
		ObjectIds: objectIds,
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcObjectCollectionRemoveResponseError_NULL {
		return ErrFailedRemoveObjectsFromList
	}

	return nil
}

// mapDataviewTypeName maps the dataview type to a string.
func (s *Service) mapDataviewTypeName(dataviewType model.BlockContentDataviewViewType) string {
	switch dataviewType {
	case model.BlockContentDataviewView_Table:
		return "grid"
	default:
		return strcase.ToSnake(model.BlockContentDataviewViewType_name[int32(dataviewType)])
	}
}
