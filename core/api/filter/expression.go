package filter

import (
	"context"
	"fmt"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

// BuildExpressionFilters converts REST API FilterExpression to internal BlockContentDataviewFilter
func BuildExpressionFilters(ctx context.Context, expr *apimodel.FilterExpression, validator *Validator, spaceId string) (*model.BlockContentDataviewFilter, error) {
	if expr == nil {
		return nil, nil
	}

	// Create nested filters array for conditions and child filters
	var nestedFilters []*model.BlockContentDataviewFilter

	// Process conditions at this level
	for _, cond := range expr.Conditions {
		filter, err := buildConditionFilter(cond, validator, spaceId)
		if err != nil {
			return nil, fmt.Errorf("failed to build condition filter: %w", err)
		}
		if filter != nil {
			nestedFilters = append(nestedFilters, filter)
		}
	}

	// Process child filter expressions recursively
	for _, childExpr := range expr.Filters {
		childFilter, err := BuildExpressionFilters(ctx, &childExpr, validator, spaceId)
		if err != nil {
			return nil, fmt.Errorf("failed to build nested filter: %w", err)
		}
		if childFilter != nil {
			nestedFilters = append(nestedFilters, childFilter)
		}
	}

	// If no filters were created, return nil
	if len(nestedFilters) == 0 {
		return nil, nil
	}

	// If only one filter and no operator specified, return it directly
	if len(nestedFilters) == 1 && expr.Operator == "" {
		return nestedFilters[0], nil
	}

	// Map operator (default to AND if not specified)
	operator := model.BlockContentDataviewFilter_And
	if expr.Operator != "" {
		var ok bool
		operator, ok = OperatorMap[expr.Operator]
		if !ok {
			return nil, fmt.Errorf("unsupported filter operator: %s", expr.Operator)
		}
	}

	filters := &model.BlockContentDataviewFilter{
		Operator:      operator,
		NestedFilters: nestedFilters,
	}

	return filters, nil
}

// buildConditionFilter builds a single condition filter
func buildConditionFilter(cond apimodel.FilterItem, validator *Validator, spaceId string) (*model.BlockContentDataviewFilter, error) {
	wrapped := cond.WrappedFilterItem
	if wrapped == nil {
		return nil, fmt.Errorf("invalid filter condition: no wrapped filter item")
	}

	dbCondition, ok := ToInternalCondition(wrapped.GetCondition())
	if !ok {
		return nil, fmt.Errorf("unsupported filter condition: %s", wrapped.GetCondition())
	}

	propertyMap := validator.apiService.GetCachedProperties(spaceId)
	property, err := validator.resolveProperty(spaceId, wrapped.GetPropertyKey(), propertyMap)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve property %s: %w", wrapped.GetPropertyKey(), err)
	}

	if !isValidConditionForType(property.Format, dbCondition) {
		return nil, fmt.Errorf("condition %v is not valid for property type %q", dbCondition, property.Format)
	}

	rk := property.RelationKey
	if dbCondition == model.BlockContentDataviewFilter_Empty || dbCondition == model.BlockContentDataviewFilter_NotEmpty {
		return &model.BlockContentDataviewFilter{
			RelationKey: rk,
			Condition:   dbCondition,
		}, nil
	}

	var value interface{}
	switch fc := wrapped.(type) {
	case apimodel.TextFilterItem:
		if fc.Text != nil {
			value = *fc.Text
		}
	case apimodel.NumberFilterItem:
		if fc.Number != nil {
			value = *fc.Number
		}
	case apimodel.SelectFilterItem:
		if fc.Select != nil {
			value = *fc.Select
		}
	case apimodel.MultiSelectFilterItem:
		if fc.MultiSelect != nil {
			value = *fc.MultiSelect
		}
	case apimodel.DateFilterItem:
		if fc.Date != nil {
			value = *fc.Date
		}
	case apimodel.CheckboxFilterItem:
		if fc.Checkbox != nil {
			value = *fc.Checkbox
		}
	case apimodel.FilesFilterItem:
		if fc.Files != nil {
			value = *fc.Files
		}
	case apimodel.UrlFilterItem:
		if fc.Url != nil {
			value = *fc.Url
		}
	case apimodel.EmailFilterItem:
		if fc.Email != nil {
			value = *fc.Email
		}
	case apimodel.PhoneFilterItem:
		if fc.Phone != nil {
			value = *fc.Phone
		}
	case apimodel.ObjectsFilterItem:
		if fc.Objects != nil {
			value = *fc.Objects
		}
	}

	validatedValue, err := validator.apiService.SanitizeAndValidatePropertyValue(spaceId, wrapped.GetPropertyKey(), value, property, propertyMap)
	if err != nil {
		return nil, fmt.Errorf("invalid value for property %s: %w", wrapped.GetPropertyKey(), err)
	}

	filter := &model.BlockContentDataviewFilter{
		RelationKey: rk,
		Condition:   dbCondition,
		Value:       pbtypes.ToValue(validatedValue),
	}

	return filter, nil
}
