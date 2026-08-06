package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/api/util"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestEnsureMetadataHeader(t *testing.T) {
	t.Run("sets correct header", func(t *testing.T) {
		// given
		middleware := ensureMetadataHeader()
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
		expectedJSON, err := json.Marshal(util.CodeToApiError(http.StatusUnauthorized, ErrMissingAuthorizationHeader.Error()))
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
		expectedJSON, err := json.Marshal(util.CodeToApiError(http.StatusUnauthorized, ErrInvalidAuthorizationHeader.Error()))
		require.NoError(t, err)
		require.JSONEq(t, string(expectedJSON), w.Body.String())
	})

	t.Run("empty bearer value is refused before the session mint", func(t *testing.T) {
		// given: "Bearer " with an empty value. It must never reach
		// WalletCreateSession, whose empty-AppKey fallthrough is the
		// Full-minting mnemonic branch — no WalletCreateSession expectation
		// is set, so a call fails the mock.
		fx := newFixture(t)
		fx.KeyToToken = make(map[string]ApiSessionEntry)
		middleware := fx.ensureAuthenticated(fx.mwMock)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Authorization", "Bearer ")
		c.Request = req

		// when
		middleware(c)

		// then
		require.Equal(t, http.StatusUnauthorized, w.Code)
		expectedJSON, err := json.Marshal(util.CodeToApiError(http.StatusUnauthorized, ErrInvalidAuthorizationHeader.Error()))
		require.NoError(t, err)
		require.JSONEq(t, string(expectedJSON), w.Body.String())
	})

	t.Run("valid token creation", func(t *testing.T) {
		// given: a non-zero AppExpireAt — the mint-to-cache copy of every
		// carried field is what this test pins; a dropped ExpireAt would
		// silently disable the per-request expiry check for fresh mints
		fx := newFixture(t)
		fx.KeyToToken = make(map[string]ApiSessionEntry)
		tokenExpected := "valid-token"
		expireAtExpected := time.Now().Unix() + 3600

		fx.mwMock.
			On("WalletCreateSession", mock.Anything, &pb.RpcWalletCreateSessionRequest{
				Auth: &pb.RpcWalletCreateSessionRequestAuthOfAppKey{AppKey: "someAppKey"},
			}).
			Return(&pb.RpcWalletCreateSessionResponse{
				Token:        tokenExpected,
				AccountScope: model.AccountAuth_JsonAPI,
				AppName:      "test-app",
				AppExpireAt:  expireAtExpected,
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
		appName, exists := c.Get("apiAppName")
		require.True(t, exists)
		require.Equal(t, "test-app", appName)

		// the app name must also ride the REQUEST context — that is the only
		// carrier the analytics middleware can see
		payload, err := util.NewAnalyticsEventForApi(c.Request.Context(), "code", http.StatusOK)
		require.NoError(t, err)
		require.Contains(t, payload, `"apiAppName":"test-app"`)

		// scope and expiry are cached for later requests
		entry := fx.KeyToToken["someAppKey"]
		want := ApiSessionEntry{
			Token:    tokenExpected,
			AppName:  "test-app",
			Scope:    model.AccountAuth_JsonAPI,
			ExpireAt: expireAtExpected,
		}
		require.Equal(t, want, entry)
	})

	t.Run("the grant is cached at mint and rides both context carriers", func(t *testing.T) {
		// given: the mint answers with a grant — it must land in the cache
		// entry (the /v2 gate reads the gin-context session) AND on the
		// request context (the v2 service layer reads it from ctx)
		fx := newFixture(t)
		fx.KeyToToken = make(map[string]ApiSessionEntry)
		fx.mwMock.
			On("WalletCreateSession", mock.Anything, mock.Anything).
			Return(&pb.RpcWalletCreateSessionResponse{
				Token:        "tok",
				AccountScope: model.AccountAuth_JsonAPI,
				AppName:      "agent",
				Grant: &model.AccountAuthAppGrant{
					SpaceIds: []string{"space1"},
					Perm:     model.AccountAuthAppGrant_Read,
				},
				Error: &pb.RpcWalletCreateSessionResponseError{
					Code: pb.RpcWalletCreateSessionResponseError_NULL,
				},
			}, nil).Once()

		middleware := fx.ensureAuthenticated(fx.mwMock)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Authorization", "Bearer grantedKey")
		c.Request = req

		// when
		middleware(c)

		// then
		require.False(t, c.IsAborted())
		wantGrant := &util.ApiGrant{Spaces: []string{"space1"}, Perms: util.GrantPermsRead}
		entry := fx.KeyToToken["grantedKey"]
		require.Equal(t, wantGrant, entry.Grant)
		require.Equal(t, wantGrant, util.ApiGrantFromCtx(c.Request.Context()))
	})

	t.Run("a Limited key authenticates: scope is not auth's concern", func(t *testing.T) {
		// Keys minted without a scope carry Limited and must keep working on
		// /v1; the scope refusal lives in ensureJsonApiScope, installed on
		// /v2 only. Auth aborting on scope here would reinstate the gate on
		// every group that shares this middleware.
		fx := newFixture(t)
		fx.KeyToToken = make(map[string]ApiSessionEntry)
		fx.mwMock.
			On("WalletCreateSession", mock.Anything, mock.Anything).
			Return(&pb.RpcWalletCreateSessionResponse{
				Token:        "some-token",
				AccountScope: model.AccountAuth_Limited,
				AppName:      "clipper",
				Error: &pb.RpcWalletCreateSessionResponseError{
					Code: pb.RpcWalletCreateSessionResponseError_NULL,
				},
			}, nil).Once()

		middleware := fx.ensureAuthenticated(fx.mwMock)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Authorization", "Bearer limitedKey")
		c.Request = req

		// when
		middleware(c)

		// then
		require.False(t, c.IsAborted())
		token, exists := c.Get("token")
		require.True(t, exists)
		require.Equal(t, "some-token", token)
		// the resolved session rides the gin context — the /v2 scope gate
		// reads it from there
		value, exists := c.Get(apiSessionContextKey)
		require.True(t, exists)
		require.Equal(t, model.AccountAuth_Limited, value.(ApiSessionEntry).Scope)
	})

	t.Run("expired key is a distinct 401", func(t *testing.T) {
		// H5: the middleware maps APP_TOKEN_EXPIRED to its own message so the
		// client knows to re-issue instead of retrying.
		fx := newFixture(t)
		fx.KeyToToken = make(map[string]ApiSessionEntry)
		fx.mwMock.
			On("WalletCreateSession", mock.Anything, mock.Anything).
			Return(&pb.RpcWalletCreateSessionResponse{
				Error: &pb.RpcWalletCreateSessionResponseError{
					Code: pb.RpcWalletCreateSessionResponseError_APP_TOKEN_EXPIRED,
				},
			}, nil).Once()

		middleware := fx.ensureAuthenticated(fx.mwMock)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Authorization", "Bearer expiredKey")
		c.Request = req

		// when
		middleware(c)

		// then
		require.Equal(t, http.StatusUnauthorized, w.Code)
		expectedJSON, err := json.Marshal(util.CodeToApiError(http.StatusUnauthorized, ErrApiKeyExpired.Error()))
		require.NoError(t, err)
		require.JSONEq(t, string(expectedJSON), w.Body.String())
	})

	t.Run("key expiring while cached stops working and is evicted", func(t *testing.T) {
		// H5: expiry is enforced per request, not only at session mint.
		fx := newFixture(t)
		fx.KeyToToken = map[string]ApiSessionEntry{
			"cachedKey": {
				Token:    "cached-token",
				Scope:    model.AccountAuth_JsonAPI,
				ExpireAt: time.Now().Unix() - 60,
			},
		}
		middleware := fx.ensureAuthenticated(fx.mwMock)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Authorization", "Bearer cachedKey")
		c.Request = req

		// when
		middleware(c)

		// then
		require.Equal(t, http.StatusUnauthorized, w.Code)
		expectedJSON, err := json.Marshal(util.CodeToApiError(http.StatusUnauthorized, ErrApiKeyExpired.Error()))
		require.NoError(t, err)
		require.JSONEq(t, string(expectedJSON), w.Body.String())
		require.NotContains(t, fx.KeyToToken, "cachedKey")
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
		expectedJSON, err := json.Marshal(util.CodeToApiError(http.StatusUnauthorized, ErrInvalidApiKey.Error()))
		require.NoError(t, err)
		require.JSONEq(t, string(expectedJSON), w.Body.String())
	})
}

func TestEnsureJsonApiScope(t *testing.T) {
	// The gate runs chained directly after ensureAuthenticated, the way the
	// /v2 group installs it — the pair is exercised together because the gate
	// reads the session entry auth resolves.
	newChain := func(fx *fixture) *gin.Engine {
		router := gin.New()
		router.GET("/test",
			fx.ensureAuthenticated(fx.mwMock),
			ensureJsonApiScope(),
			func(c *gin.Context) { c.String(http.StatusOK, "OK") },
		)
		return router
	}

	t.Run("rejects keys that are neither JsonAPI nor Full", func(t *testing.T) {
		// H2: a valid Limited (web-clipper) key must not silently grant the
		// JSON API — 403, distinct from the 401 invalid-key path.
		tests := []struct {
			name     string
			scope    model.AccountAuthLocalApiScope
			wantCode int
		}{
			{name: "Limited scope is refused", scope: model.AccountAuth_Limited, wantCode: http.StatusForbidden},
			{name: "JsonAPI scope passes", scope: model.AccountAuth_JsonAPI, wantCode: http.StatusOK},
			{name: "Full scope passes", scope: model.AccountAuth_Full, wantCode: http.StatusOK},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				// given
				fx := newFixture(t)
				fx.KeyToToken = make(map[string]ApiSessionEntry)
				fx.mwMock.
					On("WalletCreateSession", mock.Anything, mock.Anything).
					Return(&pb.RpcWalletCreateSessionResponse{
						Token:        "some-token",
						AccountScope: tt.scope,
						AppName:      "clipper",
						Error: &pb.RpcWalletCreateSessionResponseError{
							Code: pb.RpcWalletCreateSessionResponseError_NULL,
						},
					}, nil).Once()
				w := httptest.NewRecorder()
				req := httptest.NewRequest("GET", "/test", nil)
				req.Header.Set("Authorization", "Bearer scopedKey")

				// when
				newChain(fx).ServeHTTP(w, req)

				// then
				require.Equal(t, tt.wantCode, w.Code)
				if tt.wantCode == http.StatusForbidden {
					// the body must name the key and its scope, so the failure
					// reads as "re-issue the key", not as a permissions bug
					wantMessage := `api key scope does not allow json api access: key "clipper" has Limited scope, create a new api key with JsonAPI scope`
					expectedJSON, err := json.Marshal(util.CodeToApiError(http.StatusForbidden, wantMessage))
					require.NoError(t, err)
					require.JSONEq(t, string(expectedJSON), w.Body.String())
				}
			})
		}
	})

	t.Run("gate applies to cached entries too", func(t *testing.T) {
		// given: a Limited entry already in the cache — the cache
		// short-circuits the mint for the rest of the process run, so the
		// gate must be evaluated on the cached path as well
		fx := newFixture(t)
		fx.KeyToToken = map[string]ApiSessionEntry{
			"cachedKey": {Token: "cached-token", Scope: model.AccountAuth_Limited},
		}
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "Bearer cachedKey")

		// when
		newChain(fx).ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("no authenticated session fails closed with 401", func(t *testing.T) {
		// given: the gate invoked without ensureAuthenticated ahead of it —
		// no session entry in the gin context. It must refuse as
		// unauthenticated, never pass and never 403 on a nil scope.
		middleware := ensureJsonApiScope()
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/", nil)

		// when
		middleware(c)

		// then
		require.Equal(t, http.StatusUnauthorized, w.Code)
		expectedJSON, err := json.Marshal(util.CodeToApiError(http.StatusUnauthorized, ErrMissingAuthorizationHeader.Error()))
		require.NoError(t, err)
		require.JSONEq(t, string(expectedJSON), w.Body.String())
	})
}

