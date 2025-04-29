package object

import (
	"context"
	"errors"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/api/apimodel"
	"github.com/anyproto/anytype-heart/core/api/pagination"
	"github.com/anyproto/anytype-heart/core/api/util"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

var (
	ErrFailedRetrieveTypes        = errors.New("failed to retrieve types")
	ErrTypeNotFound               = errors.New("type not found")
	ErrTypeDeleted                = errors.New("type deleted")
	ErrFailedRetrieveType         = errors.New("failed to retrieve type")
	ErrFailedRetrieveTemplateType = errors.New("failed to retrieve template type")
	ErrTemplateTypeNotFound       = errors.New("template type not found")
	ErrFailedCreateType           = errors.New("failed to create type")
	ErrFailedUpdateType           = errors.New("failed to update type")
	ErrFailedDeleteType           = errors.New("failed to delete object")
)

// ListTypes returns a paginated list of types in a specific space.
func (s *service) ListTypes(ctx context.Context, spaceId string, offset int, limit int) (types []apimodel.Type, total int, hasMore bool, err error) {
	resp := s.mw.ObjectSearch(ctx, &pb.RpcObjectSearchRequest{
		SpaceId: spaceId,
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyResolvedLayout.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Int64(int64(model.ObjectType_objectType)),
			},
			{
				RelationKey: bundle.RelationKeyIsHidden.String(),
				Condition:   model.BlockContentDataviewFilter_NotEqual,
				Value:       pbtypes.Bool(true),
			},
		},
		Sorts: []*model.BlockContentDataviewSort{
			{
				RelationKey: bundle.RelationKeyName.String(),
				Type:        model.BlockContentDataviewSort_Asc,
			},
		},
		Keys: []string{
			bundle.RelationKeyId.String(),
			bundle.RelationKeyUniqueKey.String(),
			bundle.RelationKeyName.String(),
			bundle.RelationKeyIconEmoji.String(),
			bundle.RelationKeyIconName.String(),
			bundle.RelationKeyIconOption.String(),
			bundle.RelationKeyRecommendedLayout.String(),
			bundle.RelationKeyIsArchived.String(),
			bundle.RelationKeyRecommendedFeaturedRelations.String(),
			bundle.RelationKeyRecommendedRelations.String()},
	})

	if resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		return nil, 0, false, ErrFailedRetrieveTypes
	}

	total = len(resp.Records)
	paginatedTypes, hasMore := pagination.Paginate(resp.Records, offset, limit)
	types = make([]apimodel.Type, 0, len(paginatedTypes))

	propertyMap, err := s.GetPropertyMapFromStore(spaceId)
	if err != nil {
		return nil, 0, false, err
	}

	for _, record := range paginatedTypes {
		types = append(types, apimodel.Type{
			Object:     "type",
			Id:         record.Fields[bundle.RelationKeyId.String()].GetStringValue(),
			Key:        record.Fields[bundle.RelationKeyUniqueKey.String()].GetStringValue(),
			Name:       record.Fields[bundle.RelationKeyName.String()].GetStringValue(),
			Icon:       apimodel.GetIcon(s.gatewayUrl, record.Fields[bundle.RelationKeyIconEmoji.String()].GetStringValue(), "", record.Fields[bundle.RelationKeyIconName.String()].GetStringValue(), record.Fields[bundle.RelationKeyIconOption.String()].GetNumberValue()),
			Archived:   record.Fields[bundle.RelationKeyIsArchived.String()].GetBoolValue(),
			Layout:     s.otLayoutToObjectLayout(model.ObjectTypeLayout(record.Fields[bundle.RelationKeyRecommendedLayout.String()].GetNumberValue())),
			Properties: s.getRecommendedPropertiesFromLists(record.Fields[bundle.RelationKeyRecommendedFeaturedRelations.String()].GetListValue(), record.Fields[bundle.RelationKeyRecommendedRelations.String()].GetListValue(), propertyMap),
		})
	}
	return types, total, hasMore, nil
}

