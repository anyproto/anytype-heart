package filter

import (
	"testing"

	"github.com/stretchr/testify/assert"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestBidirectionalConditionMapping(t *testing.T) {
	t.Run("ToInternalCondition", func(t *testing.T) {
		expectedMappings := map[apimodel.FilterCondition]model.BlockContentDataviewFilterCondition{
			apimodel.FilterConditionEq:        model.BlockContentDataviewFilter_Equal,
			apimodel.FilterConditionNe:        model.BlockContentDataviewFilter_NotEqual,
			apimodel.FilterConditionGt:        model.BlockContentDataviewFilter_Greater,
			apimodel.FilterConditionGte:       model.BlockContentDataviewFilter_GreaterOrEqual,
			apimodel.FilterConditionLt:        model.BlockContentDataviewFilter_Less,
			apimodel.FilterConditionLte:       model.BlockContentDataviewFilter_LessOrEqual,
			apimodel.FilterConditionContains:  model.BlockContentDataviewFilter_Like,
			apimodel.FilterConditionNContains: model.BlockContentDataviewFilter_NotLike,
			apimodel.FilterConditionIn:        model.BlockContentDataviewFilter_In,
			apimodel.FilterConditionNin:       model.BlockContentDataviewFilter_NotIn,
			apimodel.FilterConditionAll:       model.BlockContentDataviewFilter_AllIn,
			apimodel.FilterConditionEmpty:     model.BlockContentDataviewFilter_Empty,
			apimodel.FilterConditionNEmpty:    model.BlockContentDataviewFilter_NotEmpty,
		}

		// Test each API condition maps to the correct internal condition
		for apiCond, expectedCondition := range expectedMappings {
			actualCondition, ok := ToInternalCondition(apiCond)
			assert.True(t, ok, "API condition %v should be mapped", apiCond)
			assert.Equal(t, expectedCondition, actualCondition, "API condition %v should map to %v", apiCond, expectedCondition)
		}
	})

	t.Run("ToApiCondition", func(t *testing.T) {
		expectedMappings := map[model.BlockContentDataviewFilterCondition]apimodel.FilterCondition{
			model.BlockContentDataviewFilter_Equal:          apimodel.FilterConditionEq,
			model.BlockContentDataviewFilter_NotEqual:       apimodel.FilterConditionNe,
			model.BlockContentDataviewFilter_Greater:        apimodel.FilterConditionGt,
			model.BlockContentDataviewFilter_GreaterOrEqual: apimodel.FilterConditionGte,
			model.BlockContentDataviewFilter_Less:           apimodel.FilterConditionLt,
			model.BlockContentDataviewFilter_LessOrEqual:    apimodel.FilterConditionLte,
			model.BlockContentDataviewFilter_Like:           apimodel.FilterConditionContains,
			model.BlockContentDataviewFilter_NotLike:        apimodel.FilterConditionNContains,
			model.BlockContentDataviewFilter_In:             apimodel.FilterConditionIn,
			model.BlockContentDataviewFilter_NotIn:          apimodel.FilterConditionNin,
			model.BlockContentDataviewFilter_AllIn:          apimodel.FilterConditionAll,
			model.BlockContentDataviewFilter_Empty:          apimodel.FilterConditionEmpty,
			model.BlockContentDataviewFilter_NotEmpty:       apimodel.FilterConditionNEmpty,
		}

		// Test each internal condition maps back to correct API condition
		for internalCond, expectedCond := range expectedMappings {
			apiCond, ok := ToApiCondition(internalCond)
			assert.True(t, ok, "Internal condition %v should be mapped", internalCond)
			assert.Equal(t, expectedCond, apiCond, "Internal condition %v should map to %v", internalCond, expectedCond)
		}
	})

	t.Run("BidirectionalConsistency", func(t *testing.T) {
		// Test that conversion is consistent in both directions
		apiConditions := []apimodel.FilterCondition{
			apimodel.FilterConditionEq,
			apimodel.FilterConditionNe,
			apimodel.FilterConditionGt,
			apimodel.FilterConditionGte,
			apimodel.FilterConditionLt,
			apimodel.FilterConditionLte,
			apimodel.FilterConditionContains,
			apimodel.FilterConditionNContains,
			apimodel.FilterConditionIn,
			apimodel.FilterConditionNin,
			apimodel.FilterConditionAll,
			apimodel.FilterConditionEmpty,
			apimodel.FilterConditionNEmpty,
		}

		for _, apiCond := range apiConditions {
			// Convert to internal and back
			internalCond, ok := ToInternalCondition(apiCond)
			assert.True(t, ok, "API condition %v should convert to internal", apiCond)

			backToAPI, ok := ToApiCondition(internalCond)
			assert.True(t, ok, "Internal condition %v should convert back to API", internalCond)

			assert.Equal(t, apiCond, backToAPI, "Round-trip conversion should preserve condition")
		}
	})

	t.Run("InvalidCondition", func(t *testing.T) {
		// Test that invalid conditions return false
		_, ok := ToInternalCondition(apimodel.FilterCondition("invalid"))
		assert.False(t, ok, "Invalid API condition should return false")

		// Test that None condition (0) is not mapped
		_, ok = ToApiCondition(model.BlockContentDataviewFilter_None)
		assert.False(t, ok, "None condition should not be mapped")
	})

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
