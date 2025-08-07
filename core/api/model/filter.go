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
	FilterConditionIn  FilterCondition = "in"  // Value is in the specified array
	FilterConditionNin FilterCondition = "nin" // Value is not in the specified array
	FilterConditionAll FilterCondition = "all" // Contains all specified values

	// Emptiness checks
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
		FilterConditionIn, FilterConditionNin, FilterConditionAll,
		FilterConditionEmpty, FilterConditionNEmpty:
		*fc = FilterCondition(s)
		return nil
	default:
		return util.ErrBadInput(fmt.Sprintf("invalid filter condition: %q", s))
	}
}

// FilterExpression represents a filter expression that can be nested with AND/OR operators
type FilterExpression struct {
	Operator   FilterOperator     `json:"operator,omitempty" enums:"and,or"`                                                                                                                                                                                                     // Logical operator for combining filters (and, or)
	Conditions []FilterItem       `json:"conditions,omitempty" oneOf:"TextFilterItem,NumberFilterItem,SelectFilterItem,MultiSelectFilterItem,DateFilterItem,CheckboxFilterItem,FilesFilterItem,UrlFilterItem,EmailFilterItem,PhoneFilterItem,ObjectsFilterItem,EmptyFilterItem"` // List of format-specific filter conditions
	Filters    []FilterExpression `json:"filters,omitempty"`                                                                                                                                                                                                                     // Nested filter expressions for complex logic
}

// FilterItem is a wrapper for format-specific filter conditions
type FilterItem struct {
	WrappedFilterItem `swaggerignore:"true"`
}

func (f FilterItem) MarshalJSON() ([]byte, error) {
	return json.Marshal(f.WrappedFilterItem)
}

func (f *FilterItem) UnmarshalJSON(data []byte) error {
	// Check for empty/nempty conditions without value
	var checkCondition struct {
		Condition FilterCondition `json:"condition"`
	}
	if err := json.Unmarshal(data, &checkCondition); err != nil {
		return err
	}

	// Check for empty/nempty conditions without value
	if checkCondition.Condition == FilterConditionEmpty || checkCondition.Condition == FilterConditionNEmpty {
		// Check which format-specific field is present to determine the type
		var aux map[string]json.RawMessage
		if err := json.Unmarshal(data, &aux); err != nil {
			return err
		}

		// If no format-specific field, use EmptyFilterItem
		if aux[PropertyFormatText.String()] == nil && aux[PropertyFormatNumber.String()] == nil && aux[PropertyFormatSelect.String()] == nil &&
			aux[PropertyFormatMultiSelect.String()] == nil && aux[PropertyFormatDate.String()] == nil && aux[PropertyFormatCheckbox.String()] == nil &&
			aux[PropertyFormatFiles.String()] == nil && aux[PropertyFormatUrl.String()] == nil && aux[PropertyFormatEmail.String()] == nil &&
			aux[PropertyFormatPhone.String()] == nil && aux[PropertyFormatObjects.String()] == nil {
			var v EmptyFilterItem
			if err := json.Unmarshal(data, &v); err != nil {
				return err
			}
			f.WrappedFilterItem = v
			return nil
		}
	}

	// Check which format-specific field is present
	var aux map[string]json.RawMessage
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	switch {
	case aux[PropertyFormatText.String()] != nil:
		var v TextFilterItem
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		f.WrappedFilterItem = v
	case aux[PropertyFormatNumber.String()] != nil:
		var v NumberFilterItem
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		f.WrappedFilterItem = v
	case aux[PropertyFormatSelect.String()] != nil:
		var v SelectFilterItem
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		f.WrappedFilterItem = v
	case aux[PropertyFormatMultiSelect.String()] != nil:
		var v MultiSelectFilterItem
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		f.WrappedFilterItem = v
	case aux[PropertyFormatDate.String()] != nil:
		var v DateFilterItem
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		f.WrappedFilterItem = v
	case aux[PropertyFormatCheckbox.String()] != nil:
		var v CheckboxFilterItem
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		f.WrappedFilterItem = v
	case aux[PropertyFormatFiles.String()] != nil:
		var v FilesFilterItem
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		f.WrappedFilterItem = v
	case aux[PropertyFormatUrl.String()] != nil:
		var v UrlFilterItem
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		f.WrappedFilterItem = v
	case aux[PropertyFormatEmail.String()] != nil:
		var v EmailFilterItem
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		f.WrappedFilterItem = v
	case aux[PropertyFormatPhone.String()] != nil:
		var v PhoneFilterItem
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		f.WrappedFilterItem = v
	case aux[PropertyFormatObjects.String()] != nil:
		var v ObjectsFilterItem
		if err := json.Unmarshal(data, &v); err != nil {
			return err
		}
		f.WrappedFilterItem = v
	default:
		return util.ErrBadInput("could not determine filter condition type")
	}

	return nil
}

