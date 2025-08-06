package filter

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestParser_ParseQueryParams(t *testing.T) {
	tests := []struct {
		name            string
		queryString     string
		expectedFilters []Filter
		expectedError   string
	}{
		{
			name:        "simple filter - property equals value",
			queryString: "name=test",
			expectedFilters: []Filter{
				{
					PropertyKey: "name",
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       "test",
				},
			},
		},
		{
			name:        "filter with explicit equal condition",
			queryString: "name[eq]=test",
			expectedFilters: []Filter{
				{
					PropertyKey: "name",
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       "test",
				},
			},
		},
		{
			name:        "filter with not equal condition",
			queryString: "status[ne]=active",
			expectedFilters: []Filter{
				{
					PropertyKey: "status",
					Condition:   model.BlockContentDataviewFilter_NotEqual,
					Value:       "active",
				},
			},
		},
		{
			name:        "filter with greater than condition",
			queryString: "age[gt]=25",
			expectedFilters: []Filter{
				{
					PropertyKey: "age",
					Condition:   model.BlockContentDataviewFilter_Greater,
					Value:       "25",
				},
			},
		},
		{
			name:        "filter with greater than or equal condition",
			queryString: "age[gte]=25",
			expectedFilters: []Filter{
				{
					PropertyKey: "age",
					Condition:   model.BlockContentDataviewFilter_GreaterOrEqual,
					Value:       "25",
				},
			},
		},
		{
			name:        "filter with less than condition",
			queryString: "age[lt]=25",
			expectedFilters: []Filter{
				{
					PropertyKey: "age",
					Condition:   model.BlockContentDataviewFilter_Less,
					Value:       "25",
				},
			},
		},
		{
			name:        "filter with less than or equal condition",
			queryString: "age[lte]=25",
			expectedFilters: []Filter{
				{
					PropertyKey: "age",
					Condition:   model.BlockContentDataviewFilter_LessOrEqual,
					Value:       "25",
				},
			},
		},
		{
			name:        "filter with contains condition",
			queryString: "title[contains]=test",
			expectedFilters: []Filter{
				{
					PropertyKey: "title",
					Condition:   model.BlockContentDataviewFilter_Like,
					Value:       "test",
				},
			},
		},
		{
			name:        "filter with not contains condition",
			queryString: "title[ncontains]=test",
			expectedFilters: []Filter{
				{
					PropertyKey: "title",
					Condition:   model.BlockContentDataviewFilter_NotLike,
					Value:       "test",
				},
			},
		},
		{
			name:        "filter with in condition - single value",
			queryString: "tags[in]=todo",
			expectedFilters: []Filter{
				{
					PropertyKey: "tags",
					Condition:   model.BlockContentDataviewFilter_In,
					Value:       []string{"todo"},
				},
			},
		},
		{
			name:        "filter with in condition - multiple values",
			queryString: "tags[in]=todo,done,pending",
			expectedFilters: []Filter{
				{
					PropertyKey: "tags",
					Condition:   model.BlockContentDataviewFilter_In,
					Value:       []string{"todo", "done", "pending"},
				},
			},
		},
		{
			name:        "filter with not in condition",
			queryString: "tags[nin]=archived,deleted",
			expectedFilters: []Filter{
				{
					PropertyKey: "tags",
					Condition:   model.BlockContentDataviewFilter_NotIn,
					Value:       []string{"archived", "deleted"},
				},
			},
		},
		{
			name:        "filter with all condition",
			queryString: "tags[all]=urgent,important",
			expectedFilters: []Filter{
				{
					PropertyKey: "tags",
					Condition:   model.BlockContentDataviewFilter_AllIn,
					Value:       []string{"urgent", "important"},
				},
			},
		},
		{
			name:        "filter with none condition",
			queryString: "tags[none]=spam,trash",
			expectedFilters: []Filter{
				{
					PropertyKey: "tags",
					Condition:   model.BlockContentDataviewFilter_NotAllIn,
					Value:       []string{"spam", "trash"},
				},
			},
		},
		{
			name:        "filter with exact in condition",
			queryString: "tags[exactin]=todo,done",
			expectedFilters: []Filter{
				{
					PropertyKey: "tags",
					Condition:   model.BlockContentDataviewFilter_ExactIn,
					Value:       []string{"todo", "done"},
				},
			},
		},
		{
			name:        "filter with not exact in condition",
			queryString: "tags[nexactin]=todo,done",
			expectedFilters: []Filter{
				{
					PropertyKey: "tags",
					Condition:   model.BlockContentDataviewFilter_NotExactIn,
					Value:       []string{"todo", "done"},
				},
			},
		},
		{
			name:        "filter with empty condition",
			queryString: "description[empty]=true",
			expectedFilters: []Filter{
				{
					PropertyKey: "description",
					Condition:   model.BlockContentDataviewFilter_Empty,
					Value:       true,
				},
			},
		},
		{
			name:        "filter with not empty condition",
			queryString: "description[nempty]=true",
			expectedFilters: []Filter{
				{
					PropertyKey: "description",
					Condition:   model.BlockContentDataviewFilter_NotEmpty,
					Value:       true,
				},
			},
		},
		{
			name:        "filter with exists condition",
			queryString: "custom_field[exists]=1",
			expectedFilters: []Filter{
				{
					PropertyKey: "custom_field",
					Condition:   model.BlockContentDataviewFilter_Exists,
					Value:       true,
				},
			},
		},
		{
			name:        "multiple filters",
			queryString: "name=test&status[ne]=archived&priority[in]=high,medium",
			expectedFilters: []Filter{
				{
					PropertyKey: "name",
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       "test",
				},
				{
					PropertyKey: "status",
					Condition:   model.BlockContentDataviewFilter_NotEqual,
					Value:       "archived",
				},
				{
					PropertyKey: "priority",
					Condition:   model.BlockContentDataviewFilter_In,
					Value:       []string{"high", "medium"},
				},
			},
		},
		{
			name:        "url encoded values",
			queryString: "title=hello%20world&description[contains]=test%2525",
			expectedFilters: []Filter{
				{
					PropertyKey: "title",
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       "hello world",
				},
				{
					PropertyKey: "description",
					Condition:   model.BlockContentDataviewFilter_Like,
					Value:       "test%",
				},
			},
		},
		{
			name:        "pagination params are ignored",
			queryString: "name=test&offset=10&limit=20&sort=name&order=asc",
			expectedFilters: []Filter{
				{
					PropertyKey: "name",
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       "test",
				},
			},
		},
		{
			name:          "invalid condition",
			queryString:   "name[invalid]=test",
			expectedError: "invalid filter key \"name[invalid]\": unsupported condition: invalid",
		},
		{
			name:        "brackets without property name",
			queryString: "[eq]=test",
			expectedFilters: []Filter{
				{
					PropertyKey: "[eq]",
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       "test",
				},
			},
		},
		{
			name:        "empty filter value is allowed",
			queryString: "name=",
			expectedFilters: []Filter{
				{
					PropertyKey: "name",
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       "",
				},
			},
		},
		{
			name:        "filter with spaces in value",
			queryString: "tags[in]=to do, in progress ,done",
			expectedFilters: []Filter{
				{
					PropertyKey: "tags",
					Condition:   model.BlockContentDataviewFilter_In,
					Value:       []string{"to do", "in progress", "done"},
				},
			},
		},
		{
			name:        "property key with special characters",
			queryString: "custom.property_name-123[eq]=value",
			expectedFilters: []Filter{
				{
					PropertyKey: "custom.property_name-123",
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       "value",
				},
			},
		},
		{
			name:        "empty array for in condition",
			queryString: "tags[in]=",
			expectedFilters: []Filter{
				{
					PropertyKey: "tags",
					Condition:   model.BlockContentDataviewFilter_In,
					Value:       []string{},
				},
			},
		},
		{
			name:        "special characters in values",
			queryString: "description[contains]=%26%3D%2B%40%23",
			expectedFilters: []Filter{
				{
					PropertyKey: "description",
					Condition:   model.BlockContentDataviewFilter_Like,
					Value:       "&= @#", // + is decoded as space in URL encoding
				},
			},
		},
		{
			name:        "malformed bracket syntax - missing closing bracket",
			queryString: "name[eq=test",
			expectedFilters: []Filter{
				{
					PropertyKey: "name[eq",
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       "test",
				},
			},
		},
		{
			name:        "malformed bracket syntax - extra bracket",
			queryString: "name][eq]=test",
			expectedFilters: []Filter{
				{
					PropertyKey: "name]",
					Condition:   model.BlockContentDataviewFilter_Equal,
					Value:       "test",
				},
			},
		},
		{
			name:        "multiple values for non-array condition",
			queryString: "priority[gt]=5,10,15",
			expectedFilters: []Filter{
				{
					PropertyKey: "priority",
					Condition:   model.BlockContentDataviewFilter_Greater,
					Value:       "5,10,15", // Treated as single string
				},
			},
		},
	}

	parser := NewParser()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock gin context with the query string
			c := createMockContext(tt.queryString)

			result, err := parser.ParseQueryParams(c)

			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Len(t, result.Filters, len(tt.expectedFilters))

				// Check filters (order may vary due to map iteration)
				filterMap := make(map[string]Filter)
				for _, f := range result.Filters {
					filterMap[f.PropertyKey] = f
				}

				for _, expected := range tt.expectedFilters {
					actual, exists := filterMap[expected.PropertyKey]
					require.True(t, exists, "Filter for property %s not found", expected.PropertyKey)
					assert.Equal(t, expected.Condition, actual.Condition)
					assert.Equal(t, expected.Value, actual.Value)
				}
			}
		})
	}
}

