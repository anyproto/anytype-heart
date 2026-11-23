package v2handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
)

func respondErrorFor(t *testing.T, err error) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/x", func(c *gin.Context) { RespondError(c, err) })
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
	return w
}

func TestRespondErrorRetryAfter(t *testing.T) {
	t.Run("a loading space is a 503 carrying Retry-After", func(t *testing.T) {
		// when
		w := respondErrorFor(t, v2model.SpaceLoading("space1"))

		// then
		require.Equal(t, http.StatusServiceUnavailable, w.Code)
		assert.Equal(t, "5", w.Header().Get("Retry-After"))
		var got v2model.Error
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
		assert.Equal(t, v2model.CodeSpaceLoading, got.Code)
		assert.Equal(t, `space "space1" is still loading on this device`, got.Message)
		assert.NotContains(t, w.Body.String(), "RetryAfter", "the delay is a header, not a body member")
	})

	t.Run("any other error carries no Retry-After", func(t *testing.T) {
		// when
		w := respondErrorFor(t, v2model.NotFound("nothing here"))

		// then
		require.Equal(t, http.StatusNotFound, w.Code)
		assert.Empty(t, w.Header().Values("Retry-After"))
	})
}
