package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/api/util"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// localApiHost is what a real client sends; httptest defaults to example.com,
// which the origin gate treats as a DNS-rebinding attempt.
const localApiHost = "127.0.0.1:31009"

func TestRouter_Unauthenticated(t *testing.T) {
	t.Run("GET /v1/spaces without auth returns 401", func(t *testing.T) {
		// given
		fx := newFixture(t)
		engine := fx.NewRouter(fx.mwMock, fx.eventMock, []byte{}, []byte{})
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/v1/spaces", nil)
		req.Host = localApiHost

		// when
		engine.ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

func TestRouter_AuthRoute(t *testing.T) {
	t.Run("POST /v1/auth/token is accessible without auth", func(t *testing.T) {
		// given
		fx := newFixture(t)
		engine := fx.NewRouter(fx.mwMock, fx.eventMock, []byte{}, []byte{})
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/v1/auth/token", nil)
		req.Host = localApiHost

		// when
		engine.ServeHTTP(w, req)

		// then
		require.NotEqual(t, http.StatusUnauthorized, w.Code)
	})
}

func TestRouter_V1KeyScopes(t *testing.T) {
	t.Run("every key scope is accepted on /v1", func(t *testing.T) {
		// The JSON-API scope gate is /v2-only. Keys minted without a scope
		// carry Limited (anytype-cli's CreateApp historically sent none) and
		// must keep working on /v1 — installing the gate on this group would
		// 403 every such key with no repair path but re-issuing.
		for _, scope := range []model.AccountAuthLocalApiScope{
			model.AccountAuth_Limited,
			model.AccountAuth_JsonAPI,
			model.AccountAuth_Full,
		} {
			t.Run(scope.String(), func(t *testing.T) {
				// given
				fx := newFixture(t)
				engine := fx.NewRouter(fx.mwMock, fx.eventMock, []byte{}, []byte{})
				fx.KeyToToken = map[string]ApiSessionEntry{
					"validKey": {Token: "dummyToken", AppName: "legacy-cli", Scope: scope},
				}
				fx.mwMock.On("ObjectSearch", mock.Anything, mock.Anything).
					Return(&pb.RpcObjectSearchResponse{
						Records: []*types.Struct{},
						Error:   &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
					}, nil).Once()
				fx.eventMock.On("Broadcast", mock.Anything).Return(nil).Maybe()

				w := httptest.NewRecorder()
				req := httptest.NewRequest("GET", "/v1/spaces", nil)
				req.Host = localApiHost
				req.Header.Set("Authorization", "Bearer validKey")

				// when
				engine.ServeHTTP(w, req)

				// then
				require.Equal(t, http.StatusOK, w.Code)
			})
		}
	})

	t.Run("expired key gets the distinct 401 on /v1", func(t *testing.T) {
		// given: expiry is enforced in ensureAuthenticated for BOTH groups —
		// only the scope refusal is /v2-only (H5 did not move)
		fx := newFixture(t)
		engine := fx.NewRouter(fx.mwMock, fx.eventMock, []byte{}, []byte{})
		fx.KeyToToken = map[string]ApiSessionEntry{
			"expiredKey": {Token: "dummyToken", Scope: model.AccountAuth_JsonAPI, ExpireAt: time.Now().Unix() - 60},
		}

		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/v1/spaces", nil)
		req.Host = localApiHost
		req.Header.Set("Authorization", "Bearer expiredKey")

		// when
		engine.ServeHTTP(w, req)

		// then
		require.Equal(t, http.StatusUnauthorized, w.Code)
		expectedJSON, err := json.Marshal(util.CodeToApiError(http.StatusUnauthorized, ErrApiKeyExpired.Error()))
		require.NoError(t, err)
		require.JSONEq(t, string(expectedJSON), w.Body.String())
	})
}

func TestRouter_MetadataHeader(t *testing.T) {
	t.Run("Response includes Anytype-Version header", func(t *testing.T) {
		// given
		fx := newFixture(t)
		engine := fx.NewRouter(fx.mwMock, fx.eventMock, []byte{}, []byte{})
		// no Scope on the entry: /v1 carries no scope gate, so a /v1 test
		// setting one would imply a requirement that does not exist
		fx.KeyToToken = map[string]ApiSessionEntry{"validKey": {Token: "dummyToken", AppName: "dummyApp"}}
		fx.mwMock.On("ObjectSearch", mock.Anything, mock.Anything).
			Return(&pb.RpcObjectSearchResponse{
				Records: []*types.Struct{},
				Error:   &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
			}, nil).Once()
		fx.eventMock.On("Broadcast", mock.Anything).Return(nil).Maybe()

		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/v1/spaces", nil)
		req.Host = localApiHost
		req.Header.Set("Authorization", "Bearer validKey")

		// when
		engine.ServeHTTP(w, req)

		// then
		require.Equal(t, "2025-11-08", w.Header().Get("Anytype-Version"))
	})
}

func TestRouter_TrustedOrigin(t *testing.T) {
	// The API answers with no CORS headers, so a site cannot read a response.
	// It can still reach a handler with a preflight-free "simple" request, and
	// the /v1/auth routes need no token, so the origin gate has to run first.
	tests := []struct {
		name    string
		host    string
		headers map[string]string
		want    int
	}{
		{
			name: "native client without an origin is served",
			host: localApiHost,
			want: http.StatusBadRequest, // reaches the handler, body is empty
		},
		{
			name:    "local browser client on a loopback origin is served",
			host:    localApiHost,
			headers: map[string]string{"Origin": "http://localhost:3000"},
			want:    http.StatusBadRequest,
		},
		{
			name:    "cross-origin form post from a site is refused",
			host:    localApiHost,
			headers: map[string]string{"Origin": "https://evil.com", "Content-Type": "text/plain"},
			want:    http.StatusForbidden,
		},
		{
			name:    "sandboxed iframe is refused",
			host:    localApiHost,
			headers: map[string]string{"Origin": "null"},
			want:    http.StatusForbidden,
		},
		{
			name: "dns rebinding is refused",
			host: "evil.com:31009",
			want: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// given
			fx := newFixture(t)
			engine := fx.NewRouter(fx.mwMock, fx.eventMock, []byte{}, []byte{})
			w := httptest.NewRecorder()
			req := httptest.NewRequest("POST", "/v1/auth/challenges", nil)
			req.Host = tt.host
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			// when
			engine.ServeHTTP(w, req)

			// then
			require.Equal(t, tt.want, w.Code)
		})
	}
}