// GetType returns a single type by its ID in a specific space.
func (s *service) GetType(ctx context.Context, spaceId string, typeId string) (apimodel.Type, error) {
	resp := s.mw.ObjectShow(ctx, &pb.RpcObjectShowRequest{
		SpaceId:  spaceId,
		ObjectId: typeId,
	})

	if resp.Error != nil {
		if resp.Error.Code == pb.RpcObjectShowResponseError_NOT_FOUND {
			return apimodel.Type{}, ErrTypeNotFound
		}

		if resp.Error.Code == pb.RpcObjectShowResponseError_OBJECT_DELETED {
			return apimodel.Type{}, ErrTypeDeleted
		}

		if resp.Error.Code != pb.RpcObjectShowResponseError_NULL {
			return apimodel.Type{}, ErrFailedRetrieveType
		}
	}

	// pre-fetch properties to fill the type
	propertyMap, err := s.GetPropertyMapFromStore(spaceId)
	if err != nil {
		return apimodel.Type{}, err
	}

	details := resp.ObjectView.Details[0].Details.Fields
	return apimodel.Type{
		Object:     "type",
		Id:         typeId,
		Key:        details[bundle.RelationKeyUniqueKey.String()].GetStringValue(),
		Name:       details[bundle.RelationKeyName.String()].GetStringValue(),
		Icon:       apimodel.GetIcon(s.gatewayUrl, details[bundle.RelationKeyIconEmoji.String()].GetStringValue(), "", details[bundle.RelationKeyIconName.String()].GetStringValue(), details[bundle.RelationKeyIconOption.String()].GetNumberValue()),
		Archived:   details[bundle.RelationKeyIsArchived.String()].GetBoolValue(),
		Layout:     s.otLayoutToObjectLayout(model.ObjectTypeLayout(details[bundle.RelationKeyRecommendedLayout.String()].GetNumberValue())),
		Properties: s.getRecommendedPropertiesFromLists(details[bundle.RelationKeyRecommendedFeaturedRelations.String()].GetListValue(), details[bundle.RelationKeyRecommendedRelations.String()].GetListValue(), propertyMap),
	}, nil
}