func TestParser_parseFilterKey(t *testing.T) {
	tests := []struct {
		name              string
		key               string
		expectedProperty  string
		expectedCondition model.BlockContentDataviewFilterCondition
		expectedError     string
	}{
		{
			name:              "simple key defaults to equal",
			key:               "name",
			expectedProperty:  "name",
			expectedCondition: model.BlockContentDataviewFilter_Equal,
		},
		{
			name:              "key with equal condition",
			key:               "name[eq]",
			expectedProperty:  "name",
			expectedCondition: model.BlockContentDataviewFilter_Equal,
		},
		{
			name:              "key with not equal condition",
			key:               "status[ne]",
			expectedProperty:  "status",
			expectedCondition: model.BlockContentDataviewFilter_NotEqual,
		},
		{
			name:              "key with greater than condition",
			key:               "age[gt]",
			expectedProperty:  "age",
			expectedCondition: model.BlockContentDataviewFilter_Greater,
		},
		{
			name:              "key with case insensitive condition",
			key:               "name[EQ]",
			expectedProperty:  "name",
			expectedCondition: model.BlockContentDataviewFilter_Equal,
		},
		{
			name:          "invalid condition",
			key:           "name[invalid]",
			expectedError: "unsupported condition: invalid",
		},
		{
			name:              "brackets without property name",
			key:               "[eq]",
			expectedProperty:  "[eq]",
			expectedCondition: model.BlockContentDataviewFilter_Equal,
		},
		{
			name:              "property with underscore",
			key:               "custom_property[contains]",
			expectedProperty:  "custom_property",
			expectedCondition: model.BlockContentDataviewFilter_Like,
		},
		{
			name:              "property with dots",
			key:               "metadata.author[eq]",
			expectedProperty:  "metadata.author",
			expectedCondition: model.BlockContentDataviewFilter_Equal,
		},
	}

	parser := NewParser()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			property, condition, err := parser.parseFilterKey(tt.key)

			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedProperty, property)
				assert.Equal(t, tt.expectedCondition, condition)
			}
		})
	}
}

