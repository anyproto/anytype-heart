package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newIdempotencyRouter builds a tiny router with the C8 middleware in front
// of a counting handler, so replay behavior is observable.
func newIdempotencyRouter(store *idempotencyStore, calls *int) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/v2/spaces/:space_id/things", ensureIdempotency(store), func(c *gin.Context) {
		*calls++
		c.JSON(http.StatusOK, gin.H{"call": *calls})
	})
	return router
}

func postWithKey(router *gin.Engine, key, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/v2/spaces/space1/things", strings.NewReader(body))
	if key != "" {
		req.Header.Set(IdempotencyKeyHeader, key)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestEnsureIdempotency(t *testing.T) {
	t.Run("same key and body replays the stored result", func(t *testing.T) {
		// given
		calls := 0
		router := newIdempotencyRouter(newIdempotencyStore(8), &calls)

		// when
		first := postWithKey(router, "key1", `{"a":1}`)
		second := postWithKey(router, "key1", `{"a":1}`)

		// then
		assert.Equal(t, http.StatusOK, first.Code)
		assert.Equal(t, http.StatusOK, second.Code)
		assert.Equal(t, first.Body.String(), second.Body.String())
		assert.Equal(t, 1, calls, "the handler ran once; the second response replayed")
		assert.Equal(t, "true", second.Header().Get("Idempotency-Replayed"))
	})

	t.Run("same key with a different body is a 409 idempotency_conflict", func(t *testing.T) {
		// given
		calls := 0
		router := newIdempotencyRouter(newIdempotencyStore(8), &calls)
		postWithKey(router, "key1", `{"a":1}`)

		// when
		conflict := postWithKey(router, "key1", `{"a":2}`)

		// then
		assert.Equal(t, http.StatusConflict, conflict.Code)
		assert.Contains(t, conflict.Body.String(), `"idempotency_conflict"`)
		assert.Equal(t, 1, calls)
	})

	t.Run("no key passes through every time", func(t *testing.T) {
		// given
		calls := 0
		router := newIdempotencyRouter(newIdempotencyStore(8), &calls)

		// when
		postWithKey(router, "", `{}`)
		postWithKey(router, "", `{}`)

		// then
		assert.Equal(t, 2, calls)
	})

	t.Run("keys are scoped per space", func(t *testing.T) {
		// given
		store := newIdempotencyStore(8)
		store.put("spaceA", "key1", storedResult{bodyHash: "h", status: 200})

		// when / then
		_, foundA := store.get("spaceA", "key1")
		_, foundB := store.get("spaceB", "key1")
		assert.True(t, foundA)
		assert.False(t, foundB)
	})

	t.Run("LRU evicts the oldest entry beyond the bound", func(t *testing.T) {
		// given
		store := newIdempotencyStore(2)
		store.put("s", "k1", storedResult{})
		store.put("s", "k2", storedResult{})

		// when
		store.put("s", "k3", storedResult{})

		// then
		_, found1 := store.get("s", "k1")
		_, found3 := store.get("s", "k3")
		assert.False(t, found1)
		assert.True(t, found3)
	})

	t.Run("failed responses are not stored for replay", func(t *testing.T) {
		// given
		gin.SetMode(gin.TestMode)
		store := newIdempotencyStore(8)
		calls := 0
		router := gin.New()
		router.POST("/v2/spaces/:space_id/things", ensureIdempotency(store), func(c *gin.Context) {
			calls++
			if calls == 1 {
				c.JSON(http.StatusInternalServerError, gin.H{"boom": true})
				return
			}
			c.JSON(http.StatusOK, gin.H{"ok": true})
		})

		// when
		first := postWithKey(router, "key1", `{}`)
		second := postWithKey(router, "key1", `{}`)

		// then
		assert.Equal(t, http.StatusInternalServerError, first.Code)
		assert.Equal(t, http.StatusOK, second.Code, "retry after failure re-executes")
		assert.Equal(t, 2, calls)
	})
}

func TestEnsureDryRun(t *testing.T) {
	newRouter := func(sink *bool) *gin.Engine {
		gin.SetMode(gin.TestMode)
		router := gin.New()
		router.POST("/v2/things", ensureDryRun(), func(c *gin.Context) {
			*sink = IsDryRun(c)
			c.Status(http.StatusOK)
		})
		return router
	}

	tests := []struct {
		name       string
		query      string
		wantStatus int
		wantDryRun bool
	}{
		{name: "absent defaults to false", query: "", wantStatus: http.StatusOK, wantDryRun: false},
		{name: "explicit false", query: "?dry_run=false", wantStatus: http.StatusOK, wantDryRun: false},
		{name: "true sets the flag", query: "?dry_run=true", wantStatus: http.StatusOK, wantDryRun: true},
		{name: "garbage is a 400 naming allowed values", query: "?dry_run=yes", wantStatus: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// given
			var dryRun bool
			router := newRouter(&dryRun)

			// when
			req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/v2/things%s", tt.query), nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			// then
			require.Equal(t, tt.wantStatus, w.Code)
			if tt.wantStatus == http.StatusOK {
				assert.Equal(t, tt.wantDryRun, dryRun)
			} else {
				assert.Contains(t, w.Body.String(), "allowed values")
			}
		})
	}
}