// WrappedFilterItem is the interface for all format-specific filter items
type WrappedFilterItem interface {
	isFilterItem()
	GetPropertyKey() string
	GetCondition() FilterCondition
}

// TextFilterItem for text property filters
type TextFilterItem struct {
	PropertyKey string          `json:"property_key" example:"description"`                                         // The property key to filter on
	Condition   FilterCondition `json:"condition" example:"contains" enums:"eq,ne,contains,ncontains,empty,nempty"` // The filter condition
	Text        *string         `json:"text,omitempty" example:"important"`                                         // The text value to filter by
}

func (TextFilterItem) isFilterItem()                   {}
func (f TextFilterItem) GetPropertyKey() string        { return f.PropertyKey }
func (f TextFilterItem) GetCondition() FilterCondition { return f.Condition }

// NumberFilterItem for number property filters
type NumberFilterItem struct {
	PropertyKey string          `json:"property_key" example:"priority"`                                 // The property key to filter on
	Condition   FilterCondition `json:"condition" example:"gt" enums:"eq,ne,gt,gte,lt,lte,empty,nempty"` // The filter condition
	Number      *float64        `json:"number,omitempty" example:"5"`                                    // The number value to filter by
}

func (NumberFilterItem) isFilterItem()                   {}
func (f NumberFilterItem) GetPropertyKey() string        { return f.PropertyKey }
func (f NumberFilterItem) GetCondition() FilterCondition { return f.Condition }

// SelectFilterItem for select property filters
type SelectFilterItem struct {
	PropertyKey string          `json:"property_key" example:"status"`                          // The property key to filter on
	Condition   FilterCondition `json:"condition" example:"in" enums:"in,all,nin,empty,nempty"` // The filter condition
	Select      *string         `json:"select,omitempty" example:"tag_id_done"`                 // Tag Id - for eq/ne conditions (single selection)
}

func (SelectFilterItem) isFilterItem()                   {}
func (f SelectFilterItem) GetPropertyKey() string        { return f.PropertyKey }
func (f SelectFilterItem) GetCondition() FilterCondition { return f.Condition }

// MultiSelectFilterItem for multi-select property filters
type MultiSelectFilterItem struct {
	PropertyKey string          `json:"property_key" example:"tags"`                             // The property key to filter on
	Condition   FilterCondition `json:"condition" example:"all" enums:"in,all,nin,empty,nempty"` // The filter condition
	MultiSelect *[]string       `json:"multi_select,omitempty"`                                  // The tag IDs to filter by
}

func (MultiSelectFilterItem) isFilterItem()                   {}
func (f MultiSelectFilterItem) GetPropertyKey() string        { return f.PropertyKey }
func (f MultiSelectFilterItem) GetCondition() FilterCondition { return f.Condition }

// DateFilterItem for date property filters
type DateFilterItem struct {
	PropertyKey string          `json:"property_key" example:"due_date"`                                  // The property key to filter on
	Condition   FilterCondition `json:"condition" example:"lte" enums:"eq,gt,gte,lt,lte,in,empty,nempty"` // The filter condition
	Date        *string         `json:"date,omitempty" example:"2024-12-31T23:59:59Z"`                    // The date value to filter by. Accepts dates in RFC3339 format (2006-01-02T15:04:05Z) or date-only format (2006-01-02)
}

func (DateFilterItem) isFilterItem()                   {}
func (f DateFilterItem) GetPropertyKey() string        { return f.PropertyKey }
func (f DateFilterItem) GetCondition() FilterCondition { return f.Condition }

