package filter

import (
	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

// Filter represents a single filter with property key, condition, and value
type Filter struct {
	PropertyKey string                                    `json:"property_key"`
	Condition   model.BlockContentDataviewFilterCondition `json:"condition"`
	Value       interface{}                               `json:"value"`
}

// ParsedFilters represents filters parsed from query parameters
type ParsedFilters struct {
	Filters []Filter `json:"filters"`
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

// ConditionsForPropertyType defines which conditions are valid for each property type
var ConditionsForPropertyType = map[apimodel.PropertyFormat][]model.BlockContentDataviewFilterCondition{
	apimodel.PropertyFormatText: {
		model.BlockContentDataviewFilter_Equal,
		model.BlockContentDataviewFilter_NotEqual,
		model.BlockContentDataviewFilter_Like,
		model.BlockContentDataviewFilter_NotLike,
		model.BlockContentDataviewFilter_Empty,
		model.BlockContentDataviewFilter_NotEmpty,
		model.BlockContentDataviewFilter_Exists,
	},
	apimodel.PropertyFormatNumber: {
		model.BlockContentDataviewFilter_Equal,
		model.BlockContentDataviewFilter_NotEqual,
		model.BlockContentDataviewFilter_Greater,
		model.BlockContentDataviewFilter_GreaterOrEqual,
		model.BlockContentDataviewFilter_Less,
		model.BlockContentDataviewFilter_LessOrEqual,
		model.BlockContentDataviewFilter_Empty,
		model.BlockContentDataviewFilter_NotEmpty,
		model.BlockContentDataviewFilter_Exists,
	},
	apimodel.PropertyFormatDate: {
		model.BlockContentDataviewFilter_Equal,
		model.BlockContentDataviewFilter_NotEqual,
		model.BlockContentDataviewFilter_Greater,
		model.BlockContentDataviewFilter_GreaterOrEqual,
		model.BlockContentDataviewFilter_Less,
		model.BlockContentDataviewFilter_LessOrEqual,
		model.BlockContentDataviewFilter_Empty,
		model.BlockContentDataviewFilter_NotEmpty,
		model.BlockContentDataviewFilter_Exists,
	},
	apimodel.PropertyFormatCheckbox: {
		model.BlockContentDataviewFilter_Equal,
		model.BlockContentDataviewFilter_NotEqual,
		model.BlockContentDataviewFilter_Exists,
	},
	apimodel.PropertyFormatUrl: {
		model.BlockContentDataviewFilter_Equal,
		model.BlockContentDataviewFilter_NotEqual,
		model.BlockContentDataviewFilter_Like,
		model.BlockContentDataviewFilter_NotLike,
		model.BlockContentDataviewFilter_Empty,
		model.BlockContentDataviewFilter_NotEmpty,
		model.BlockContentDataviewFilter_Exists,
	},
	apimodel.PropertyFormatEmail: {
		model.BlockContentDataviewFilter_Equal,
		model.BlockContentDataviewFilter_NotEqual,
		model.BlockContentDataviewFilter_Like,
		model.BlockContentDataviewFilter_NotLike,
		model.BlockContentDataviewFilter_Empty,
		model.BlockContentDataviewFilter_NotEmpty,
		model.BlockContentDataviewFilter_Exists,
	},
	apimodel.PropertyFormatPhone: {
		model.BlockContentDataviewFilter_Equal,
		model.BlockContentDataviewFilter_NotEqual,
		model.BlockContentDataviewFilter_Like,
		model.BlockContentDataviewFilter_NotLike,
		model.BlockContentDataviewFilter_Empty,
		model.BlockContentDataviewFilter_NotEmpty,
		model.BlockContentDataviewFilter_Exists,
	},
	apimodel.PropertyFormatSelect: {
		model.BlockContentDataviewFilter_Equal,
		model.BlockContentDataviewFilter_NotEqual,
		model.BlockContentDataviewFilter_In,
		model.BlockContentDataviewFilter_NotIn,
		model.BlockContentDataviewFilter_Empty,
		model.BlockContentDataviewFilter_NotEmpty,
		model.BlockContentDataviewFilter_Exists,
	},
	apimodel.PropertyFormatMultiSelect: {
		model.BlockContentDataviewFilter_In,
		model.BlockContentDataviewFilter_NotIn,
		model.BlockContentDataviewFilter_AllIn,
		model.BlockContentDataviewFilter_NotAllIn,
		model.BlockContentDataviewFilter_ExactIn,
		model.BlockContentDataviewFilter_NotExactIn,
		model.BlockContentDataviewFilter_Empty,
		model.BlockContentDataviewFilter_NotEmpty,
		model.BlockContentDataviewFilter_Exists,
	},
	apimodel.PropertyFormatFiles: {
		model.BlockContentDataviewFilter_In,
		model.BlockContentDataviewFilter_NotIn,
		model.BlockContentDataviewFilter_AllIn,
		model.BlockContentDataviewFilter_NotAllIn,
		model.BlockContentDataviewFilter_ExactIn,
		model.BlockContentDataviewFilter_NotExactIn,
		model.BlockContentDataviewFilter_Empty,
		model.BlockContentDataviewFilter_NotEmpty,
		model.BlockContentDataviewFilter_Exists,
	},
	apimodel.PropertyFormatObjects: {
		model.BlockContentDataviewFilter_In,
		model.BlockContentDataviewFilter_NotIn,
		model.BlockContentDataviewFilter_AllIn,
		model.BlockContentDataviewFilter_NotAllIn,
		model.BlockContentDataviewFilter_ExactIn,
		model.BlockContentDataviewFilter_NotExactIn,
		model.BlockContentDataviewFilter_Empty,
		model.BlockContentDataviewFilter_NotEmpty,
		model.BlockContentDataviewFilter_Exists,
	},
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
