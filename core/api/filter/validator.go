package filter

import (
	"fmt"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/api/util"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

// getTopLevelAttributeAsProperty returns a synthetic property for top-level attributes
func getTopLevelAttributeAsProperty(key string) *apimodel.Property {
	switch key {
	case bundle.RelationKeyName.String(), bundle.RelationKeyGlobalName.String(), bundle.RelationKeySnippet.String():
		return &apimodel.Property{
			Key:         key,
			RelationKey: key,
			Format:      apimodel.PropertyFormatText,
		}
	case bundle.RelationKeyType.String():
		return &apimodel.Property{
			Key:         key,
			RelationKey: key,
			Format:      apimodel.PropertyFormatObjects,
		}
	default:
		return nil
	}
}

type ApiService interface {
	GetCachedProperties(spaceId string) map[string]*apimodel.Property
	GetCachedTypes(spaceId string) map[string]*apimodel.Type
	ResolvePropertyApiKey(properties map[string]*apimodel.Property, key string) (string, bool)
	SanitizeAndValidatePropertyValue(spaceId string, key string, value interface{}, property *apimodel.Property, propertyMap map[string]*apimodel.Property) (interface{}, error)
}

// Validator validates filters against property definitions
type Validator struct {
	apiService ApiService
}

// NewValidator creates a new filter validator
func NewValidator(s ApiService) *Validator {
	return &Validator{apiService: s}
}

// ValidateFilters validates all filters in the parsed filters
func (v *Validator) ValidateFilters(spaceId string, filters *ParsedFilters) error {
	if filters == nil || len(filters.Filters) == 0 {
		return nil
	}

	propertyMap := v.apiService.GetCachedProperties(spaceId)

	for i, filter := range filters.Filters {
		if err := v.validateFilter(spaceId, &filter, propertyMap); err != nil {
			return fmt.Errorf("invalid filter at index %d: %w", i, err)
		}
		filters.Filters[i] = filter
	}

	return nil
}

// validateFilter validates a single filter
func (v *Validator) validateFilter(spaceId string, filter *Filter, propertyMap map[string]*apimodel.Property) error {
	property, err := v.resolveProperty(spaceId, filter.PropertyKey, propertyMap)
	if err != nil {
		return fmt.Errorf("failed to resolve property %q: %w", filter.PropertyKey, err)
	}

	// Check if condition is valid for property type
	// Type filter supports equality and array conditions
	isTypeFilter := property.RelationKey == bundle.RelationKeyType.String()
	if isTypeFilter {
		if !isValidConditionForType(property.Format, filter.Condition) &&
			filter.Condition != model.BlockContentDataviewFilter_Equal &&
			filter.Condition != model.BlockContentDataviewFilter_NotEqual {
			apiCondition, _ := ToApiCondition(filter.Condition)
			return util.ErrBadInput(fmt.Sprintf("condition %q is not valid for type filter", apiCondition))
		}
	} else if !isValidConditionForType(property.Format, filter.Condition) {
		apiCondition, _ := ToApiCondition(filter.Condition)
		return util.ErrBadInput(fmt.Sprintf("condition %q is not valid for property type %q", apiCondition, property.Format))
	}

	convertedValue, err := v.convertAndValidateValue(spaceId, filter, property, propertyMap)
	if err != nil {
		return fmt.Errorf("invalid value for property %q: %w", filter.PropertyKey, err)
	}

	filter.PropertyKey = property.RelationKey
	filter.Value = convertedValue
	return nil
}

// resolveProperty resolves a property by key and returns it or an error if not found
func (v *Validator) resolveProperty(spaceId string, propertyKey string, propertyMap map[string]*apimodel.Property) (*apimodel.Property, error) {
	// Check top-level attributes first
	if prop := getTopLevelAttributeAsProperty(propertyKey); prop != nil {
		return prop, nil
	}

	rk, found := v.apiService.ResolvePropertyApiKey(propertyMap, propertyKey)
	if !found {
		return nil, util.ErrBadInput(fmt.Sprintf("property %q not found", propertyKey))
	}

	prop, exists := propertyMap[rk]
	if !exists {
		return nil, util.ErrBadInput(fmt.Sprintf("property %q not found in cache", propertyKey))
	}

	return prop, nil
}

// resolveTypeValue resolves a type API key or ID to the type's object ID
func (v *Validator) resolveTypeValue(spaceId string, value string) (string, error) {
	typeMap := v.apiService.GetCachedTypes(spaceId)
	if t, exists := typeMap[value]; exists {
		return t.Id, nil
	}
	return "", util.ErrBadInput(fmt.Sprintf("type %q not found", value))
}

// buildTypeFilter builds a dataview filter for type filtering, resolving the value through the type cache
func (v *Validator) buildTypeFilter(spaceId string, relationKey string, condition model.BlockContentDataviewFilterCondition, value interface{}) (*model.BlockContentDataviewFilter, error) {
	switch val := value.(type) {
	case string:
		resolved, err := v.resolveTypeValue(spaceId, val)
		if err != nil {
			return nil, err
		}
		return &model.BlockContentDataviewFilter{
			RelationKey: relationKey,
			Condition:   condition,
			Value:       pbtypes.ToValue(resolved),
		}, nil
	case []string:
		resolved := make([]string, 0, len(val))
		for _, item := range val {
			id, err := v.resolveTypeValue(spaceId, item)
			if err != nil {
				return nil, err
			}
			resolved = append(resolved, id)
		}
		return &model.BlockContentDataviewFilter{
			RelationKey: relationKey,
			Condition:   condition,
			Value:       pbtypes.ToValue(resolved),
		}, nil
	default:
		return nil, util.ErrBadInput(fmt.Sprintf("invalid type filter value: expected string, got %T", value))
	}
}

// convertAndValidateValue converts and validates the filter value based on property type
func (v *Validator) convertAndValidateValue(spaceId string, filter *Filter, property *apimodel.Property, propertyMap map[string]*apimodel.Property) (interface{}, error) {
	switch filter.Condition {
	case model.BlockContentDataviewFilter_Empty, model.BlockContentDataviewFilter_NotEmpty:
		if boolVal, ok := filter.Value.(bool); ok {
			return boolVal, nil
		}
		return true, nil
	}

	// Special handling for type filter: resolve type API key to type object ID
	if property.RelationKey == bundle.RelationKeyType.String() {
		switch val := filter.Value.(type) {
		case string:
			return v.resolveTypeValue(spaceId, val)
		case []string:
			resolved := make([]string, 0, len(val))
			for _, item := range val {
				id, err := v.resolveTypeValue(spaceId, item)
				if err != nil {
					return nil, err
				}
				resolved = append(resolved, id)
			}
			return resolved, nil
		default:
			return nil, util.ErrBadInput(fmt.Sprintf("invalid type filter value: expected string, got %T", filter.Value))
		}
	}

	value := filter.Value
	if property.Format == apimodel.PropertyFormatSelect &&
		(filter.Condition == model.BlockContentDataviewFilter_In || filter.Condition == model.BlockContentDataviewFilter_NotIn) {
		var items []interface{}
		switch v := value.(type) {
		case []string:
			items = make([]interface{}, 0, len(v))
			for _, item := range v {
				items = append(items, item)
			}
		case []interface{}:
			items = v
		default:
			items = []interface{}{v}
		}

		values := make([]string, 0, len(items))
		for _, item := range items {
			itemStr, ok := item.(string)
			if !ok {
				return nil, util.ErrBadInput(fmt.Sprintf("invalid select filter value for property %q: expected string, got %T (%v)", filter.PropertyKey, item, item))
			}
			sanitized, err := v.apiService.SanitizeAndValidatePropertyValue(spaceId, filter.PropertyKey, itemStr, property, propertyMap)
			if err != nil {
				return nil, err
			}
			tagId, ok := sanitized.(string)
			if !ok {
				return nil, util.ErrBadInput(fmt.Sprintf("invalid select option for property %q: could not resolve %v to a tag id", filter.PropertyKey, itemStr))
			}
			values = append(values, tagId)
		}
		return values, nil
	}
	if filter.Condition == model.BlockContentDataviewFilter_In || filter.Condition == model.BlockContentDataviewFilter_NotIn {
		switch v := value.(type) {
		case []string, []interface{}:
			// Already an array, keep as-is
		default:
			// Wrap single value in array
			value = []interface{}{v}
		}
	}

	return v.apiService.SanitizeAndValidatePropertyValue(spaceId, filter.PropertyKey, value, property, propertyMap)
}
