package server

// inproc.go — the in-process transport of the mobile tool bridge (spec §1,
// docs/superpowers/specs/2026-09-04-mobile-tool-bridge-design.md): an
// http.RoundTripper that serves each request by calling the gin engine's
// ServeHTTP directly. No listener, no socket, no port — the "HTTP" is Go
// structs passed between functions in one process, and every middleware
// between the transport and the service layer runs unchanged. Responses
// are buffered whole: the chat SSE stream is not served this way (nothing
// in the wrapper's tool table calls it).

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
)

// InProcessBaseURL is the base URL the in-process transport answers for.
// The host is an IP literal so the origin policy's Host check
// (localorigin.AllowHost) admits it; no port, because nothing listens.
const InProcessBaseURL = "http://127.0.0.1"

// inProcessRemoteAddr is the RemoteAddr stamped on every in-process
// request: the write rate limiter keys on it, and loopback is the honest
// answer.
const inProcessRemoteAddr = "127.0.0.1:0"

// InProcessResolver returns the engine to serve a request with and the
// bearer key to authenticate it. It is called per request, so a rebuilt
// engine (ReassignAddress on desktop) and its fresh internal key are
// picked up without rebuilding the transport. An error means the API is
// not available (no account running); http.Client wraps it in *url.Error,
// which the wrapper reports as "unreachable".
type InProcessResolver func() (handler http.Handler, bearer string, err error)

// NewInProcessTransport builds the transport over resolve.
func NewInProcessTransport(resolve InProcessResolver) http.RoundTripper {
	return &inProcessTransport{resolve: resolve}
}

type inProcessTransport struct {
	resolve InProcessResolver
}

// RoundTrip serves req in-process. The request is cloned before being
// handed to the engine (a RoundTripper must not modify its argument), and
// the authorization is set here rather than by the client, so a rotated
// internal key never goes stale in a long-lived client.
func (t *inProcessTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	handler, bearer, err := t.resolve()
	if err != nil {
		return nil, fmt.Errorf("in-process api: %w", err)
	}
	served := req.Clone(req.Context())
	served.RemoteAddr = inProcessRemoteAddr
	served.Header.Set("Authorization", "Bearer "+bearer)
	if served.Body == nil {
		served.Body = http.NoBody
	}
	rec := &bufferedResponse{header: http.Header{}}
	handler.ServeHTTP(rec, served)
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
// headers, one status, the whole body. Flush is a no-op — a buffered
// writer cannot stream, and this transport does not claim to.
type bufferedResponse struct {
	header      http.Header
	code        int
	wroteHeader bool
	body        bytes.Buffer
}

func (r *bufferedResponse) Header() http.Header { return r.header }

func (r *bufferedResponse) WriteHeader(code int) {
	if r.wroteHeader {
		return
	}
	r.code = code
	r.wroteHeader = true
}

func (r *bufferedResponse) Write(p []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	return r.body.Write(p)
}

// Flush satisfies http.Flusher for handlers that probe for it; there is
// nothing to flush to.
func (r *bufferedResponse) Flush() {}

func (r *bufferedResponse) status() int {
	if !r.wroteHeader {
		return http.StatusOK
	}
	return r.code
}
