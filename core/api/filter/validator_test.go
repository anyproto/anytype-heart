package filter_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/api/filter"
	"github.com/anyproto/anytype-heart/core/api/filter/mock_filter"
	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestValidator_ValidateFilters(t *testing.T) {
	const testSpaceId = "test-space"

	mockProperties := map[string]*apimodel.Property{
		"title": {
			Id:          "rel-title",
			Key:         "title",
			RelationKey: "title",
			Format:      apimodel.PropertyFormatText,
		},
		"age": {
			Id:          "rel-age",
			Key:         "age",
			RelationKey: "age",
			Format:      apimodel.PropertyFormatNumber,
		},
		"is_active": {
			Id:          "rel-is-active",
			Key:         "is_active",
			RelationKey: "is_active",
			Format:      apimodel.PropertyFormatCheckbox,
		},
		"tags": {
			Id:          "rel-tags",
			Key:         "tags",
			RelationKey: "tags",
			Format:      apimodel.PropertyFormatMultiSelect,
		},
		"custom_prop": {
			Id:          "rel-custom",
			Key:         "my_custom_property",
			RelationKey: "custom_prop",
			Format:      apimodel.PropertyFormatText,
		},
		"created_date": {
			Id:          "rel-created-date",
			Key:         "created_date",
			RelationKey: "created_date",
			Format:      apimodel.PropertyFormatDate,
		},
		"status": {
			Id:          "rel-status",
			Key:         "status",
			RelationKey: "status",
			Format:      apimodel.PropertyFormatSelect,
		},
	}

	tests := []struct {
		name          string
		filters       *filter.ParsedFilters
		setupMock     func(m *mock_filter.MockApiService)
		expectedError string
		checkResult   func(t *testing.T, filters *filter.ParsedFilters)
	}{
		{
			name:    "nil filters",
			filters: nil,
			setupMock: func(m *mock_filter.MockApiService) {
				// No calls expected
			},
			checkResult: func(t *testing.T, filters *filter.ParsedFilters) {
				assert.Nil(t, filters)
			},
		},
		{
			name:    "empty filters",
			filters: &filter.ParsedFilters{Filters: []filter.Filter{}},
			setupMock: func(m *mock_filter.MockApiService) {
				// No calls expected
			},
			checkResult: func(t *testing.T, filters *filter.ParsedFilters) {
				assert.Empty(t, filters.Filters)
			},
		},
		{
			name: "valid text filter",
			filters: &filter.ParsedFilters{
				Filters: []filter.Filter{
					{
						PropertyKey: "title",
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       "test",
					},
				},
			},
			setupMock: func(m *mock_filter.MockApiService) {
				m.On("GetCachedProperties", testSpaceId).Return(mockProperties)
				m.On("ResolvePropertyApiKey", mockProperties, "title").Return("title", true)
				m.On("SanitizeAndValidatePropertyValue",
					testSpaceId,
					"title",
					"test",
					mockProperties["title"],
					mockProperties,
				).Return("test", nil)
			},
			checkResult: func(t *testing.T, filters *filter.ParsedFilters) {
				require.Len(t, filters.Filters, 1)
				assert.Equal(t, "title", filters.Filters[0].PropertyKey)
				assert.Equal(t, "test", filters.Filters[0].Value)
			},
		},
		{
			name: "top-level attribute name filter",
			filters: &filter.ParsedFilters{
				Filters: []filter.Filter{
					{
						PropertyKey: "name",
						Condition:   model.BlockContentDataviewFilter_Like,
						Value:       "test",
					},
				},
			},
			setupMock: func(m *mock_filter.MockApiService) {
				m.On("GetCachedProperties", testSpaceId).Return(mockProperties)
				// name is a top-level attribute, so ResolvePropertyApiKey is not called
				m.On("SanitizeAndValidatePropertyValue",
					testSpaceId,
					"name",
					"test",
					mock.Anything,
					mockProperties,
				).Return("test", nil)
			},
			checkResult: func(t *testing.T, filters *filter.ParsedFilters) {
				require.Len(t, filters.Filters, 1)
				assert.Equal(t, "name", filters.Filters[0].PropertyKey)
				assert.Equal(t, "test", filters.Filters[0].Value)
			},
		},
		{
			name: "valid number filter",
			filters: &filter.ParsedFilters{
				Filters: []filter.Filter{
					{
						PropertyKey: "age",
						Condition:   model.BlockContentDataviewFilter_Greater,
						Value:       "25",
					},
				},
			},
			setupMock: func(m *mock_filter.MockApiService) {
				m.On("GetCachedProperties", testSpaceId).Return(mockProperties)
				m.On("ResolvePropertyApiKey", mockProperties, "age").Return("age", true)
				m.On("SanitizeAndValidatePropertyValue",
					testSpaceId,
					"age",
					"25",
					mockProperties["age"],
					mockProperties,
				).Return(float64(25), nil)
			},
			checkResult: func(t *testing.T, filters *filter.ParsedFilters) {
				require.Len(t, filters.Filters, 1)
				assert.Equal(t, "age", filters.Filters[0].PropertyKey)
				assert.Equal(t, float64(25), filters.Filters[0].Value)
			},
		},
		{
			name: "property resolved by API key",
			filters: &filter.ParsedFilters{
				Filters: []filter.Filter{
					{
						PropertyKey: "my_custom_property",
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       "test",
					},
				},
			},
			setupMock: func(m *mock_filter.MockApiService) {
				m.On("GetCachedProperties", testSpaceId).Return(mockProperties)
				m.On("ResolvePropertyApiKey", mockProperties, "my_custom_property").Return("custom_prop", true)
				m.On("SanitizeAndValidatePropertyValue",
					testSpaceId,
					"my_custom_property",
					"test",
					mockProperties["custom_prop"],
					mockProperties,
				).Return("test", nil)
			},
			checkResult: func(t *testing.T, filters *filter.ParsedFilters) {
				require.Len(t, filters.Filters, 1)
				assert.Equal(t, "custom_prop", filters.Filters[0].PropertyKey) // Should be updated to relation key
				assert.Equal(t, "test", filters.Filters[0].Value)
			},
		},
		{
			name: "invalid property",
			filters: &filter.ParsedFilters{
				Filters: []filter.Filter{
					{
						PropertyKey: "unknown_property",
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       "test",
					},
				},
			},
			setupMock: func(m *mock_filter.MockApiService) {
				m.On("GetCachedProperties", testSpaceId).Return(mockProperties)
				m.On("ResolvePropertyApiKey", mockProperties, "unknown_property").Return("", false)
			},
			expectedError: "invalid filter at index 0: failed to resolve property \"unknown_property\": bad input: property \"unknown_property\" not found",
		},
		{
			name: "invalid condition for text property",
			filters: &filter.ParsedFilters{
				Filters: []filter.Filter{
					{
						PropertyKey: "title",
						Condition:   model.BlockContentDataviewFilter_Greater,
						Value:       "test",
					},
				},
			},
			setupMock: func(m *mock_filter.MockApiService) {
				m.On("GetCachedProperties", testSpaceId).Return(mockProperties)
				m.On("ResolvePropertyApiKey", mockProperties, "title").Return("title", true)
			},
			expectedError: "invalid filter at index 0: bad input: condition \"gt\" is not valid for property type \"text\"",
		},
		{
			name: "invalid value for number property",
			filters: &filter.ParsedFilters{
				Filters: []filter.Filter{
					{
						PropertyKey: "age",
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       "not a number",
					},
				},
			},
			setupMock: func(m *mock_filter.MockApiService) {
				m.On("GetCachedProperties", testSpaceId).Return(mockProperties)
				m.On("ResolvePropertyApiKey", mockProperties, "age").Return("age", true)
				m.On("SanitizeAndValidatePropertyValue",
					testSpaceId,
					"age",
					"not a number",
					mockProperties["age"],
					mockProperties,
				).Return(nil, errors.New("invalid number format"))
			},
			expectedError: "invalid filter at index 0: invalid value for property \"age\": invalid number format",
		},
		{
			name: "empty condition with boolean value",
			filters: &filter.ParsedFilters{
				Filters: []filter.Filter{
					{
						PropertyKey: "title",
						Condition:   model.BlockContentDataviewFilter_Empty,
						Value:       true,
					},
				},
			},
			setupMock: func(m *mock_filter.MockApiService) {
				m.On("GetCachedProperties", testSpaceId).Return(mockProperties)
				m.On("ResolvePropertyApiKey", mockProperties, "title").Return("title", true)
				// Empty condition doesn't call SanitizeAndValidatePropertyValue
			},
			checkResult: func(t *testing.T, filters *filter.ParsedFilters) {
				require.Len(t, filters.Filters, 1)
				assert.Equal(t, true, filters.Filters[0].Value)
			},
		},
		{
			name: "in condition with array value",
			filters: &filter.ParsedFilters{
				Filters: []filter.Filter{
					{
						PropertyKey: "tags",
						Condition:   model.BlockContentDataviewFilter_In,
						Value:       []string{"todo", "done"},
					},
				},
			},
			setupMock: func(m *mock_filter.MockApiService) {
				m.On("GetCachedProperties", testSpaceId).Return(mockProperties)
				m.On("ResolvePropertyApiKey", mockProperties, "tags").Return("tags", true)
				m.On("SanitizeAndValidatePropertyValue",
					testSpaceId,
					"tags",
					[]string{"todo", "done"},
					mockProperties["tags"],
					mockProperties,
				).Return([]string{"todo", "done"}, nil)
			},
			checkResult: func(t *testing.T, filters *filter.ParsedFilters) {
				require.Len(t, filters.Filters, 1)
				assert.Equal(t, []string{"todo", "done"}, filters.Filters[0].Value)
			},
		},
		{
			name: "in condition with single value converted to array",
			filters: &filter.ParsedFilters{
				Filters: []filter.Filter{
					{
						PropertyKey: "tags",
						Condition:   model.BlockContentDataviewFilter_In,
						Value:       "single",
					},
				},
			},
			setupMock: func(m *mock_filter.MockApiService) {
				m.On("GetCachedProperties", testSpaceId).Return(mockProperties)
				m.On("ResolvePropertyApiKey", mockProperties, "tags").Return("tags", true)
				m.On("SanitizeAndValidatePropertyValue",
					testSpaceId,
					"tags",
					[]interface{}{"single"},
					mockProperties["tags"],
					mockProperties,
				).Return([]interface{}{"single"}, nil)
			},
			checkResult: func(t *testing.T, filters *filter.ParsedFilters) {
				require.Len(t, filters.Filters, 1)
				assert.Equal(t, []interface{}{"single"}, filters.Filters[0].Value)
			},
		},
		{
			name: "multiple filters with one invalid",
			filters: &filter.ParsedFilters{
				Filters: []filter.Filter{
					{
						PropertyKey: "title",
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       "test",
					},
					{
						PropertyKey: "unknown",
						Condition:   model.BlockContentDataviewFilter_Equal,
						Value:       "value",
					},
				},
			},
			setupMock: func(m *mock_filter.MockApiService) {
				m.On("GetCachedProperties", testSpaceId).Return(mockProperties)
				// First filter passes
				m.On("ResolvePropertyApiKey", mockProperties, "title").Return("title", true)
				m.On("SanitizeAndValidatePropertyValue",
					testSpaceId,
					"title",
					"test",
					mockProperties["title"],
					mockProperties,
				).Return("test", nil)
				// Second filter fails
				m.On("ResolvePropertyApiKey", mockProperties, "unknown").Return("", false)
			},
			expectedError: "invalid filter at index 1: failed to resolve property \"unknown\": bad input: property \"unknown\" not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockService := mock_filter.NewMockApiService(t)
			if tt.setupMock != nil {
				tt.setupMock(mockService)
			}

			validator := filter.NewValidator(mockService)
			err := validator.ValidateFilters(testSpaceId, tt.filters)

			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Equal(t, tt.expectedError, err.Error())
			} else {
				require.NoError(t, err)
				if tt.checkResult != nil {
					tt.checkResult(t, tt.filters)
				}
			}
		})
	}
}

