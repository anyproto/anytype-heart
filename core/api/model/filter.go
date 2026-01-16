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
	GetValue() interface{}
}

// TextFilterItem for text property filters
type TextFilterItem struct {
	PropertyKey string          `json:"property_key" example:"description"`                                         // The property key to filter on
	Condition   FilterCondition `json:"condition" example:"contains" enums:"eq,ne,contains,ncontains,empty,nempty"` // The filter condition
	Text        *string         `json:"text" example:"Some text..."`                                                // The text value to filter by
}

func (TextFilterItem) isFilterItem()                   {}
func (f TextFilterItem) GetPropertyKey() string        { return f.PropertyKey }
func (f TextFilterItem) GetCondition() FilterCondition { return f.Condition }
func (f TextFilterItem) GetValue() interface{} {
	if f.Text != nil {
		return *f.Text
	}
	return nil
}

// NumberFilterItem for number property filters
type NumberFilterItem struct {
	PropertyKey string          `json:"property_key" example:"height"`                                   // The property key to filter on
	Condition   FilterCondition `json:"condition" example:"gt" enums:"eq,ne,gt,gte,lt,lte,empty,nempty"` // The filter condition
	Number      *float64        `json:"number" example:"42"`                                             // The number value to filter by
}

func (NumberFilterItem) isFilterItem()                   {}
func (f NumberFilterItem) GetPropertyKey() string        { return f.PropertyKey }
func (f NumberFilterItem) GetCondition() FilterCondition { return f.Condition }
func (f NumberFilterItem) GetValue() interface{} {
	if f.Number != nil {
		return *f.Number
	}
	return nil
}

// SelectFilterItem for select property filters
type SelectFilterItem struct {
	PropertyKey string          `json:"property_key" example:"status"`                          // The property key to filter on
	Condition   FilterCondition `json:"condition" example:"in" enums:"in,all,nin,empty,nempty"` // The filter condition
	Select      *string         `json:"select" example:"tag_id"`                                // Tag Id - for eq/ne conditions (single selection)
}

func (SelectFilterItem) isFilterItem()                   {}
func (f SelectFilterItem) GetPropertyKey() string        { return f.PropertyKey }
func (f SelectFilterItem) GetCondition() FilterCondition { return f.Condition }
func (f SelectFilterItem) GetValue() interface{} {
	if f.Select != nil {
		return *f.Select
	}
	return nil
}

// MultiSelectFilterItem for multi-select property filters
type MultiSelectFilterItem struct {
	PropertyKey string          `json:"property_key" example:"tag"`                                                                                                                     // The property key to filter on
	Condition   FilterCondition `json:"condition" example:"all" enums:"in,all,nin,empty,nempty"`                                                                                        // The filter condition
	MultiSelect *[]string       `json:"multi_select" example:"bafyreiaixlnaefu3ci22zdenjhsdlyaeeoyjrsid5qhfeejzlccijbj7sq,bafyreie6n5l5nkbjal37su54cha4coy7qzuhrnajluzv5qd5jvtsrxkequ"` // The tag IDs to filter by
}

func (MultiSelectFilterItem) isFilterItem()                   {}
func (f MultiSelectFilterItem) GetPropertyKey() string        { return f.PropertyKey }
func (f MultiSelectFilterItem) GetCondition() FilterCondition { return f.Condition }
func (f MultiSelectFilterItem) GetValue() interface{} {
	if f.MultiSelect != nil {
		return *f.MultiSelect
	}
	return nil
}

// DateFilterItem for date property filters
type DateFilterItem struct {
	PropertyKey string          `json:"property_key" example:"last_modified_date"`                        // The property key to filter on
	Condition   FilterCondition `json:"condition" example:"lte" enums:"eq,gt,gte,lt,lte,in,empty,nempty"` // The filter condition
	Date        *string         `json:"date" example:"2006-01-02T15:04:05Z"`                              // The date value to filter by. Accepts dates in RFC3339 format (2006-01-02T15:04:05Z) or date-only format (2006-01-02)
}

func (DateFilterItem) isFilterItem()                   {}
func (f DateFilterItem) GetPropertyKey() string        { return f.PropertyKey }
func (f DateFilterItem) GetCondition() FilterCondition { return f.Condition }
func (f DateFilterItem) GetValue() interface{} {
	if f.Date != nil {
		return *f.Date
	}
	return nil
}

// CheckboxFilterItem for checkbox property filters
type CheckboxFilterItem struct {
	PropertyKey string          `json:"property_key" example:"done"`          // The property key to filter on
	Condition   FilterCondition `json:"condition" example:"eq" enums:"eq,ne"` // The filter condition
	Checkbox    *bool           `json:"checkbox" example:"true"`              // The checkbox value to filter by
}

