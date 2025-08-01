package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/gogo/protobuf/types"
	"github.com/iancoleman/strcase"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
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
func (s *Service) ListTypes(ctx context.Context, spaceId string, additionalFilters []*model.BlockContentDataviewFilter, offset int, limit int) (types []*apimodel.Type, total int, hasMore bool, err error) {
	filters := append([]*model.BlockContentDataviewFilter{
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
	}, additionalFilters...)

	resp := s.mw.ObjectSearch(ctx, &pb.RpcObjectSearchRequest{
		SpaceId: spaceId,
		Filters: filters,
		Sorts: []*model.BlockContentDataviewSort{
			{
				RelationKey: bundle.RelationKeyName.String(),
				Type:        model.BlockContentDataviewSort_Asc,
			},
		},
		Keys: []string{
			bundle.RelationKeyId.String(),
			bundle.RelationKeyUniqueKey.String(),
			bundle.RelationKeyApiObjectKey.String(),
			bundle.RelationKeyName.String(),
			bundle.RelationKeyIconEmoji.String(),
			bundle.RelationKeyIconName.String(),
			bundle.RelationKeyPluralName.String(),
			bundle.RelationKeyIconOption.String(),
			bundle.RelationKeyRecommendedLayout.String(),
			bundle.RelationKeyIsArchived.String(),
			bundle.RelationKeyRecommendedFeaturedRelations.String(),
			bundle.RelationKeyRecommendedRelations.String()},
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		return nil, 0, false, ErrFailedRetrieveTypes
	}

	total = len(resp.Records)
	paginatedTypes, hasMore := pagination.Paginate(resp.Records, offset, limit)
	types = make([]*apimodel.Type, 0, len(paginatedTypes))

	propertyMap := s.cache.getProperties(spaceId)

	for _, record := range paginatedTypes {
		_, _, t := s.getTypeFromStruct(record, propertyMap)
		types = append(types, t)
	}
	return types, total, hasMore, nil
}

// GetType returns a single type by its ID in a specific space.
func (s *Service) GetType(ctx context.Context, spaceId string, typeId string) (*apimodel.Type, error) {
	resp := s.mw.ObjectShow(ctx, &pb.RpcObjectShowRequest{
		SpaceId:  spaceId,
		ObjectId: typeId,
	})

	if resp.Error != nil {
		if resp.Error.Code == pb.RpcObjectShowResponseError_NOT_FOUND {
			return nil, ErrTypeNotFound
		}

		if resp.Error.Code == pb.RpcObjectShowResponseError_OBJECT_DELETED {
			return nil, ErrTypeDeleted
		}

		if resp.Error != nil && resp.Error.Code != pb.RpcObjectShowResponseError_NULL {
			return nil, ErrFailedRetrieveType
		}
	}

	// pre-fetch properties to fill the type
	propertyMap := s.cache.getProperties(spaceId)

	_, _, t := s.getTypeFromStruct(resp.ObjectView.Details[0].Details, propertyMap)
	return t, nil
}

// CreateType creates a new type in a specific space.
func (s *Service) CreateType(ctx context.Context, spaceId string, request apimodel.CreateTypeRequest) (*apimodel.Type, error) {
	details, err := s.buildTypeDetails(ctx, spaceId, request)
	if err != nil {
		return nil, err
	}

	resp := s.mw.ObjectCreateObjectType(ctx, &pb.RpcObjectCreateObjectTypeRequest{
		SpaceId: spaceId,
		Details: details,
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcObjectCreateObjectTypeResponseError_NULL {
		return nil, ErrFailedCreateType
	}

	return s.GetType(ctx, spaceId, resp.ObjectId)
}

// UpdateType updates an existing type in a specific space.
func (s *Service) UpdateType(ctx context.Context, spaceId string, typeId string, request apimodel.UpdateTypeRequest) (*apimodel.Type, error) {
	t, err := s.GetType(ctx, spaceId, typeId)
	if err != nil {
		return nil, err
	}

	details, err := s.buildUpdatedTypeDetails(ctx, spaceId, t, request)
	if err != nil {
		return nil, err
	}

	resp := s.mw.ObjectSetDetails(ctx, &pb.RpcObjectSetDetailsRequest{
		ContextId: typeId,
		Details:   structToDetails(details),
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcObjectSetDetailsResponseError_NULL {
		return nil, ErrFailedUpdateType
	}

	return s.GetType(ctx, spaceId, typeId)
}

// DeleteType deletes a type by its ID in a specific space.
func (s *Service) DeleteType(ctx context.Context, spaceId string, typeId string) (*apimodel.Type, error) {
	t, err := s.GetType(ctx, spaceId, typeId)
	if err != nil {
		return nil, err
	}

	resp := s.mw.ObjectSetIsArchived(ctx, &pb.RpcObjectSetIsArchivedRequest{
		ContextId:  typeId,
		IsArchived: true,
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcObjectSetIsArchivedResponseError_NULL {
		return nil, ErrFailedDeleteType
	}

	return t, nil
}

// getTypeFromStruct maps a type's details into an apimodel.Type.
// `uk` is what we use internally, `key` is the key being referenced in the API.
func (s *Service) getTypeFromStruct(details *types.Struct, propertyMap map[string]*apimodel.Property) (string, string, *apimodel.Type) {
	uk := details.Fields[bundle.RelationKeyUniqueKey.String()].GetStringValue()
	apiKey := util.ToTypeApiKey(uk)

	// apiObjectKey as key takes precedence over unique key
	if apiObjectKeyField, exists := details.Fields[bundle.RelationKeyApiObjectKey.String()]; exists {
		if apiObjectKey := apiObjectKeyField.GetStringValue(); apiObjectKey != "" {
			apiKey = apiObjectKey
		}
	}

	return uk, apiKey, &apimodel.Type{
		Object:     "type",
		Id:         details.Fields[bundle.RelationKeyId.String()].GetStringValue(),
		Key:        apiKey,
		Name:       details.Fields[bundle.RelationKeyName.String()].GetStringValue(),
		PluralName: details.Fields[bundle.RelationKeyPluralName.String()].GetStringValue(),
		Icon:       GetIcon(s.gatewayUrl, details.Fields[bundle.RelationKeyIconEmoji.String()].GetStringValue(), "", details.Fields[bundle.RelationKeyIconName.String()].GetStringValue(), details.Fields[bundle.RelationKeyIconOption.String()].GetNumberValue()),
		Archived:   details.Fields[bundle.RelationKeyIsArchived.String()].GetBoolValue(),
		Layout:     s.otLayoutToObjectLayout(model.ObjectTypeLayout(details.Fields[bundle.RelationKeyRecommendedLayout.String()].GetNumberValue())),
		Properties: s.getRecommendedPropertiesFromLists(details.Fields[bundle.RelationKeyRecommendedFeaturedRelations.String()].GetListValue(), details.Fields[bundle.RelationKeyRecommendedRelations.String()].GetListValue(), propertyMap),
		UniqueKey:  uk, // internal only for simplified lookup
	}
}

// getTypeFromMap retrieves the type from the details.
func (s *Service) getTypeFromMap(details *types.Struct) *apimodel.Type {
	spaceId := details.Fields[bundle.RelationKeySpaceId.String()].GetStringValue()
	typeMap := s.cache.getTypes(spaceId)
	if t, ok := typeMap[details.Fields[bundle.RelationKeyType.String()].GetStringValue()]; ok {
		return t
	}
	return nil
}

// buildTypeDetails builds the type details from the CreateTypeRequest.
func (s *Service) buildTypeDetails(ctx context.Context, spaceId string, request apimodel.CreateTypeRequest) (*types.Struct, error) {
	fields := map[string]*types.Value{
		bundle.RelationKeyName.String():              pbtypes.String(s.sanitizedString(request.Name)),
		bundle.RelationKeyPluralName.String():        pbtypes.String(s.sanitizedString(request.PluralName)),
		bundle.RelationKeyRecommendedLayout.String(): pbtypes.Int64(int64(s.typeLayoutToObjectTypeLayout(request.Layout))),
		bundle.RelationKeyOrigin.String():            pbtypes.Int64(int64(model.ObjectOrigin_api)),
	}

	if request.Key != "" {
		apiKey := strcase.ToSnake(s.sanitizedString(request.Key))
		if _, exists := s.cache.getTypes(spaceId)[apiKey]; exists {
			return nil, util.ErrBadInput(fmt.Sprintf("type key %q already exists", apiKey))
		}
		fields[bundle.RelationKeyApiObjectKey.String()] = pbtypes.String(apiKey)
	}

	iconFields, err := s.processIconFields(spaceId, request.Icon, true)
	if err != nil {
		return nil, err
	}
	for k, v := range iconFields {
		fields[k] = v
	}

	relationIds, err := s.buildRelationIds(ctx, spaceId, request.Properties)
	if err != nil {
		return nil, err
	}
	fields[bundle.RelationKeyRecommendedRelations.String()] = pbtypes.StringList(relationIds)

	propertyMap := s.cache.getProperties(spaceId)
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

// buildUpdatedTypeDetails builds a partial details struct for UpdateTypeRequest.
func (s *Service) buildUpdatedTypeDetails(ctx context.Context, spaceId string, t *apimodel.Type, request apimodel.UpdateTypeRequest) (*types.Struct, error) {
	fields := make(map[string]*types.Value)
	if request.Name != nil {
		fields[bundle.RelationKeyName.String()] = pbtypes.String(s.sanitizedString(*request.Name))
	}
	if request.PluralName != nil {
		fields[bundle.RelationKeyPluralName.String()] = pbtypes.String(s.sanitizedString(*request.PluralName))
	}
	if request.Layout != nil {
		fields[bundle.RelationKeyRecommendedLayout.String()] = pbtypes.Int64(int64(s.typeLayoutToObjectTypeLayout(*request.Layout)))
	}
	if request.Key != nil {
		apiKey := strcase.ToSnake(s.sanitizedString(*request.Key))
		if apiKey != t.Key {
			if existing, exists := s.cache.getTypes(spaceId)[apiKey]; exists && existing.Id != t.Id {
				return nil, util.ErrBadInput(fmt.Sprintf("type key %q already exists", apiKey))
			}
			if bundle.HasObjectTypeByKey(domain.TypeKey(util.ToTypeApiKey(t.UniqueKey))) {
				return nil, util.ErrBadInput("type key of bundled types cannot be changed")
			}
			fields[bundle.RelationKeyApiObjectKey.String()] = pbtypes.String(apiKey)
		}
	}

	if request.Icon != nil {
		iconFields, err := s.processIconFields(spaceId, *request.Icon, true)
		if err != nil {
			return nil, err
		}
		for k, v := range iconFields {
			fields[k] = v
		}
	}

	if request.Properties == nil {
		return &types.Struct{Fields: fields}, nil
	}

	currentFields, err := util.GetFieldsByID(s.mw, spaceId, t.Id, []string{bundle.RelationKeyRecommendedFeaturedRelations.String()})
	if err != nil {
		return nil, err
	}

	relationIds, err := s.buildRelationIds(ctx, spaceId, *request.Properties)
	if err != nil {
		return nil, err
	}

	var featuredIds []string
	if fv, exists := currentFields[bundle.RelationKeyRecommendedFeaturedRelations.String()]; exists {
		for _, v := range fv.GetListValue().Values {
			if id := v.GetStringValue(); id != "" {
				featuredIds = append(featuredIds, id)
			}
		}
	}
	// Filter out IDs already featured
	var filteredRelationIds []string
	for _, id := range relationIds {
		skip := false
		for _, fid := range featuredIds {
			if id == fid {
				skip = true
				break
			}
		}
		if !skip {
			filteredRelationIds = append(filteredRelationIds, id)
		}
	}
	fields[bundle.RelationKeyRecommendedRelations.String()] = pbtypes.StringList(filteredRelationIds)

	return &types.Struct{Fields: fields}, nil
}

// buildRelationIds constructs relation IDs for property links, creating new properties if necessary.
func (s *Service) buildRelationIds(ctx context.Context, spaceId string, props []apimodel.PropertyLink) ([]string, error) {
	propertyMap := s.cache.getProperties(spaceId)
	relationIds := make([]string, 0, len(props))

	for _, propLink := range props {
		rk := s.ResolvePropertyApiKey(propertyMap, propLink.Key)
		if propDef, exists := propertyMap[rk]; exists {
			relationIds = append(relationIds, propDef.Id)
			continue
		}
		newProp, err2 := s.CreateProperty(ctx, spaceId, apimodel.CreatePropertyRequest{
			Key:    propLink.Key,
			Name:   propLink.Name,
			Format: propLink.Format,
		})
		if err2 != nil {
			return nil, err2
		}
		relationIds = append(relationIds, newProp.Id)
	}

	return relationIds, nil
}

// ResolveTypeApiKey returns the internal uniqueKey for a clientKey by looking it up in the typeMap
// TODO: If not found, this detail shouldn't be set by clients, and strict validation errors
func (s *Service) ResolveTypeApiKey(spaceId string, clientKey string) string {
	typeMap := s.cache.getTypes(spaceId)
	if p, ok := typeMap[clientKey]; ok {
		return p.UniqueKey
	}
	return ""
	// TODO: enable later for strict validation
	// return "", false
}

func (s *Service) otLayoutToObjectLayout(objectTypeLayout model.ObjectTypeLayout) apimodel.ObjectLayout {
	switch objectTypeLayout {
	case model.ObjectType_basic:
		return apimodel.ObjectLayoutBasic
	case model.ObjectType_profile:
		return apimodel.ObjectLayoutProfile
	case model.ObjectType_todo:
		return apimodel.ObjectLayoutAction
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

func (s *Service) typeLayoutToObjectTypeLayout(typeLayout apimodel.TypeLayout) model.ObjectTypeLayout {
	switch typeLayout {
	case apimodel.TypeLayoutBasic:
		return model.ObjectType_basic
	case apimodel.TypeLayoutProfile:
		return model.ObjectType_profile
	case apimodel.TypeLayoutAction:
		return model.ObjectType_todo
	case apimodel.TypeLayoutNote:
		return model.ObjectType_note
	default:
		return model.ObjectType_basic
	}
}
