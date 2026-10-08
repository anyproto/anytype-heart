package server

// inproc.go — an http.RoundTripper that serves each request by calling the
// gin engine's ServeHTTP directly (the full-wrapper design,
// docs/superpowers/specs/2026-09-18-full-wrapper-in-heart-design.md §3.3).
// No listener, no socket, no port: the "HTTP" is Go structs passed between
// functions in one process, and every middleware between the transport and
// the service layer runs unchanged — origin policy, bearer authentication,
// the scope and grant gates, the idempotency store, the write rate limiter.
//
// The transport is engine-only. It never supplies a credential: the client
// that uses it sends whatever Authorization it was built with, so an inner
// /v2 call made on behalf of an /mcp caller carries that caller's own key
// and nothing wider. A test pins that a request without a bearer arrives at
// the engine without one.
//
// Responses are buffered whole and capped: the chat and search SSE streams
// are not served this way (the full tier excludes them), and a runaway body
// fails the round trip instead of growing without bound.

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// InProcessBaseURL is the base URL a client over the in-process transport
// is built with. The host is an IP literal so the origin policy's Host
// check (localorigin.AllowHost) admits it; no port, because nothing listens.
const InProcessBaseURL = "http://127.0.0.1"

// inProcessRemoteAddr is the RemoteAddr stamped on every in-process
// request: the write rate limiter keys on it, and loopback is the honest
// answer — an /mcp caller is a loopback caller and shares the budget with
// the other loopback callers by design.
const inProcessRemoteAddr = "127.0.0.1:0"

// InProcessMaxResponseBytes caps one buffered response. It equals the
// wrapper client's own read bound (client.go reads at most 32 MiB), so the
// cap is reached here, before the bytes are copied a second time.
const InProcessMaxResponseBytes = 32 << 20

// ErrInProcessResponseTooLarge is returned by RoundTrip when the engine
// wrote more than InProcessMaxResponseBytes.
var ErrInProcessResponseTooLarge = errors.New("in-process response exceeds the buffer cap")

// EngineResolver returns the engine to serve a request with. It is called
// per request, so a rebuilt engine (ReassignAddress on desktop) is picked
// up without rebuilding the transport. An error means the API is not
// available (no account running); http.Client wraps it in *url.Error, which
// the wrapper reports as "unreachable".
type EngineResolver func() (http.Handler, error)

// NewInProcessTransport builds the transport over resolve.
func NewInProcessTransport(resolve EngineResolver) http.RoundTripper {
	return &inProcessTransport{resolve: resolve}
}

type inProcessTransport struct {
	resolve EngineResolver
}

// RoundTrip serves req in-process. The request is cloned before being
// handed to the engine (a RoundTripper must not modify its argument). The
// Authorization header is passed through exactly as the client set it.
func (t *inProcessTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	handler, err := t.resolve()
	if err != nil {
		return nil, fmt.Errorf("in-process api: %w", err)
	}
	served := req.Clone(req.Context())
	served.RemoteAddr = inProcessRemoteAddr
	if served.Body == nil {
		served.Body = http.NoBody
	}
	rec := &bufferedResponse{header: http.Header{}, limit: InProcessMaxResponseBytes}
	handler.ServeHTTP(rec, served)
	if rec.overflow {
		return nil, fmt.Errorf("in-process api: %w", ErrInProcessResponseTooLarge)
	}
	body := rec.body.Bytes()
	status := rec.status()
	return &http.Response{
		Status:        fmt.Sprintf("%d %s", status, http.StatusText(status)),
		StatusCode:    status,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        rec.header,
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
		Request:       req,
	}, nil
}

// bufferedResponse is the http.ResponseWriter the engine writes into:
// headers, one status, the whole body up to limit. Flush is a no-op — a
// buffered writer cannot stream, and this transport does not claim to.
type bufferedResponse struct {
	header   http.Header
	code     int
	body     bytes.Buffer
	limit    int
	overflow bool
}

func (r *bufferedResponse) Header() http.Header { return r.header }

func (r *bufferedResponse) WriteHeader(code int) {
	if r.code == 0 {
		r.code = code
	}
}

func (r *bufferedResponse) Write(p []byte) (int, error) {
	if r.code == 0 {
		r.code = http.StatusOK
	}
	if r.overflow {
		return 0, ErrInProcessResponseTooLarge
	}
	if r.body.Len()+len(p) > r.limit {
		r.overflow = true
		return 0, ErrInProcessResponseTooLarge
	}
	return r.body.Write(p)
}

// Flush satisfies http.Flusher so a handler probing for it does not take a
// different code path; nothing is flushed, the body is served whole.
func (r *bufferedResponse) Flush() {}

func (r *bufferedResponse) status() int {
	if r.code == 0 {
		return http.StatusOK
	}
	return r.code
}
