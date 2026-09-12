package adapter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	notionclient "github.com/anyproto/anytype-heart/core/block/importv2/notion/client"
	"github.com/anyproto/anytype-heart/pb"
)

// The probe must hit /users/me, not /users. /users lists workspace members
// and 403s ("restricted_resource") whenever the integration's User
// Capabilities are "No user information" — a capability the importer never
// uses, since nothing resolves a user id against the API. Probing it turned
// a working token into a hard validation failure.
func TestProbeNotionTokenUsesBotEndpoint(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Write([]byte(`{"object":"user","id":"bot-id","type":"bot"}`))
	}))
	defer srv.Close()

	code := probeNotionToken(context.Background(), "secret", notionclient.WithBaseURL(srv.URL))

	assert.Equal(t, pb.RpcObjectImportNotionValidateTokenResponseError_NULL, code)
	assert.Equal(t, "/users/me", gotPath)
}

// A real integration with user capabilities switched off: /users 403s,
// /users/me still answers. Validation must pass.
func TestProbeNotionTokenWithoutUserCapability(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/users/me" {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte(`{"object":"error","status":403,"code":"restricted_resource","message":"Insufficient permissions for this endpoint."}`))
			return
		}
		w.Write([]byte(`{"object":"user","id":"bot-id","type":"bot"}`))
	}))
	defer srv.Close()

	code := probeNotionToken(context.Background(), "secret", notionclient.WithBaseURL(srv.URL))

	assert.Equal(t, pb.RpcObjectImportNotionValidateTokenResponseError_NULL, code)
}

func TestProbeNotionTokenErrorCodes(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   pb.RpcObjectImportNotionValidateTokenResponseErrorCode
	}{
		{
			name:   "bad token",
			status: http.StatusUnauthorized,
			body:   `{"object":"error","status":401,"code":"unauthorized","message":"API token is invalid."}`,
			want:   pb.RpcObjectImportNotionValidateTokenResponseError_UNAUTHORIZED,
		},
		{
			// /users/me should not produce this any more, but the mapping
			// still has to hold: Notion returns 403 for a revoked or
			// workspace-restricted integration too.
			name:   "forbidden",
			status: http.StatusForbidden,
			body:   `{"object":"error","status":403,"code":"restricted_resource","message":"Insufficient permissions."}`,
			want:   pb.RpcObjectImportNotionValidateTokenResponseError_FORBIDDEN,
		},
		{
			name:   "notion down",
			status: http.StatusServiceUnavailable,
			body:   `{"object":"error","status":503,"code":"service_unavailable","message":"Notion is unavailable."}`,
			want:   pb.RpcObjectImportNotionValidateTokenResponseError_SERVICE_UNAVAILABLE,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			code := probeNotionToken(context.Background(), "secret", notionclient.WithBaseURL(srv.URL))
			assert.Equal(t, tc.want, code)
		})
	}
}