func TestParser_parseFilterValue(t *testing.T) {
	tests := []struct {
		name          string
		rawValue      string
		condition     model.BlockContentDataviewFilterCondition
		expectedValue interface{}
		expectedError string
	}{
		{
			name:          "simple string value",
			rawValue:      "test",
			condition:     model.BlockContentDataviewFilter_Equal,
			expectedValue: "test",
		},
		{
			name:          "url encoded value",
			rawValue:      "hello%20world",
			condition:     model.BlockContentDataviewFilter_Equal,
			expectedValue: "hello world",
		},
		{
			name:          "empty condition with true",
			rawValue:      "true",
			condition:     model.BlockContentDataviewFilter_Empty,
			expectedValue: true,
		},
		{
			name:          "empty condition with false",
			rawValue:      "false",
			condition:     model.BlockContentDataviewFilter_Empty,
			expectedValue: false,
		},
		{
			name:          "empty condition with 1",
			rawValue:      "1",
			condition:     model.BlockContentDataviewFilter_Empty,
			expectedValue: true,
		},
		{
			name:          "empty condition with empty string",
			rawValue:      "",
			condition:     model.BlockContentDataviewFilter_Empty,
			expectedValue: true,
		},
		{
			name:          "in condition with single value",
			rawValue:      "todo",
			condition:     model.BlockContentDataviewFilter_In,
			expectedValue: []string{"todo"},
		},
		{
			name:          "in condition with multiple values",
			rawValue:      "todo,done,pending",
			condition:     model.BlockContentDataviewFilter_In,
			expectedValue: []string{"todo", "done", "pending"},
		},
		{
			name:          "in condition with empty value",
			rawValue:      "",
			condition:     model.BlockContentDataviewFilter_In,
			expectedValue: []string{},
		},
		{
			name:          "in condition with spaces",
			rawValue:      "to do, in progress ,done",
			condition:     model.BlockContentDataviewFilter_In,
			expectedValue: []string{"to do", "in progress", "done"},
		},
		{
			name:          "invalid url encoding",
			rawValue:      "test%",
			condition:     model.BlockContentDataviewFilter_Equal,
			expectedError: "failed to decode value",
		},
	}

	parser := NewParser()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, err := parser.parseFilterValue(tt.rawValue, tt.condition)

			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedValue, value)
			}
		})
	}
}

// Helper function to create a mock gin.Context with query parameters
func createMockContext(queryString string) *gin.Context {
	gin.SetMode(gin.TestMode)

	req := &http.Request{
		URL: &url.URL{
			RawQuery: queryString,
		},
	}

	c, _ := gin.CreateTestContext(nil)
	c.Request = req

	return c
}
