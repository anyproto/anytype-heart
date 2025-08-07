package filter

import (
	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// ConditionMap maps API filter conditions to internal database conditions
var ConditionMap = map[apimodel.FilterCondition]model.BlockContentDataviewFilterCondition{
	// Basic comparisons
	apimodel.FilterConditionEq:  model.BlockContentDataviewFilter_Equal,
	apimodel.FilterConditionNe:  model.BlockContentDataviewFilter_NotEqual,
	apimodel.FilterConditionGt:  model.BlockContentDataviewFilter_Greater,
	apimodel.FilterConditionGte: model.BlockContentDataviewFilter_GreaterOrEqual,
	apimodel.FilterConditionLt:  model.BlockContentDataviewFilter_Less,
	apimodel.FilterConditionLte: model.BlockContentDataviewFilter_LessOrEqual,

	// Text operations
	apimodel.FilterConditionContains:  model.BlockContentDataviewFilter_Like,
	apimodel.FilterConditionNContains: model.BlockContentDataviewFilter_NotLike,

	// Array operations
	apimodel.FilterConditionIn:  model.BlockContentDataviewFilter_In,
	apimodel.FilterConditionNin: model.BlockContentDataviewFilter_NotIn,
	apimodel.FilterConditionAll: model.BlockContentDataviewFilter_AllIn,

	// Emptiness checks
	apimodel.FilterConditionEmpty:  model.BlockContentDataviewFilter_Empty,
	apimodel.FilterConditionNEmpty: model.BlockContentDataviewFilter_NotEmpty,
}

// OperatorMap maps API filter operators to internal database operators
var OperatorMap = map[apimodel.FilterOperator]model.BlockContentDataviewFilterOperator{
	apimodel.FilterOperatorAnd: model.BlockContentDataviewFilter_And,
	apimodel.FilterOperatorOr:  model.BlockContentDataviewFilter_Or,
}

// ReverseConditionMap maps internal database conditions to API filter conditions
// This is used when converting from internal representations to API responses
var ReverseConditionMap = map[model.BlockContentDataviewFilterCondition]apimodel.FilterCondition{
	// Note: BlockContentDataviewFilter_None (0) is intentionally omitted as it represents no condition

	// Basic comparisons
	model.BlockContentDataviewFilter_Equal:          apimodel.FilterConditionEq,
	model.BlockContentDataviewFilter_NotEqual:       apimodel.FilterConditionNe,
	model.BlockContentDataviewFilter_Greater:        apimodel.FilterConditionGt,
	model.BlockContentDataviewFilter_GreaterOrEqual: apimodel.FilterConditionGte,
	model.BlockContentDataviewFilter_Less:           apimodel.FilterConditionLt,
	model.BlockContentDataviewFilter_LessOrEqual:    apimodel.FilterConditionLte,

	// Text operations
	model.BlockContentDataviewFilter_Like:    apimodel.FilterConditionContains,
	model.BlockContentDataviewFilter_NotLike: apimodel.FilterConditionNContains,

	// Array operations
	model.BlockContentDataviewFilter_In:    apimodel.FilterConditionIn,
	model.BlockContentDataviewFilter_NotIn: apimodel.FilterConditionNin,
	model.BlockContentDataviewFilter_AllIn: apimodel.FilterConditionAll,
	// Note: NotAllIn, ExactIn, NotExactIn are internal only - not exposed in API

	// Emptiness checks
	model.BlockContentDataviewFilter_Empty:    apimodel.FilterConditionEmpty,
	model.BlockContentDataviewFilter_NotEmpty: apimodel.FilterConditionNEmpty,
	// Note: Exists is internal only - not exposed in API
}
