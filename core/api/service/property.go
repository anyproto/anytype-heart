package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

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
	ErrFailedRetrieveProperties = errors.New("failed to retrieve properties")
	ErrPropertyNotFound         = errors.New("property not found")
	ErrPropertyDeleted          = errors.New("property deleted")
	ErrFailedRetrieveProperty   = errors.New("failed to retrieve property")
	ErrFailedCreateProperty     = errors.New("failed to create property")
	ErrPropertyCannotBeUpdated  = errors.New("property cannot be updated")
	ErrFailedUpdateProperty     = errors.New("failed to update property")
	ErrFailedDeleteProperty     = errors.New("failed to delete property")
)

var excludedSystemProperties = map[string]bool{
	bundle.RelationKeyId.String():                     true,
	bundle.RelationKeySpaceId.String():                true,
	bundle.RelationKeyName.String():                   true,
	bundle.RelationKeyIconEmoji.String():              true,
	bundle.RelationKeyIconImage.String():              true,
	bundle.RelationKeyType.String():                   true,
	bundle.RelationKeyResolvedLayout.String():         true,
	bundle.RelationKeyIsFavorite.String():             true,
	bundle.RelationKeyIsArchived.String():             true,
	bundle.RelationKeyIsDeleted.String():              true,
	bundle.RelationKeyIsHidden.String():               true,
	bundle.RelationKeyWorkspaceId.String():            true,
	bundle.RelationKeyInternalFlags.String():          true,
	bundle.RelationKeyRestrictions.String():           true,
	bundle.RelationKeyOrigin.String():                 true,
	bundle.RelationKeySnippet.String():                true,
	bundle.RelationKeySyncStatus.String():             true,
	bundle.RelationKeySyncError.String():              true,
	bundle.RelationKeySyncDate.String():               true,
	bundle.RelationKeyCoverId.String():                true,
	bundle.RelationKeyCoverType.String():              true,
	bundle.RelationKeyCoverScale.String():             true,
	bundle.RelationKeyCoverX.String():                 true,
	bundle.RelationKeyCoverY.String():                 true,
	bundle.RelationKeyMentions.String():               true,
	bundle.RelationKeyOldAnytypeID.String():           true,
	bundle.RelationKeySourceFilePath.String():         true,
	bundle.RelationKeyImportType.String():             true,
	bundle.RelationKeyTargetObjectType.String():       true,
	bundle.RelationKeyFeaturedRelations.String():      true,
	bundle.RelationKeySetOf.String():                  true,
	bundle.RelationKeySourceObject.String():           true,
	bundle.RelationKeyLayoutAlign.String():            true,
	bundle.RelationKeyIsHiddenDiscovery.String():      true,
	bundle.RelationKeyLayout.String():                 true,
	bundle.RelationKeyIsReadonly.String():             true,
	bundle.RelationKeyParticipantStatus.String():      true,
	bundle.RelationKeyParticipantPermissions.String(): true,
	bundle.RelationKeyIconOption.String():             true,
	bundle.RelationKeyIconName.String():               true,
	bundle.RelationKeyPicture.String():                true,
}

var PropertyFormatToRelationFormat = map[apimodel.PropertyFormat]model.RelationFormat{
	apimodel.PropertyFormatText:        model.RelationFormat_longtext,
	apimodel.PropertyFormatNumber:      model.RelationFormat_number,
	apimodel.PropertyFormatSelect:      model.RelationFormat_status,
	apimodel.PropertyFormatMultiSelect: model.RelationFormat_tag,
	apimodel.PropertyFormatDate:        model.RelationFormat_date,
	apimodel.PropertyFormatFiles:       model.RelationFormat_file,
	apimodel.PropertyFormatCheckbox:    model.RelationFormat_checkbox,
	apimodel.PropertyFormatUrl:         model.RelationFormat_url,
	apimodel.PropertyFormatEmail:       model.RelationFormat_email,
	apimodel.PropertyFormatPhone:       model.RelationFormat_phone,
	apimodel.PropertyFormatObjects:     model.RelationFormat_object,
}