// CreateType creates a new type in a specific space.
func (s *service) CreateType(ctx context.Context, spaceId string, request apimodel.CreateTypeRequest) (apimodel.Type, error) {
	details, err := s.buildTypeDetails(ctx, spaceId, request)
	if err != nil {
		return apimodel.Type{}, err
	}

	resp := s.mw.ObjectCreateObjectType(ctx, &pb.RpcObjectCreateObjectTypeRequest{
		SpaceId: spaceId,
		Details: details,
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcObjectCreateObjectTypeResponseError_NULL {
		return apimodel.Type{}, ErrFailedCreateType
	}

	return s.GetType(ctx, spaceId, resp.ObjectId)
}

// UpdateType updates an existing type in a specific space.
func (s *service) UpdateType(ctx context.Context, spaceId string, typeId string, request apimodel.UpdateTypeRequest) (apimodel.Type, error) {
	// TODO
	return apimodel.Type{}, nil
}

// DeleteType deletes a type by its ID in a specific space.
func (s *service) DeleteType(ctx context.Context, spaceId string, typeId string) (apimodel.Type, error) {
	t, err := s.GetType(ctx, spaceId, typeId)
	if err != nil {
		return apimodel.Type{}, err
	}

	resp := s.mw.ObjectSetIsArchived(ctx, &pb.RpcObjectSetIsArchivedRequest{
		ContextId:  typeId,
		IsArchived: true,
	})

	if resp.Error.Code != pb.RpcObjectSetIsArchivedResponseError_NULL {
		return apimodel.Type{}, ErrFailedDeleteObject
	}

	return t, nil
}

// GetTypeMapsFromStore retrieves all types from all spaces.
func (s *service) GetTypeMapsFromStore(spaceIds []string, propertyMap map[string]map[string]apimodel.Property) (map[string]map[string]apimodel.Type, error) {
	spacesToTypes := make(map[string]map[string]apimodel.Type, len(spaceIds))

	for _, spaceId := range spaceIds {
		typeMap, err := s.GetTypeMapFromStore(spaceId, propertyMap[spaceId])
		if err != nil {
			return nil, err
		}
		spacesToTypes[spaceId] = typeMap
	}

	return spacesToTypes, nil
}

// GetTypeMapFromStore retrieves all types for a specific space.
func (s *service) GetTypeMapFromStore(spaceId string, propertyMap map[string]apimodel.Property) (map[string]apimodel.Type, error) {
	resp := s.mw.ObjectSearch(context.Background(), &pb.RpcObjectSearchRequest{
		SpaceId: spaceId,
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyResolvedLayout.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Int64(int64(model.ObjectType_objectType)),
			},
			{
				// resolve deleted types as well
				RelationKey: bundle.RelationKeyIsDeleted.String(),
			},
		},
		Keys: []string{
			bundle.RelationKeyId.String(),
			bundle.RelationKeyUniqueKey.String(),
			bundle.RelationKeyName.String(),
			bundle.RelationKeyIconEmoji.String(),
			bundle.RelationKeyIconName.String(),
			bundle.RelationKeyIconOption.String(),
			bundle.RelationKeyRecommendedLayout.String(),
			bundle.RelationKeyIsArchived.String(),
			bundle.RelationKeyRecommendedFeaturedRelations.String(),
			bundle.RelationKeyRecommendedRelations.String(),
		},
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		return nil, ErrFailedRetrieveTypes
	}

	typeMap := make(map[string]apimodel.Type, len(resp.Records))
	for _, record := range resp.Records {
		typeMap[record.Fields[bundle.RelationKeyId.String()].GetStringValue()] = apimodel.Type{
			Object:     "type",
			Id:         record.Fields[bundle.RelationKeyId.String()].GetStringValue(),
			Key:        record.Fields[bundle.RelationKeyUniqueKey.String()].GetStringValue(),
			Name:       record.Fields[bundle.RelationKeyName.String()].GetStringValue(),
			Icon:       apimodel.GetIcon(s.gatewayUrl, record.Fields[bundle.RelationKeyIconEmoji.String()].GetStringValue(), "", record.Fields[bundle.RelationKeyIconName.String()].GetStringValue(), record.Fields[bundle.RelationKeyIconOption.String()].GetNumberValue()),
			Archived:   record.Fields[bundle.RelationKeyIsArchived.String()].GetBoolValue(),
			Layout:     s.otLayoutToObjectLayout(model.ObjectTypeLayout(record.Fields[bundle.RelationKeyRecommendedLayout.String()].GetNumberValue())),
			Properties: s.getRecommendedPropertiesFromLists(record.Fields[bundle.RelationKeyRecommendedFeaturedRelations.String()].GetListValue(), record.Fields[bundle.RelationKeyRecommendedRelations.String()].GetListValue(), propertyMap),
		}
	}
	return typeMap, nil
}

// getTypeFromStruct retrieves the type from the details.
func (s *service) getTypeFromStruct(details *types.Struct, typeMap map[string]apimodel.Type) apimodel.Type {
	return typeMap[details.Fields[bundle.RelationKeyType.String()].GetStringValue()]
}

// buildTypeDetails builds the type details from the CreateTypeRequest.
func (s *service) buildTypeDetails(ctx context.Context, spaceId string, request apimodel.CreateTypeRequest) (*types.Struct, error) {
	fields := make(map[string]*types.Value)

	fields[bundle.RelationKeyName.String()] = pbtypes.String(s.sanitizedString(request.Name))
	fields[bundle.RelationKeyPluralName.String()] = pbtypes.String(s.sanitizedString(request.PluralName))
	fields[bundle.RelationKeyRecommendedLayout.String()] = pbtypes.Int64(int64(s.typeLayoutToObjectTypeLayout(request.Layout)))

	iconFields, err := s.processIconFields(ctx, spaceId, request.Icon)
	if err != nil {
		return nil, err
	}
	for k, v := range iconFields {
		fields[k] = v
	}

	propertyMap, err := s.GetPropertyMapFromStore(spaceId)
	if err != nil {
		return nil, err
	}

	var relationIds []string
	for _, propLink := range request.Properties {
		rk := util.FromPropertyApiKey(propLink.Key)
		if propDef, exists := propertyMap[rk]; exists {
			relationIds = append(relationIds, propDef.Id)
		} else {
			newProp, err2 := s.CreateProperty(ctx, spaceId, apimodel.CreatePropertyRequest{
				Name:   propLink.Name,
				Format: propLink.Format,
			})
			if err2 != nil {
				return nil, err2
			}
			relationIds = append(relationIds, newProp.Id)
		}
	}

	fields[bundle.RelationKeyRecommendedRelations.String()] = pbtypes.StringList(relationIds)

	featuredKeys := []domain.RelationKey{
		bundle.RelationKeyType,
		bundle.RelationKeyTag,
		bundle.RelationKeyBacklinks,
	}
	var featuredIds []string
	for _, rk := range featuredKeys {
		if propDef, exists := propertyMap[rk.String()]; exists {
			featuredIds = append(featuredIds, propDef.Id)
		}
	}
	fields[bundle.RelationKeyRecommendedFeaturedRelations.String()] = pbtypes.StringList(featuredIds)

	hiddenKeys := []domain.RelationKey{
		bundle.RelationKeyLastModifiedDate,
		bundle.RelationKeyLastModifiedBy,
		bundle.RelationKeyLastOpenedDate,
	}
	var hiddenIds []string
	for _, rk := range hiddenKeys {
		if propDef, exists := propertyMap[rk.String()]; exists {
			hiddenIds = append(hiddenIds, propDef.Id)
		}
	}
	fields[bundle.RelationKeyRecommendedHiddenRelations.String()] = pbtypes.StringList(hiddenIds)

	return &types.Struct{Fields: fields}, nil
}

func (s *service) objectLayoutToObjectTypeLayout(objectLayout apimodel.ObjectLayout) model.ObjectTypeLayout {
	switch objectLayout {
	case apimodel.ObjectLayoutBasic:
		return model.ObjectType_basic
	case apimodel.ObjectLayoutProfile:
		return model.ObjectType_profile
	case apimodel.ObjectLayoutTodo:
		return model.ObjectType_todo
	case apimodel.ObjectLayoutNote:
		return model.ObjectType_note
	case apimodel.ObjectLayoutBookmark:
		return model.ObjectType_bookmark
	case apimodel.ObjectLayoutSet:
		return model.ObjectType_set
	case apimodel.ObjectLayoutCollection:
		return model.ObjectType_collection
	case apimodel.ObjectLayoutParticipant:
		return model.ObjectType_participant
	default:
		return model.ObjectType_basic
	}
}

func (s *service) otLayoutToObjectLayout(objectTypeLayout model.ObjectTypeLayout) apimodel.ObjectLayout {
	switch objectTypeLayout {
	case model.ObjectType_basic:
		return apimodel.ObjectLayoutBasic
	case model.ObjectType_profile:
		return apimodel.ObjectLayoutProfile
	case model.ObjectType_todo:
		return apimodel.ObjectLayoutTodo
	case model.ObjectType_note:
		return apimodel.ObjectLayoutNote
	case model.ObjectType_bookmark:
		return apimodel.ObjectLayoutBookmark
	case model.ObjectType_set:
		return apimodel.ObjectLayoutSet
	case model.ObjectType_collection:
		return apimodel.ObjectLayoutCollection
	case model.ObjectType_participant:
		return apimodel.ObjectLayoutParticipant
	default:
		return apimodel.ObjectLayoutBasic
	}
}

func (s *service) typeLayoutToObjectTypeLayout(typeLayout apimodel.TypeLayout) model.ObjectTypeLayout {
	switch typeLayout {
	case apimodel.TypeLayoutBasic:
		return model.ObjectType_basic
	case apimodel.TypeLayoutProfile:
		return model.ObjectType_profile
	case apimodel.TypeLayoutTodo:
		return model.ObjectType_todo
	case apimodel.TypeLayoutNote:
		return model.ObjectType_note
	default:
		return model.ObjectType_basic
	}
}

func (s *service) otLayoutToTypeLayout(objectTypeLayout model.ObjectTypeLayout) apimodel.TypeLayout {
	switch objectTypeLayout {
	case model.ObjectType_basic:
		return apimodel.TypeLayoutBasic
	case model.ObjectType_profile:
		return apimodel.TypeLayoutProfile
	case model.ObjectType_todo:
		return apimodel.TypeLayoutTodo
	case model.ObjectType_note:
		return apimodel.TypeLayoutNote
	default:
		return apimodel.TypeLayoutBasic
	}
}