func (CheckboxFilterItem) isFilterItem()                   {}
func (f CheckboxFilterItem) GetPropertyKey() string        { return f.PropertyKey }
func (f CheckboxFilterItem) GetCondition() FilterCondition { return f.Condition }
func (f CheckboxFilterItem) GetValue() interface{} {
	if f.Checkbox != nil {
		return *f.Checkbox
	}
	return nil
}

// FilesFilterItem for files property filters
type FilesFilterItem struct {
	PropertyKey string          `json:"property_key" example:"files"`                                                // The property key to filter on
	Condition   FilterCondition `json:"condition" example:"in" enums:"in,all,nin,empty,nempty"`                      // The filter condition
	Files       *[]string       `json:"files" example:"bafyreie6n5l5nkbjal37su54cha4coy7qzuhrnajluzv5qd5jvtsrxkequ"` // File IDs for contains condition
}

func (FilesFilterItem) isFilterItem()                   {}
func (f FilesFilterItem) GetPropertyKey() string        { return f.PropertyKey }
func (f FilesFilterItem) GetCondition() FilterCondition { return f.Condition }
func (f FilesFilterItem) GetValue() interface{} {
	if f.Files != nil {
		return *f.Files
	}
	return nil
}

// UrlFilterItem for URL property filters
type UrlFilterItem struct {
	PropertyKey string          `json:"property_key" example:"source"`                                              // The property key to filter on
	Condition   FilterCondition `json:"condition" example:"contains" enums:"eq,ne,contains,ncontains,empty,nempty"` // The filter condition
	Url         *string         `json:"url" example:"https://example.com"`                                          // The Url value to filter by
}

func (UrlFilterItem) isFilterItem()                   {}
func (f UrlFilterItem) GetPropertyKey() string        { return f.PropertyKey }
func (f UrlFilterItem) GetCondition() FilterCondition { return f.Condition }
func (f UrlFilterItem) GetValue() interface{} {
	if f.Url != nil {
		return *f.Url
	}
	return nil
}

// EmailFilterItem for email property filters
type EmailFilterItem struct {
	PropertyKey string          `json:"property_key" example:"email"`                                         // The property key to filter on
	Condition   FilterCondition `json:"condition" example:"eq" enums:"eq,ne,contains,ncontains,empty,nempty"` // The filter condition
	Email       *string         `json:"email" example:"example@example.com"`                                  // The email value to filter by
}

func (EmailFilterItem) isFilterItem()                   {}
func (f EmailFilterItem) GetPropertyKey() string        { return f.PropertyKey }
func (f EmailFilterItem) GetCondition() FilterCondition { return f.Condition }
func (f EmailFilterItem) GetValue() interface{} {
	if f.Email != nil {
		return *f.Email
	}
	return nil
}

// PhoneFilterItem for phone property filters
type PhoneFilterItem struct {
	PropertyKey string          `json:"property_key" example:"phone"`                                         // The property key to filter on
	Condition   FilterCondition `json:"condition" example:"eq" enums:"eq,ne,contains,ncontains,empty,nempty"` // The filter condition
	Phone       *string         `json:"phone" example:"+1234567890"`                                          // The phone value to filter by
}

func (PhoneFilterItem) isFilterItem()                   {}
func (f PhoneFilterItem) GetPropertyKey() string        { return f.PropertyKey }
func (f PhoneFilterItem) GetCondition() FilterCondition { return f.Condition }
func (f PhoneFilterItem) GetValue() interface{} {
	if f.Phone != nil {
		return *f.Phone
	}
	return nil
}

// ObjectsFilterItem for objects/relation property filters
type ObjectsFilterItem struct {
	PropertyKey string          `json:"property_key" example:"creator"`                                                // The property key to filter on
	Condition   FilterCondition `json:"condition" example:"in" enums:"in,all,nin,empty,nempty"`                        // The filter condition
	Objects     *[]string       `json:"objects" example:"bafyreie6n5l5nkbjal37su54cha4coy7qzuhrnajluzv5qd5jvtsrxkequ"` // Object Ids to filter by
}

func (ObjectsFilterItem) isFilterItem()                   {}
func (f ObjectsFilterItem) GetPropertyKey() string        { return f.PropertyKey }
func (f ObjectsFilterItem) GetCondition() FilterCondition { return f.Condition }
func (f ObjectsFilterItem) GetValue() interface{} {
	if f.Objects != nil {
		return *f.Objects
	}
	return nil
}

// EmptyFilterItem for checking if property is empty/not empty (without specifying format)
type EmptyFilterItem struct {
	PropertyKey string          `json:"property_key" example:"description"`             // The property key to filter on
	Condition   FilterCondition `json:"condition" example:"empty" enums:"empty,nempty"` // The filter condition
}

func (EmptyFilterItem) isFilterItem()                   {}
func (f EmptyFilterItem) GetPropertyKey() string        { return f.PropertyKey }
func (f EmptyFilterItem) GetCondition() FilterCondition { return f.Condition }
func (f EmptyFilterItem) GetValue() interface{}         { return nil }
