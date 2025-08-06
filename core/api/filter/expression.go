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
		filter, err := buildConditionFilter(ctx, cond, validator, spaceId)
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

	// Create combined filter with operator
	return &model.BlockContentDataviewFilter{
		Operator:      operator,
		NestedFilters: nestedFilters,
	}, nil
}

// buildConditionFilter builds a single condition filter
func buildConditionFilter(ctx context.Context, cond apimodel.FilterItem, validator *Validator, spaceId string) (*model.BlockContentDataviewFilter, error) {
	// Map condition
	dbCondition, ok := ConditionMap[cond.Condition]
	if !ok {
		return nil, fmt.Errorf("unsupported filter condition: %s", cond.Condition)
	}

	// Get property map
	propertyMap := validator.apiService.GetCachedProperties(spaceId)

	// Resolve and validate property
	property, err := validator.resolveProperty(spaceId, cond.PropertyKey, propertyMap)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve property %s: %w", cond.PropertyKey, err)
	}

	// Check if condition is valid for property type
	if !isValidConditionForType(property.Format, dbCondition) {
		return nil, fmt.Errorf("condition %v is not valid for property type %q", dbCondition, property.Format)
	}

	// Use the resolved relation key
	relationKey := property.RelationKey

	// For conditions that don't require values (empty, exists, etc.)
	if dbCondition == model.BlockContentDataviewFilter_Empty ||
		dbCondition == model.BlockContentDataviewFilter_NotEmpty ||
		dbCondition == model.BlockContentDataviewFilter_Exists {
		return &model.BlockContentDataviewFilter{
			RelationKey: relationKey,
			Condition:   dbCondition,
		}, nil
	}

	// Validate value against property type
	validatedValue, err := validator.apiService.SanitizeAndValidatePropertyValue(spaceId, property.Key, property.Format, cond.Value, property, propertyMap)
	if err != nil {
		return nil, fmt.Errorf("invalid value for property %s: %w", cond.PropertyKey, err)
	}

	// Convert value to protobuf format
	protoValue := pbtypes.ToValue(validatedValue)

	return &model.BlockContentDataviewFilter{
		RelationKey: relationKey,
		Condition:   dbCondition,
		Value:       protoValue,
	}, nil
}