func TestAuthWwwAuthenticateHeaders(t *testing.T) {
	// The WWW-Authenticate values are wire surface MCP clients are required
	// to parse (spec rev 2025-06-18): the bare challenge when no credential
	// was presented (RFC 6750 §3), invalid_token for a present but unusable
	// one, insufficient_scope on authorization refusals.
	t.Run("missing credentials get the bare challenge", func(t *testing.T) {
		fx := newFixture(t)
		fx.KeyToToken = make(map[string]ApiSessionEntry)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/", nil)

		fx.ensureAuthenticated(fx.mwMock)(c)

		require.Equal(t, http.StatusUnauthorized, w.Code)
		require.Equal(t, `Bearer realm="anytype"`, w.Header().Get("WWW-Authenticate"))
	})

	t.Run("an invalid key gets invalid_token", func(t *testing.T) {
		fx := newFixture(t)
		fx.KeyToToken = make(map[string]ApiSessionEntry)
		fx.mwMock.
			On("WalletCreateSession", mock.Anything, mock.Anything).
			Return(&pb.RpcWalletCreateSessionResponse{
				Error: &pb.RpcWalletCreateSessionResponseError{
					Code: pb.RpcWalletCreateSessionResponseError_UNKNOWN_ERROR,
				},
			}, nil).Once()
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Authorization", "Bearer badKey")
		c.Request = req

		fx.ensureAuthenticated(fx.mwMock)(c)

		require.Equal(t, http.StatusUnauthorized, w.Code)
		require.Equal(t, `Bearer realm="anytype", error="invalid_token"`, w.Header().Get("WWW-Authenticate"))
	})

	t.Run("the scope gate's 403 carries insufficient_scope", func(t *testing.T) {
		fx := newFixture(t)
		fx.KeyToToken = map[string]ApiSessionEntry{
			"limitedKey": {Token: "tok", AppName: "clipper", Scope: model.AccountAuth_Limited},
		}
		router := gin.New()
		router.GET("/test",
			fx.ensureAuthenticated(fx.mwMock),
			ensureJsonApiScope(),
			func(c *gin.Context) { c.String(http.StatusOK, "OK") },
		)
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "Bearer limitedKey")

		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusForbidden, w.Code)
		require.Equal(t, `Bearer error="insufficient_scope"`, w.Header().Get("WWW-Authenticate"))
	})
}

