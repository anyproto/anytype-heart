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
		require.Equal(t, ApiVersion, w.Header().Get("Anytype-Version"))
	})
}

func TestEnsureAuthenticated(t *testing.T) {
	t.Run("missing auth header", func(t *testing.T) {
		// given
		fx := newFixture(t)
		fx.KeyToToken = make(map[string]ApiSessionEntry)
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
		fx.KeyToToken = make(map[string]ApiSessionEntry)
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
		fx.KeyToToken = make(map[string]ApiSessionEntry)
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
		fx.KeyToToken = make(map[string]ApiSessionEntry)
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
		expectedJSON, err := json.Marshal(util.CodeToAPIError(http.StatusUnauthorized, ErrInvalidApiKey.Error()))
		require.NoError(t, err)
		require.JSONEq(t, string(expectedJSON), w.Body.String())
	})
}

func TestRateLimit(t *testing.T) {
	router := gin.New()
	router.GET("/", ensureRateLimit(1, 1, false), func(c *gin.Context) {
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

	t.Run("burst of size 2 allows two requests", func(t *testing.T) {
		burstRouter := gin.New()
		burstRouter.GET("/", ensureRateLimit(1, 2, false), func(c *gin.Context) {
			c.String(http.StatusOK, "OK")
		})

		// first request (within burst)
		w1 := httptest.NewRecorder()
		req1 := httptest.NewRequest("GET", "/", nil)
		req1.RemoteAddr = "1.2.3.4:5678"
		burstRouter.ServeHTTP(w1, req1)
		require.Equal(t, http.StatusOK, w1.Code)

		// second request (within burst)
		w2 := httptest.NewRecorder()
		req2 := httptest.NewRequest("GET", "/", nil)
		req2.RemoteAddr = "1.2.3.4:5678"
		burstRouter.ServeHTTP(w2, req2)
		require.Equal(t, http.StatusOK, w2.Code)

		// third request should be rate-limited
		w3 := httptest.NewRecorder()
		req3 := httptest.NewRequest("GET", "/", nil)
		req3.RemoteAddr = "1.2.3.4:5678"
		burstRouter.ServeHTTP(w3, req3)
		require.Equal(t, http.StatusTooManyRequests, w3.Code)
	})

	t.Run("disabled rate limit allows all requests", func(t *testing.T) {
		// given
		disabledRouter := gin.New()
		disabledRouter.GET("/", ensureRateLimit(1, 1, true), func(c *gin.Context) {
			c.String(http.StatusOK, "OK")
		})

		// when
		for i := 0; i < 5; i++ {
			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/", nil)
			req.RemoteAddr = "1.2.3.4:5678"
			disabledRouter.ServeHTTP(w, req)

			// then
			require.Equal(t, http.StatusOK, w.Code)
		}
	})
}
