package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/api/util"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
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
	for _, version := range []string{"/v1", "/v2"} {
		t.Run(version, func(t *testing.T) {
			fx := newV2ServerFixture(t)
			fx.mwMock.On("AccountLocalLinkNewChallenge", mock.Anything, &pb.RpcAccountLocalLinkNewChallengeRequest{
				AppName: "pairing-client", Scope: model.AccountAuth_JsonAPI,
			}).Run(func(args mock.Arguments) {
				require.Equal(t, "http://localhost:3000", localorigin.OriginFromContext(args.Get(0).(context.Context)))
			}).Return(&pb.RpcAccountLocalLinkNewChallengeResponse{ChallengeId: "challenge-id"}).Once()
			fx.mwMock.On("AccountLocalLinkSolveChallenge", mock.Anything, &pb.RpcAccountLocalLinkSolveChallengeRequest{
				ChallengeId: "challenge-id", Answer: "1234",
			}).Return(&pb.RpcAccountLocalLinkSolveChallengeResponse{AppKey: "issued-key", SessionToken: "private-session"}).Once()

			post := func(path, body string) *httptest.ResponseRecorder {
				w := httptest.NewRecorder()
				req := httptest.NewRequest(http.MethodPost, version+path, strings.NewReader(body))
				req.Host = localApiHost
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Origin", "http://localhost:3000")
				fx.Engine().ServeHTTP(w, req)
				return w
			}

			challenge := post("/auth/challenges", `{"app_name":"pairing-client"}`)
			require.Equal(t, http.StatusCreated, challenge.Code)
			require.JSONEq(t, `{"challenge_id":"challenge-id"}`, challenge.Body.String())
			key := post("/auth/api_keys", `{"challenge_id":"challenge-id","code":"1234"}`)
			require.Equal(t, http.StatusCreated, key.Code)
			require.JSONEq(t, `{"api_key":"issued-key","grant":null}`, key.Body.String())

			// A key obtained through either prefix authenticates on v2 with
			// the grant returned by the account, without another pairing step.
			fx.mwMock.On("WalletCreateSession", mock.Anything, &pb.RpcWalletCreateSessionRequest{
				Auth: &pb.RpcWalletCreateSessionRequestAuthOfAppKey{AppKey: "issued-key"},
			}).Return(&pb.RpcWalletCreateSessionResponse{
				Token: "session", AppName: "pairing-client", AccountScope: model.AccountAuth_JsonAPI,
				Grant: &model.AccountAuthAppGrant{SpaceIds: []string{"spaceA"}, Perm: model.AccountAuthAppGrant_Read},
				Error: &pb.RpcWalletCreateSessionResponseError{Code: pb.RpcWalletCreateSessionResponseError_NULL},
			}).Once()
			fx.eventMock.On("Broadcast", mock.Anything).Return(nil).Maybe()
			whoami := serveWithKey(fx, http.MethodGet, "/v2/auth/whoami", "issued-key")
			require.Equal(t, http.StatusOK, whoami.Code)
			require.Contains(t, whoami.Body.String(), `"permission":"read"`)
			require.Contains(t, whoami.Body.String(), `"scoped":true`)
		})
	}
}

func TestRouter_ApiKeyApprovedGrant(t *testing.T) {
	for _, version := range []string{"/v1", "/v2"} {
		for _, tc := range []struct {
			name  string
			grant *model.AccountAuthAppGrant
			want  string
		}{
			{"selected spaces", &model.AccountAuthAppGrant{SpaceIds: []string{"spaceA", "spaceB"}, Perm: model.AccountAuthAppGrant_Read},
				`{"all_spaces":false,"space_ids":["spaceA","spaceB"],"permission":"read"}`},
			{"all spaces", &model.AccountAuthAppGrant{AllSpaces: true, Perm: model.AccountAuthAppGrant_ReadWrite},
				`{"all_spaces":true,"space_ids":[],"permission":"readwrite"}`},
		} {
			t.Run(version+"/"+tc.name, func(t *testing.T) {
				fx := newV2ServerFixture(t)
				fx.mwMock.On("AccountLocalLinkSolveChallenge", mock.Anything, &pb.RpcAccountLocalLinkSolveChallengeRequest{
					ChallengeId: "challenge-id", Answer: "1234",
				}).Return(&pb.RpcAccountLocalLinkSolveChallengeResponse{
					AppKey: "issued-key", Grant: tc.grant, SessionToken: "private-session",
				}).Once()
				req := httptest.NewRequest(http.MethodPost, version+"/auth/api_keys", strings.NewReader(`{"challenge_id":"challenge-id","code":"1234"}`))
				req.Host = localApiHost
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				fx.Engine().ServeHTTP(w, req)
				require.Equal(t, http.StatusCreated, w.Code)
				require.JSONEq(t, `{"api_key":"issued-key","grant":`+tc.want+`}`, w.Body.String())
			})
		}
	}
}

func TestRouter_AuthErrors(t *testing.T) {
	for _, version := range []string{"/v1", "/v2"} {
		for _, endpoint := range []string{"/auth/challenges", "/auth/api_keys"} {
			t.Run(version+endpoint, func(t *testing.T) {
				fx := newFixture(t)
				for _, body := range []string{`{`, `{}`} {
					req := httptest.NewRequest(http.MethodPost, version+endpoint, strings.NewReader(body))
					req.Host = localApiHost
					w := httptest.NewRecorder()
					fx.Engine().ServeHTTP(w, req)
					require.Equal(t, http.StatusBadRequest, w.Code)
					require.Contains(t, w.Body.String(), `"code":"bad_request"`)
				}
				if endpoint == "/auth/api_keys" {
					fx.mwMock.On("AccountLocalLinkSolveChallenge", mock.Anything, mock.Anything).
						Return(&pb.RpcAccountLocalLinkSolveChallengeResponse{
							Error: &pb.RpcAccountLocalLinkSolveChallengeResponseError{Code: pb.RpcAccountLocalLinkSolveChallengeResponseError_CHALLENGE_NOT_APPROVED},
						}).Once()
					req := httptest.NewRequest(http.MethodPost, version+endpoint, strings.NewReader(`{"challenge_id":"pending","code":"1234"}`))
					req.Host = localApiHost
					w := httptest.NewRecorder()
					fx.Engine().ServeHTTP(w, req)
					require.Equal(t, http.StatusInternalServerError, w.Code)
					require.NotContains(t, w.Body.String(), `"api_key"`)
				}

				// Neither alias may reach a pairing RPC from an untrusted origin.
				req := httptest.NewRequest(http.MethodPost, version+endpoint, strings.NewReader(`{}`))
				req.Host = localApiHost
				req.Header.Set("Origin", "https://untrusted.example")
				w := httptest.NewRecorder()
				fx.Engine().ServeHTTP(w, req)
				require.Equal(t, http.StatusForbidden, w.Code)
			})
		}
	}
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
