package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/api/util"
	"github.com/anyproto/anytype-heart/pb"
)

func TestEnsureMetadataHeader(t *testing.T) {
	t.Run("sets correct header", func(t *testing.T) {
		// given
		fx := newFixture(t)
		middleware := fx.ensureMetadataHeader()
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)

		// when
		middleware(c)

		// then
		require.Equal(t, "2025-03-17", w.Header().Get("Anytype-Version"))
	})
}

func TestEnsureAuthenticated(t *testing.T) {
	t.Run("missing auth header", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.KeyToToken = make(map[string]string)
		middleware := fx.ensureAuthenticated(fx.mwMock)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		req := httptest.NewRequest("GET", "/", nil)
		c.Request = req

		// when
		middleware(c)

		// then
		require.Equal(t, http.StatusUnauthorized, w.Code)
		expectedJSON, err := json.Marshal(util.CodeToAPIError(http.StatusUnauthorized, ErrMissingAuthorizationHeader.Error()))
		require.NoError(t, err)
		require.JSONEq(t, string(expectedJSON), w.Body.String())
	})

	t.Run("invalid auth header format", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.KeyToToken = make(map[string]string)
		middleware := fx.ensureAuthenticated(fx.mwMock)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Authorization", "InvalidToken")
		c.Request = req

		// when
		middleware(c)

		// then
		require.Equal(t, http.StatusUnauthorized, w.Code)
		expectedJSON, err := json.Marshal(util.CodeToAPIError(http.StatusUnauthorized, ErrInvalidAuthorizationHeader.Error()))
		require.NoError(t, err)
		require.JSONEq(t, string(expectedJSON), w.Body.String())
	})

	t.Run("valid token creation", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.KeyToToken = make(map[string]string)
		tokenExpected := "valid-token"

		fx.mwMock.
			On("WalletCreateSession", mock.Anything, &pb.RpcWalletCreateSessionRequest{
				Auth: &pb.RpcWalletCreateSessionRequestAuthOfAppKey{AppKey: "someAppKey"},
			}).
			Return(&pb.RpcWalletCreateSessionResponse{
				Token: tokenExpected,
				Error: &pb.RpcWalletCreateSessionResponseError{
					Code: pb.RpcWalletCreateSessionResponseError_NULL,
				},
			}, nil).Once()

		middleware := fx.ensureAuthenticated(fx.mwMock)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Authorization", "Bearer someAppKey")
		c.Request = req

		// when
		middleware(c)

		// then
		token, exists := c.Get("token")
		require.True(t, exists)
		require.Equal(t, tokenExpected, token)
	})

	t.Run("invalid token", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.KeyToToken = make(map[string]string)
		middleware := fx.ensureAuthenticated(fx.mwMock)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Authorization", "Bearer invalidKey")
		c.Request = req

		fx.mwMock.
			On("WalletCreateSession", mock.Anything, &pb.RpcWalletCreateSessionRequest{
				Auth: &pb.RpcWalletCreateSessionRequestAuthOfAppKey{AppKey: "invalidKey"},
			}).
			Return(&pb.RpcWalletCreateSessionResponse{
				Token: "",
				Error: &pb.RpcWalletCreateSessionResponseError{
					Code: pb.RpcWalletCreateSessionResponseError_UNKNOWN_ERROR,
				},
			}, nil).Once()

		// when
		middleware(c)

		// then
		require.Equal(t, http.StatusUnauthorized, w.Code)
		expectedJSON, err := json.Marshal(util.CodeToAPIError(http.StatusUnauthorized, ErrInvalidToken.Error()))
		require.NoError(t, err)
		require.JSONEq(t, string(expectedJSON), w.Body.String())
	})
}

func TestRateLimit(t *testing.T) {
	fx := newFixture(t)
	router := gin.New()
	router.GET("/", fx.rateLimit(1), func(c *gin.Context) {
		c.String(http.StatusOK, "OK")
	})

	t.Run("first request allowed", func(t *testing.T) {
		// given
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "1.2.3.4:5678"

		// when
		router.ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("second request rate-limited", func(t *testing.T) {
		// given
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "1.2.3.4:5678"

		// when
		router.ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusTooManyRequests, w.Code)
	})
}
