package filter

import (
	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type ParsedFilters struct {
	Filters []Filter `json:"filters"`
}

type Filter struct {
	PropertyKey string                                    `json:"property_key"`
	Condition   model.BlockContentDataviewFilterCondition `json:"condition"`
	Value       interface{}                               `json:"value"`
}

// ToDataviewFilters converts parsed filters to dataview filter format
func (pf *ParsedFilters) ToDataviewFilters() []*model.BlockContentDataviewFilter {
	if pf == nil || len(pf.Filters) == 0 {
		return nil
	}

	filters := make([]*model.BlockContentDataviewFilter, 0, len(pf.Filters))
	for _, f := range pf.Filters {
		filters = append(filters, &model.BlockContentDataviewFilter{
			RelationKey: f.PropertyKey,
			Condition:   f.Condition,
			Value:       pbtypes.ToValue(f.Value),
		})
	}

	return filters
}

var (
	// Text-like properties support equality, pattern matching, and emptiness checks
	textConditions = []model.BlockContentDataviewFilterCondition{
		model.BlockContentDataviewFilter_Equal,
		model.BlockContentDataviewFilter_NotEqual,
		model.BlockContentDataviewFilter_Like,
		model.BlockContentDataviewFilter_NotLike,
		model.BlockContentDataviewFilter_Empty,
		model.BlockContentDataviewFilter_NotEmpty,
	}

	// Array-like properties support set operations and emptiness checks
	arrayConditions = []model.BlockContentDataviewFilterCondition{
		model.BlockContentDataviewFilter_In,
		model.BlockContentDataviewFilter_AllIn,
		model.BlockContentDataviewFilter_NotIn,
		model.BlockContentDataviewFilter_Empty,
		model.BlockContentDataviewFilter_NotEmpty,
	}

	// Number properties support comparison operations
	numberConditions = []model.BlockContentDataviewFilterCondition{
		model.BlockContentDataviewFilter_Equal,
		model.BlockContentDataviewFilter_NotEqual,
		model.BlockContentDataviewFilter_Greater,
		model.BlockContentDataviewFilter_GreaterOrEqual,
		model.BlockContentDataviewFilter_Less,
		model.BlockContentDataviewFilter_LessOrEqual,
		model.BlockContentDataviewFilter_Empty,
		model.BlockContentDataviewFilter_NotEmpty,
	}

	// Date properties support comparison and range operations
	dateConditions = []model.BlockContentDataviewFilterCondition{
		model.BlockContentDataviewFilter_Equal,
		model.BlockContentDataviewFilter_Greater,
		model.BlockContentDataviewFilter_Less,
		model.BlockContentDataviewFilter_GreaterOrEqual,
		model.BlockContentDataviewFilter_LessOrEqual,
		model.BlockContentDataviewFilter_In,
		model.BlockContentDataviewFilter_Empty,
		model.BlockContentDataviewFilter_NotEmpty,
	}

	// Checkbox properties only support equality checks
	checkboxConditions = []model.BlockContentDataviewFilterCondition{
		model.BlockContentDataviewFilter_Equal,
		model.BlockContentDataviewFilter_NotEqual,
	}
)

// ConditionsForPropertyType defines which conditions are valid for each property type
var ConditionsForPropertyType = map[apimodel.PropertyFormat][]model.BlockContentDataviewFilterCondition{
	// Text-like types
	apimodel.PropertyFormatText:  textConditions,
	apimodel.PropertyFormatUrl:   textConditions,
	apimodel.PropertyFormatEmail: textConditions,
	apimodel.PropertyFormatPhone: textConditions,

	// Numeric type
	apimodel.PropertyFormatNumber: numberConditions,

	// Date type
	apimodel.PropertyFormatDate: dateConditions,

	// Boolean type
	apimodel.PropertyFormatCheckbox: checkboxConditions,

	// Array-like types
	apimodel.PropertyFormatSelect:      arrayConditions,
	apimodel.PropertyFormatMultiSelect: arrayConditions,
	apimodel.PropertyFormatFiles:       arrayConditions,
	apimodel.PropertyFormatObjects:     arrayConditions,
}

// isValidConditionForType checks if a condition is valid for a property type
func isValidConditionForType(format apimodel.PropertyFormat, condition model.BlockContentDataviewFilterCondition) bool {
	validConditions, ok := ConditionsForPropertyType[format]
	if !ok {
		return false
	}

	for _, validCondition := range validConditions {
		if validCondition == condition {
			return true
		}
	}

	return false
}
