package object

import (
	"context"
	"errors"

	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/api/pagination"
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
func (s *service) ListTypes(ctx context.Context, spaceId string, offset int, limit int) (types []Type, total int, hasMore bool, err error) {
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
	types = make([]Type, 0, len(paginatedTypes))

	propertyMap, err := s.GetPropertyMapFromStore(spaceId)
	if err != nil {
		return nil, 0, false, err
	}

	for _, record := range paginatedTypes {
		types = append(types, Type{
			Object:     "type",
			Id:         record.Fields[bundle.RelationKeyId.String()].GetStringValue(),
			Key:        record.Fields[bundle.RelationKeyUniqueKey.String()].GetStringValue(),
			Name:       record.Fields[bundle.RelationKeyName.String()].GetStringValue(),
			Icon:       GetIcon(s.gatewayUrl, record.Fields[bundle.RelationKeyIconEmoji.String()].GetStringValue(), "", record.Fields[bundle.RelationKeyIconName.String()].GetStringValue(), record.Fields[bundle.RelationKeyIconOption.String()].GetNumberValue()),
			Archived:   record.Fields[bundle.RelationKeyIsArchived.String()].GetBoolValue(),
			Layout:     s.otLayoutToObjectLayout(model.ObjectTypeLayout(record.Fields[bundle.RelationKeyRecommendedLayout.String()].GetNumberValue())),
			Properties: s.getRecommendedPropertiesFromLists(record.Fields[bundle.RelationKeyRecommendedFeaturedRelations.String()].GetListValue(), record.Fields[bundle.RelationKeyRecommendedRelations.String()].GetListValue(), propertyMap),
		})
	}
	return types, total, hasMore, nil
}

// GetType returns a single type by its ID in a specific space.
func (s *service) GetType(ctx context.Context, spaceId string, typeId string) (Type, error) {
	resp := s.mw.ObjectShow(ctx, &pb.RpcObjectShowRequest{
		SpaceId:  spaceId,
		ObjectId: typeId,
	})

	if resp.Error != nil {
		if resp.Error.Code == pb.RpcObjectShowResponseError_NOT_FOUND {
			return Type{}, ErrTypeNotFound
		}

		if resp.Error.Code == pb.RpcObjectShowResponseError_OBJECT_DELETED {
			return Type{}, ErrTypeDeleted
		}

		if resp.Error.Code != pb.RpcObjectShowResponseError_NULL {
			return Type{}, ErrFailedRetrieveType
		}
	}

	// pre-fetch properties to fill the type
	propertyMap, err := s.GetPropertyMapFromStore(spaceId)
	if err != nil {
		return Type{}, err
	}

	details := resp.ObjectView.Details[0].Details.Fields
	return Type{
		Object:     "type",
		Id:         typeId,
		Key:        details[bundle.RelationKeyUniqueKey.String()].GetStringValue(),
		Name:       details[bundle.RelationKeyName.String()].GetStringValue(),
		Icon:       GetIcon(s.gatewayUrl, details[bundle.RelationKeyIconEmoji.String()].GetStringValue(), "", details[bundle.RelationKeyIconName.String()].GetStringValue(), details[bundle.RelationKeyIconOption.String()].GetNumberValue()),
		Archived:   details[bundle.RelationKeyIsArchived.String()].GetBoolValue(),
		Layout:     s.otLayoutToObjectLayout(model.ObjectTypeLayout(details[bundle.RelationKeyRecommendedLayout.String()].GetNumberValue())),
		Properties: s.getRecommendedPropertiesFromLists(details[bundle.RelationKeyRecommendedFeaturedRelations.String()].GetListValue(), details[bundle.RelationKeyRecommendedRelations.String()].GetListValue(), propertyMap),
	}, nil
}

// CreateType creates a new type in a specific space.
func (s *service) CreateType(ctx context.Context, spaceId string, request CreateTypeRequest) (Type, error) {
	details, err := s.buildTypeDetails(ctx, spaceId, request)
	if err != nil {
		return Type{}, err
	}

	resp := s.mw.ObjectCreateObjectType(ctx, &pb.RpcObjectCreateObjectTypeRequest{
		SpaceId: spaceId,
		Details: details,
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcObjectCreateObjectTypeResponseError_NULL {
		return Type{}, ErrFailedCreateType
	}

	return s.GetType(ctx, spaceId, resp.ObjectId)
}

// UpdateType updates an existing type in a specific space.
func (s *service) UpdateType(ctx context.Context, spaceId string, typeId string, request UpdateTypeRequest) (Type, error) {
	// TODO
	return Type{}, nil
}

// DeleteType deletes a type by its ID in a specific space.
func (s *service) DeleteType(ctx context.Context, spaceId string, typeId string) (Type, error) {
	t, err := s.GetType(ctx, spaceId, typeId)
	if err != nil {
		return Type{}, err
	}

	resp := s.mw.ObjectSetIsArchived(ctx, &pb.RpcObjectSetIsArchivedRequest{
		ContextId:  typeId,
		IsArchived: true,
	})

	if resp.Error.Code != pb.RpcObjectSetIsArchivedResponseError_NULL {
		return Type{}, ErrFailedDeleteObject
	}

	return t, nil
}

// GetTypeMapsFromStore retrieves all types from all spaces.
func (s *service) GetTypeMapsFromStore(spaceIds []string, propertyMap map[string]map[string]Property) (map[string]map[string]Type, error) {
	spacesToTypes := make(map[string]map[string]Type, len(spaceIds))

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
func (s *service) GetTypeMapFromStore(spaceId string, propertyMap map[string]Property) (map[string]Type, error) {
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

	typeMap := make(map[string]Type, len(resp.Records))
	for _, record := range resp.Records {
		typeMap[record.Fields[bundle.RelationKeyId.String()].GetStringValue()] = Type{
			Object:     "type",
			Id:         record.Fields[bundle.RelationKeyId.String()].GetStringValue(),
			Key:        record.Fields[bundle.RelationKeyUniqueKey.String()].GetStringValue(),
			Name:       record.Fields[bundle.RelationKeyName.String()].GetStringValue(),
			Icon:       GetIcon(s.gatewayUrl, record.Fields[bundle.RelationKeyIconEmoji.String()].GetStringValue(), "", record.Fields[bundle.RelationKeyIconName.String()].GetStringValue(), record.Fields[bundle.RelationKeyIconOption.String()].GetNumberValue()),
			Archived:   record.Fields[bundle.RelationKeyIsArchived.String()].GetBoolValue(),
			Layout:     s.otLayoutToObjectLayout(model.ObjectTypeLayout(record.Fields[bundle.RelationKeyRecommendedLayout.String()].GetNumberValue())),
			Properties: s.getRecommendedPropertiesFromLists(record.Fields[bundle.RelationKeyRecommendedFeaturedRelations.String()].GetListValue(), record.Fields[bundle.RelationKeyRecommendedRelations.String()].GetListValue(), propertyMap),
		}
	}
	return typeMap, nil
}

// getTypeFromStruct retrieves the type from the details.
func (s *service) getTypeFromStruct(details *types.Struct, typeMap map[string]Type) Type {
	return typeMap[details.Fields[bundle.RelationKeyType.String()].GetStringValue()]
}

// buildTypeDetails builds the type details from the CreateTypeRequest.
func (s *service) buildTypeDetails(ctx context.Context, spaceId string, request CreateTypeRequest) (*types.Struct, error) {
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
		rk := FromPropertyApiKey(propLink.Key)
		if propDef, exists := propertyMap[rk]; exists {
			relationIds = append(relationIds, propDef.Id)
		} else {
			newProp, err2 := s.CreateProperty(ctx, spaceId, CreatePropertyRequest{
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

func (s *service) objectLayoutToObjectTypeLayout(objectLayout ObjectLayout) model.ObjectTypeLayout {
	switch objectLayout {
	case ObjectLayoutBasic:
		return model.ObjectType_basic
	case ObjectLayoutProfile:
		return model.ObjectType_profile
	case ObjectLayoutTodo:
		return model.ObjectType_todo
	case ObjectLayoutNote:
		return model.ObjectType_note
	case ObjectLayoutBookmark:
		return model.ObjectType_bookmark
	case ObjectLayoutSet:
		return model.ObjectType_set
	case ObjectLayoutCollection:
		return model.ObjectType_collection
	case ObjectLayoutParticipant:
		return model.ObjectType_participant
	default:
		return model.ObjectType_basic
	}
}

func (s *service) otLayoutToObjectLayout(objectTypeLayout model.ObjectTypeLayout) ObjectLayout {
	switch objectTypeLayout {
	case model.ObjectType_basic:
		return ObjectLayoutBasic
	case model.ObjectType_profile:
		return ObjectLayoutProfile
	case model.ObjectType_todo:
		return ObjectLayoutTodo
	case model.ObjectType_note:
		return ObjectLayoutNote
	case model.ObjectType_bookmark:
		return ObjectLayoutBookmark
	case model.ObjectType_set:
		return ObjectLayoutSet
	case model.ObjectType_collection:
		return ObjectLayoutCollection
	case model.ObjectType_participant:
		return ObjectLayoutParticipant
	default:
		return ObjectLayoutBasic
	}
}

func (s *service) typeLayoutToObjectTypeLayout(typeLayout TypeLayout) model.ObjectTypeLayout {
	switch typeLayout {
	case TypeLayoutBasic:
		return model.ObjectType_basic
	case TypeLayoutProfile:
		return model.ObjectType_profile
	case TypeLayoutTodo:
		return model.ObjectType_todo
	case TypeLayoutNote:
		return model.ObjectType_note
	default:
		return model.ObjectType_basic
	}
}

func (s *service) otLayoutToTypeLayout(objectTypeLayout model.ObjectTypeLayout) TypeLayout {
	switch objectTypeLayout {
	case model.ObjectType_basic:
		return TypeLayoutBasic
	case model.ObjectType_profile:
		return TypeLayoutProfile
	case model.ObjectType_todo:
		return TypeLayoutTodo
	case model.ObjectType_note:
		return TypeLayoutNote
	default:
		return TypeLayoutBasic
	}
}