func TestEnsureAnalyticsEvent(t *testing.T) {
	t.Run("broadcasts analytics event after successful request", func(t *testing.T) {
		// given
		fx := newFixture(t)
		code := "test-code"
		fx.eventMock.On("Broadcast", mock.AnythingOfType("*pb.Event")).Return()
		middleware := ensureAnalyticsEvent(code, fx.eventMock)
		router := gin.New()
		router.Use(middleware)
		router.GET("/test", func(c *gin.Context) {
			c.String(http.StatusAccepted, "OK")
		})

		// when
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/test", nil)
		router.ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusAccepted, w.Code)
		fx.eventMock.AssertCalled(t, "Broadcast", mock.AnythingOfType("*pb.Event"))

		expectedPayload, err := util.NewAnalyticsEventForApi(context.Background(), code, http.StatusAccepted)
		require.NoError(t, err)
		msgArg := fx.eventMock.Calls[0].Arguments.Get(0).(*pb.Event)
		require.Len(t, msgArg.Messages, 1)

		wrapper := msgArg.Messages[0].GetPayloadBroadcast()
		require.NotNil(t, wrapper)
		require.Equal(t, expectedPayload, wrapper.Payload)

	})

	t.Run("event is attributed to the authenticated app", func(t *testing.T) {
		// given: an upstream middleware stores the app name on the request
		// context, the way ensureAuthenticated does
		fx := newFixture(t)
		fx.eventMock.On("Broadcast", mock.AnythingOfType("*pb.Event")).Return()
		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Request = c.Request.WithContext(util.CtxWithApiAppName(c.Request.Context(), "my-integration"))
			c.Next()
		})
		router.Use(ensureAnalyticsEvent("test-code", fx.eventMock))
		router.GET("/test", func(c *gin.Context) {
			c.String(http.StatusOK, "OK")
		})

		// when
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("GET", "/test", nil))

		// then
		msgArg := fx.eventMock.Calls[0].Arguments.Get(0).(*pb.Event)
		require.Len(t, msgArg.Messages, 1)
		payload := msgArg.Messages[0].GetPayloadBroadcast().Payload
		require.Contains(t, payload, `"apiAppName":"my-integration"`)
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

		// the third request should be rate-limited
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
