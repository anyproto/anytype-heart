package filter_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/api/filter"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// Integration test for the complete filter flow
func TestFilterIntegration(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name            string
		queryString     string
		expectedFilters int
		checkFilter     func(t *testing.T, filters []*model.BlockContentDataviewFilter)
	}{
		{
			name:            "simple property filter",
			queryString:     "name=test&status=active",
			expectedFilters: 2,
			checkFilter: func(t *testing.T, filters []*model.BlockContentDataviewFilter) {
				require.Len(t, filters, 2)

				// Check filters exist (order may vary due to map iteration)
				filterMap := make(map[string]*model.BlockContentDataviewFilter)
				for _, f := range filters {
					filterMap[f.RelationKey] = f
				}

				// Check name filter
				nameFilter, ok := filterMap["name"]
				require.True(t, ok, "name filter should exist")
				assert.Equal(t, model.BlockContentDataviewFilter_Equal, nameFilter.Condition)
				assert.NotNil(t, nameFilter.Value)

				// Check status filter
				statusFilter, ok := filterMap["status"]
				require.True(t, ok, "status filter should exist")
				assert.Equal(t, model.BlockContentDataviewFilter_Equal, statusFilter.Condition)
				assert.NotNil(t, statusFilter.Value)
			},
		},
		{
			name:            "filter with conditions",
			queryString:     "age[gt]=25&name[like]=john&tags[in]=todo,done",
			expectedFilters: 3,
			checkFilter: func(t *testing.T, filters []*model.BlockContentDataviewFilter) {
				require.Len(t, filters, 3)

				// Check filters exist (order may vary due to map iteration)
				filterMap := make(map[string]*model.BlockContentDataviewFilter)
				for _, f := range filters {
					filterMap[f.RelationKey] = f
				}

				// Check age filter
				ageFilter, ok := filterMap["age"]
				require.True(t, ok, "age filter should exist")
				assert.Equal(t, model.BlockContentDataviewFilter_Greater, ageFilter.Condition)

				// Check name filter
				nameFilter, ok := filterMap["name"]
				require.True(t, ok, "name filter should exist")
				assert.Equal(t, model.BlockContentDataviewFilter_Like, nameFilter.Condition)

				// Check tags filter
				tagsFilter, ok := filterMap["tags"]
				require.True(t, ok, "tags filter should exist")
				assert.Equal(t, model.BlockContentDataviewFilter_In, tagsFilter.Condition)
			},
		},
		{
			name:            "filter with special conditions",
			queryString:     "description[empty]=true&optional_field[exists]=1",
			expectedFilters: 2,
			checkFilter: func(t *testing.T, filters []*model.BlockContentDataviewFilter) {
				require.Len(t, filters, 2)

				// Check filters exist (order may vary due to map iteration)
				filterMap := make(map[string]*model.BlockContentDataviewFilter)
				for _, f := range filters {
					filterMap[f.RelationKey] = f
				}

				// Check empty condition
				descFilter, ok := filterMap["description"]
				require.True(t, ok, "description filter should exist")
				assert.Equal(t, model.BlockContentDataviewFilter_Empty, descFilter.Condition)

				// Check exists condition
				optionalFilter, ok := filterMap["optional_field"]
				require.True(t, ok, "optional_field filter should exist")
				assert.Equal(t, model.BlockContentDataviewFilter_Exists, optionalFilter.Condition)
			},
		},
		{
			name:            "mixed with pagination params",
			queryString:     "name=test&offset=10&limit=20&created[gte]=2024-01-01",
			expectedFilters: 2,
			checkFilter: func(t *testing.T, filters []*model.BlockContentDataviewFilter) {
				require.Len(t, filters, 2)
				// Pagination params should be ignored
			},
		},
		{
			name:            "empty query",
			queryString:     "",
			expectedFilters: 0,
			checkFilter: func(t *testing.T, filters []*model.BlockContentDataviewFilter) {
				assert.Nil(t, filters)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create parser
			parser := filter.NewParser()

			// Create a test request
			req := httptest.NewRequest(http.MethodGet, "/?"+tt.queryString, nil)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = req

			// Parse filters
			parsedFilters, err := parser.ParseQueryParams(c)
			require.NoError(t, err)

			// Convert to dataview filters
			filters := parsedFilters.ToDataviewFilters()

			// Check results
			if tt.expectedFilters == 0 {
				assert.Nil(t, filters)
			} else {
				assert.Len(t, filters, tt.expectedFilters)
			}

			if tt.checkFilter != nil {
				tt.checkFilter(t, filters)
			}
		})
	}
}

// Test error handling
func TestFilterErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	parser := filter.NewParser()

	tests := []struct {
		name          string
		queryString   string
		expectedError string
	}{
		{
			name:          "invalid condition",
			queryString:   "name[invalid]=test",
			expectedError: "unsupported condition: invalid",
		},
		{
			name:          "empty property name",
			queryString:   "=test",
			expectedError: "empty property name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/?"+tt.queryString, nil)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = req

			_, err := parser.ParseQueryParams(c)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectedError)
		})
	}
}
