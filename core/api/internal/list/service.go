package list

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/gogo/protobuf/types"
	"github.com/iancoleman/strcase"

	"github.com/anyproto/anytype-heart/core/api/apicore"
	"github.com/anyproto/anytype-heart/core/api/internal/object"
	"github.com/anyproto/anytype-heart/core/api/pagination"
	"github.com/anyproto/anytype-heart/core/api/util"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
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

type Service interface {
	GetListViews(ctx context.Context, spaceId string, listId string, offset, limit int) ([]View, int, bool, error)
	GetObjectsInList(ctx context.Context, spaceId string, listId string, viewId string, offset, limit int) ([]object.Object, int, bool, error)
	AddObjectsToList(ctx context.Context, spaceId string, listId string, objectIds []string) error
	CreateObjectsCollection(ctx context.Context, spaceId string, req CreateCollectionRequest) error
	RemoveObjectsFromList(ctx context.Context, spaceId string, listId string, objectIds []string) error
}

type service struct {
	mw            apicore.ClientCommands
	objectService object.Service
}

func NewService(mw apicore.ClientCommands, objectService object.Service) Service {
	return &service{mw: mw, objectService: objectService}
}

// GetListViews retrieves views of a list
func (s *service) GetListViews(ctx context.Context, spaceId string, listId string, offset, limit int) ([]View, int, bool, error) {
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

	var views []View
	switch content := dataviewBlock.Content.(type) {
	case *model.BlockContentOfDataview:
		for _, view := range content.Dataview.Views {
			var filters []Filter
			for _, f := range view.Filters {
				filters = append(filters, Filter{
					Id:          f.Id,
					PropertyKey: f.RelationKey,
					Format:      s.objectService.MapRelationFormat(f.Format),
					Condition:   strcase.ToSnake(model.BlockContentDataviewFilterCondition_name[int32(f.Condition)]),
					Value:       f.Value.GetStringValue(),
				})
			}
			var sorts []Sort
			for _, srt := range view.Sorts {
				sorts = append(sorts, Sort{
					Id:          srt.Id,
					PropertyKey: srt.RelationKey,
					Format:      s.objectService.MapRelationFormat(srt.Format),
					SortType:    strcase.ToSnake(model.BlockContentDataviewSortType_name[int32(srt.Type)]),
				})
			}
			views = append(views, View{
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
func (s *service) GetObjectsInList(ctx context.Context, spaceId string, listId string, viewId string, offset, limit int) ([]object.Object, int, bool, error) {
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
	case model.ObjectType_set:
		// for queries, we search within the space for objects of the setOf type
		setOfValues := resp.ObjectView.Details[0].Details.Fields[bundle.RelationKeySetOf.String()].GetListValue().Values
		for _, value := range setOfValues {
			typeKey, err := util.ResolveIdtoUniqueKey(s.mw, spaceId, value.GetStringValue())
			if err != nil {
				return nil, 0, false, err
			}
			source = append(source, typeKey)
		}
	case model.ObjectType_collection:
		// for collections, we need to search within that collection
		collectionId = listId
	default:
		return nil, 0, false, ErrUnsupportedListType
	}

	// TODO: use subscription service with internal: 'true' to not send events to clients
	searchResp := s.mw.ObjectSearchSubscribe(ctx, &pb.RpcObjectSearchSubscribeRequest{
		SpaceId: spaceId,
		SubId:   subId,
		Limit:   int64(limit),  // nolint: gosec
		Offset:  int64(offset), // nolint: gosec
		// TODO: fix not all keys being returned
		// Keys:         []string{bundle.RelationKeyId.String()},
		Sorts:        sorts,
		Filters:      filters,
		Source:       source,
		CollectionId: collectionId,
	})

	if searchResp.Error != nil && searchResp.Error.Code != pb.RpcObjectSearchSubscribeResponseError_NULL {
		return nil, 0, false, ErrFailedGetObjectsInList
	}

	total := int(searchResp.Counters.Total)
	hasMore := searchResp.Counters.Total > int64(offset+limit)

	propertyMap, err := s.objectService.GetPropertyMapFromStore(spaceId)
	if err != nil {
		return nil, 0, false, err
	}
	typeMap, err := s.objectService.GetTypeMapFromStore(spaceId, propertyMap)
	if err != nil {
		return nil, 0, false, err
	}

	objects := make([]object.Object, 0, len(searchResp.Records))
	for _, record := range searchResp.Records {
		objects = append(objects, s.objectService.GetObjectFromStruct(record, propertyMap, typeMap))
	}

	return objects, total, hasMore, nil
}

// AddObjectsToList adds objects to a list
func (s *service) AddObjectsToList(ctx context.Context, spaceId string, listId string, objectIds []string) error {
	resp := s.mw.ObjectCollectionAdd(ctx, &pb.RpcObjectCollectionAddRequest{
		ContextId: listId,
		ObjectIds: objectIds,
	})

	if resp.Error.Code != pb.RpcObjectCollectionAddResponseError_NULL {
		return ErrFailedAddObjectsToList
	}

	return nil
}

// CreateObjectsCollection creates a collection and add the objects there
func (s *service) CreateObjectsCollection(ctx context.Context, spaceId string, req CreateCollectionRequest) error {
	reqObject := object.CreateObjectRequest{
		Name:        req.Name,
		Icon:        util.Icon{Format: util.IconFormatEmoji, Emoji: &req.IconEmoji},
		Description: req.Description,
		TypeKey:     bundle.TypeKeyCollection.URL(),
	}
	collectionObject, err := s.objectService.CreateObject(ctx, spaceId, reqObject)
	if err != nil {
		return err
	}
	resp := s.mw.ObjectCollectionAdd(ctx, &pb.RpcObjectCollectionAddRequest{
		ContextId: collectionObject.Id,
		ObjectIds: req.ObjectIds,
	})

	if resp.Error.Code != pb.RpcObjectCollectionAddResponseError_NULL {
		return ErrFailedAddObjectsToList
	}

	resp4 := s.mw.ObjectSearch(ctx, &pb.RpcObjectSearchRequest{
		SpaceId: spaceId,
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyId.String(),
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       pbtypes.StringList(req.ObjectIds),
			},
		},
		Keys: nil,
	})

	var uniqueKeys = make(map[string]struct{})
	for _, record := range resp4.Records {
		for key, v := range record.GetFields() {
			if key == bundle.RelationKeyOrigin.String() || key == bundle.RelationKeyLinks.String() {
				continue
			}

			if v == nil {
				continue
			}

			switch vt := v.Kind.(type) {
			case *types.Value_StringValue:
				if vt.StringValue == "" {
					continue
				}
			case *types.Value_NullValue:
				continue
			case *types.Value_ListValue:
				if vt.ListValue == nil || len(vt.ListValue.Values) == 0 {
					continue
				}
			}
			if br, e := bundle.GetRelation(domain.RelationKey(key)); e == nil {
				if br.Hidden && br.Key != bundle.RelationKeyName.String() {
					continue
				}
			}
			uniqueKeys[key] = struct{}{}
		}
	}
	relationKeys := []string{
		bundle.RelationKeyName.String(),
		bundle.RelationKeyType.String(),
	}
	for key := range uniqueKeys {
		// if not in the list, add it
		if slices.Contains(relationKeys, key) {
			continue
		}
		relationKeys = append(relationKeys, key)
	}
	fmt.Printf("### unique keys: %v\n", relationKeys)

	resp2 := s.mw.BlockDataviewRelationSet(ctx, &pb.RpcBlockDataviewRelationSetRequest{
		ContextId:    collectionObject.Id,
		BlockId:      "dataview",
		RelationKeys: relationKeys,
	})

	if resp2.Error.Code != pb.RpcBlockDataviewRelationSetResponseError_NULL {
		fmt.Printf("failed to set collection relations: %s", resp2.Error.Description)
	}

	return nil
}

// RemoveObjectsFromList removes objects from a list
func (s *service) RemoveObjectsFromList(ctx context.Context, spaceId string, listId string, objectIds []string) error {
	resp := s.mw.ObjectCollectionRemove(ctx, &pb.RpcObjectCollectionRemoveRequest{
		ContextId: listId,
		ObjectIds: objectIds,
	})

	if resp.Error.Code != pb.RpcObjectCollectionRemoveResponseError_NULL {
		return ErrFailedRemoveObjectsFromList
	}

	return nil
}

// mapDataviewTypeName maps the dataview type to a string.
func (s *service) mapDataviewTypeName(dataviewType model.BlockContentDataviewViewType) string {
	switch dataviewType {
	case model.BlockContentDataviewView_Table:
		return "grid"
	default:
		return strcase.ToSnake(model.BlockContentDataviewViewType_name[int32(dataviewType)])
	}
}
