package filter

import (
	"testing"

	"github.com/stretchr/testify/assert"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestConditionMap(t *testing.T) {
	expectedMappings := map[string]model.BlockContentDataviewFilterCondition{
		"eq":        model.BlockContentDataviewFilter_Equal,
		"ne":        model.BlockContentDataviewFilter_NotEqual,
		"gt":        model.BlockContentDataviewFilter_Greater,
		"gte":       model.BlockContentDataviewFilter_GreaterOrEqual,
		"lt":        model.BlockContentDataviewFilter_Less,
		"lte":       model.BlockContentDataviewFilter_LessOrEqual,
		"contains":  model.BlockContentDataviewFilter_Like,
		"ncontains": model.BlockContentDataviewFilter_NotLike,
		"in":        model.BlockContentDataviewFilter_In,
		"nin":       model.BlockContentDataviewFilter_NotIn,
		"all":       model.BlockContentDataviewFilter_AllIn,
		"empty":     model.BlockContentDataviewFilter_Empty,
		"nempty":    model.BlockContentDataviewFilter_NotEmpty,
	}

	// Test each string maps to the correct condition
	for str, expectedCondition := range expectedMappings {
		filterCond := apimodel.FilterCondition(str)
		actualCondition, ok := ConditionMap[filterCond]
		assert.True(t, ok, "Condition string %q should be mapped", str)
		assert.Equal(t, expectedCondition, actualCondition, "Condition %q should map to %v", str, expectedCondition)
	}

	// Also verify that the constants have the expected string values
	assert.Equal(t, "eq", apimodel.FilterConditionEq.String())
	assert.Equal(t, "ne", apimodel.FilterConditionNe.String())
	assert.Equal(t, "gt", apimodel.FilterConditionGt.String())
	assert.Equal(t, "gte", apimodel.FilterConditionGte.String())
	assert.Equal(t, "lt", apimodel.FilterConditionLt.String())
	assert.Equal(t, "lte", apimodel.FilterConditionLte.String())
	assert.Equal(t, "contains", apimodel.FilterConditionContains.String())
	assert.Equal(t, "ncontains", apimodel.FilterConditionNContains.String())
	assert.Equal(t, "in", apimodel.FilterConditionIn.String())
	assert.Equal(t, "nin", apimodel.FilterConditionNin.String())
	assert.Equal(t, "all", apimodel.FilterConditionAll.String())
	assert.Equal(t, "empty", apimodel.FilterConditionEmpty.String())
	assert.Equal(t, "nempty", apimodel.FilterConditionNEmpty.String())
}

func TestOperatorMap(t *testing.T) {
	assert.Equal(t, model.BlockContentDataviewFilter_And, OperatorMap[apimodel.FilterOperatorAnd])
	assert.Equal(t, model.BlockContentDataviewFilter_Or, OperatorMap[apimodel.FilterOperatorOr])
}
