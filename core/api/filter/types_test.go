package filter

import (
	"testing"

	st "github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestParsedFilters_ToDataviewFilters(t *testing.T) {
	tests := []struct {
		name           string
		parsedFilters  *ParsedFilters
		expectedResult []*model.BlockContentDataviewFilter
	}{
		{
			name:           "nil parsed filters",
			parsedFilters:  nil,
			expectedResult: nil,
		},
		{
			name: "empty filters",
			parsedFilters: &ParsedFilters{
				Filters: []Filter{},
			},
			expectedResult: nil,
		},
		{
			name: "single text filter",
			parsedFilters: &ParsedFilters{
				Filters: []Filter{
					{
						PropertyKey: "name",
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       "test",
					},
				},
			},
			expectedResult: []*model.BlockContentDataviewFilter{
				{
					RelationKey: "name",
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value: &st.Value{
						Kind: &st.Value_StringValue{
							StringValue: "test",
						},
					},
				},
			},
		},
		{
			name: "multiple filters",
			parsedFilters: &ParsedFilters{
				Filters: []Filter{
					{
						PropertyKey: "name",
						Condition:   model.BlockContentDataviewFilter_Like,
						Value:       "test",
					},
					{
						PropertyKey: "age",
						Condition:   model.BlockContentDataviewFilter_Greater,
						Value:       float64(25),
					},
					{
						PropertyKey: "tags",
						Condition:   model.BlockContentDataviewFilter_In,
						Value:       []string{"todo", "done"},
					},
				},
			},
			expectedResult: []*model.BlockContentDataviewFilter{
				{
					RelationKey: "name",
					Condition:   model.BlockContentDataviewFilter_Like,
					Value: &st.Value{
						Kind: &st.Value_StringValue{
							StringValue: "test",
						},
					},
				},
				{
					RelationKey: "age",
					Condition:   model.BlockContentDataviewFilter_Greater,
					Value: &st.Value{
						Kind: &st.Value_NumberValue{
							NumberValue: float64(25),
						},
					},
				},
				{
					RelationKey: "tags",
					Condition:   model.BlockContentDataviewFilter_In,
					Value: &st.Value{
						Kind: &st.Value_ListValue{
							ListValue: &st.ListValue{
								Values: []*st.Value{
									{Kind: &st.Value_StringValue{StringValue: "todo"}},
									{Kind: &st.Value_StringValue{StringValue: "done"}},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "filter with boolean value",
			parsedFilters: &ParsedFilters{
				Filters: []Filter{
					{
						PropertyKey: "is_active",
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       true,
					},
				},
			},
			expectedResult: []*model.BlockContentDataviewFilter{
				{
					RelationKey: "is_active",
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value: &st.Value{
						Kind: &st.Value_BoolValue{
							BoolValue: true,
						},
					},
				},
			},
		},
		{
			name: "filter with empty condition",
			parsedFilters: &ParsedFilters{
				Filters: []Filter{
					{
						PropertyKey: "description",
						Condition:   model.BlockContentDataviewFilter_Empty,
						Value:       true,
					},
				},
			},
			expectedResult: []*model.BlockContentDataviewFilter{
				{
					RelationKey: "description",
					Condition:   model.BlockContentDataviewFilter_Empty,
					Value: &st.Value{
						Kind: &st.Value_BoolValue{
							BoolValue: true,
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.parsedFilters.ToDataviewFilters()

			if tt.expectedResult == nil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.Len(t, result, len(tt.expectedResult))

				// Check all properties including values
				for i, expected := range tt.expectedResult {
					assert.Equal(t, expected.RelationKey, result[i].RelationKey)
					assert.Equal(t, expected.Condition, result[i].Condition)
					assert.Equal(t, expected.Value, result[i].Value)
				}
			}
		})
	}
}

func TestIsValidConditionForType(t *testing.T) {
	tests := []struct {
		name      string
		format    apimodel.PropertyFormat
		condition model.BlockContentDataviewFilterCondition
		expected  bool
	}{
		// Text format tests
		{
			name:      "text with equal",
			format:    apimodel.PropertyFormatText,
			condition: model.BlockContentDataviewFilter_Equal,
			expected:  true,
		},
		{
			name:      "text with like",
			format:    apimodel.PropertyFormatText,
			condition: model.BlockContentDataviewFilter_Like,
			expected:  true,
		},
		{
			name:      "text with greater (invalid)",
			format:    apimodel.PropertyFormatText,
			condition: model.BlockContentDataviewFilter_Greater,
			expected:  false,
		},
		{
			name:      "text with in (invalid)",
			format:    apimodel.PropertyFormatText,
			condition: model.BlockContentDataviewFilter_In,
			expected:  false,
		},
		// Number format tests
		{
			name:      "number with equal",
			format:    apimodel.PropertyFormatNumber,
			condition: model.BlockContentDataviewFilter_Equal,
			expected:  true,
		},
		{
			name:      "number with greater",
			format:    apimodel.PropertyFormatNumber,
			condition: model.BlockContentDataviewFilter_Greater,
			expected:  true,
		},
		{
			name:      "number with less or equal",
			format:    apimodel.PropertyFormatNumber,
			condition: model.BlockContentDataviewFilter_LessOrEqual,
			expected:  true,
		},
		{
			name:      "number with like (invalid)",
			format:    apimodel.PropertyFormatNumber,
			condition: model.BlockContentDataviewFilter_Like,
			expected:  false,
		},
		// Checkbox format tests
		{
			name:      "checkbox with equal",
			format:    apimodel.PropertyFormatCheckbox,
			condition: model.BlockContentDataviewFilter_Equal,
			expected:  true,
		},
		{
			name:      "checkbox with not equal",
			format:    apimodel.PropertyFormatCheckbox,
			condition: model.BlockContentDataviewFilter_NotEqual,
			expected:  true,
		},
		{
			name:      "checkbox with exists",
			format:    apimodel.PropertyFormatCheckbox,
			condition: model.BlockContentDataviewFilter_Exists,
			expected:  true,
		},
		{
			name:      "checkbox with greater (invalid)",
			format:    apimodel.PropertyFormatCheckbox,
			condition: model.BlockContentDataviewFilter_Greater,
			expected:  false,
		},
		// Select format tests
		{
			name:      "select with equal",
			format:    apimodel.PropertyFormatSelect,
			condition: model.BlockContentDataviewFilter_Equal,
			expected:  true,
		},
		{
			name:      "select with in",
			format:    apimodel.PropertyFormatSelect,
			condition: model.BlockContentDataviewFilter_In,
			expected:  true,
		},
		{
			name:      "select with not in",
			format:    apimodel.PropertyFormatSelect,
			condition: model.BlockContentDataviewFilter_NotIn,
			expected:  true,
		},
		{
			name:      "select with like (invalid)",
			format:    apimodel.PropertyFormatSelect,
			condition: model.BlockContentDataviewFilter_Like,
			expected:  false,
		},
		// MultiSelect format tests
		{
			name:      "multi-select with in",
			format:    apimodel.PropertyFormatMultiSelect,
			condition: model.BlockContentDataviewFilter_In,
			expected:  true,
		},
		{
			name:      "multi-select with not in",
			format:    apimodel.PropertyFormatMultiSelect,
			condition: model.BlockContentDataviewFilter_NotIn,
			expected:  true,
		},
		{
			name:      "multi-select with equal (invalid)",
			format:    apimodel.PropertyFormatMultiSelect,
			condition: model.BlockContentDataviewFilter_Equal,
			expected:  false,
		},
		// Date format tests
		{
			name:      "date with equal",
			format:    apimodel.PropertyFormatDate,
			condition: model.BlockContentDataviewFilter_Equal,
			expected:  true,
		},
		{
			name:      "date with greater",
			format:    apimodel.PropertyFormatDate,
			condition: model.BlockContentDataviewFilter_Greater,
			expected:  true,
		},
		{
			name:      "date with less",
			format:    apimodel.PropertyFormatDate,
			condition: model.BlockContentDataviewFilter_Less,
			expected:  true,
		},
		{
			name:      "date with like (invalid)",
			format:    apimodel.PropertyFormatDate,
			condition: model.BlockContentDataviewFilter_Like,
			expected:  false,
		},
		// URL format tests
		{
			name:      "url with equal",
			format:    apimodel.PropertyFormatUrl,
			condition: model.BlockContentDataviewFilter_Equal,
			expected:  true,
		},
		{
			name:      "url with like",
			format:    apimodel.PropertyFormatUrl,
			condition: model.BlockContentDataviewFilter_Like,
			expected:  true,
		},
		{
			name:      "url with greater (invalid)",
			format:    apimodel.PropertyFormatUrl,
			condition: model.BlockContentDataviewFilter_Greater,
			expected:  false,
		},
		// Email format tests
		{
			name:      "email with equal",
			format:    apimodel.PropertyFormatEmail,
			condition: model.BlockContentDataviewFilter_Equal,
			expected:  true,
		},
		{
			name:      "email with like",
			format:    apimodel.PropertyFormatEmail,
			condition: model.BlockContentDataviewFilter_Like,
			expected:  true,
		},
		// Phone format tests
		{
			name:      "phone with equal",
			format:    apimodel.PropertyFormatPhone,
			condition: model.BlockContentDataviewFilter_Equal,
			expected:  true,
		},
		{
			name:      "phone with like",
			format:    apimodel.PropertyFormatPhone,
			condition: model.BlockContentDataviewFilter_Like,
			expected:  true,
		},
		// Files format tests
		{
			name:      "files with in",
			format:    apimodel.PropertyFormatFiles,
			condition: model.BlockContentDataviewFilter_In,
			expected:  true,
		},
		{
			name:      "files with equal (invalid)",
			format:    apimodel.PropertyFormatFiles,
			condition: model.BlockContentDataviewFilter_Equal,
			expected:  false,
		},
		// Objects format tests
		{
			name:      "objects with in",
			format:    apimodel.PropertyFormatObjects,
			condition: model.BlockContentDataviewFilter_In,
			expected:  true,
		},
		{
			name:      "objects with equal (invalid)",
			format:    apimodel.PropertyFormatObjects,
			condition: model.BlockContentDataviewFilter_Equal,
			expected:  false,
		},
		// Unknown format tests
		{
			name:      "unknown format",
			format:    apimodel.PropertyFormat("unknown"),
			condition: model.BlockContentDataviewFilter_Equal,
			expected:  false,
		},
		// Common conditions across types
		{
			name:      "text with empty",
			format:    apimodel.PropertyFormatText,
			condition: model.BlockContentDataviewFilter_Empty,
			expected:  true,
		},
		{
			name:      "number with not empty",
			format:    apimodel.PropertyFormatNumber,
			condition: model.BlockContentDataviewFilter_NotEmpty,
			expected:  true,
		},
		{
			name:      "select with exists",
			format:    apimodel.PropertyFormatSelect,
			condition: model.BlockContentDataviewFilter_Exists,
			expected:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidConditionForType(tt.format, tt.condition)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSupportedConditions(t *testing.T) {
	// Test that all supported condition strings map to valid conditions
	expectedConditions := map[string]model.BlockContentDataviewFilterCondition{
		"eq":       model.BlockContentDataviewFilter_Equal,
		"ne":       model.BlockContentDataviewFilter_NotEqual,
		"gt":       model.BlockContentDataviewFilter_Greater,
		"gte":      model.BlockContentDataviewFilter_GreaterOrEqual,
		"lt":       model.BlockContentDataviewFilter_Less,
		"lte":      model.BlockContentDataviewFilter_LessOrEqual,
		"like":     model.BlockContentDataviewFilter_Like,
		"notlike":  model.BlockContentDataviewFilter_NotLike,
		"in":       model.BlockContentDataviewFilter_In,
		"notin":    model.BlockContentDataviewFilter_NotIn,
		"empty":    model.BlockContentDataviewFilter_Empty,
		"notempty": model.BlockContentDataviewFilter_NotEmpty,
		"exists":   model.BlockContentDataviewFilter_Exists,
	}

	assert.Equal(t, expectedConditions, SupportedConditions)
}

func TestConditionsForPropertyType(t *testing.T) {
	// Test that all property formats have conditions defined
	expectedFormats := []apimodel.PropertyFormat{
		apimodel.PropertyFormatText,
		apimodel.PropertyFormatNumber,
		apimodel.PropertyFormatDate,
		apimodel.PropertyFormatCheckbox,
		apimodel.PropertyFormatUrl,
		apimodel.PropertyFormatEmail,
		apimodel.PropertyFormatPhone,
		apimodel.PropertyFormatSelect,
		apimodel.PropertyFormatMultiSelect,
		apimodel.PropertyFormatFiles,
		apimodel.PropertyFormatObjects,
	}

	for _, format := range expectedFormats {
		conditions, exists := ConditionsForPropertyType[format]
		assert.True(t, exists, "Property format %s should have conditions defined", format)
		assert.NotEmpty(t, conditions, "Property format %s should have at least one condition", format)
	}

	// Test specific conditions for each type
	t.Run("text conditions", func(t *testing.T) {
		conditions := ConditionsForPropertyType[apimodel.PropertyFormatText]
		assert.Contains(t, conditions, model.BlockContentDataviewFilter_Equal)
		assert.Contains(t, conditions, model.BlockContentDataviewFilter_Like)
		assert.Contains(t, conditions, model.BlockContentDataviewFilter_NotLike)
		assert.NotContains(t, conditions, model.BlockContentDataviewFilter_Greater)
	})

	t.Run("number conditions", func(t *testing.T) {
		conditions := ConditionsForPropertyType[apimodel.PropertyFormatNumber]
		assert.Contains(t, conditions, model.BlockContentDataviewFilter_Equal)
		assert.Contains(t, conditions, model.BlockContentDataviewFilter_Greater)
		assert.Contains(t, conditions, model.BlockContentDataviewFilter_LessOrEqual)
		assert.NotContains(t, conditions, model.BlockContentDataviewFilter_Like)
	})

	t.Run("checkbox conditions", func(t *testing.T) {
		conditions := ConditionsForPropertyType[apimodel.PropertyFormatCheckbox]
		assert.Contains(t, conditions, model.BlockContentDataviewFilter_Equal)
		assert.Contains(t, conditions, model.BlockContentDataviewFilter_NotEqual)
		assert.Contains(t, conditions, model.BlockContentDataviewFilter_Exists)
		assert.Len(t, conditions, 3) // Only these 3 conditions
	})

	t.Run("multi-select conditions", func(t *testing.T) {
		conditions := ConditionsForPropertyType[apimodel.PropertyFormatMultiSelect]
		assert.Contains(t, conditions, model.BlockContentDataviewFilter_In)
		assert.Contains(t, conditions, model.BlockContentDataviewFilter_NotIn)
		assert.NotContains(t, conditions, model.BlockContentDataviewFilter_Equal)
	})
}
