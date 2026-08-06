package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIdempotencyReservation(t *testing.T) {
	// M4: begin must reserve the key so a concurrent same-key caller blocks
	// until the owner finishes, then replays — never double-executes.
	t.Run("a concurrent caller blocks on the reservation then replays", func(t *testing.T) {
		store := newIdempotencyStore(8)

		_, replay, owner := store.begin("space1", "key1")
		require.False(t, replay)
		require.True(t, owner, "first caller owns execution")

		type outcome struct {
			replay bool
			res    storedResult
		}
		done := make(chan outcome, 1)
		go func() {
			res, replay, _ := store.begin("space1", "key1")
			done <- outcome{replay, res}
		}()

		// the second begin must not return while the reservation is held
		select {
		case <-done:
			t.Fatal("second begin returned before finish — the reservation did not block")
		case <-time.After(20 * time.Millisecond):
		}

		store.finish("space1", "key1", &storedResult{bodyHash: "h", status: 200, body: []byte("ok")})

		got := <-done
		assert.True(t, got.replay, "the second caller replays the stored result")
		assert.Equal(t, "h", got.res.bodyHash)
	})

	t.Run("a retry after a failed owner re-executes", func(t *testing.T) {
		store := newIdempotencyStore(8)

		_, _, owner := store.begin("space1", "key2")
		require.True(t, owner)
		store.finish("space1", "key2", nil) // failure: nothing cached

		_, replay, owner2 := store.begin("space1", "key2")
		assert.False(t, replay, "the retry is not served a cached result")
		assert.True(t, owner2, "the retry becomes the new owner and re-executes")
		store.finish("space1", "key2", nil)
	})
}

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
	return postWithKeyAndQuery(router, key, body, "")
}

