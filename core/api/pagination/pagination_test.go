package pagination

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestNew(t *testing.T) {
	commonConfig := Config{
		DefaultPage:     0,
		DefaultPageSize: 10,
		MinPageSize:     1,
		MaxPageSize:     50,
	}

	tests := []struct {
		name           string
		queryParams    map[string]string
		overrideConfig func(cfg Config) Config
		expectedStatus int
		expectedOffset int
		expectedLimit  int
	}{
		{
			name: "Valid offset and limit",
			queryParams: map[string]string{
				QueryParamOffset: "10",
				QueryParamLimit:  "20",
			},
			overrideConfig: nil,
			expectedStatus: http.StatusOK,
			expectedOffset: 10,
			expectedLimit:  20,
		},
		{
			name: "Offset missing, use default",
			queryParams: map[string]string{
				QueryParamLimit: "20",
			},
			overrideConfig: nil,
			expectedStatus: http.StatusOK,
			expectedOffset: 0,
			expectedLimit:  20,
		},
		{
			name: "Limit missing, use default",
			queryParams: map[string]string{
				QueryParamOffset: "5",
			},
			overrideConfig: nil,
			expectedStatus: http.StatusOK,
			expectedOffset: 5,
			expectedLimit:  10,
		},
		{
			name: "Limit below minimum",
			queryParams: map[string]string{
				QueryParamOffset: "5",
				QueryParamLimit:  "0",
			},
			overrideConfig: nil,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "Limit above maximum",
			queryParams: map[string]string{
				QueryParamOffset: "5",
				QueryParamLimit:  "100",
			},
			overrideConfig: nil,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "Negative offset, use default",
			queryParams: map[string]string{
				QueryParamOffset: "-5",
				QueryParamLimit:  "10",
			},
			overrideConfig: nil,
			expectedStatus: http.StatusOK,
			expectedOffset: 0,
			expectedLimit:  10,
		},
		{
			name: "Custom min and max page size",
			queryParams: map[string]string{
				QueryParamOffset: "5",
				QueryParamLimit:  "15",
			},
			overrideConfig: func(cfg Config) Config {
				cfg.MinPageSize = 10
				cfg.MaxPageSize = 20
				return cfg
			},
			expectedStatus: http.StatusOK,
			expectedOffset: 5,
			expectedLimit:  15,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Apply overrideConfig if provided
			cfg := commonConfig
			if tt.overrideConfig != nil {
				cfg = tt.overrideConfig(cfg)
			}

			// Set up Gin
			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.Use(New(cfg))

			// Define a test endpoint
			r.GET("/", func(c *gin.Context) {
				offset, _ := c.Get(QueryParamOffset)
				limit, _ := c.Get(QueryParamLimit)

				c.JSON(http.StatusOK, gin.H{
					"offset": offset,
					"limit":  limit,
				})
			})

			// Create a test request
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			q := req.URL.Query()
			for k, v := range tt.queryParams {
				q.Add(k, v)
			}
			req.URL.RawQuery = q.Encode()

			// Perform the request
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			// Check the response
			assert.Equal(t, tt.expectedStatus, w.Code)

			if w.Code == http.StatusOK {
				var resp map[string]interface{}
				err := json.Unmarshal(w.Body.Bytes(), &resp)
				assert.NoError(t, err)

				// Validate offset and limit
				if offset, ok := resp["offset"].(float64); ok {
					assert.Equal(t, float64(tt.expectedOffset), offset)
				}
				if limit, ok := resp["limit"].(float64); ok {
					assert.Equal(t, float64(tt.expectedLimit), limit)
				}
			}
		})
	}
}

func TestPaginate(t *testing.T) {
	type args struct {
		records []int
		offset  int
		limit   int
	}
	tests := []struct {
		name          string
		args          args
		wantPaginated []int
		wantHasMore   bool
	}{
		{
			name: "Offset=0, Limit=2 (first two items)",
			args: args{
				records: []int{1, 2, 3, 4, 5},
				offset:  0,
				limit:   2,
			},
			wantPaginated: []int{1, 2},
			wantHasMore:   true, // items remain: [3,4,5]
		},
		{
			name: "Offset=2, Limit=2 (middle slice)",
			args: args{
				records: []int{1, 2, 3, 4, 5},
				offset:  2,
				limit:   2,
			},
			wantPaginated: []int{3, 4},
			wantHasMore:   true, // item 5 remains
		},
		{
			name: "Offset=4, Limit=2 (tail of the slice)",
			args: args{
				records: []int{1, 2, 3, 4, 5},
				offset:  4,
				limit:   2,
			},
			wantPaginated: []int{5},
			wantHasMore:   false,
		},
		{
			name: "Offset > length (should return empty)",
			args: args{
				records: []int{1, 2, 3, 4, 5},
				offset:  10,
				limit:   2,
			},
			wantPaginated: []int{},
			wantHasMore:   false,
		},
		{
			name: "Limit > length (should return entire slice)",
			args: args{
				records: []int{1, 2, 3},
				offset:  0,
				limit:   10,
			},
			wantPaginated: []int{1, 2, 3},
			wantHasMore:   false,
		},
		{
			name: "Zero limit (no items returned)",
			args: args{
				records: []int{1, 2, 3, 4, 5},
				offset:  1,
				limit:   0,
			},
			wantPaginated: []int{},
			wantHasMore:   true, // items remain: [2,3,4,5]
		},
		{
			name: "Negative offset and limit (should return empty)",
			args: args{
				records: []int{1, 2, 3, 4, 5},
				offset:  -1,
				limit:   -1,
			},
			wantPaginated: []int{},
			wantHasMore:   true, // items remain: [1,2,3,4,5]
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPaginated, gotHasMore := Paginate(tt.args.records, tt.args.offset, tt.args.limit)

			assert.Equal(t, tt.wantPaginated, gotPaginated, "Paginate() gotPaginated = %v, want %v", gotPaginated, tt.wantPaginated)
			assert.Equal(t, tt.wantHasMore, gotHasMore, "Paginate() gotHasMore = %v, want %v", gotHasMore, tt.wantHasMore)
		})
	}
}
