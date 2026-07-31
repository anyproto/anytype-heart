package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apimodel "github.com/anyproto/anytype-heart/core/api/model"
	"github.com/anyproto/anytype-heart/core/api/pagination"
)

// searchRouter mounts the space-search route with the C10 pagination
// defaults the /v2 group provides.
func searchRouter(fx *v2HandlerFixture) {
	fx.router.Use(pagination.New(pagination.Config{
		DefaultPage:     0,
		DefaultPageSize: 25,
		MinPageSize:     1,
		MaxPageSize:     1000,
	}))
	fx.router.POST("/v2/spaces/:space_id/search", SearchObjectsV2Handler(fx.svc))
}

func TestSearchObjectsV2Handler(t *testing.T) {
	t.Run("a body limit is rejected by the strict schema with C10 steering", func(t *testing.T) {
		// given
		fx := newV2HandlerFixture(t)
		searchRouter(fx)

		// when
		req := httptest.NewRequest(http.MethodPost, "/v2/spaces/space1/search",
			strings.NewReader(`{"query":"x","limit":50}`))
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusBadRequest, w.Code)
		var got apimodel.V2Error
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
		assert.Equal(t, apimodel.V2CodeValidationFailed, got.Code)
		require.Len(t, got.Issues, 1)
		assert.Equal(t, "/limit", got.Issues[0].Path)
		assert.Contains(t, got.Issues[0].Hint, "?offset=&limit= query params")
	})

	t.Run("any unknown body field names the allowed fields", func(t *testing.T) {
		// given
		fx := newV2HandlerFixture(t)
		searchRouter(fx)

		// when
		req := httptest.NewRequest(http.MethodPost, "/v2/spaces/space1/search",
			strings.NewReader(`{"sort":[{"property":"name"}]}`))
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusBadRequest, w.Code)
		var got apimodel.V2Error
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
		require.Len(t, got.Issues, 1)
		assert.Equal(t, "/sort", got.Issues[0].Path)
		assert.Contains(t, got.Issues[0].Hint, "query, type, filter, filters, sorts, fields")
	})

	t.Run("an empty body is a match-everything search", func(t *testing.T) {
		// given
		fx := newV2HandlerFixture(t)
		searchRouter(fx)

		// when
		req := httptest.NewRequest(http.MethodPost, "/v2/spaces/space1/search", strings.NewReader(""))
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"data":[]`)
	})

	t.Run("dry_run is ignored on search (a read is its own dry run)", func(t *testing.T) {
		// given
		fx := newV2HandlerFixture(t)
		searchRouter(fx)

		// when
		req := httptest.NewRequest(http.MethodPost, "/v2/spaces/space1/search?dry_run=true",
			strings.NewReader(`{}`))
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)

		// then: a normal 200 result, no dry-run envelope
		require.Equal(t, http.StatusOK, w.Code)
		assert.NotContains(t, w.Body.String(), "dry_run")
	})

	t.Run("filter and filters together map to 400 ambiguous_input", func(t *testing.T) {
		// given
		fx := newV2HandlerFixture(t)
		searchRouter(fx)

		// when
		req := httptest.NewRequest(http.MethodPost, "/v2/spaces/space1/search",
			strings.NewReader(`{"filter":"name CONTAINS \"x\"","filters":[]}`))
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusBadRequest, w.Code)
		var got apimodel.V2Error
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
		assert.Equal(t, apimodel.V2CodeAmbiguousInput, got.Code)
	})
}

func TestGlobalSearchObjectsV2Handler(t *testing.T) {
	t.Run("global search responds with rows across spaces", func(t *testing.T) {
		// given
		fx := newV2HandlerFixture(t)
		fx.router.Use(pagination.New(pagination.Config{DefaultPage: 0, DefaultPageSize: 25, MinPageSize: 1, MaxPageSize: 1000}))
		fx.router.POST("/v2/search", GlobalSearchObjectsV2Handler(fx.svc))

		// when
		req := httptest.NewRequest(http.MethodPost, "/v2/search", strings.NewReader(`{}`))
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusOK, w.Code)
		var got apimodel.V2ListResponse[apimodel.V2ObjectRow]
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
		assert.Equal(t, 0, got.Total)
	})
}
