package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/improbable-eng/grpc-web/go/grpcweb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/util/localorigin"
)

const proxyHost = "127.0.0.1:31008"

// newTestProxy wires the real grpcweb wrapper, with the same options production
// uses, in front of a sentinel that records whether the RPC was dispatched.
func newTestProxy(t *testing.T, withWebsockets bool) (http.HandlerFunc, *atomic.Bool) {
	t.Helper()

	var dispatched atomic.Bool
	sentinel := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dispatched.Store(true)
		w.WriteHeader(http.StatusOK)
	})

	policy := localorigin.New("", localorigin.AllowFileOrigin())
	// WrapServer auto-registers the real gRPC methods in production; WrapHandler
	// does not, so register the probed endpoint to keep the default
	// corsForRegisteredEndpointsOnly preflight check faithful.
	opts := append(wrapOptions(policy, withWebsockets), grpcweb.WithEndpointsFunc(func() []string {
		return []string{"/anytype.ClientCommands/AccountLocalLinkNewChallenge"}
	}))
	webrpc := grpcweb.WrapHandler(sentinel, opts...)
	return newProxyHandler(webrpc, policy, withWebsockets), &dispatched
}

func newGrpcWebRequest(host, origin string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/anytype.ClientCommands/AccountLocalLinkNewChallenge", strings.NewReader(""))
	r.Host = host
	r.Header.Set("Content-Type", "application/grpc-web+proto")
	r.Header.Set("X-Grpc-Web", "1")
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	return r
}

func newWebsocketRequest(host, origin string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/anytype.ClientCommands/AccountLocalLinkNewChallenge", nil)
	r.Host = host
	r.Header.Set("Connection", "Upgrade")
	r.Header.Set("Upgrade", "websocket")
	r.Header.Set("Sec-Websocket-Protocol", "grpc-websockets")
	r.Header.Set("Sec-Websocket-Version", "13")
	r.Header.Set("Sec-Websocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	return r
}

// TestProxyHandler_RejectsUntrustedOriginBeforeDispatch is the regression test
// for the core defect: rs/cors does not reject a disallowed origin on a
// non-preflight request, it only drops the Access-Control-* headers and lets
// the RPC run. Blocking the browser's read of the response is not enough when
// the method needs no token.
func TestProxyHandler_RejectsUntrustedOriginBeforeDispatch(t *testing.T) {
	tests := []struct {
		name   string
		host   string
		origin string
	}{
		{"malicious site", proxyHost, "https://evil.com"},
		{"malicious site on the proxy port", proxyHost, "https://evil.com:31008"},
		{"sandboxed iframe or data url", proxyHost, "null"},
		{"non-loopback lan origin", proxyHost, "http://192.168.1.5:3030"},
		{"dns rebinding", "evil.com:31008", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// given
			handler, dispatched := newTestProxy(t, false)
			w := httptest.NewRecorder()

			// when
			handler(w, newGrpcWebRequest(tt.host, tt.origin))

			// then
			assert.Equal(t, http.StatusForbidden, w.Code)
			assert.False(t, dispatched.Load(), "the rpc must not reach the handler")
			assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
		})
	}
}

func TestProxyHandler_AllowsTrustedCallers(t *testing.T) {
	tests := []struct {
		name    string
		origin  string
		headers map[string]string
	}{
		{
			// Measured against the real Electron runtime: the packaged file://
			// renderer sends no Origin on a gRPC-Web POST.
			name:    "packaged electron renderer",
			origin:  "",
			headers: map[string]string{"Sec-Fetch-Site": "cross-site", "Sec-Fetch-Mode": "cors"},
		},
		{"native client", "", nil},
		{"web build on a loopback origin", "http://127.0.0.1:3030", nil},
		{"electron dev renderer", "http://localhost:8080", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// given
			handler, dispatched := newTestProxy(t, false)
			w := httptest.NewRecorder()
			req := newGrpcWebRequest(proxyHost, tt.origin)
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			// when
			handler(w, req)

			// then
			require.NotEqual(t, http.StatusForbidden, w.Code)
			assert.True(t, dispatched.Load(), "the rpc should reach the handler")
		})
	}
}

func TestProxyHandler_Preflight(t *testing.T) {
	newPreflight := func(origin string) *http.Request {
		r := httptest.NewRequest(http.MethodOptions, "/anytype.ClientCommands/AccountLocalLinkNewChallenge", nil)
		r.Host = proxyHost
		r.Header.Set("Origin", origin)
		r.Header.Set("Access-Control-Request-Method", http.MethodPost)
		r.Header.Set("Access-Control-Request-Headers", "x-grpc-web,content-type")
		return r
	}

	t.Run("a trusted origin gets its cors headers", func(t *testing.T) {
		// given
		handler, _ := newTestProxy(t, false)
		w := httptest.NewRecorder()

		// when
		handler(w, newPreflight("http://127.0.0.1:3030"))

		// then
		assert.NotEqual(t, http.StatusForbidden, w.Code)
		assert.Equal(t, "http://127.0.0.1:3030", w.Header().Get("Access-Control-Allow-Origin"))
	})

	t.Run("an untrusted origin is refused and gets no cors headers", func(t *testing.T) {
		// given
		handler, _ := newTestProxy(t, false)
		w := httptest.NewRecorder()

		// when
		handler(w, newPreflight("https://evil.com"))

		// then
		assert.Equal(t, http.StatusForbidden, w.Code)
		assert.Empty(t, w.Header().Get("Access-Control-Allow-Origin"))
	})
}

// TestProxyHandler_Websockets covers the reported bypass: a WebSocket handshake
// is not subject to a CORS preflight, so WithWebsocketOriginFunc was the only
// gate on it and it accepted everything.
func TestProxyHandler_Websockets(t *testing.T) {
	t.Run("disabled by default, whatever the origin", func(t *testing.T) {
		for _, origin := range []string{"https://evil.com", "null", "file://", "http://127.0.0.1:3030", ""} {
			// given
			handler, dispatched := newTestProxy(t, false)
			w := httptest.NewRecorder()

			// when
			handler(w, newWebsocketRequest(proxyHost, origin))

			// then
			assert.Equal(t, http.StatusForbidden, w.Code, "origin %q", origin)
			assert.False(t, dispatched.Load())
		}
	})

	t.Run("when enabled, an untrusted origin is still refused", func(t *testing.T) {
		for _, origin := range []string{"https://evil.com", "null", "http://192.168.1.5:3030"} {
			// given
			handler, dispatched := newTestProxy(t, true)
			w := httptest.NewRecorder()

			// when
			handler(w, newWebsocketRequest(proxyHost, origin))

			// then
			assert.Equal(t, http.StatusForbidden, w.Code, "origin %q", origin)
			assert.False(t, dispatched.Load())
		}
	})

	t.Run("when enabled, dns rebinding is refused", func(t *testing.T) {
		// given
		handler, _ := newTestProxy(t, true)
		w := httptest.NewRecorder()

		// when
		handler(w, newWebsocketRequest("evil.com:31008", "http://evil.com:31008"))

		// then
		assert.Equal(t, http.StatusForbidden, w.Code)
	})
}

func TestProxyHandler_IgnoresNonGrpcWebRequests(t *testing.T) {
	// given a plain browser navigation, which the proxy has never served
	handler, dispatched := newTestProxy(t, false)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = proxyHost

	// when
	handler(w, req)

	// then
	assert.False(t, dispatched.Load())
}
