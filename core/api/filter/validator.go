package filter

import (
	"fmt"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/api/util"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type ApiService interface {
	GetCachedProperties(spaceId string) map[string]*apimodel.Property
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
	if !isValidConditionForType(property.Format, filter.Condition) {
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

// convertAndValidateValue converts and validates the filter value based on property type
func (v *Validator) convertAndValidateValue(spaceId string, filter *Filter, property *apimodel.Property, propertyMap map[string]*apimodel.Property) (interface{}, error) {
	switch filter.Condition {
	case model.BlockContentDataviewFilter_Empty, model.BlockContentDataviewFilter_NotEmpty:
		if boolVal, ok := filter.Value.(bool); ok {
			return boolVal, nil
		}
		return true, nil
	}

	value := filter.Value
	if filter.Condition == model.BlockContentDataviewFilter_In || filter.Condition == model.BlockContentDataviewFilter_NotIn {
		// Ensure value is an array for In/NotIn conditions
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
