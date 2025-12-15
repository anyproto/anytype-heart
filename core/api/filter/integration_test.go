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

				// Check filters exist
				filterMap := make(map[string]*model.BlockContentDataviewFilter)
				for _, f := range filters {
					filterMap[f.RelationKey] = f
				}

				// Check name filter (text property defaults to contains)
				nameFilter, ok := filterMap["name"]
				require.True(t, ok, "name filter should exist")
				assert.Equal(t, model.BlockContentDataviewFilter_Like, nameFilter.Condition)
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
			queryString:     "age[gt]=25&name[contains]=john&tags[in]=todo,done",
			expectedFilters: 3,
			checkFilter: func(t *testing.T, filters []*model.BlockContentDataviewFilter) {
				require.Len(t, filters, 3)

				// Check filters exist
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
			queryString:     "description[empty]=true&tags[nempty]=true",
			expectedFilters: 2,
			checkFilter: func(t *testing.T, filters []*model.BlockContentDataviewFilter) {
				require.Len(t, filters, 2)

				// Check filters exist
				filterMap := make(map[string]*model.BlockContentDataviewFilter)
				for _, f := range filters {
					filterMap[f.RelationKey] = f
				}

				// Check empty condition
				descFilter, ok := filterMap["description"]
				require.True(t, ok, "description filter should exist")
				assert.Equal(t, model.BlockContentDataviewFilter_Empty, descFilter.Condition)

				// Check not empty condition
				tagsFilter, ok := filterMap["tags"]
				require.True(t, ok, "tags filter should exist")
				assert.Equal(t, model.BlockContentDataviewFilter_NotEmpty, tagsFilter.Condition)
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
			parser := filter.CreateTestParser(t)

			req := httptest.NewRequest(http.MethodGet, "/?"+tt.queryString, nil)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = req

			parsedFilters, err := parser.ParseQueryParams(c, "test-space")
			require.NoError(t, err)

			filters := parsedFilters.ToDataviewFilters()

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

func TestSpacesEndpointIntegration(t *testing.T) {
	// Test the /v1/spaces endpoint scenario where there's no spaceId
	// and 'name' is a top-level attribute that defaults to contains
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name            string
		queryString     string
		spaceId         string
		expectedFilters int
		checkFilter     func(t *testing.T, filters []*model.BlockContentDataviewFilter)
	}{
		{
			name:            "spaces endpoint with name filter defaults to contains",
			queryString:     "name=test",
			spaceId:         "", // No spaceId for spaces endpoint
			expectedFilters: 1,
			checkFilter: func(t *testing.T, filters []*model.BlockContentDataviewFilter) {
				require.Len(t, filters, 1)

				nameFilter := filters[0]
				assert.Equal(t, "name", nameFilter.RelationKey)
				assert.Equal(t, model.BlockContentDataviewFilter_Like, nameFilter.Condition)
				assert.NotNil(t, nameFilter.Value)
			},
		},
		{
			name:            "spaces endpoint with explicit equal condition",
			queryString:     "name[eq]=exact-space-name",
			spaceId:         "", // No spaceId for spaces endpoint
			expectedFilters: 1,
			checkFilter: func(t *testing.T, filters []*model.BlockContentDataviewFilter) {
				require.Len(t, filters, 1)

				nameFilter := filters[0]
				assert.Equal(t, "name", nameFilter.RelationKey)
				assert.Equal(t, model.BlockContentDataviewFilter_Equal, nameFilter.Condition)
				assert.NotNil(t, nameFilter.Value)
			},
		},
		{
			name:            "spaces endpoint with multiple filters",
			queryString:     "name=test&archived=false",
			spaceId:         "", // No spaceId for spaces endpoint
			expectedFilters: 2,
			checkFilter: func(t *testing.T, filters []*model.BlockContentDataviewFilter) {
				require.Len(t, filters, 2)

				filterMap := make(map[string]*model.BlockContentDataviewFilter)
				for _, f := range filters {
					filterMap[f.RelationKey] = f
				}

				// Name should default to contains
				nameFilter, ok := filterMap["name"]
				require.True(t, ok)
				assert.Equal(t, model.BlockContentDataviewFilter_Like, nameFilter.Condition)

				// Other properties default to equal
				archivedFilter, ok := filterMap["archived"]
				require.True(t, ok)
				assert.Equal(t, model.BlockContentDataviewFilter_Equal, archivedFilter.Condition)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := filter.CreateTestParser(t)

			req := httptest.NewRequest(http.MethodGet, "/?"+tt.queryString, nil)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = req

			parsedFilters, err := parser.ParseQueryParams(c, tt.spaceId)
			require.NoError(t, err)

			filters := parsedFilters.ToDataviewFilters()

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

func TestFilterErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name          string
		queryString   string
		expectedError string
	}{
		{
			name:          "invalid condition",
			queryString:   "name[invalid]=test",
			expectedError: "unsupported condition: \"invalid\"",
		},
		{
			name:          "empty property name",
			queryString:   "=test",
			expectedError: "empty property name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := filter.CreateTestParser(t)

			req := httptest.NewRequest(http.MethodGet, "/?"+tt.queryString, nil)
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = req

			_, err := parser.ParseQueryParams(c, "test-space")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectedError)
		})
	}
}