// CheckboxFilterItem for checkbox property filters
type CheckboxFilterItem struct {
	PropertyKey string          `json:"property_key" example:"done"`          // The property key to filter on
	Condition   FilterCondition `json:"condition" example:"eq" enums:"eq,ne"` // The filter condition
	Checkbox    *bool           `json:"checkbox" example:"true"`              // The checkbox value to filter by
}

func (CheckboxFilterItem) isFilterItem()                   {}
func (f CheckboxFilterItem) GetPropertyKey() string        { return f.PropertyKey }
func (f CheckboxFilterItem) GetCondition() FilterCondition { return f.Condition }

// FilesFilterItem for files property filters
type FilesFilterItem struct {
	PropertyKey string          `json:"property_key" example:"attachments"`                     // The property key to filter on
	Condition   FilterCondition `json:"condition" example:"in" enums:"in,all,nin,empty,nempty"` // The filter condition
	Files       *[]string       `json:"files,omitempty"`                                        // File IDs for contains condition
}

func (FilesFilterItem) isFilterItem()                   {}
func (f FilesFilterItem) GetPropertyKey() string        { return f.PropertyKey }
func (f FilesFilterItem) GetCondition() FilterCondition { return f.Condition }

// UrlFilterItem for URL property filters
type UrlFilterItem struct {
	PropertyKey string          `json:"property_key" example:"source"`                                              // The property key to filter on
	Condition   FilterCondition `json:"condition" example:"contains" enums:"eq,ne,contains,ncontains,empty,nempty"` // The filter condition
	Url         *string         `json:"url,omitempty" example:"https://example.com"`                                // The Url value to filter by
}

func (UrlFilterItem) isFilterItem()                   {}
func (f UrlFilterItem) GetPropertyKey() string        { return f.PropertyKey }
func (f UrlFilterItem) GetCondition() FilterCondition { return f.Condition }

// EmailFilterItem for email property filters
type EmailFilterItem struct {
	PropertyKey string          `json:"property_key" example:"contact_email"`                                 // The property key to filter on
	Condition   FilterCondition `json:"condition" example:"eq" enums:"eq,ne,contains,ncontains,empty,nempty"` // The filter condition
	Email       *string         `json:"email,omitempty" example:"user@example.com"`                           // The email value to filter by
}

func (EmailFilterItem) isFilterItem()                   {}
func (f EmailFilterItem) GetPropertyKey() string        { return f.PropertyKey }
func (f EmailFilterItem) GetCondition() FilterCondition { return f.Condition }

// PhoneFilterItem for phone property filters
type PhoneFilterItem struct {
	PropertyKey string          `json:"property_key" example:"contact_phone"`                                 // The property key to filter on
	Condition   FilterCondition `json:"condition" example:"eq" enums:"eq,ne,contains,ncontains,empty,nempty"` // The filter condition
	Phone       *string         `json:"phone,omitempty" example:"+1234567890"`                                // The phone value to filter by
}

func (PhoneFilterItem) isFilterItem()                   {}
func (f PhoneFilterItem) GetPropertyKey() string        { return f.PropertyKey }
func (f PhoneFilterItem) GetCondition() FilterCondition { return f.Condition }

// ObjectsFilterItem for objects/relation property filters
type ObjectsFilterItem struct {
	PropertyKey string          `json:"property_key" example:"assignee"`                        // The property key to filter on
	Condition   FilterCondition `json:"condition" example:"in" enums:"in,all,nin,empty,nempty"` // The filter condition
	Objects     *[]string       `json:"objects,omitempty"`                                      // Object Ids to filter by
}

func (ObjectsFilterItem) isFilterItem()                   {}
func (f ObjectsFilterItem) GetPropertyKey() string        { return f.PropertyKey }
func (f ObjectsFilterItem) GetCondition() FilterCondition { return f.Condition }

// EmptyFilterItem for checking if property is empty/not empty (without specifying format)
type EmptyFilterItem struct {
	PropertyKey string          `json:"property_key" example:"description"`             // The property key to filter on
	Condition   FilterCondition `json:"condition" example:"empty" enums:"empty,nempty"` // The filter condition
}

func (EmptyFilterItem) isFilterItem()                   {}
func (f EmptyFilterItem) GetPropertyKey() string        { return f.PropertyKey }
func (f EmptyFilterItem) GetCondition() FilterCondition { return f.Condition }
