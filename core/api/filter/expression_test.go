package filter

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/api/filter/mock_filter"
	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/api/util"
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
						WrappedFilterItem: apimodel.CheckboxFilterItem{
							PropertyKey: "done",
							Condition:   apimodel.FilterConditionEq,
							Checkbox:    util.PtrBool(true),
						},
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
				}
				m.On("GetCachedProperties", spaceId).Return(propertyMap)
				m.On("ResolvePropertyApiKey", propertyMap, "done").Return("done", true)
				m.On("SanitizeAndValidatePropertyValue", spaceId, "done", true, propertyMap["done"], propertyMap).Return(true, nil)
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
						WrappedFilterItem: apimodel.CheckboxFilterItem{
							PropertyKey: "done",
							Condition:   apimodel.FilterConditionEq,
							Checkbox:    util.PtrBool(true),
						},
					},
					{
						WrappedFilterItem: apimodel.NumberFilterItem{
							PropertyKey: "priority",
							Condition:   apimodel.FilterConditionGt,
							Number:      util.PtrFloat64(5),
						},
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
				m.On("ResolvePropertyApiKey", propertyMap, "done").Return("done", true)
				m.On("ResolvePropertyApiKey", propertyMap, "priority").Return("priority", true)
				m.On("SanitizeAndValidatePropertyValue", spaceId, "done", true, propertyMap["done"], propertyMap).Return(true, nil)
				m.On("SanitizeAndValidatePropertyValue", spaceId, "priority", float64(5), propertyMap["priority"], propertyMap).Return(float64(5), nil)
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
						WrappedFilterItem: apimodel.TextFilterItem{
							PropertyKey: "type",
							Condition:   apimodel.FilterConditionEq,
							Text:        util.PtrString("page"),
						},
					},
					{
						WrappedFilterItem: apimodel.TextFilterItem{
							PropertyKey: "type",
							Condition:   apimodel.FilterConditionEq,
							Text:        util.PtrString("task"),
						},
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
				m.On("ResolvePropertyApiKey", propertyMap, "type").Return("type", true)
				m.On("SanitizeAndValidatePropertyValue", spaceId, "type", "page", propertyMap["type"], propertyMap).Return("page", nil)
				m.On("SanitizeAndValidatePropertyValue", spaceId, "type", "task", propertyMap["type"], propertyMap).Return("task", nil)
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
						WrappedFilterItem: apimodel.CheckboxFilterItem{
							PropertyKey: "is_archived",
							Condition:   apimodel.FilterConditionNe,
							Checkbox:    util.PtrBool(true),
						},
					},
				},
				Filters: []apimodel.FilterExpression{
					{
						Operator: apimodel.FilterOperatorOr,
						Conditions: []apimodel.FilterItem{
							{
								WrappedFilterItem: apimodel.NumberFilterItem{
									PropertyKey: "priority",
									Condition:   apimodel.FilterConditionGte,
									Number:      util.PtrFloat64(7),
								},
							},
							{
								WrappedFilterItem: apimodel.MultiSelectFilterItem{
									PropertyKey: "tags",
									Condition:   apimodel.FilterConditionIn,
									MultiSelect: util.PtrStrings([]string{"urgent", "critical"}),
								},
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
				m.On("ResolvePropertyApiKey", propertyMap, "is_archived").Return("is_archived", true)
				m.On("ResolvePropertyApiKey", propertyMap, "priority").Return("priority", true)
				m.On("ResolvePropertyApiKey", propertyMap, "tags").Return("tags", true)
				m.On("SanitizeAndValidatePropertyValue", spaceId, "is_archived", true, propertyMap["is_archived"], propertyMap).Return(true, nil)
				m.On("SanitizeAndValidatePropertyValue", spaceId, "priority", float64(7), propertyMap["priority"], propertyMap).Return(float64(7), nil)
				m.On("SanitizeAndValidatePropertyValue", spaceId, "tags", []string{"urgent", "critical"}, propertyMap["tags"], propertyMap).Return([]string{"urgent", "critical"}, nil)
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
						WrappedFilterItem: apimodel.EmptyFilterItem{
							PropertyKey: "description",
							Condition:   apimodel.FilterConditionEmpty,
						},
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
				m.On("ResolvePropertyApiKey", propertyMap, "description").Return("description", true)
			},
			checkResult: func(t *testing.T, result *model.BlockContentDataviewFilter) {
				require.NotNil(t, result)
				assert.Equal(t, bundle.RelationKeyDescription.String(), result.RelationKey)
				assert.Equal(t, model.BlockContentDataviewFilter_Empty, result.Condition)
				assert.Nil(t, result.Value)
			},
		},
		{
			name: "date filters with RFC3339 and date-only formats",
			expr: &apimodel.FilterExpression{
				Operator: apimodel.FilterOperatorAnd,
				Conditions: []apimodel.FilterItem{
					{
						WrappedFilterItem: apimodel.DateFilterItem{
							PropertyKey: "created_date",
							Condition:   apimodel.FilterConditionGte,
							Date:        util.PtrString("2024-01-01"),
						},
					},
					{
						WrappedFilterItem: apimodel.DateFilterItem{
							PropertyKey: "due_date",
							Condition:   apimodel.FilterConditionLt,
							Date:        util.PtrString("2024-12-31T23:59:59Z"),
						},
					},
				},
			},
			setupMock: func(m *mock_filter.MockApiService) {
				propertyMap := map[string]*apimodel.Property{
					"created_date": {
						Key:         "created_date",
						RelationKey: bundle.RelationKeyCreatedDate.String(),
						Format:      apimodel.PropertyFormatDate,
					},
					"due_date": {
						Key:         "due_date",
						RelationKey: bundle.RelationKeyDueDate.String(),
						Format:      apimodel.PropertyFormatDate,
					},
				}
				m.On("GetCachedProperties", spaceId).Return(propertyMap)
				m.On("ResolvePropertyApiKey", propertyMap, "created_date").Return("created_date", true)
				m.On("ResolvePropertyApiKey", propertyMap, "due_date").Return("due_date", true)
				// The service should accept both date formats and convert them to timestamps
				m.On("SanitizeAndValidatePropertyValue", spaceId, "created_date", "2024-01-01", propertyMap["created_date"], propertyMap).Return(float64(1704067200), nil)   // 2024-01-01 00:00:00 UTC
				m.On("SanitizeAndValidatePropertyValue", spaceId, "due_date", "2024-12-31T23:59:59Z", propertyMap["due_date"], propertyMap).Return(float64(1735689599), nil) // 2024-12-31 23:59:59 UTC
			},
			checkResult: func(t *testing.T, result *model.BlockContentDataviewFilter) {
				require.NotNil(t, result)
				assert.Equal(t, model.BlockContentDataviewFilter_And, result.Operator)
				assert.Len(t, result.NestedFilters, 2)

				// Check first filter (created_date >= 2024-01-01)
				assert.Equal(t, bundle.RelationKeyCreatedDate.String(), result.NestedFilters[0].RelationKey)
				assert.Equal(t, model.BlockContentDataviewFilter_GreaterOrEqual, result.NestedFilters[0].Condition)
				assert.Equal(t, pbtypes.Float64(1704067200), result.NestedFilters[0].Value)

				// Check second filter (due_date < 2024-12-31T23:59:59Z)
				assert.Equal(t, bundle.RelationKeyDueDate.String(), result.NestedFilters[1].RelationKey)
				assert.Equal(t, model.BlockContentDataviewFilter_Less, result.NestedFilters[1].Condition)
				assert.Equal(t, pbtypes.Float64(1735689599), result.NestedFilters[1].Value)
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
										WrappedFilterItem: apimodel.NumberFilterItem{
											PropertyKey: "priority",
											Condition:   apimodel.FilterConditionGt,
											Number:      util.PtrFloat64(5),
										},
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
				m.On("ResolvePropertyApiKey", propertyMap, "priority").Return("priority", true)
				m.On("SanitizeAndValidatePropertyValue", spaceId, "priority", float64(5), propertyMap["priority"], propertyMap).Return(float64(5), nil)
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
			name: "invalid condition for property type",
			expr: &apimodel.FilterExpression{
				Conditions: []apimodel.FilterItem{
					{
						WrappedFilterItem: apimodel.TextFilterItem{
							PropertyKey: "name",
							Condition:   apimodel.FilterConditionGt, // Greater than is invalid for text
							Text:        util.PtrString("test"),
						},
					},
				},
			},
			setupMock: func(m *mock_filter.MockApiService) {
				propertyMap := map[string]*apimodel.Property{
					"name": {
						Key:         "name",
						RelationKey: bundle.RelationKeyName.String(),
						Format:      apimodel.PropertyFormatText,
					},
				}
				m.On("GetCachedProperties", spaceId).Return(propertyMap)
				m.On("ResolvePropertyApiKey", propertyMap, "name").Return("name", true)
			},
			expectedError: "failed to build condition filter: bad input: condition \"gt\" is not valid for property type \"text\"",
		},
		{
			name: "valid array condition for multi-select property",
			expr: &apimodel.FilterExpression{
				Conditions: []apimodel.FilterItem{
					{
						WrappedFilterItem: apimodel.MultiSelectFilterItem{
							PropertyKey: "tags",
							Condition:   apimodel.FilterConditionAll, // AllIn is valid for multi-select
							MultiSelect: util.PtrStrings([]string{"important", "urgent"}),
						},
					},
				},
			},
			setupMock: func(m *mock_filter.MockApiService) {
				propertyMap := map[string]*apimodel.Property{
					"tags": {
						Key:         "tags",
						RelationKey: "tags",
						Format:      apimodel.PropertyFormatMultiSelect,
					},
				}
				m.On("GetCachedProperties", spaceId).Return(propertyMap)
				m.On("ResolvePropertyApiKey", propertyMap, "tags").Return("tags", true)
				m.On("SanitizeAndValidatePropertyValue", spaceId, "tags", []string{"important", "urgent"}, propertyMap["tags"], propertyMap).Return([]string{"important", "urgent"}, nil)
			},
			checkResult: func(t *testing.T, result *model.BlockContentDataviewFilter) {
				require.NotNil(t, result)
				assert.Equal(t, "tags", result.RelationKey)
				assert.Equal(t, model.BlockContentDataviewFilter_AllIn, result.Condition)
			},
		},
		{
			name: "invalid property key",
			expr: &apimodel.FilterExpression{
				Conditions: []apimodel.FilterItem{
					{
						WrappedFilterItem: apimodel.TextFilterItem{
							PropertyKey: "invalid_prop",
							Condition:   apimodel.FilterConditionEq,
							Text:        util.PtrString("test"),
						},
					},
				},
			},
			setupMock: func(m *mock_filter.MockApiService) {
				m.On("GetCachedProperties", spaceId).Return(map[string]*apimodel.Property{})
				m.On("ResolvePropertyApiKey", mock.Anything, "invalid_prop").Return("", false)
			},
			expectedError: "failed to build condition filter: failed to resolve property \"invalid_prop\": bad input: property \"invalid_prop\" not found",
		},
		{
			name: "empty expression with only operator",
			expr: &apimodel.FilterExpression{
				Operator: apimodel.FilterOperatorAnd,
				// No conditions or filters
			},
			setupMock: func(m *mock_filter.MockApiService) {
				// No setup needed
			},
			checkResult: func(t *testing.T, result *model.BlockContentDataviewFilter) {
				assert.Nil(t, result)
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