func TestValidator_ConditionValidation(t *testing.T) {
	// Test that all property types have their conditions validated correctly
	tests := []struct {
		name           string
		propertyFormat apimodel.PropertyFormat
		condition      model.BlockContentDataviewFilterCondition
		shouldBeValid  bool
	}{
		// Text format
		{"text with equal", apimodel.PropertyFormatText, model.BlockContentDataviewFilter_Equal, true},
		{"text with like", apimodel.PropertyFormatText, model.BlockContentDataviewFilter_Like, true},
		{"text with greater", apimodel.PropertyFormatText, model.BlockContentDataviewFilter_Greater, false},
		{"text with in", apimodel.PropertyFormatText, model.BlockContentDataviewFilter_In, false},

		// Number format
		{"number with equal", apimodel.PropertyFormatNumber, model.BlockContentDataviewFilter_Equal, true},
		{"number with greater", apimodel.PropertyFormatNumber, model.BlockContentDataviewFilter_Greater, true},
		{"number with less or equal", apimodel.PropertyFormatNumber, model.BlockContentDataviewFilter_LessOrEqual, true},
		{"number with like", apimodel.PropertyFormatNumber, model.BlockContentDataviewFilter_Like, false},

		// Checkbox format
		{"checkbox with equal", apimodel.PropertyFormatCheckbox, model.BlockContentDataviewFilter_Equal, true},
		{"checkbox with not equal", apimodel.PropertyFormatCheckbox, model.BlockContentDataviewFilter_NotEqual, true},
		{"checkbox with greater", apimodel.PropertyFormatCheckbox, model.BlockContentDataviewFilter_Greater, false},

		// Select format
		{"select with equal", apimodel.PropertyFormatSelect, model.BlockContentDataviewFilter_Equal, false},
		{"select with in", apimodel.PropertyFormatSelect, model.BlockContentDataviewFilter_In, true},
		{"select with like", apimodel.PropertyFormatSelect, model.BlockContentDataviewFilter_Like, false},

		// Multi-select format
		{"multi-select with in", apimodel.PropertyFormatMultiSelect, model.BlockContentDataviewFilter_In, true},
		{"multi-select with not in", apimodel.PropertyFormatMultiSelect, model.BlockContentDataviewFilter_NotIn, true},
		{"multi-select with equal", apimodel.PropertyFormatMultiSelect, model.BlockContentDataviewFilter_Equal, false},

		// Date format
		{"date with equal", apimodel.PropertyFormatDate, model.BlockContentDataviewFilter_Equal, true},
		{"date with greater", apimodel.PropertyFormatDate, model.BlockContentDataviewFilter_Greater, true},
		{"date with like", apimodel.PropertyFormatDate, model.BlockContentDataviewFilter_Like, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockService := mock_filter.NewMockApiService(t)

			property := &apimodel.Property{
				Id:          "test-id",
				Key:         "test-key",
				RelationKey: "test-key",
				Format:      tt.propertyFormat,
			}

			mockProperties := map[string]*apimodel.Property{
				"test-key": property,
			}

			filters := &filter.ParsedFilters{
				Filters: []filter.Filter{
					{
						PropertyKey: "test-key",
						Condition:   tt.condition,
						Value:       "test-value",
					},
				},
			}

			mockService.On("GetCachedProperties", "test-space").Return(mockProperties)
			mockService.On("ResolvePropertyApiKey", mockProperties, "test-key").Return("test-key", true)

			if tt.shouldBeValid {
				mockService.On("SanitizeAndValidatePropertyValue",
					mock.Anything,
					mock.Anything,
					mock.Anything,
					mock.Anything,
					mock.Anything,
					mock.Anything,
				).Return("test-value", nil)
			}

			validator := filter.NewValidator(mockService)
			err := validator.ValidateFilters("test-space", filters)

			if tt.shouldBeValid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), "is not valid for property type")
			}
		})
	}
}