func postWithKeyAndQuery(router *gin.Engine, key, body, query string) *httptest.ResponseRecorder {
	target := "/v2/spaces/space1/things"
	if query != "" {
		target += "?" + query
	}
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
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

	t.Run("a cached dry run never replays as the real request (C9)", func(t *testing.T) {
		// given: same key and body, but the first request was ?dry_run=true —
		// the query string is part of the request identity
		calls := 0
		router := newIdempotencyRouter(newIdempotencyStore(8), &calls)
		dry := postWithKeyAndQuery(router, "key1", `{"a":1}`, "dry_run=true")

		// when
		real := postWithKey(router, "key1", `{"a":1}`)

		// then
		assert.Equal(t, http.StatusOK, dry.Code)
		assert.Equal(t, http.StatusConflict, real.Code, "different query under one key is a conflict, not a replay")
		assert.Contains(t, real.Body.String(), `"idempotency_conflict"`)
		assert.Equal(t, 1, calls)
	})

	t.Run("body over the size limit is rejected 413 before the handler", func(t *testing.T) {
		// C3: the middleware buffers the body ahead of the handler, so it must
		// bound the read or a keyed POST could OOM the process.
		// given
		calls := 0
		router := newIdempotencyRouter(newIdempotencyStore(8), &calls)
		oversized := strings.Repeat("x", maxV2RequestBody+1)

		// when
		w := postWithKey(router, "key1", oversized)

		// then
		assert.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
		assert.Contains(t, w.Body.String(), `"request_too_large"`)
		assert.Equal(t, 0, calls, "the handler never ran")
	})

	t.Run("a replayed PATCH with the same key and body runs the handler once", func(t *testing.T) {
		// C8 v0.3.5: PATCH is where a blind agent retry does damage — a
		// retried successful insertBlocks duplicates blocks — so the
		// middleware covers it exactly like POST.
		// given
		gin.SetMode(gin.TestMode)
		store := newIdempotencyStore(8)
		calls := 0
		router := gin.New()
		router.PATCH("/v2/spaces/:space_id/objects/:object_id", ensureIdempotency(store), func(c *gin.Context) {
			calls++
			c.JSON(http.StatusOK, gin.H{"call": calls})
		})
		patch := func() *httptest.ResponseRecorder {
			req := httptest.NewRequest(http.MethodPatch, "/v2/spaces/space1/objects/obj1",
				strings.NewReader(`{"ops":[{"op":"deleteBlock","id":"b1"}]}`))
			req.Header.Set(IdempotencyKeyHeader, "key1")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			return w
		}

		// when
		first := patch()
		second := patch()

		// then
		assert.Equal(t, http.StatusOK, first.Code)
		assert.Equal(t, http.StatusOK, second.Code)
		assert.Equal(t, first.Body.String(), second.Body.String(), "the stored result is replayed")
		assert.Equal(t, 1, calls, "the handler ran once")
		assert.Equal(t, "true", second.Header().Get("Idempotency-Replayed"))
	})

	t.Run("a replayed DELETE with the same key runs the handler once (Phase-6 widening)", func(t *testing.T) {
		// C8 covers DELETE where registered (the chat message delete): a
		// blindly retried delete would otherwise 404 misleadingly after the
		// first success.
		// given
		gin.SetMode(gin.TestMode)
		store := newIdempotencyStore(8)
		calls := 0
		router := gin.New()
		router.DELETE("/v2/spaces/:space_id/chats/:chat_id/messages/:message_id", ensureIdempotency(store), func(c *gin.Context) {
			calls++
			c.JSON(http.StatusOK, gin.H{"call": calls})
		})
		del := func() *httptest.ResponseRecorder {
			req := httptest.NewRequest(http.MethodDelete, "/v2/spaces/space1/chats/chat1/messages/msg1", nil)
			req.Header.Set(IdempotencyKeyHeader, "key1")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			return w
		}

		// when
		first := del()
		second := del()

		// then
		assert.Equal(t, http.StatusOK, first.Code)
		assert.Equal(t, http.StatusOK, second.Code)
		assert.Equal(t, first.Body.String(), second.Body.String(), "the stored result is replayed")
		assert.Equal(t, 1, calls, "the handler ran once — the retry never re-deleted")
		assert.Equal(t, "true", second.Header().Get("Idempotency-Replayed"))
	})

	t.Run("the same key and body on a DIFFERENT object never replays", func(t *testing.T) {
		// the target object lives in the PATH, so hashing only the body would
		// replay object A's success for an edit of object B — leaving B
		// unedited with a 2xx and A's etag, which no error message can repair
		gin.SetMode(gin.TestMode)
		store := newIdempotencyStore(8)
		calls := 0
		router := gin.New()
		router.PATCH("/v2/spaces/:space_id/objects/:object_id", ensureIdempotency(store), func(c *gin.Context) {
			calls++
			c.JSON(http.StatusOK, gin.H{"object": c.Param("object_id")})
		})
		patch := func(objectId string) *httptest.ResponseRecorder {
			req := httptest.NewRequest(http.MethodPatch, "/v2/spaces/space1/objects/"+objectId,
				strings.NewReader(`{"ops":[{"op":"updateBlock","id":"b5","set":{"checked":true}}]}`))
			req.Header.Set(IdempotencyKeyHeader, "key1")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			return w
		}

		first := patch("objA")
		second := patch("objB")

		assert.Equal(t, http.StatusOK, first.Code)
		assert.Contains(t, first.Body.String(), "objA")
		assert.Equal(t, http.StatusConflict, second.Code,
			"a reused key against another object is a conflict, never a replay")
		assert.Contains(t, second.Body.String(), `"idempotency_conflict"`)
		assert.Equal(t, 1, calls)
	})

	t.Run("the same key and body under a different METHOD never replays", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		store := newIdempotencyStore(8)
		calls := 0
		router := gin.New()
		handler := func(c *gin.Context) { calls++; c.JSON(http.StatusOK, gin.H{"m": c.Request.Method}) }
		router.PATCH("/v2/spaces/:space_id/objects/:object_id", ensureIdempotency(store), handler)
		router.PUT("/v2/spaces/:space_id/objects/:object_id", ensureIdempotency(store), handler)
		call := func(method string) *httptest.ResponseRecorder {
			req := httptest.NewRequest(method, "/v2/spaces/space1/objects/obj1", strings.NewReader(`{"a":1}`))
			req.Header.Set(IdempotencyKeyHeader, "key1")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			return w
		}

		assert.Equal(t, http.StatusOK, call(http.MethodPatch).Code)
		assert.Equal(t, http.StatusConflict, call(http.MethodPut).Code)
		assert.Equal(t, 1, calls)
	})

	t.Run("a replayed PUT with the same key and body runs the handler once", func(t *testing.T) {
		// C8 v0.3.5 covers POST, PATCH and PUT; PUT is the full-document
		// replace, where a re-executed retry is the most destructive
		gin.SetMode(gin.TestMode)
		store := newIdempotencyStore(8)
		calls := 0
		router := gin.New()
		router.PUT("/v2/spaces/:space_id/objects/:object_id", ensureIdempotency(store), func(c *gin.Context) {
			calls++
			c.JSON(http.StatusOK, gin.H{"call": calls})
		})
		put := func() *httptest.ResponseRecorder {
			req := httptest.NewRequest(http.MethodPut, "/v2/spaces/space1/objects/obj1",
				strings.NewReader(`{"version":1,"blocks":[]}`))
			req.Header.Set(IdempotencyKeyHeader, "key1")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			return w
		}

		first, second := put(), put()

		assert.Equal(t, http.StatusOK, first.Code)
		assert.Equal(t, first.Body.String(), second.Body.String())
		assert.Equal(t, 1, calls, "the handler ran once")
		assert.Equal(t, "true", second.Header().Get("Idempotency-Replayed"))
	})

	t.Run("a keyed GET passes through untouched", func(t *testing.T) {
		// the middleware acts on mutation methods only
		gin.SetMode(gin.TestMode)
		store := newIdempotencyStore(8)
		calls := 0
		router := gin.New()
		router.GET("/v2/spaces/:space_id/things", ensureIdempotency(store), func(c *gin.Context) {
			calls++
			c.JSON(http.StatusOK, gin.H{"call": calls})
		})
		get := func() {
			req := httptest.NewRequest(http.MethodGet, "/v2/spaces/space1/things", nil)
			req.Header.Set(IdempotencyKeyHeader, "key1")
			router.ServeHTTP(httptest.NewRecorder(), req)
		}

		get()
		get()

		assert.Equal(t, 2, calls, "GETs are never keyed or replayed")
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
