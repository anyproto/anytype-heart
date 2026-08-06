package v2handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// withDryRunFlag mimics the server's ensureDryRun middleware for handler
// tests: it parses ?dry_run into the context flag the handlers read.
func withDryRunFlag() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(v2DryRunContextKey, c.Query("dry_run") == "true")
		c.Next()
	}
}

func TestCreateObjectV2Handler(t *testing.T) {
	t.Run("a created object responds 201 with id and etag", func(t *testing.T) {
		// given
		fx := newV2HandlerFixture(t)
		fx.router.POST("/v2/spaces/:space_id/objects", withDryRunFlag(), CreateObjectV2Handler(fx.svc))
		fx.creatorMock.EXPECT().CreateObjectFromSnapshot(mock.Anything, "space1", mock.Anything).Return("newObj", nil)
		fx.readerMock.EXPECT().ReadObject(mock.Anything, "space1", "newObj").
			Return(apicore.ObjectRead{Heads: []string{"h"}}, nil)

		// when
		req := httptest.NewRequest(http.MethodPost, "/v2/spaces/space1/objects",
			strings.NewReader(`{"type":"task","name":"Buy milk"}`))
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusCreated, w.Code)
		var got v2model.CreateResult
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
		assert.Equal(t, "newObj", got.Id)
		assert.NotEmpty(t, got.Etag)
		assert.NotEmpty(t, w.Header().Get("ETag"))
	})

	t.Run("dry_run responds 200 and commits nothing", func(t *testing.T) {
		// given: no creator expectations — a create call would fail the test
		fx := newV2HandlerFixture(t)
		fx.router.POST("/v2/spaces/:space_id/objects", withDryRunFlag(), CreateObjectV2Handler(fx.svc))

		// when
		req := httptest.NewRequest(http.MethodPost, "/v2/spaces/space1/objects?dry_run=true",
			strings.NewReader(`{"type":"task","name":"Buy milk"}`))
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusOK, w.Code)
		var got v2model.CreateResult
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
		assert.True(t, got.DryRun)
		assert.Empty(t, got.Id)
	})

	t.Run("validation failures respond with the C6 envelope", func(t *testing.T) {
		// given
		fx := newV2HandlerFixture(t)
		fx.router.POST("/v2/spaces/:space_id/objects", withDryRunFlag(), CreateObjectV2Handler(fx.svc))

		// when
		req := httptest.NewRequest(http.MethodPost, "/v2/spaces/space1/objects",
			strings.NewReader(`{"version":1,"blocks":[{"type":"wat"}]}`))
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusBadRequest, w.Code)
		var got v2model.Error
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
		assert.Equal(t, v2model.CodeValidationFailed, got.Code)
		require.NotEmpty(t, got.Issues)
	})
}

func TestCreatePropertyV2Handler(t *testing.T) {
	t.Run("malformed body is a 400", func(t *testing.T) {
		// given
		fx := newV2HandlerFixture(t)
		fx.router.POST("/v2/spaces/:space_id/properties", withDryRunFlag(), CreatePropertyV2Handler(fx.svc))

		// when
		req := httptest.NewRequest(http.MethodPost, "/v2/spaces/space1/properties", strings.NewReader(`{"name":`))
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("dry run reports the would-be property", func(t *testing.T) {
		// given
		fx := newV2HandlerFixture(t)
		fx.router.POST("/v2/spaces/:space_id/properties", withDryRunFlag(), CreatePropertyV2Handler(fx.svc))

		// when
		req := httptest.NewRequest(http.MethodPost, "/v2/spaces/space1/properties?dry_run=true",
			strings.NewReader(`{"key":"vibe","name":"Vibe","format":"select"}`))
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusOK, w.Code)
		var got v2model.CreateResult
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
		assert.True(t, got.DryRun)
		require.NotNil(t, got.Created)
		require.Len(t, got.Created.Properties, 1)
	})
}

func TestUploadFileV2Handler(t *testing.T) {
	t.Run("json body without url is a 400", func(t *testing.T) {
		// given
		fx := newV2HandlerFixture(t)
		fx.router.POST("/v2/spaces/:space_id/files", withDryRunFlag(), UploadFileV2Handler(fx.svc))

		// when
		req := httptest.NewRequest(http.MethodPost, "/v2/spaces/space1/files", strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestSchemaV2Handlers(t *testing.T) {
	t.Run("index and kind round-trip", func(t *testing.T) {
		// given
		fx := newV2HandlerFixture(t)
		fx.router.GET("/v2/schemas", SchemaIndexV2Handler(fx.svc))
		fx.router.GET("/v2/schemas/:kind", SchemaKindV2Handler(fx.svc))

		// when: index
		w := httptest.NewRecorder()
		fx.router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v2/schemas", nil))
		require.Equal(t, http.StatusOK, w.Code)
		var index v2model.SchemaIndex
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &index))
		require.NotEmpty(t, index.Kinds)

		// when: first kind
		w = httptest.NewRecorder()
		fx.router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, index.Kinds[0].Url, nil))
		require.Equal(t, http.StatusOK, w.Code)

		// when: unknown kind
		w = httptest.NewRecorder()
		fx.router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v2/schemas/wat", nil))
		require.Equal(t, http.StatusNotFound, w.Code)
	})
}

// contextCancelGuard: the create handlers must pass the request context
// through so client disconnects propagate to the middleware calls.
func TestCreateObjectV2HandlerContext(t *testing.T) {
	fx := newV2HandlerFixture(t)
	fx.router.POST("/v2/spaces/:space_id/objects", withDryRunFlag(), CreateObjectV2Handler(fx.svc))
	var gotCtx context.Context
	fx.creatorMock.EXPECT().CreateObjectFromSnapshot(mock.Anything, "space1", mock.Anything).
		RunAndReturn(func(ctx context.Context, spaceId string, snapshot *model.SmartBlockSnapshotBase) (string, error) {
			gotCtx = ctx
			return "obj", nil
		})
	fx.readerMock.EXPECT().ReadObject(mock.Anything, "space1", "obj").
		Return(apicore.ObjectRead{Heads: []string{"h"}}, nil)

	req := httptest.NewRequest(http.MethodPost, "/v2/spaces/space1/objects", strings.NewReader(`{"type":"page"}`))
	w := httptest.NewRecorder()
	fx.router.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)
	require.NotNil(t, gotCtx)
}
