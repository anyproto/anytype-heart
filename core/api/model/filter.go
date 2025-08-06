package apimodel

import (
	"encoding/json"
	"fmt"

	"github.com/anyproto/anytype-heart/core/api/util"
)

// FilterOperator represents the logical operator for combining filters
type FilterOperator string

const (
	FilterOperatorAnd FilterOperator = "and"
	FilterOperatorOr  FilterOperator = "or"
)

func (fo FilterOperator) String() string {
	return string(fo)
}

func (fo *FilterOperator) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	switch FilterOperator(s) {
	case FilterOperatorAnd, FilterOperatorOr:
		*fo = FilterOperator(s)
		return nil
	default:
		return util.ErrBadInput(fmt.Sprintf("invalid filter operator: %q", s))
	}
}

// FilterCondition represents the condition type for filtering
type FilterCondition string

const (
	// Basic comparisons
	FilterConditionEq  FilterCondition = "eq"  // Equal to value
	FilterConditionNe  FilterCondition = "ne"  // Not equal to value
	FilterConditionGt  FilterCondition = "gt"  // Greater than value
	FilterConditionGte FilterCondition = "gte" // Greater than or equal to value
	FilterConditionLt  FilterCondition = "lt"  // Less than value
	FilterConditionLte FilterCondition = "lte" // Less than or equal to value

	// Text operations
	FilterConditionContains  FilterCondition = "contains"  // Contains substring
	FilterConditionNContains FilterCondition = "ncontains" // Does not contain substring

	// Array operations
	FilterConditionIn         FilterCondition = "in"       // Value is in the specified array
	FilterConditionNin        FilterCondition = "nin"      // Value is not in the specified array
	FilterConditionAll        FilterCondition = "all"      // Contains all specified values
	FilterConditionNone       FilterCondition = "none"     // Contains none of the specified values
	FilterConditionExactIn    FilterCondition = "exactin"  // Array exactly matches specified values
	FilterConditionNotExactIn FilterCondition = "nexactin" // Array does not exactly match specified values

	// Existence checks
	FilterConditionExists FilterCondition = "exists" // Property exists on the object
	FilterConditionEmpty  FilterCondition = "empty"  // Property value is empty
	FilterConditionNEmpty FilterCondition = "nempty" // Property value is not empty
)

func (fc FilterCondition) String() string {
	return string(fc)
}

func (fc *FilterCondition) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	switch FilterCondition(s) {
	case FilterConditionEq, FilterConditionNe, FilterConditionGt, FilterConditionGte,
		FilterConditionLt, FilterConditionLte, FilterConditionContains, FilterConditionNContains,
		FilterConditionIn, FilterConditionNin, FilterConditionAll, FilterConditionNone,
		FilterConditionExactIn, FilterConditionNotExactIn,
		FilterConditionExists, FilterConditionEmpty, FilterConditionNEmpty:
		*fc = FilterCondition(s)
		return nil
	default:
		return util.ErrBadInput(fmt.Sprintf("invalid filter condition: %q", s))
	}
}

// FilterExpression represents a filter expression that can be nested with AND/OR operators
type FilterExpression struct {
	Operator   FilterOperator     `json:"operator,omitempty" enums:"and,or"` // Logical operator for combining filters (and, or)
	Conditions []FilterItem       `json:"conditions,omitempty"`              // List of filter conditions
	Filters    []FilterExpression `json:"filters,omitempty"`                 // Nested filter expressions for complex logic
}

// FilterItem represents a single filter condition on a property
type FilterItem struct {
	PropertyKey string          `json:"property_key" example:"status"`                                                                                              // The property key to filter on
	Condition   FilterCondition `json:"condition" example:"eq" enums:"eq,ne,gt,gte,lt,lte,contains,ncontains,in,nin,all,none,exactin,nexactin,exists,empty,nempty"` // The filter condition
	Value       interface{}     `json:"value,omitempty" example:"done"`                                                                                             // The value to compare against (omit for exists/empty conditions)
}
