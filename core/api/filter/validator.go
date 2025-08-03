package filter

import (
	"fmt"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

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
	if !isValidConditionForType(property.Format, filter.Condition) {
		return fmt.Errorf("condition %v is not valid for property type %q",
			filter.Condition, property.Format)
	}

	convertedValue, err := v.convertAndValidateValue(spaceId, property, filter.Condition, filter.Value, propertyMap)
	if err != nil {
		return fmt.Errorf("invalid value for property %q: %w", filter.PropertyKey, err)
	}

	filter.PropertyKey = property.RelationKey
	filter.Value = convertedValue
	return nil
}

// resolveProperty resolves a property by key, checking both system and custom properties
func (v *Validator) resolveProperty(spaceId string, propertyKey string, propertyMap map[string]*apimodel.Property) (*apimodel.Property, error) {
	rk := v.apiService.ResolvePropertyApiKey(propertyMap, propertyKey)
	if rk == "" {
		return nil, fmt.Errorf("property %q not found", propertyKey)
	}

	prop, exists := propertyMap[rk]
	if !exists {
		return nil, fmt.Errorf("property %q not found in cache", propertyKey)
	}

	return prop, nil
}

// convertAndValidateValue converts and validates the filter value based on property type
func (v *Validator) convertAndValidateValue(spaceId string, property *apimodel.Property, condition model.BlockContentDataviewFilterCondition, value interface{}, propertyMap map[string]*apimodel.Property) (interface{}, error) {
	switch condition {
	case model.BlockContentDataviewFilter_Empty, model.BlockContentDataviewFilter_NotEmpty, model.BlockContentDataviewFilter_Exists:
		if boolVal, ok := value.(bool); ok {
			return boolVal, nil
		}
		return true, nil
	}

	if condition == model.BlockContentDataviewFilter_In || condition == model.BlockContentDataviewFilter_NotIn {
		switch v := value.(type) {
		case []string:
		case []interface{}:
		case string:
			value = []interface{}{v}
		default:
			value = []interface{}{v}
		}
	}

	return v.apiService.SanitizeAndValidatePropertyValue(spaceId, property.Key, property.Format, value, property, propertyMap)
}
