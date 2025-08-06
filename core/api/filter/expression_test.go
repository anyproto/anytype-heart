package filter

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/api/filter/mock_filter"
	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

func TestBuildExpressionFilters(t *testing.T) {
	ctx := context.Background()
	spaceId := "space123"

	tests := []struct {
		name          string
		expr          *apimodel.FilterExpression
		setupMock     func(m *mock_filter.MockApiService)
		expectedError string
		checkResult   func(t *testing.T, result *model.BlockContentDataviewFilter)
	}{
		{
			name: "nil expression returns nil",
			expr: nil,
			setupMock: func(m *mock_filter.MockApiService) {
				// No setup needed
			},
			checkResult: func(t *testing.T, result *model.BlockContentDataviewFilter) {
				assert.Nil(t, result)
			},
		},
		{
			name: "single condition filter",
			expr: &apimodel.FilterExpression{
				Conditions: []apimodel.FilterItem{
					{
						PropertyKey: "done",
						Condition:   apimodel.FilterConditionEq,
						Value:       true,
					},
				},
			},
			setupMock: func(m *mock_filter.MockApiService) {
				m.On("GetCachedProperties", spaceId).Return(map[string]*apimodel.Property{
					"done": {
						Key:         "done",
						RelationKey: bundle.RelationKeyDone.String(),
						Format:      apimodel.PropertyFormatCheckbox,
					},
				})
				m.On("ResolvePropertyApiKey", mock.Anything, "done").Return("done")
				m.On("SanitizeAndValidatePropertyValue", spaceId, "done", apimodel.PropertyFormatCheckbox, true, mock.Anything, mock.Anything).Return(true, nil)
			},
			checkResult: func(t *testing.T, result *model.BlockContentDataviewFilter) {
				require.NotNil(t, result)
				assert.Equal(t, bundle.RelationKeyDone.String(), result.RelationKey)
				assert.Equal(t, model.BlockContentDataviewFilter_Equal, result.Condition)
				assert.Equal(t, pbtypes.Bool(true), result.Value)
			},
		},
		{
			name: "AND operator with multiple conditions",
			expr: &apimodel.FilterExpression{
				Operator: apimodel.FilterOperatorAnd,
				Conditions: []apimodel.FilterItem{
					{
						PropertyKey: "done",
						Condition:   apimodel.FilterConditionEq,
						Value:       true,
					},
					{
						PropertyKey: "priority",
						Condition:   apimodel.FilterConditionGt,
						Value:       5,
					},
				},
			},
			setupMock: func(m *mock_filter.MockApiService) {
				propertyMap := map[string]*apimodel.Property{
					"done": {
						Key:         "done",
						RelationKey: bundle.RelationKeyDone.String(),
						Format:      apimodel.PropertyFormatCheckbox,
					},
					"priority": {
						Key:         "priority",
						RelationKey: bundle.RelationKeyPriority.String(),
						Format:      apimodel.PropertyFormatNumber,
					},
				}
				m.On("GetCachedProperties", spaceId).Return(propertyMap)
				m.On("ResolvePropertyApiKey", propertyMap, "done").Return("done")
				m.On("ResolvePropertyApiKey", propertyMap, "priority").Return("priority")
				m.On("SanitizeAndValidatePropertyValue", spaceId, "done", apimodel.PropertyFormatCheckbox, true, mock.Anything, propertyMap).Return(true, nil)
				m.On("SanitizeAndValidatePropertyValue", spaceId, "priority", apimodel.PropertyFormatNumber, 5, mock.Anything, propertyMap).Return(float64(5), nil)
			},
			checkResult: func(t *testing.T, result *model.BlockContentDataviewFilter) {
				require.NotNil(t, result)
				assert.Equal(t, model.BlockContentDataviewFilter_And, result.Operator)
				assert.Len(t, result.NestedFilters, 2)

				// Check first filter
				assert.Equal(t, bundle.RelationKeyDone.String(), result.NestedFilters[0].RelationKey)
				assert.Equal(t, model.BlockContentDataviewFilter_Equal, result.NestedFilters[0].Condition)

				// Check second filter
				assert.Equal(t, bundle.RelationKeyPriority.String(), result.NestedFilters[1].RelationKey)
				assert.Equal(t, model.BlockContentDataviewFilter_Greater, result.NestedFilters[1].Condition)
			},
		},
		{
			name: "OR operator with conditions",
			expr: &apimodel.FilterExpression{
				Operator: apimodel.FilterOperatorOr,
				Conditions: []apimodel.FilterItem{
					{
						PropertyKey: "type",
						Condition:   apimodel.FilterConditionEq,
						Value:       "page",
					},
					{
						PropertyKey: "type",
						Condition:   apimodel.FilterConditionEq,
						Value:       "task",
					},
				},
			},
			setupMock: func(m *mock_filter.MockApiService) {
				propertyMap := map[string]*apimodel.Property{
					"type": {
						Key:         "type",
						RelationKey: bundle.RelationKeyType.String(),
						Format:      apimodel.PropertyFormatText,
					},
				}
				m.On("GetCachedProperties", spaceId).Return(propertyMap)
				m.On("ResolvePropertyApiKey", propertyMap, "type").Return("type")
				m.On("SanitizeAndValidatePropertyValue", spaceId, "type", apimodel.PropertyFormatText, "page", mock.Anything, propertyMap).Return("page", nil)
				m.On("SanitizeAndValidatePropertyValue", spaceId, "type", apimodel.PropertyFormatText, "task", mock.Anything, propertyMap).Return("task", nil)
			},
			checkResult: func(t *testing.T, result *model.BlockContentDataviewFilter) {
				require.NotNil(t, result)
				assert.Equal(t, model.BlockContentDataviewFilter_Or, result.Operator)
				assert.Len(t, result.NestedFilters, 2)
			},
		},
		{
			name: "nested filters with AND and OR",
			expr: &apimodel.FilterExpression{
				Operator: apimodel.FilterOperatorAnd,
				Conditions: []apimodel.FilterItem{
					{
						PropertyKey: "is_archived",
						Condition:   apimodel.FilterConditionNe,
						Value:       true,
					},
				},
				Filters: []apimodel.FilterExpression{
					{
						Operator: apimodel.FilterOperatorOr,
						Conditions: []apimodel.FilterItem{
							{
								PropertyKey: "priority",
								Condition:   apimodel.FilterConditionGte,
								Value:       7,
							},
							{
								PropertyKey: "tags",
								Condition:   apimodel.FilterConditionIn,
								Value:       []string{"urgent", "critical"},
							},
						},
					},
				},
			},
			setupMock: func(m *mock_filter.MockApiService) {
				propertyMap := map[string]*apimodel.Property{
					"is_archived": {
						Key:         "is_archived",
						RelationKey: bundle.RelationKeyIsArchived.String(),
						Format:      apimodel.PropertyFormatCheckbox,
					},
					"priority": {
						Key:         "priority",
						RelationKey: bundle.RelationKeyPriority.String(),
						Format:      apimodel.PropertyFormatNumber,
					},
					"tags": {
						Key:         "tags",
						RelationKey: bundle.RelationKeyTag.String(),
						Format:      apimodel.PropertyFormatMultiSelect,
					},
				}
				m.On("GetCachedProperties", spaceId).Return(propertyMap)
				m.On("ResolvePropertyApiKey", propertyMap, "is_archived").Return("is_archived")
				m.On("ResolvePropertyApiKey", propertyMap, "priority").Return("priority")
				m.On("ResolvePropertyApiKey", propertyMap, "tags").Return("tags")
				m.On("SanitizeAndValidatePropertyValue", spaceId, "is_archived", apimodel.PropertyFormatCheckbox, true, mock.Anything, propertyMap).Return(true, nil)
				m.On("SanitizeAndValidatePropertyValue", spaceId, "priority", apimodel.PropertyFormatNumber, 7, mock.Anything, propertyMap).Return(float64(7), nil)
				m.On("SanitizeAndValidatePropertyValue", spaceId, "tags", apimodel.PropertyFormatMultiSelect, []string{"urgent", "critical"}, mock.Anything, propertyMap).Return([]string{"urgent", "critical"}, nil)
			},
			checkResult: func(t *testing.T, result *model.BlockContentDataviewFilter) {
				require.NotNil(t, result)
				assert.Equal(t, model.BlockContentDataviewFilter_And, result.Operator)
				assert.Len(t, result.NestedFilters, 2)

				// Check first filter (is_archived != true)
				assert.Equal(t, bundle.RelationKeyIsArchived.String(), result.NestedFilters[0].RelationKey)
				assert.Equal(t, model.BlockContentDataviewFilter_NotEqual, result.NestedFilters[0].Condition)

				// Check nested OR filter
				orFilter := result.NestedFilters[1]
				assert.Equal(t, model.BlockContentDataviewFilter_Or, orFilter.Operator)
				assert.Len(t, orFilter.NestedFilters, 2)
			},
		},
		{
			name: "empty condition filter",
			expr: &apimodel.FilterExpression{
				Conditions: []apimodel.FilterItem{
					{
						PropertyKey: "description",
						Condition:   apimodel.FilterConditionEmpty,
					},
				},
			},
			setupMock: func(m *mock_filter.MockApiService) {
				propertyMap := map[string]*apimodel.Property{
					"description": {
						Key:         "description",
						RelationKey: bundle.RelationKeyDescription.String(),
						Format:      apimodel.PropertyFormatText,
					},
				}
				m.On("GetCachedProperties", spaceId).Return(propertyMap)
				m.On("ResolvePropertyApiKey", propertyMap, "description").Return("description")
			},
			checkResult: func(t *testing.T, result *model.BlockContentDataviewFilter) {
				require.NotNil(t, result)
				assert.Equal(t, bundle.RelationKeyDescription.String(), result.RelationKey)
				assert.Equal(t, model.BlockContentDataviewFilter_Empty, result.Condition)
				assert.Nil(t, result.Value)
			},
		},
		{
			name: "exists condition filter",
			expr: &apimodel.FilterExpression{
				Conditions: []apimodel.FilterItem{
					{
						PropertyKey: "due_date",
						Condition:   apimodel.FilterConditionExists,
					},
				},
			},
			setupMock: func(m *mock_filter.MockApiService) {
				propertyMap := map[string]*apimodel.Property{
					"due_date": {
						Key:         "due_date",
						RelationKey: bundle.RelationKeyDueDate.String(),
						Format:      apimodel.PropertyFormatDate,
					},
				}
				m.On("GetCachedProperties", spaceId).Return(propertyMap)
				m.On("ResolvePropertyApiKey", propertyMap, "due_date").Return("due_date")
			},
			checkResult: func(t *testing.T, result *model.BlockContentDataviewFilter) {
				require.NotNil(t, result)
				assert.Equal(t, bundle.RelationKeyDueDate.String(), result.RelationKey)
				assert.Equal(t, model.BlockContentDataviewFilter_Exists, result.Condition)
				assert.Nil(t, result.Value)
			},
		},
		{
			name: "deeply nested filters",
			expr: &apimodel.FilterExpression{
				Operator: apimodel.FilterOperatorAnd,
				Filters: []apimodel.FilterExpression{
					{
						Operator: apimodel.FilterOperatorOr,
						Filters: []apimodel.FilterExpression{
							{
								Operator: apimodel.FilterOperatorAnd,
								Conditions: []apimodel.FilterItem{
									{
										PropertyKey: "priority",
										Condition:   apimodel.FilterConditionGt,
										Value:       5,
									},
								},
							},
						},
					},
				},
			},
			setupMock: func(m *mock_filter.MockApiService) {
				propertyMap := map[string]*apimodel.Property{
					"priority": {
						Key:         "priority",
						RelationKey: bundle.RelationKeyPriority.String(),
						Format:      apimodel.PropertyFormatNumber,
					},
				}
				m.On("GetCachedProperties", spaceId).Return(propertyMap)
				m.On("ResolvePropertyApiKey", propertyMap, "priority").Return("priority")
				m.On("SanitizeAndValidatePropertyValue", spaceId, "priority", apimodel.PropertyFormatNumber, 5, mock.Anything, propertyMap).Return(float64(5), nil)
			},
			checkResult: func(t *testing.T, result *model.BlockContentDataviewFilter) {
				require.NotNil(t, result)
				assert.Equal(t, model.BlockContentDataviewFilter_And, result.Operator)
				assert.Len(t, result.NestedFilters, 1)

				// Check first level OR
				orFilter := result.NestedFilters[0]
				assert.Equal(t, model.BlockContentDataviewFilter_Or, orFilter.Operator)
				assert.Len(t, orFilter.NestedFilters, 1)

				// Check second level AND
				andFilter := orFilter.NestedFilters[0]
				assert.Equal(t, model.BlockContentDataviewFilter_And, andFilter.Operator)
				assert.Len(t, andFilter.NestedFilters, 1)

				// Check the actual condition
				condition := andFilter.NestedFilters[0]
				assert.Equal(t, bundle.RelationKeyPriority.String(), condition.RelationKey)
				assert.Equal(t, model.BlockContentDataviewFilter_Greater, condition.Condition)
			},
		},
		{
			name: "invalid property key",
			expr: &apimodel.FilterExpression{
				Conditions: []apimodel.FilterItem{
					{
						PropertyKey: "invalid_prop",
						Condition:   apimodel.FilterConditionEq,
						Value:       "test",
					},
				},
			},
			setupMock: func(m *mock_filter.MockApiService) {
				m.On("GetCachedProperties", spaceId).Return(map[string]*apimodel.Property{})
				m.On("ResolvePropertyApiKey", mock.Anything, "invalid_prop").Return("")
			},
			expectedError: "failed to build condition filter: failed to resolve property invalid_prop",
		},
		{
			name: "unsupported condition",
			expr: &apimodel.FilterExpression{
				Conditions: []apimodel.FilterItem{
					{
						PropertyKey: "status",
						Condition:   "unsupported",
					},
				},
			},
			setupMock: func(m *mock_filter.MockApiService) {
				// No setup needed as it should fail before property resolution
			},
			expectedError: "failed to build condition filter: unsupported filter condition: unsupported",
		},
		{
			name: "all supported conditions test",
			expr: &apimodel.FilterExpression{
				Operator: apimodel.FilterOperatorAnd,
				Conditions: []apimodel.FilterItem{
					{PropertyKey: "p1", Condition: apimodel.FilterConditionEq, Value: "v1"},
					{PropertyKey: "p2", Condition: apimodel.FilterConditionNe, Value: "v2"},
					{PropertyKey: "p3", Condition: apimodel.FilterConditionGt, Value: 5},
					{PropertyKey: "p4", Condition: apimodel.FilterConditionGte, Value: 10},
					{PropertyKey: "p5", Condition: apimodel.FilterConditionLt, Value: 20},
					{PropertyKey: "p6", Condition: apimodel.FilterConditionLte, Value: 30},
					{PropertyKey: "p7", Condition: apimodel.FilterConditionContains, Value: "text"},
					{PropertyKey: "p8", Condition: apimodel.FilterConditionNContains, Value: "exclude"},
					{PropertyKey: "p9", Condition: apimodel.FilterConditionIn, Value: []string{"a", "b"}},
					{PropertyKey: "p10", Condition: apimodel.FilterConditionNin, Value: []string{"c", "d"}},
					{PropertyKey: "p11", Condition: apimodel.FilterConditionAll, Value: []string{"e", "f"}},
					{PropertyKey: "p12", Condition: apimodel.FilterConditionNone, Value: []string{"g", "h"}},
					{PropertyKey: "p13", Condition: apimodel.FilterConditionExactIn, Value: []string{"i", "j"}},
					{PropertyKey: "p14", Condition: apimodel.FilterConditionNotExactIn, Value: []string{"k", "l"}},
					{PropertyKey: "p15", Condition: apimodel.FilterConditionExists},
					{PropertyKey: "p16", Condition: apimodel.FilterConditionEmpty},
					{PropertyKey: "p17", Condition: apimodel.FilterConditionNEmpty},
				},
			},
			setupMock: func(m *mock_filter.MockApiService) {
				propertyMap := map[string]*apimodel.Property{
					"p1":  {Key: "p1", RelationKey: bundle.RelationKeyName.String(), Format: apimodel.PropertyFormatText},
					"p2":  {Key: "p2", RelationKey: bundle.RelationKeyDescription.String(), Format: apimodel.PropertyFormatText},
					"p3":  {Key: "p3", RelationKey: bundle.RelationKeyPriority.String(), Format: apimodel.PropertyFormatNumber},
					"p4":  {Key: "p4", RelationKey: bundle.RelationKeyProgress.String(), Format: apimodel.PropertyFormatNumber},
					"p5":  {Key: "p5", RelationKey: bundle.RelationKeyCreatedDate.String(), Format: apimodel.PropertyFormatNumber},
					"p6":  {Key: "p6", RelationKey: bundle.RelationKeyLastModifiedDate.String(), Format: apimodel.PropertyFormatNumber},
					"p7":  {Key: "p7", RelationKey: bundle.RelationKeySnippet.String(), Format: apimodel.PropertyFormatText},
					"p8":  {Key: "p8", RelationKey: bundle.RelationKeySource.String(), Format: apimodel.PropertyFormatText},
					"p9":  {Key: "p9", RelationKey: bundle.RelationKeyTag.String(), Format: apimodel.PropertyFormatMultiSelect},
					"p10": {Key: "p10", RelationKey: bundle.RelationKeyAssignee.String(), Format: apimodel.PropertyFormatMultiSelect},
					"p11": {Key: "p11", RelationKey: bundle.RelationKeyLinks.String(), Format: apimodel.PropertyFormatMultiSelect},
					"p12": {Key: "p12", RelationKey: bundle.RelationKeyBacklinks.String(), Format: apimodel.PropertyFormatMultiSelect},
					"p13": {Key: "p13", RelationKey: bundle.RelationKeySetOf.String(), Format: apimodel.PropertyFormatMultiSelect},
					"p14": {Key: "p14", RelationKey: bundle.RelationKeyFeaturedRelations.String(), Format: apimodel.PropertyFormatMultiSelect},
					"p15": {Key: "p15", RelationKey: bundle.RelationKeyDone.String(), Format: apimodel.PropertyFormatCheckbox},
					"p16": {Key: "p16", RelationKey: bundle.RelationKeyIsArchived.String(), Format: apimodel.PropertyFormatCheckbox},
					"p17": {Key: "p17", RelationKey: bundle.RelationKeyIsHidden.String(), Format: apimodel.PropertyFormatCheckbox},
				}

				m.On("GetCachedProperties", spaceId).Return(propertyMap)

				// Mock all property resolutions
				for i := 1; i <= 17; i++ {
					key := fmt.Sprintf("p%d", i)
					m.On("ResolvePropertyApiKey", propertyMap, key).Return(key)
				}

				// Mock value validations for properties with values
				m.On("SanitizeAndValidatePropertyValue", spaceId, "p1", apimodel.PropertyFormatText, "v1", mock.Anything, propertyMap).Return("v1", nil)
				m.On("SanitizeAndValidatePropertyValue", spaceId, "p2", apimodel.PropertyFormatText, "v2", mock.Anything, propertyMap).Return("v2", nil)
				m.On("SanitizeAndValidatePropertyValue", spaceId, "p3", apimodel.PropertyFormatNumber, 5, mock.Anything, propertyMap).Return(float64(5), nil)
				m.On("SanitizeAndValidatePropertyValue", spaceId, "p4", apimodel.PropertyFormatNumber, 10, mock.Anything, propertyMap).Return(float64(10), nil)
				m.On("SanitizeAndValidatePropertyValue", spaceId, "p5", apimodel.PropertyFormatNumber, 20, mock.Anything, propertyMap).Return(float64(20), nil)
				m.On("SanitizeAndValidatePropertyValue", spaceId, "p6", apimodel.PropertyFormatNumber, 30, mock.Anything, propertyMap).Return(float64(30), nil)
				m.On("SanitizeAndValidatePropertyValue", spaceId, "p7", apimodel.PropertyFormatText, "text", mock.Anything, propertyMap).Return("text", nil)
				m.On("SanitizeAndValidatePropertyValue", spaceId, "p8", apimodel.PropertyFormatText, "exclude", mock.Anything, propertyMap).Return("exclude", nil)
				m.On("SanitizeAndValidatePropertyValue", spaceId, "p9", apimodel.PropertyFormatMultiSelect, []string{"a", "b"}, mock.Anything, propertyMap).Return([]string{"a", "b"}, nil)
				m.On("SanitizeAndValidatePropertyValue", spaceId, "p10", apimodel.PropertyFormatMultiSelect, []string{"c", "d"}, mock.Anything, propertyMap).Return([]string{"c", "d"}, nil)
				m.On("SanitizeAndValidatePropertyValue", spaceId, "p11", apimodel.PropertyFormatMultiSelect, []string{"e", "f"}, mock.Anything, propertyMap).Return([]string{"e", "f"}, nil)
				m.On("SanitizeAndValidatePropertyValue", spaceId, "p12", apimodel.PropertyFormatMultiSelect, []string{"g", "h"}, mock.Anything, propertyMap).Return([]string{"g", "h"}, nil)
				m.On("SanitizeAndValidatePropertyValue", spaceId, "p13", apimodel.PropertyFormatMultiSelect, []string{"i", "j"}, mock.Anything, propertyMap).Return([]string{"i", "j"}, nil)
				m.On("SanitizeAndValidatePropertyValue", spaceId, "p14", apimodel.PropertyFormatMultiSelect, []string{"k", "l"}, mock.Anything, propertyMap).Return([]string{"k", "l"}, nil)
			},
			checkResult: func(t *testing.T, result *model.BlockContentDataviewFilter) {
				require.NotNil(t, result)
				assert.Equal(t, model.BlockContentDataviewFilter_And, result.Operator)
				assert.Len(t, result.NestedFilters, 17)

				// Check that all conditions are properly mapped
				conditionChecks := map[int]model.BlockContentDataviewFilterCondition{
					0:  model.BlockContentDataviewFilter_Equal,
					1:  model.BlockContentDataviewFilter_NotEqual,
					2:  model.BlockContentDataviewFilter_Greater,
					3:  model.BlockContentDataviewFilter_GreaterOrEqual,
					4:  model.BlockContentDataviewFilter_Less,
					5:  model.BlockContentDataviewFilter_LessOrEqual,
					6:  model.BlockContentDataviewFilter_Like,
					7:  model.BlockContentDataviewFilter_NotLike,
					8:  model.BlockContentDataviewFilter_In,
					9:  model.BlockContentDataviewFilter_NotIn,
					10: model.BlockContentDataviewFilter_AllIn,
					11: model.BlockContentDataviewFilter_NotAllIn,
					12: model.BlockContentDataviewFilter_ExactIn,
					13: model.BlockContentDataviewFilter_NotExactIn,
					14: model.BlockContentDataviewFilter_Exists,
					15: model.BlockContentDataviewFilter_Empty,
					16: model.BlockContentDataviewFilter_NotEmpty,
				}

				for i, expectedCond := range conditionChecks {
					assert.Equal(t, expectedCond, result.NestedFilters[i].Condition,
						"Filter at index %d should have condition %v", i, expectedCond)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockService := mock_filter.NewMockApiService(t)
			tt.setupMock(mockService)

			validator := &Validator{apiService: mockService}

			result, err := BuildExpressionFilters(ctx, tt.expr, validator, spaceId)

			if tt.expectedError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				assert.NoError(t, err)
				tt.checkResult(t, result)
			}

			mockService.AssertExpectations(t)
		})
	}
}

func TestConditionMap(t *testing.T) {
	// Test that string values map correctly to internal conditions
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
		"none":      model.BlockContentDataviewFilter_NotAllIn,
		"exactin":   model.BlockContentDataviewFilter_ExactIn,
		"nexactin":  model.BlockContentDataviewFilter_NotExactIn,
		"exists":    model.BlockContentDataviewFilter_Exists,
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
	assert.Equal(t, "none", apimodel.FilterConditionNone.String())
	assert.Equal(t, "exactin", apimodel.FilterConditionExactIn.String())
	assert.Equal(t, "nexactin", apimodel.FilterConditionNotExactIn.String())
	assert.Equal(t, "exists", apimodel.FilterConditionExists.String())
	assert.Equal(t, "empty", apimodel.FilterConditionEmpty.String())
	assert.Equal(t, "nempty", apimodel.FilterConditionNEmpty.String())
}

func TestOperatorMap(t *testing.T) {
	// Verify all operators are mapped
	assert.Equal(t, model.BlockContentDataviewFilter_And, OperatorMap[apimodel.FilterOperatorAnd])
	assert.Equal(t, model.BlockContentDataviewFilter_Or, OperatorMap[apimodel.FilterOperatorOr])
}
