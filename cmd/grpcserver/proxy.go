package main

import (
	"net/http"
	"os"

	"github.com/improbable-eng/grpc-web/go/grpcweb"
	"google.golang.org/grpc"

	"github.com/anyproto/anytype-heart/pkg/lib/logging"
	"github.com/anyproto/anytype-heart/util/localorigin"
)

// envAllowedOrigins adds comma-separated exact origins to the gRPC-Web
// allowlist, for embedders and dev servers that are not on a loopback host.
const envAllowedOrigins = "ANYTYPE_GRPCWEB_ALLOWED_ORIGINS"

// envAllowedHosts adds comma-separated Host header values to the allowlist, for
// operators who bind the proxy to a routable interface and reach it by name.
const envAllowedHosts = "ANYTYPE_GRPCWEB_ALLOWED_HOSTS"

// envEnableWebsockets re-enables the grpc-websockets transport, which is still
// origin-checked when on.
const envEnableWebsockets = "ANYTYPE_GRPCWEB_ENABLE_WEBSOCKETS"

var log = logging.Logger("anytype-grpc-server")

// newOriginPolicy builds the allowlist guarding the gRPC-Web proxy.
//
// The packaged Electron renderer is a file:// page, so it reaches the proxy
// cross-origin: it sends no Origin on gRPC-Web POSTs and "file://" on a
// WebSocket handshake. Neither can be produced by a remote page, which always
// has to attach its own Origin.
func newOriginPolicy() *localorigin.Policy {
	return localorigin.New(os.Getenv(envAllowedOrigins),
		localorigin.AllowFileOrigin(),
		localorigin.AllowHosts(os.Getenv(envAllowedHosts)),
	)
}

// websocketsEnabled reports whether the grpc-websockets transport is on. It is
// off by default: no client uses it (they speak gRPC-Web over XHR) and, unlike
// the HTTP path, it is not gated by a CORS preflight, so it would let any site
// reach the RPC surface directly.
func websocketsEnabled() bool {
	return os.Getenv(envEnableWebsockets) == "1"
}

func wrapOptions(policy *localorigin.Policy, withWebsockets bool) []grpcweb.Option {
	opts := []grpcweb.Option{grpcweb.WithOriginFunc(policy.AllowOrigin)}
	if withWebsockets {
		opts = append(opts,
			grpcweb.WithWebsockets(true),
			grpcweb.WithWebsocketOriginFunc(policy.AllowRequest),
		)
	}
	return opts
}

func wrapGrpcServer(server *grpc.Server, policy *localorigin.Policy, withWebsockets bool) *grpcweb.WrappedGrpcServer {
	return grpcweb.WrapServer(server, wrapOptions(policy, withWebsockets)...)
}

// newProxyHandler serves gRPC-Web only to callers the policy trusts.
func newProxyHandler(webrpc *grpcweb.WrappedGrpcServer, policy *localorigin.Policy, withWebsockets bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		isWebsocket := webrpc.IsGrpcWebSocketRequest(r)
		if isWebsocket && !withWebsockets {
			http.Error(w, "grpc-websockets transport is disabled", http.StatusForbidden)
			return
		}
		if !webrpc.IsGrpcWebRequest(r) && !webrpc.IsAcceptableGrpcCorsRequest(r) && !isWebsocket {
			return
		}
		// grpcweb hands the origin func to rs/cors, which does not reject a
		// disallowed origin on a non-preflight request: it omits the
		// Access-Control-* headers and dispatches the RPC anyway, so only the
		// browser's read of the response is blocked. Reject here instead, so
		// the call never reaches the handler.
		if !policy.AllowRequest(r) {
			log.Warnf("rejected grpc-web request from untrusted origin %q (host %q)", r.Header.Get("Origin"), r.Host)
			http.Error(w, "forbidden origin", http.StatusForbidden)
			return
		}
		webrpc.ServeHTTP(w, r)
	}
}
