// Package localorigin decides which callers are allowed to drive Anytype's
// loopback HTTP surfaces: the gRPC-Web proxy and the local JSON API.
//
// Both surfaces listen on 127.0.0.1 and expose methods that need no token
// (see core/auth.go noAuthMethods), so the only thing standing between a
// visited web page and the local account is the origin check performed here.
//
// The rules follow what browsers can and cannot forge:
//
//   - A page cannot set the Origin header. Cross-origin fetch/XHR always
//     attaches it, and so does every WebSocket handshake.
//   - An absent Origin therefore means a non-browser caller (CLI, tests,
//     native clients) or the packaged Electron renderer, whose file:// page
//     omits Origin on gRPC-Web POSTs.
//   - Opaque initiators (sandboxed iframes, data: URLs) send "null", which is
//     forgeable by any site and must never be trusted.
package localorigin

import (
	"net"
	"net/http"
	"strings"
)

// fileOrigin is the origin an Electron renderer loaded over file:// sends on a
// WebSocket handshake. A remote page can never obtain it.
const fileOrigin = "file://"

// Option configures a Policy.
type Option func(*Policy)

// AllowFileOrigin trusts the literal "file://" origin. Enable it for surfaces
// reachable from the packaged Electron renderer; leave it off for surfaces that
// only native clients talk to.
func AllowFileOrigin() Option {
	return func(p *Policy) { p.allowFileOrigin = true }
}

// AllowHosts accepts a comma-separated list of Host header values (port
// optional) in addition to loopback ones, for operators who deliberately bind
// the surface to a routable interface and reach it by name.
func AllowHosts(hosts string) Option {
	return func(p *Policy) {
		for _, host := range strings.Split(hosts, ",") {
			if h := hostname(strings.ToLower(strings.TrimSpace(host))); h != "" {
				p.extraHosts[h] = struct{}{}
			}
		}
	}
}

// Policy reports whether a request may reach a loopback HTTP surface.
type Policy struct {
	extra           map[string]struct{}
	extraHosts      map[string]struct{}
	allowFileOrigin bool
}

// New builds a Policy. allowedOrigins is a comma-separated list of extra exact
// origins (e.g. "http://192.168.1.5:3030"), normally sourced from an env var so
// embedders and non-loopback dev servers can opt in without a rebuild.
func New(allowedOrigins string, opts ...Option) *Policy {
	p := &Policy{extra: make(map[string]struct{}), extraHosts: make(map[string]struct{})}
	for _, origin := range strings.Split(allowedOrigins, ",") {
		if normalized := normalize(origin); normalized != "" {
			p.extra[normalized] = struct{}{}
		}
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// AllowOrigin reports whether the value of an Origin header may drive the API.
// An empty origin is allowed: browsers cannot suppress the header cross-origin.
func (p *Policy) AllowOrigin(origin string) bool {
	normalized := normalize(origin)
	if normalized == "" {
		return true
	}
	// Opaque origin: sandboxed iframes and data: URLs. Any site can produce it,
	// so it stays untrusted even if someone lists it in the env allowlist.
	if normalized == "null" {
		return false
	}
	if _, ok := p.extra[normalized]; ok {
		return true
	}
	if normalized == fileOrigin {
		return p.allowFileOrigin
	}
	scheme, host, ok := splitOrigin(normalized)
	if !ok {
		return false
	}
	if scheme != "http" && scheme != "https" {
		return false
	}
	return isLoopbackHost(host)
}

// AllowRequest applies AllowOrigin plus a Host-header check that closes DNS
// rebinding, where the attacker's page and the local API appear same-origin to
// the browser and the Origin check alone would pass.
func (p *Policy) AllowRequest(r *http.Request) bool {
	if !p.allowHost(r.Host) {
		return false
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		// A rebound page reaches us as same-origin. Real callers are either
		// non-browsers (no Sec-Fetch-Site) or the Electron renderer, which is
		// cross-site relative to 127.0.0.1.
		switch strings.ToLower(r.Header.Get("Sec-Fetch-Site")) {
		case "same-origin", "same-site":
			return false
		}
		return true
	}
	return p.AllowOrigin(origin)
}

func (p *Policy) allowHost(host string) bool {
	if AllowHost(host) {
		return true
	}
	_, ok := p.extraHosts[hostname(strings.ToLower(host))]
	return ok
}

// AllowHost reports whether the Host header names a loopback endpoint. DNS
// rebinding needs a hostname to re-point, so IP literals and "localhost" are
// safe while any other name is rejected.
func AllowHost(host string) bool {
	host = hostname(host)
	if host == "" {
		return true
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	return net.ParseIP(host) != nil
}

// splitOrigin splits "scheme://host[:port]" without pulling in net/url, whose
// parser accepts many shapes that are not serialized origins.
func splitOrigin(origin string) (scheme, host string, ok bool) {
	scheme, host, ok = strings.Cut(origin, "://")
	if !ok || scheme == "" || host == "" {
		return "", "", false
	}
	// A serialized origin carries no path, query, userinfo or fragment.
	if strings.ContainsAny(host, "/?#@") {
		return "", "", false
	}
	return scheme, host, true
}

func isLoopbackHost(host string) bool {
	host = hostname(host)
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// hostname strips the port and IPv6 brackets from a host[:port] pair.
func hostname(host string) string {
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.TrimPrefix(host, "[")
	host = strings.TrimSuffix(host, "]")
	// "localhost." is the same name as "localhost" but would not compare equal.
	return strings.TrimSuffix(host, ".")
}

func normalize(origin string) string {
	origin = strings.ToLower(strings.TrimSpace(origin))
	if origin == fileOrigin {
		return origin
	}
	return strings.TrimSuffix(origin, "/")
}
