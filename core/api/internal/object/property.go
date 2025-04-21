package object

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/gogo/protobuf/types"
	"github.com/iancoleman/strcase"

	"github.com/anyproto/anytype-heart/core/api/pagination"
	"github.com/anyproto/anytype-heart/core/api/util"
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
	ErrFailedUpdateProperty     = errors.New("failed to update property")
	ErrFailedDeleteProperty     = errors.New("failed to delete property")
	ErrFailedRetrieveTags       = errors.New("failed to retrieve tags")
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
	bundle.RelationKeySource.String():                 true,
	bundle.RelationKeySourceFilePath.String():         true,
	bundle.RelationKeyImportType.String():             true,
	bundle.RelationKeyTargetObjectType.String():       true,
	bundle.RelationKeyFeaturedRelations.String():      true,
	bundle.RelationKeySetOf.String():                  true,
	bundle.RelationKeyLinks.String():                  true,
	bundle.RelationKeyBacklinks.String():              true,
	bundle.RelationKeySourceObject.String():           true,
	bundle.RelationKeyLayoutAlign.String():            true,
	bundle.RelationKeyIsHiddenDiscovery.String():      true,
	bundle.RelationKeyLayout.String():                 true,
	bundle.RelationKeyIsReadonly.String():             true,
	bundle.RelationKeyParticipantStatus.String():      true,
	bundle.RelationKeyParticipantPermissions.String(): true,
	bundle.RelationKeyIconOption.String():             true,
	bundle.RelationKeyIconName.String():               true,
}

// ListProperties returns a list of properties for a specific space.
func (s *service) ListProperties(ctx context.Context, spaceId string, offset int, limit int) (properties []Property, total int, hasMore bool, err error) {
	resp := s.mw.ObjectSearch(context.Background(), &pb.RpcObjectSearchRequest{
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
			bundle.RelationKeyUniqueKey.String(),
			bundle.RelationKeyName.String(),
			bundle.RelationKeyRelationFormat.String(),
		},
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		return nil, 0, false, ErrFailedRetrieveProperties
	}

	filteredRecords := make([]*types.Struct, 0, len(resp.Records))
	for _, record := range resp.Records {
		rk, _ := s.mapPropertyFromRecord(record)
		if _, isExcluded := excludedSystemProperties[rk]; isExcluded {
			continue
		}
		filteredRecords = append(filteredRecords, record)
	}

	total = len(filteredRecords)
	paginatedProperties, hasMore := pagination.Paginate(filteredRecords, offset, limit)
	properties = make([]Property, 0, len(paginatedProperties))

	for _, record := range paginatedProperties {
		_, property := s.mapPropertyFromRecord(record)
		properties = append(properties, property)
	}

	return properties, total, hasMore, nil
}

// GetProperty retrieves a single property by its ID in a specific space.
func (s *service) GetProperty(ctx context.Context, spaceId string, propertyId string) (Property, error) {
	resp := s.mw.ObjectShow(context.Background(), &pb.RpcObjectShowRequest{
		SpaceId:  spaceId,
		ObjectId: propertyId,
	})

	if resp.Error != nil {
		if resp.Error.Code == pb.RpcObjectShowResponseError_NOT_FOUND {
			return Property{}, ErrPropertyNotFound
		}

		if resp.Error.Code == pb.RpcObjectShowResponseError_OBJECT_DELETED {
			return Property{}, ErrPropertyDeleted
		}

		if resp.Error.Code != pb.RpcObjectShowResponseError_NULL {
			return Property{}, ErrFailedRetrieveProperty
		}
	}

	rk, property := s.mapPropertyFromRecord(resp.ObjectView.Details[0].Details)
	if _, isExcluded := excludedSystemProperties[rk]; isExcluded {
		return Property{}, ErrPropertyNotFound
	}
	return property, nil
}

