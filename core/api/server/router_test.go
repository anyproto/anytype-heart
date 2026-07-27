package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/util/grpcprocess"
	"github.com/anyproto/anytype-heart/util/localorigin"
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

func TestRouter_MetadataHeader(t *testing.T) {
	t.Run("Response includes Anytype-Version header", func(t *testing.T) {
		// given
		fx := newFixture(t)
		engine := fx.NewRouter(fx.mwMock, fx.eventMock, []byte{}, []byte{})
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

func TestRouter_ChallengeCarriesOrigin(t *testing.T) {
	// The pairing dialog names the caller. app_name comes from the body and is
	// the caller's to choose, so the Origin header is the only attributable
	// part of the request and has to reach the challenge.
	tests := []struct {
		name    string
		headers map[string]string
		want    string
	}{
		{
			name:    "browser origin reaches the challenge",
			headers: map[string]string{"Origin": "http://localhost:3000"},
			want:    "http://localhost:3000",
		},
		{
			name:    "origin is passed through verbatim, not normalized",
			headers: map[string]string{"Origin": "HTTP://LocalHost:3000"},
			want:    "HTTP://LocalHost:3000",
		},
		{
			name: "native client without an origin carries none",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// given
			fx := newFixture(t)
			engine := fx.NewRouter(fx.mwMock, fx.eventMock, []byte{}, []byte{})

			var gotOrigin string
			fx.mwMock.On("AccountLocalLinkNewChallenge", mock.Anything, mock.Anything).
				Run(func(args mock.Arguments) {
					gotOrigin = localorigin.OriginFromContext(args.Get(0).(context.Context))
				}).
				Return(&pb.RpcAccountLocalLinkNewChallengeResponse{
					ChallengeId: "challengeId",
					Error:       &pb.RpcAccountLocalLinkNewChallengeResponseError{Code: pb.RpcAccountLocalLinkNewChallengeResponseError_NULL},
				}).Once()

			w := httptest.NewRecorder()
			req := httptest.NewRequest("POST", "/v1/auth/challenges", strings.NewReader(`{"app_name":"Save to Anytype"}`))
			req.Host = localApiHost
			req.Header.Set("Content-Type", "application/json")
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			// when
			engine.ServeHTTP(w, req)

			// then
			require.Equal(t, http.StatusCreated, w.Code)
			require.Equal(t, tt.want, gotOrigin)
		})
	}
}

func TestEnsureClientProcess(t *testing.T) {
	// Resolution is best effort: the pairing dialog still has the origin and
	// the app name, so a caller we cannot identify must not be turned away.
	tests := []struct {
		name       string
		remoteAddr string
	}{
		{name: "peer that is not on this machine", remoteAddr: "192.0.2.1:1234"},
		{name: "malformed remote address", remoteAddr: "not-an-address"},
		{name: "loopback peer with no matching connection", remoteAddr: "127.0.0.1:1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// given
			gin.SetMode(gin.TestMode)
			engine := gin.New()
			var served bool
			var info *grpcprocess.ProcessInfo
			engine.POST("/probe", ensureClientProcess(), func(c *gin.Context) {
				served = true
				info, _ = grpcprocess.FromContext(c.Request.Context())
				c.Status(http.StatusOK)
			})

			w := httptest.NewRecorder()
			req := httptest.NewRequest("POST", "/probe", nil)
			req.RemoteAddr = tt.remoteAddr

			// when
			engine.ServeHTTP(w, req)

			// then
			require.True(t, served, "an unidentifiable caller must still reach the handler")
			require.Equal(t, http.StatusOK, w.Code)
			require.Nil(t, info)
		})
	}
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