var RelationFormatToPropertyFormat = map[model.RelationFormat]apimodel.PropertyFormat{
	model.RelationFormat_longtext:  apimodel.PropertyFormatText,
	model.RelationFormat_shorttext: apimodel.PropertyFormatText,
	model.RelationFormat_number:    apimodel.PropertyFormatNumber,
	model.RelationFormat_status:    apimodel.PropertyFormatSelect,
	model.RelationFormat_tag:       apimodel.PropertyFormatMultiSelect,
	model.RelationFormat_date:      apimodel.PropertyFormatDate,
	model.RelationFormat_file:      apimodel.PropertyFormatFiles,
	model.RelationFormat_checkbox:  apimodel.PropertyFormatCheckbox,
	model.RelationFormat_url:       apimodel.PropertyFormatUrl,
	model.RelationFormat_email:     apimodel.PropertyFormatEmail,
	model.RelationFormat_phone:     apimodel.PropertyFormatPhone,
	model.RelationFormat_object:    apimodel.PropertyFormatObjects,
}

// ListProperties returns a list of properties for a specific space.
func (s *Service) ListProperties(ctx context.Context, spaceId string, additionalFilters []*model.BlockContentDataviewFilter, offset int, limit int) (properties []apimodel.Property, total int, hasMore bool, err error) {
	filters := append([]*model.BlockContentDataviewFilter{
		{
			RelationKey: bundle.RelationKeyResolvedLayout.String(),
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       pbtypes.Int64(int64(model.ObjectType_relation)),
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
			bundle.RelationKeyRelationKey.String(),
			bundle.RelationKeyApiObjectKey.String(),
			bundle.RelationKeyName.String(),
			bundle.RelationKeyRelationFormat.String(),
		},
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		return nil, 0, false, ErrFailedRetrieveProperties
	}

	filteredRecords := make([]*types.Struct, 0, len(resp.Records))
	for _, record := range resp.Records {
		rk, _, _ := s.getPropertyFromStruct(record)
		if _, isExcluded := excludedSystemProperties[rk]; isExcluded {
			continue
		}
		filteredRecords = append(filteredRecords, record)
	}

	total = len(filteredRecords)
	paginatedProperties, hasMore := pagination.Paginate(filteredRecords, offset, limit)
	properties = make([]apimodel.Property, 0, len(paginatedProperties))

	for _, record := range paginatedProperties {
		_, _, property := s.getPropertyFromStruct(record)
		properties = append(properties, property)
	}

	return properties, total, hasMore, nil
}

// GetProperty retrieves a single property by its ID in a specific space.
func (s *Service) GetProperty(ctx context.Context, spaceId string, propertyId string) (apimodel.Property, error) {
	resp := s.mw.ObjectShow(ctx, &pb.RpcObjectShowRequest{
		SpaceId:  spaceId,
		ObjectId: propertyId,
	})

	if resp.Error != nil {
		if resp.Error.Code == pb.RpcObjectShowResponseError_NOT_FOUND {
			return apimodel.Property{}, ErrPropertyNotFound
		}

		if resp.Error.Code == pb.RpcObjectShowResponseError_OBJECT_DELETED {
			return apimodel.Property{}, ErrPropertyDeleted
		}

		if resp.Error != nil && resp.Error.Code != pb.RpcObjectShowResponseError_NULL {
			return apimodel.Property{}, ErrFailedRetrieveProperty
		}
	}

	rk, _, property := s.getPropertyFromStruct(resp.ObjectView.Details[0].Details)
	if _, isExcluded := excludedSystemProperties[rk]; isExcluded {
		return apimodel.Property{}, ErrPropertyNotFound
	}
	return property, nil
}

// CreateProperty creates a new property in a specific space.
func (s *Service) CreateProperty(ctx context.Context, spaceId string, request apimodel.CreatePropertyRequest) (apimodel.Property, error) {
	details := &types.Struct{
		Fields: map[string]*types.Value{
			bundle.RelationKeyName.String():           pbtypes.String(s.sanitizedString(request.Name)),
			bundle.RelationKeyRelationFormat.String(): pbtypes.Int64(int64(PropertyFormatToRelationFormat[request.Format])),
			bundle.RelationKeyOrigin.String():         pbtypes.Int64(int64(model.ObjectOrigin_api)),
		},
	}

	if request.Key != "" {
		apiKey := strcase.ToSnake(s.sanitizedString(request.Key))
		propertyMap, err := s.getPropertyMapFromStore(ctx, spaceId, false)
		if err != nil {
			return apimodel.Property{}, err
		}
		if _, exists := propertyMap[apiKey]; exists {
			return apimodel.Property{}, util.ErrBadInput(fmt.Sprintf("property key %q already exists", apiKey))
		}
		details.Fields[bundle.RelationKeyApiObjectKey.String()] = pbtypes.String(apiKey)
	}

	resp := s.mw.ObjectCreateRelation(ctx, &pb.RpcObjectCreateRelationRequest{
		SpaceId: spaceId,
		Details: details,
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcObjectCreateRelationResponseError_NULL {
		return apimodel.Property{}, ErrFailedCreateProperty
	}

	if len(request.Tags) > 0 && (request.Format == apimodel.PropertyFormatSelect || request.Format == apimodel.PropertyFormatMultiSelect) {
		err := s.createTagsForProperty(ctx, spaceId, resp.ObjectId, request.Tags)
		if err != nil {
			return apimodel.Property{}, fmt.Errorf("property created but tag creation failed: %w", err)
		}
	}

	return s.GetProperty(ctx, spaceId, resp.ObjectId)
}

// UpdateProperty updates an existing property in a specific space.
func (s *Service) UpdateProperty(ctx context.Context, spaceId string, propertyId string, request apimodel.UpdatePropertyRequest) (apimodel.Property, error) {
	prop, err := s.GetProperty(ctx, spaceId, propertyId)
	if err != nil {
		return apimodel.Property{}, err
	}

	rel, err := bundle.PickRelation(domain.RelationKey(prop.RelationKey))
	if err == nil && rel.ReadOnly {
		return apimodel.Property{}, ErrPropertyCannotBeUpdated
	}

	var detailsToUpdate []*model.Detail
	if request.Name != nil {
		detailsToUpdate = append(detailsToUpdate, &model.Detail{
			Key:   bundle.RelationKeyName.String(),
			Value: pbtypes.String(s.sanitizedString(*request.Name)),
		})
	}
	if request.Key != nil {
		apiKey := strcase.ToSnake(s.sanitizedString(*request.Key))
		propertyMap, err := s.getPropertyMapFromStore(ctx, spaceId, false)
		if err != nil {
			return apimodel.Property{}, err
		}
		if existing, exists := propertyMap[apiKey]; exists && existing.Id != propertyId {
			return apimodel.Property{}, util.ErrBadInput(fmt.Sprintf("property key %q already exists", apiKey))
		}
		if bundle.HasRelation(domain.RelationKey(prop.RelationKey)) {
			return apimodel.Property{}, util.ErrBadInput("property key of bundled properties cannot be changed")
		}
		detailsToUpdate = append(detailsToUpdate, &model.Detail{
			Key:   bundle.RelationKeyApiObjectKey.String(),
			Value: pbtypes.String(apiKey),
		})
	}

	if len(detailsToUpdate) > 0 {
		resp := s.mw.ObjectSetDetails(ctx, &pb.RpcObjectSetDetailsRequest{
			ContextId: propertyId,
			Details:   detailsToUpdate,
		})
		if resp.Error != nil && resp.Error.Code != pb.RpcObjectSetDetailsResponseError_NULL {
			return apimodel.Property{}, ErrFailedUpdateProperty
		}
	}

	return s.GetProperty(ctx, spaceId, propertyId)
}

// DeleteProperty deletes a property in a specific space.
func (s *Service) DeleteProperty(ctx context.Context, spaceId string, propertyId string) (apimodel.Property, error) {
	property, err := s.GetProperty(ctx, spaceId, propertyId)
	if err != nil {
		return apimodel.Property{}, err
	}

	resp := s.mw.ObjectSetIsArchived(ctx, &pb.RpcObjectSetIsArchivedRequest{
		ContextId:  propertyId,
		IsArchived: true,
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcObjectSetIsArchivedResponseError_NULL {
		return apimodel.Property{}, ErrFailedDeleteProperty
	}

	return property, nil
}

func (s *Service) sanitizedString(str string) string {
	return strings.TrimSpace(str)
}

// createTagsForProperty creates tags for a newly created property
func (s *Service) createTagsForProperty(ctx context.Context, spaceId string, propertyId string, tagsToCreate []apimodel.CreateTagRequest) error {
	for _, tagRequest := range tagsToCreate {
		_, err := s.CreateTag(ctx, spaceId, propertyId, tagRequest)
		if err != nil {
			return fmt.Errorf("failed to create tag %q: %w", tagRequest.Name, err)
		}
	}

	return nil
}

// processProperties builds detail fields for the given property entries, applying sanitization and validation for each.
func (s *Service) processProperties(ctx context.Context, spaceId string, entries []apimodel.PropertyLinkWithValue) (map[string]*types.Value, error) {
	fields := make(map[string]*types.Value)
	if len(entries) == 0 {
		return fields, nil
	}
	propertyMap, err := s.getPropertyMapFromStore(ctx, spaceId, false)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		key := entry.Key()
		value := entry.Value()

		rk := s.ResolvePropertyApiKey(propertyMap, key)
		if _, excluded := excludedSystemProperties[rk]; excluded {
			continue
		}

		if value == nil {
			fields[rk] = pbtypes.ToValue(nil)
			continue
		}

		if slices.Contains(bundle.LocalAndDerivedRelationKeys, domain.RelationKey(key)) {
			return nil, util.ErrBadInput("property '" + key + "' cannot be set directly as it is a reserved system property")
		}
		prop, ok := propertyMap[rk]
		if !ok {
			return nil, util.ErrBadInput(fmt.Sprintf("unknown property key: %q", rk))
		}

		sanitized, err := s.sanitizeAndValidatePropertyValue(spaceId, key, prop.Format, value, prop, propertyMap)
		if err != nil {
			return nil, err
		}
		fields[rk] = pbtypes.ToValue(sanitized)
	}
	return fields, nil
}

// sanitizeAndValidatePropertyValue checks the value for a property according to its format and ensures referenced IDs exist and are valid.
func (s *Service) sanitizeAndValidatePropertyValue(spaceId string, key string, format apimodel.PropertyFormat, value interface{}, property *apimodel.Property, propertyMap map[string]*apimodel.Property) (interface{}, error) {
	switch format {
	case apimodel.PropertyFormatText, apimodel.PropertyFormatUrl, apimodel.PropertyFormatEmail, apimodel.PropertyFormatPhone:
		str, ok := value.(string)
		if !ok {
			return nil, util.ErrBadInput("property '" + key + "' must be a string")
		}
		return s.sanitizedString(str), nil
	case apimodel.PropertyFormatNumber:
		num, ok := value.(float64)
		if !ok {
			return nil, util.ErrBadInput("property '" + key + "' must be a number")
		}
		return num, nil
	case apimodel.PropertyFormatSelect:
		id, ok := value.(string)
		id = s.sanitizedString(id)
		if !ok {
			return nil, util.ErrBadInput("property '" + key + "' must be a string (tag id)")
		}
		if !s.isValidSelectOption(spaceId, property, id, propertyMap) {
			return nil, util.ErrBadInput("invalid select option for '" + key + "': " + id)
		}
		return id, nil
	case apimodel.PropertyFormatMultiSelect:
		ids, ok := value.([]interface{})
		if !ok {
			return nil, util.ErrBadInput("property '" + key + "' must be an array of tag ids")
		}
		var validIds []string
		for _, v := range ids {
			id, ok := v.(string)
			if !ok {
				return nil, util.ErrBadInput("property '" + key + "' must be an array of strings (tag ids)")
			}
			id = s.sanitizedString(id)
			if !s.isValidSelectOption(spaceId, property, id, propertyMap) {
				return nil, util.ErrBadInput("invalid multi_select option for '" + key + "': " + id)
			}
			validIds = append(validIds, id)
		}
		return validIds, nil
	case apimodel.PropertyFormatDate:
		dateStr, ok := value.(string)
		if !ok {
			return nil, util.ErrBadInput("property '" + key + "' must be a string (date in RFC3339 format)")
		}
		dateStr = s.sanitizedString(dateStr)
		layouts := []string{time.RFC3339, time.DateOnly}
		for _, layout := range layouts {
			if t, err := time.Parse(layout, dateStr); err == nil {
				return t.Unix(), nil
			}
		}
		return nil, util.ErrBadInput("invalid date format for '" + key + "': " + dateStr)
	case apimodel.PropertyFormatCheckbox:
		b, ok := value.(bool)
		if !ok {
			return nil, util.ErrBadInput("property '" + key + "' must be a boolean")
		}
		return b, nil
	case apimodel.PropertyFormatObjects, apimodel.PropertyFormatFiles:
		ids, ok := value.([]interface{})
		if !ok {
			return nil, util.ErrBadInput("property '" + key + "' must be an array of strings (object/file ids)")
		}
		var validIds []string
		for _, v := range ids {
			id, ok := v.(string)
			if !ok {
				return nil, util.ErrBadInput("property '" + key + "' must be an array of strings (object/file ids)")
			}
			id = s.sanitizedString(id)
			if format == apimodel.PropertyFormatFiles && !s.isValidFileReference(spaceId, id) {
				return nil, util.ErrBadInput("invalid file reference for '" + key + "': " + id)
			} else if format == apimodel.PropertyFormatObjects && !s.isValidObjectReference(spaceId, id) {
				return nil, util.ErrBadInput("invalid object reference for '" + key + "': " + id)
			}
			validIds = append(validIds, id)
		}
		return validIds, nil
	default:
		return nil, util.ErrBadInput("unsupported property format: " + string(format))
	}
}

// isValidSelectOption checks if the option id is valid for the given property.
func (s *Service) isValidSelectOption(spaceId string, property *apimodel.Property, tagId string, propertyMap map[string]*apimodel.Property) bool {
	fields, err := util.GetFieldsByID(s.mw, spaceId, tagId, []string{bundle.RelationKeyResolvedLayout.String(), bundle.RelationKeyRelationKey.String()})
	if err != nil {
		return false
	}
	layout := model.ObjectTypeLayout(fields[bundle.RelationKeyResolvedLayout.String()].GetNumberValue())
	rk := fields[bundle.RelationKeyRelationKey.String()].GetStringValue()
	return util.IsTagLayout(layout) && rk == s.ResolvePropertyApiKey(propertyMap, property.Key)
}

func (s *Service) isValidObjectReference(spaceId string, objectId string) bool {
	fields, err := util.GetFieldsByID(s.mw, spaceId, objectId, []string{bundle.RelationKeyResolvedLayout.String()})
	if err != nil {
		return false
	}
	layout := model.ObjectTypeLayout(fields[bundle.RelationKeyResolvedLayout.String()].GetNumberValue())
	return util.IsObjectLayout(layout)
}

func (s *Service) isValidFileReference(spaceId string, fileId string) bool {
	fields, err := util.GetFieldsByID(s.mw, spaceId, fileId, []string{bundle.RelationKeyResolvedLayout.String()})
	if err != nil {
		return false
	}
	layout := model.ObjectTypeLayout(fields[bundle.RelationKeyResolvedLayout.String()].GetNumberValue())
	return util.IsFileLayout(layout)
}

// getRecommendedPropertiesFromLists combines featured and regular properties into a list of Properties.
func (s *Service) getRecommendedPropertiesFromLists(featured, regular *types.ListValue, propertyMap map[string]*apimodel.Property) []apimodel.Property {
	var props []apimodel.Property
	lists := []*types.ListValue{featured, regular}
	for _, lst := range lists {
		if lst == nil {
			continue
		}
		for _, v := range lst.Values {
			id := v.GetStringValue()
			if id == "" {
				continue
			}
			p, ok := propertyMap[id]
			if !ok {
				continue
			}
			if _, excluded := excludedSystemProperties[p.RelationKey]; excluded {
				continue
			}
			props = append(props, *p)
		}
	}
	return props
}

// getPropertyMapsFromStore retrieves all properties for all spaces.
// Property entries can also be keyed by property id. Required for filling types with properties, as recommended properties are referenced by id and not key.
func (s *Service) getPropertyMapsFromStore(ctx context.Context, spaceIds []string, keyByPropertyId bool) (map[string]map[string]*apimodel.Property, error) {
	spacesToProperties := make(map[string]map[string]*apimodel.Property, len(spaceIds))

	for _, spaceId := range spaceIds {
		propertyMap, err := s.getPropertyMapFromStore(ctx, spaceId, keyByPropertyId)
		if err != nil {
			return nil, err
		}
		spacesToProperties[spaceId] = propertyMap
	}

	return spacesToProperties, nil
}

// getPropertyMapFromStore retrieves all properties for a specific space
// Property entries can also be keyed by property id. Required for filling types with properties, as recommended properties are referenced by id and not key.
func (s *Service) getPropertyMapFromStore(ctx context.Context, spaceId string, keyByPropertyId bool) (map[string]*apimodel.Property, error) {
	resp := s.mw.ObjectSearch(ctx, &pb.RpcObjectSearchRequest{
		SpaceId: spaceId,
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyResolvedLayout.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.Int64(int64(model.ObjectType_relation)),
			},
			{
				RelationKey: bundle.RelationKeyIsHidden.String(),
				Condition:   model.BlockContentDataviewFilter_NotEqual,
				Value:       pbtypes.Bool(true),
			},
		},
		Keys: []string{
			bundle.RelationKeyId.String(),
			bundle.RelationKeyRelationKey.String(),
			bundle.RelationKeyApiObjectKey.String(),
			bundle.RelationKeyName.String(),
			bundle.RelationKeyRelationFormat.String(),
		},
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		return nil, ErrFailedRetrievePropertyMap
	}

	propertyMap := make(map[string]*apimodel.Property, len(resp.Records))
	for _, record := range resp.Records {
		rk, apiKey, p := s.getPropertyFromStruct(record)
		prop := p
		propertyMap[rk] = &prop
		propertyMap[apiKey] = &prop
		if keyByPropertyId {
			propertyMap[p.Id] = &prop // add property under id as key to map as well
		}
	}

	return propertyMap, nil
}

// getPropertyFromStruct maps a property's details into an apimodel.Property.
// `rk` is what we use internally, `key` is the key being referenced in the API.
func (s *Service) getPropertyFromStruct(details *types.Struct) (string, string, apimodel.Property) {
	rk := details.Fields[bundle.RelationKeyRelationKey.String()].GetStringValue()
	apiKey := util.ToPropertyApiKey(rk)

	// apiObjectKey as key takes precedence over relation key
	if apiObjectKeyField, exists := details.Fields[bundle.RelationKeyApiObjectKey.String()]; exists {
		if apiObjectKey := apiObjectKeyField.GetStringValue(); apiObjectKey != "" {
			apiKey = apiObjectKey
		}
	}

	return rk, apiKey, apimodel.Property{
		Object:      "property",
		Id:          details.Fields[bundle.RelationKeyId.String()].GetStringValue(),
		Key:         apiKey,
		Name:        details.Fields[bundle.RelationKeyName.String()].GetStringValue(),
		Format:      RelationFormatToPropertyFormat[model.RelationFormat(details.Fields[bundle.RelationKeyRelationFormat.String()].GetNumberValue())],
		RelationKey: rk, // internal-only for simplified lookup
	}
}

// getPropertiesFromStruct retrieves the properties from the details.
func (s *Service) getPropertiesFromStruct(details *types.Struct, propertyMap map[string]*apimodel.Property, tagMap map[string]apimodel.Tag) []apimodel.PropertyWithValue {
	properties := make([]apimodel.PropertyWithValue, 0)
	for rk, value := range details.GetFields() {
		if _, isExcluded := excludedSystemProperties[rk]; isExcluded {
			continue
		}

		prop, ok := propertyMap[rk]
		if !ok {
			// relation key present in details but missing from propertyMap; skip it
			continue
		}

		key := prop.Key
		format := prop.Format
		convertedVal := s.convertPropertyValue(key, value, format, details, tagMap)

		id := prop.Id
		name := prop.Name
		if pwv := s.buildPropertyWithValue(id, key, name, format, convertedVal); pwv != nil {
			properties = append(properties, *pwv)
		}
	}

	return properties
}

// convertPropertyValue converts a protobuf types.Value into a native Go value.
func (s *Service) convertPropertyValue(key string, value *types.Value, format apimodel.PropertyFormat, details *types.Struct, tagMap map[string]apimodel.Tag) interface{} {
	switch kind := value.Kind.(type) {
	case *types.Value_NullValue:
		return nil
	case *types.Value_NumberValue:
		if format == apimodel.PropertyFormatDate {
			return time.Unix(int64(kind.NumberValue), 0).UTC().Format(time.RFC3339)
		}
		return kind.NumberValue
	case *types.Value_StringValue:
		if kind.StringValue == "_missing_object" {
			return nil
		}
		if format == apimodel.PropertyFormatSelect {
			tags := s.getTagsFromStruct([]string{kind.StringValue}, tagMap)
			if len(tags) > 0 {
				return tags[0]
			}
			return nil
		}
		if format == apimodel.PropertyFormatMultiSelect {
			return s.getTagsFromStruct([]string{kind.StringValue}, tagMap)
		}
		return kind.StringValue
	case *types.Value_BoolValue:
		return kind.BoolValue
	case *types.Value_StructValue:
		m := make(map[string]interface{})
		for k, v := range kind.StructValue.Fields {
			m[k] = s.convertPropertyValue(key, v, format, details, tagMap)
		}
		return m
	case *types.Value_ListValue:
		if format == apimodel.PropertyFormatSelect {
			listValues := kind.ListValue.Values
			if len(listValues) > 0 {
				tags := s.getTagsFromStruct([]string{listValues[0].GetStringValue()}, tagMap)
				if len(tags) > 0 {
					return tags[0]
				}
			}
			return nil
		}
		if format == apimodel.PropertyFormatMultiSelect {
			listValues := kind.ListValue.Values
			if len(listValues) > 0 {
				listStringValues := make([]string, len(listValues))
				for i, v := range listValues {
					listStringValues[i] = v.GetStringValue()
				}
				return s.getTagsFromStruct(listStringValues, tagMap)
			}
			return nil
		}
		var list []interface{}
		for _, v := range kind.ListValue.Values {
			list = append(list, s.convertPropertyValue(key, v, format, details, tagMap))
		}
		return list
	default:
		return nil
	}
}

// buildPropertyWithValue creates a Property based on the format and converted value.
func (s *Service) buildPropertyWithValue(id string, key string, name string, format apimodel.PropertyFormat, val interface{}) *apimodel.PropertyWithValue {
	base := apimodel.PropertyBase{
		Object: "property",
		Id:     id,
	}

	switch format {
	case apimodel.PropertyFormatText:
		if str, ok := val.(string); ok {
			return &apimodel.PropertyWithValue{WrappedPropertyWithValue: apimodel.TextPropertyValue{
				PropertyBase: base, Key: key, Name: name, Format: format,
				Text: str,
			}}
		}
	case apimodel.PropertyFormatNumber:
		if num, ok := val.(float64); ok {
			return &apimodel.PropertyWithValue{WrappedPropertyWithValue: apimodel.NumberPropertyValue{
				PropertyBase: base, Key: key, Name: name, Format: format,
				Number: num,
			}}
		}
	case apimodel.PropertyFormatSelect:
		if sel, ok := val.(apimodel.Tag); ok {
			return &apimodel.PropertyWithValue{WrappedPropertyWithValue: apimodel.SelectPropertyValue{
				PropertyBase: base, Key: key, Name: name, Format: format,
				Select: sel,
			}}
		}
	case apimodel.PropertyFormatMultiSelect:
		if ms, ok := val.([]apimodel.Tag); ok {
			return &apimodel.PropertyWithValue{WrappedPropertyWithValue: apimodel.MultiSelectPropertyValue{
				PropertyBase: base, Key: key, Name: name, Format: format,
				MultiSelect: ms,
			}}
		}
	case apimodel.PropertyFormatDate:
		if dateStr, ok := val.(string); ok {
			return &apimodel.PropertyWithValue{WrappedPropertyWithValue: apimodel.DatePropertyValue{
				PropertyBase: base, Key: key, Name: name, Format: format,
				Date: dateStr,
			}}
		}
	case apimodel.PropertyFormatFiles:
		if fileList, ok := val.([]interface{}); ok {
			files := make([]string, 0, len(fileList))
			for _, v := range fileList {
				if str, ok := v.(string); ok {
					files = append(files, str)
				}
			}
			return &apimodel.PropertyWithValue{WrappedPropertyWithValue: apimodel.FilesPropertyValue{
				PropertyBase: base, Key: key, Name: name, Format: format,
				Files: files,
			}}
		}
	case apimodel.PropertyFormatCheckbox:
		if cb, ok := val.(bool); ok {
			return &apimodel.PropertyWithValue{WrappedPropertyWithValue: apimodel.CheckboxPropertyValue{
				PropertyBase: base, Key: key, Name: name, Format: format,
				Checkbox: cb,
			}}
		}
	case apimodel.PropertyFormatUrl:
		if urlStr, ok := val.(string); ok {
			return &apimodel.PropertyWithValue{WrappedPropertyWithValue: apimodel.URLPropertyValue{
				PropertyBase: base, Key: key, Name: name, Format: format,
				Url: urlStr,
			}}
		}
	case apimodel.PropertyFormatEmail:
		if email, ok := val.(string); ok {
			return &apimodel.PropertyWithValue{WrappedPropertyWithValue: apimodel.EmailPropertyValue{
				PropertyBase: base, Key: key, Name: name, Format: format,
				Email: email,
			}}
		}
	case apimodel.PropertyFormatPhone:
		if phone, ok := val.(string); ok {
			return &apimodel.PropertyWithValue{WrappedPropertyWithValue: apimodel.PhonePropertyValue{
				PropertyBase: base, Key: key, Name: name, Format: format,
				Phone: phone,
			}}
		}
	case apimodel.PropertyFormatObjects:
		var objs []string
		switch v := val.(type) {
		case string:
			objs = []string{v}
		case []interface{}:
			for _, item := range v {
				if str, ok := item.(string); ok {
					objs = append(objs, str)
				}
			}
		}
		return &apimodel.PropertyWithValue{WrappedPropertyWithValue: apimodel.ObjectsPropertyValue{
			PropertyBase: base,
			Key:          key,
			Name:         name,
			Format:       format,
			Objects:      objs,
		}}
	}

	return nil
}

// ResolvePropertyApiKey returns the internal relationKey for a clientKey by looking it up in the propertyMap
// TODO: If not found, this detail shouldn't be set by clients, and strict validation errors
func (s *Service) ResolvePropertyApiKey(propertyMap map[string]*apimodel.Property, clientKey string) string {
	if p, ok := propertyMap[clientKey]; ok {
		return p.RelationKey
	}
	return ""
	// TODO: enable later for strict validation
	// return "", false
}