// CreateProperty creates a new property in a specific space.
func (s *service) CreateProperty(ctx context.Context, spaceId string, request CreatePropertyRequest) (Property, error) {
	details := &types.Struct{
		Fields: map[string]*types.Value{
			bundle.RelationKeyName.String():           pbtypes.String(request.Name),
			bundle.RelationKeyRelationFormat.String(): pbtypes.Int64(int64(s.MapPropertyFormat(request.Format))),
		},
	}

	resp := s.mw.ObjectCreateRelation(ctx, &pb.RpcObjectCreateRelationRequest{
		SpaceId: spaceId,
		Details: details,
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcObjectCreateRelationResponseError_NULL {
		return Property{}, ErrFailedCreateProperty
	}

	return s.GetProperty(ctx, spaceId, resp.ObjectId)
}

// UpdateProperty updates an existing property in a specific space.
func (s *service) UpdateProperty(ctx context.Context, spaceId string, propertyId string, request UpdatePropertyRequest) (Property, error) {
	_, err := s.GetProperty(ctx, spaceId, propertyId)
	if err != nil {
		return Property{}, err
	}

	detail := model.Detail{
		Key:   bundle.RelationKeyName.String(),
		Value: pbtypes.String(s.sanitizedString(request.Name)),
	}

	resp := s.mw.ObjectSetDetails(ctx, &pb.RpcObjectSetDetailsRequest{
		ContextId: propertyId,
		Details:   []*model.Detail{&detail},
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcObjectSetDetailsResponseError_NULL {
		return Property{}, ErrFailedUpdateProperty
	}

	return s.GetProperty(ctx, spaceId, propertyId)
}

func (s *service) sanitizedString(str string) string {
	return strings.TrimSpace(str)
}

// DeleteProperty deletes a property in a specific space.
func (s *service) DeleteProperty(ctx context.Context, spaceId string, propertyId string) (Property, error) {
	property, err := s.GetProperty(ctx, spaceId, propertyId)
	if err != nil {
		return Property{}, err
	}

	resp := s.mw.ObjectSetIsArchived(ctx, &pb.RpcObjectSetIsArchivedRequest{
		ContextId: propertyId,
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcObjectSetIsArchivedResponseError_NULL {
		return Property{}, ErrFailedDeleteProperty
	}

	return property, nil
}

// sanitizeAndValidatePropertyValue checks the value for a property according to its format and ensures referenced IDs exist and are valid.
func (s *service) sanitizeAndValidatePropertyValue(ctx context.Context, spaceId string, key string, format PropertyFormat, value interface{}, property Property) (interface{}, error) {
	switch format {
	case PropertyFormatText, PropertyFormatUrl, PropertyFormatEmail, PropertyFormatPhone:
		str, ok := value.(string)
		if !ok {
			return nil, util.ErrBadInput("property '" + key + "' must be a string")
		}
		return s.sanitizedString(str), nil
	case PropertyFormatNumber:
		num, ok := value.(float64)
		if !ok {
			return nil, util.ErrBadInput("property '" + key + "' must be a number")
		}
		return num, nil
	case PropertyFormatSelect:
		id, ok := value.(string)
		id = s.sanitizedString(id)
		if !ok {
			return nil, util.ErrBadInput("property '" + key + "' must be a string (option id)")
		}
		if !s.isValidSelectOption(ctx, spaceId, property, id) {
			return nil, util.ErrBadInput("invalid select option for '" + key + "': " + id)
		}
		return id, nil
	case PropertyFormatMultiSelect:
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
			if !s.isValidSelectOption(ctx, spaceId, property, id) {
				return nil, util.ErrBadInput("invalid multi_select option for '" + key + "': " + id)
			}
			validIds = append(validIds, id)
		}
		return validIds, nil
	case PropertyFormatDate:
		dateStr, ok := value.(string)
		if !ok {
			return nil, util.ErrBadInput("property '" + key + "' must be a string (date in RFC3339 format)")
		}
		dateStr = s.sanitizedString(dateStr)
		t, err := time.Parse(time.RFC3339, dateStr)
		if err != nil {
			return nil, util.ErrBadInput("invalid date format for '" + key + "': " + dateStr)
		}
		return t.Unix(), nil
	case PropertyFormatCheckbox:
		b, ok := value.(bool)
		if !ok {
			return nil, util.ErrBadInput("property '" + key + "' must be a boolean")
		}
		return b, nil
	case PropertyFormatObject, PropertyFormatFile:
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
			if !s.isValidObjectReference(ctx, spaceId, id) {
				return nil, util.ErrBadInput("invalid " + string(format) + " id for '" + key + "': " + id)
			}
			validIds = append(validIds, id)
		}
		return validIds, nil
	default:
		return nil, util.ErrBadInput("unsupported property format: " + string(format))
	}
}

// isValidSelectOption checks if the option id is valid for the given property.
func (s *service) isValidSelectOption(ctx context.Context, spaceId string, property Property, optionId string) bool {
	// TODO: refine logic
	tags, _, _, err := s.ListTags(ctx, spaceId, property.Key, 0, 1000) // TODO: revert to prop.ID
	if err != nil {
		return false
	}
	for _, tag := range tags {
		if tag.Id == optionId {
			return true
		}
	}
	return false
}

func (s *service) isValidObjectReference(ctx context.Context, spaceId string, objectId string) bool {
	// TODO: implement proper validation
	return true
}

// getRecommendedPropertiesFromLists combines featured and regular properties into a list of Properties.
func (s *service) getRecommendedPropertiesFromLists(featured, regular *types.ListValue, propertyMap map[string]Property) []Property {
	var props []Property
	lists := []*types.ListValue{featured, regular}
	for _, lst := range lists {
		if lst == nil {
			continue
		}
		for _, v := range lst.Values {
			key := v.GetStringValue()
			if key == "" {
				continue
			}
			if p, ok := propertyMap[key]; ok {
				props = append(props, p)
			}
		}
	}
	return props
}

// MapPropertyFormat maps the property format to relation format.
func (s *service) MapPropertyFormat(format PropertyFormat) model.RelationFormat {
	switch format {
	case PropertyFormatText:
		return model.RelationFormat_longtext
	case PropertyFormatNumber:
		return model.RelationFormat_number
	case PropertyFormatSelect:
		return model.RelationFormat_status
	case PropertyFormatMultiSelect:
		return model.RelationFormat_tag
	case PropertyFormatDate:
		return model.RelationFormat_date
	case PropertyFormatFile:
		return model.RelationFormat_file
	case PropertyFormatCheckbox:
		return model.RelationFormat_checkbox
	case PropertyFormatUrl:
		return model.RelationFormat_url
	case PropertyFormatEmail:
		return model.RelationFormat_email
	case PropertyFormatPhone:
		return model.RelationFormat_phone
	case PropertyFormatObject:
		return model.RelationFormat_object
	default:
		return model.RelationFormat_longtext
	}
}

// MapRelationFormat maps the relation format to API format.
func (s *service) MapRelationFormat(format model.RelationFormat) PropertyFormat {
	switch format {
	case model.RelationFormat_longtext:
		return PropertyFormatText
	case model.RelationFormat_shorttext:
		return PropertyFormatText
	case model.RelationFormat_tag:
		return PropertyFormatMultiSelect
	case model.RelationFormat_status:
		return PropertyFormatSelect
	default:
		return PropertyFormat(strcase.ToSnake(model.RelationFormat_name[int32(format)]))
	}
}

// GetPropertyMapsFromStore retrieves all properties for all spaces.
func (s *service) GetPropertyMapsFromStore(spaceIds []string) (map[string]map[string]Property, error) {
	spacesToProperties := make(map[string]map[string]Property, len(spaceIds))

	for _, spaceId := range spaceIds {
		propertyMap, err := s.GetPropertyMapFromStore(spaceId)
		if err != nil {
			return nil, err
		}
		spacesToProperties[spaceId] = propertyMap
	}

	return spacesToProperties, nil
}

// GetPropertyMapFromStore retrieves all properties for a specific space
// Property entries are also keyed by property id. Required for filling types with properties, as recommended properties are referenced by id and not key.
func (s *service) GetPropertyMapFromStore(spaceId string) (map[string]Property, error) {
	resp := s.mw.ObjectSearch(context.Background(), &pb.RpcObjectSearchRequest{
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
			bundle.RelationKeyUniqueKey.String(),
			bundle.RelationKeyName.String(),
			bundle.RelationKeyRelationFormat.String(),
		},
	})

	if resp.Error != nil && resp.Error.Code != pb.RpcObjectSearchResponseError_NULL {
		return nil, ErrFailedRetrievePropertyMap
	}

	propertyMap := make(map[string]Property, len(resp.Records))
	for _, record := range resp.Records {
		rk, p := s.mapPropertyFromRecord(record)
		propertyMap[rk] = p
		propertyMap[p.Id] = p // add property under id as key to map as well
	}

	return propertyMap, nil
}

// mapPropertyFromRecord maps a single property record into a Property and returns its trimmed relation key.
func (s *service) mapPropertyFromRecord(record *types.Struct) (string, Property) {
	rk := strings.TrimPrefix(record.Fields[bundle.RelationKeyUniqueKey.String()].GetStringValue(), "rel-")

	var key, name string
	switch rk {
	case bundle.RelationKeyCreator.String():
		key = ToPropertyApiKey("created_by")
		name = "Created By"
	case bundle.RelationKeyCreatedDate.String():
		key = ToPropertyApiKey("created_date")
		name = "Created Date"
	default:
		key = ToPropertyApiKey(rk)
		name = record.Fields[bundle.RelationKeyName.String()].GetStringValue()
	}

	return rk, Property{
		Id:     record.Fields[bundle.RelationKeyId.String()].GetStringValue(),
		Key:    key,
		Name:   name,
		Format: s.MapRelationFormat(model.RelationFormat(record.Fields[bundle.RelationKeyRelationFormat.String()].GetNumberValue())),
	}
}

// getPropertiesFromStruct retrieves the properties from the details.
func (s *service) getPropertiesFromStruct(details *types.Struct, propertyMap map[string]Property) []Property {
	properties := make([]Property, 0)
	for rk, value := range details.GetFields() {
		if _, isExcluded := excludedSystemProperties[rk]; isExcluded {
			continue
		}

		key := propertyMap[rk].Key
		format := propertyMap[rk].Format
		convertedVal := s.convertPropertyValue(key, value, format, details)

		if s.isMissingObject(convertedVal) {
			continue
		}

		id := propertyMap[rk].Id
		name := propertyMap[rk].Name
		properties = append(properties, s.buildProperty(id, key, name, format, convertedVal))
	}

	return properties
}

// convertPropertyValue converts a protobuf types.Value into a native Go value.
func (s *service) convertPropertyValue(key string, value *types.Value, format PropertyFormat, details *types.Struct) interface{} {
	switch kind := value.Kind.(type) {
	case *types.Value_NullValue:
		return nil
	case *types.Value_NumberValue:
		if format == PropertyFormatDate {
			return time.Unix(int64(kind.NumberValue), 0).UTC().Format(time.RFC3339)
		}
		return kind.NumberValue
	case *types.Value_StringValue:
		// TODO: investigate how this is possible? select option not list and not returned in further details
		if format == PropertyFormatSelect {
			tags := s.getTagsFromStore(details.Fields[bundle.RelationKeySpaceId.String()].GetStringValue(), []string{kind.StringValue})
			if len(tags) > 0 {
				return tags[0]
			}
			return nil
		}
		if format == PropertyFormatMultiSelect {
			return s.getTagsFromStore(details.Fields[bundle.RelationKeySpaceId.String()].GetStringValue(), []string{kind.StringValue})
		}
		return kind.StringValue
	case *types.Value_BoolValue:
		return kind.BoolValue
	case *types.Value_StructValue:
		m := make(map[string]interface{})
		for k, v := range kind.StructValue.Fields {
			m[k] = s.convertPropertyValue(key, v, format, details)
		}
		return m
	case *types.Value_ListValue:
		if format == PropertyFormatSelect {
			listValues := kind.ListValue.Values
			if len(listValues) > 0 {
				tags := s.getTagsFromStore(details.Fields[bundle.RelationKeySpaceId.String()].GetStringValue(), []string{listValues[0].GetStringValue()})
				if len(tags) > 0 {
					return tags[0]
				}
			}
			return nil
		}
		if format == PropertyFormatMultiSelect {
			listValues := kind.ListValue.Values
			if len(listValues) > 0 {
				listStringValues := make([]string, len(listValues))
				for i, v := range listValues {
					listStringValues[i] = v.GetStringValue()
				}
				return s.getTagsFromStore(details.Fields[bundle.RelationKeySpaceId.String()].GetStringValue(), listStringValues)
			}
			return nil
		}
		var list []interface{}
		for _, v := range kind.ListValue.Values {
			list = append(list, s.convertPropertyValue(key, v, format, details))
		}
		return list
	default:
		return nil
	}
}

// buildProperty creates a Property based on the format and converted value.
func (s *service) buildProperty(id string, key string, name string, format PropertyFormat, val interface{}) Property {
	p := &Property{
		Id:     id,
		Key:    key,
		Name:   name,
		Format: format,
	}

	switch format {
	case PropertyFormatText:
		if str, ok := val.(string); ok {
			p.Text = &str
		}
	case PropertyFormatNumber:
		if num, ok := val.(float64); ok {
			p.Number = &num
		}
	case PropertyFormatSelect:
		if sel, ok := val.(Tag); ok {
			p.Select = &sel
		}
	case PropertyFormatMultiSelect:
		if ms, ok := val.([]Tag); ok {
			p.MultiSelect = ms
		}
	case PropertyFormatDate:
		if dateStr, ok := val.(string); ok {
			p.Date = &dateStr
		}
	case PropertyFormatFile:
		if fileList, ok := val.([]interface{}); ok {
			var files []string
			for _, v := range fileList {
				if str, ok := v.(string); ok {
					files = append(files, str)
				}
			}
			p.File = files
		}
	case PropertyFormatCheckbox:
		if cb, ok := val.(bool); ok {
			p.Checkbox = &cb
		}
	case PropertyFormatUrl:
		if urlStr, ok := val.(string); ok {
			p.Url = &urlStr
		}
	case PropertyFormatEmail:
		if email, ok := val.(string); ok {
			p.Email = &email
		}
	case PropertyFormatPhone:
		if phone, ok := val.(string); ok {
			p.Phone = &phone
		}
	case PropertyFormatObject:
		if obj, ok := val.(string); ok {
			p.Object = []string{obj}
		} else if objSlice, ok := val.([]interface{}); ok {
			var objects []string
			for _, v := range objSlice {
				if str, ok := v.(string); ok {
					objects = append(objects, str)
				}
			}
			p.Object = objects
		}
	default:
		if str, ok := val.(string); ok {
			p.Text = &str
		}
	}

	return *p
}
