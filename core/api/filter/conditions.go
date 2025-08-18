package filter

import (
	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// BiDirectionalConditionMap handles bidirectional mapping between API and internal conditions
type BiDirectionalConditionMap struct {
	apiToInternal map[apimodel.FilterCondition]model.BlockContentDataviewFilterCondition
	internalToAPI map[model.BlockContentDataviewFilterCondition]apimodel.FilterCondition
}

// NewBiDirectionalConditionMap creates a new bidirectional condition map from a single source of truth
func NewBiDirectionalConditionMap() *BiDirectionalConditionMap {
	mappings := []struct {
		api      apimodel.FilterCondition
		internal model.BlockContentDataviewFilterCondition
	}{
		// Basic comparisons
		{apimodel.FilterConditionEq, model.BlockContentDataviewFilter_Equal},
		{apimodel.FilterConditionNe, model.BlockContentDataviewFilter_NotEqual},
		{apimodel.FilterConditionGt, model.BlockContentDataviewFilter_Greater},
		{apimodel.FilterConditionGte, model.BlockContentDataviewFilter_GreaterOrEqual},
		{apimodel.FilterConditionLt, model.BlockContentDataviewFilter_Less},
		{apimodel.FilterConditionLte, model.BlockContentDataviewFilter_LessOrEqual},

		// Text operations
		{apimodel.FilterConditionContains, model.BlockContentDataviewFilter_Like},
		{apimodel.FilterConditionNContains, model.BlockContentDataviewFilter_NotLike},

		// Array operations
		{apimodel.FilterConditionIn, model.BlockContentDataviewFilter_In},
		{apimodel.FilterConditionNin, model.BlockContentDataviewFilter_NotIn},
		{apimodel.FilterConditionAll, model.BlockContentDataviewFilter_AllIn},

		// Emptiness checks
		{apimodel.FilterConditionEmpty, model.BlockContentDataviewFilter_Empty},
		{apimodel.FilterConditionNEmpty, model.BlockContentDataviewFilter_NotEmpty},
	}

	m := &BiDirectionalConditionMap{
		apiToInternal: make(map[apimodel.FilterCondition]model.BlockContentDataviewFilterCondition),
		internalToAPI: make(map[model.BlockContentDataviewFilterCondition]apimodel.FilterCondition),
	}

	for _, mapping := range mappings {
		m.apiToInternal[mapping.api] = mapping.internal
		m.internalToAPI[mapping.internal] = mapping.api
	}

	return m
}

// ToInternal converts an API condition to internal representation
func (m *BiDirectionalConditionMap) ToInternal(api apimodel.FilterCondition) (model.BlockContentDataviewFilterCondition, bool) {
	internal, ok := m.apiToInternal[api]
	return internal, ok
}

// ToAPI converts an internal condition to API representation
func (m *BiDirectionalConditionMap) ToAPI(internal model.BlockContentDataviewFilterCondition) (apimodel.FilterCondition, bool) {
	api, ok := m.internalToAPI[internal]
	return api, ok
}

// conditionMapper is the singleton instance for condition mapping
var conditionMapper = NewBiDirectionalConditionMap()

// ToInternalCondition converts an API condition to internal representation
func ToInternalCondition(api apimodel.FilterCondition) (model.BlockContentDataviewFilterCondition, bool) {
	return conditionMapper.ToInternal(api)
}

// ToAPICondition converts an internal condition to API representation
func ToAPICondition(internal model.BlockContentDataviewFilterCondition) (apimodel.FilterCondition, bool) {
	return conditionMapper.ToAPI(internal)
}

// OperatorMap maps API filter operators to internal database operators
var OperatorMap = map[apimodel.FilterOperator]model.BlockContentDataviewFilterOperator{
	apimodel.FilterOperatorAnd: model.BlockContentDataviewFilter_And,
	apimodel.FilterOperatorOr:  model.BlockContentDataviewFilter_Or,
}
