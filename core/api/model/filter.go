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
	FilterConditionEq  FilterCondition = "eq"  // Equal
	FilterConditionNe  FilterCondition = "ne"  // Not Equal
	FilterConditionGt  FilterCondition = "gt"  // Greater Than
	FilterConditionGte FilterCondition = "gte" // Greater or Equal
	FilterConditionLt  FilterCondition = "lt"  // Less Than
	FilterConditionLte FilterCondition = "lte" // Less or Equal

	// Text operations
	FilterConditionContains  FilterCondition = "contains"  // Like/Contains substring
	FilterConditionNContains FilterCondition = "ncontains" // Not Contains

	// Array operations
	FilterConditionIn         FilterCondition = "in"       // In array
	FilterConditionNin        FilterCondition = "nin"      // Not in array
	FilterConditionAll        FilterCondition = "all"      // Contains all (AllIn)
	FilterConditionNone       FilterCondition = "none"     // Contains none (NotAllIn)
	FilterConditionExactIn    FilterCondition = "exactin"  // Exact in array (ExactIn)
	FilterConditionNotExactIn FilterCondition = "nexactin" // Not exact in array (NotExactIn)

	// Existence checks
	FilterConditionExists FilterCondition = "exists" // Field exists
	FilterConditionEmpty  FilterCondition = "empty"  // Field is empty
	FilterConditionNEmpty FilterCondition = "nempty" // Field not empty
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
